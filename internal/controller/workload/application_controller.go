/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package workload

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	workloadv1alpha1 "go.funccloud.dev/fcp/api/workload/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"knative.dev/networking/pkg/apis/networking" // Import for Destination
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/serving/pkg/apis/autoscaling"
	"knative.dev/serving/pkg/apis/serving"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	servingv1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ApplicationReconciler reconciles a Application object
type ApplicationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=*,resources=*,verbs=*

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	l := logf.FromContext(ctx).WithValues("application", req.NamespacedName)
	l.Info("Reconciling Application")

	// Fetch the Application instance
	app := &workloadv1alpha1.Application{}
	if err := r.Get(ctx, req.NamespacedName, app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Initialize status if necessary (using embedded Status field)
	if app.Status.Conditions == nil {
		app.Status.Conditions = []metav1.Condition{}
	}

	// Defer status update
	defer func() {
		// Re-fetch the application to ensure we have the latest version for status update
		latestApp := &workloadv1alpha1.Application{}
		// Use the original app name/namespace from the request,
		// as the 'app' variable might be modified or become stale.
		if getErr := r.Get(ctx, req.NamespacedName, latestApp); getErr != nil {
			// If the app is not found, it might have been deleted concurrently.
			if client.IgnoreNotFound(getErr) == nil {
				l.Info("Application not found during deferred status update, likely deleted.", "application", req.NamespacedName)
				return
			}
			// For other errors during re-fetch, log and aggregate the error.
			l.Error(getErr, "unable to re-fetch Application for status update", "application", req.NamespacedName)
			err = kerrors.NewAggregate([]error{err, fmt.Errorf("failed to re-fetch application for status update: %w", getErr)})
			return
		}

		// Only update if the status has changed. Compare the whole ApplicationStatus.
		if !equality.Semantic.DeepEqual(latestApp.Status, app.Status) {
			latestApp.Status = app.Status // Assign the potentially modified status
			if updateErr := r.Status().Update(ctx, latestApp); updateErr != nil {
				// Ignore conflicts on update, as they should trigger a new reconcile anyway.
				if apierrors.IsConflict(updateErr) {
					l.Info("Conflict during status update, requeueing.", "application", req.NamespacedName)
					if err == nil && result.IsZero() {
						result = ctrl.Result{Requeue: true}
					}
					return // Don't aggregate conflict errors, let requeue handle it.
				}
				l.Error(updateErr, "unable to update Application status", "application", req.NamespacedName)
				err = kerrors.NewAggregate([]error{err, fmt.Errorf("failed to update application status: %w", updateErr)})
			}
		}
	}()

	// Handle deletion
	if !app.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.reconcileDeletion(ctx, l, app)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(app, workloadv1alpha1.ApplicationFinalizer) {
		l.Info("Adding finalizer")
		controllerutil.AddFinalizer(app, workloadv1alpha1.ApplicationFinalizer)
		if err := r.Update(ctx, app); err != nil {
			l.Error(err, "unable to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil // Requeue to process after finalizer is added
	}

	// Reconcile the application resources (e.g., Knative Service, DomainMapping)
	err = r.reconcileResources(ctx, l, app)
	if err != nil {
		// Use embedded Status struct's SetCondition method
		app.Status.SetCondition(metav1.Condition{
			Type:    workloadv1alpha1.ReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  workloadv1alpha1.ReconciliationFailedReason,
			Message: fmt.Sprintf("Failed to reconcile resources: %v", err),
		})
		return ctrl.Result{}, err // Return error to requeue
	}

	// Update status to Ready
	// Use embedded Status struct's SetCondition method
	app.Status.SetCondition(metav1.Condition{
		Type:    workloadv1alpha1.ReadyConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  workloadv1alpha1.ResourcesCreatedReason,
		Message: fmt.Sprintf("Application %s is ready", app.Name),
	})
	// Use embedded Status struct's ObservedGeneration field
	app.Status.ObservedGeneration = app.Generation
	l.Info("Application reconciled successfully")
	return ctrl.Result{}, nil
}

// reconcileResources handles the creation/update of resources owned by the Application.
func (r *ApplicationReconciler) reconcileResources(ctx context.Context, l logr.Logger, app *workloadv1alpha1.Application) error {
	l.Info("Reconciling Application resources", "application", app.Name)

	// Reconcile Knative Service
	ksvc := &servingv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
		},
	}
	if err := r.reconcileOwnedResource(ctx, l, app, ksvc, func() error {
		// Set the annotations for the Knative Service
		if ksvc.Annotations == nil {
			ksvc.Annotations = make(map[string]string)
		}
		// Default EnableTLS to true if nil or not set
		enableTLS := workloadv1alpha1.DefaultEnableTLS
		if app.Spec.EnableTLS != nil {
			enableTLS = *app.Spec.EnableTLS
		}
		ksvc.Annotations[networking.DisableExternalDomainTLSAnnotationKey] = strconv.FormatBool(!enableTLS)

		// Default Scale values if nil
		minReplicas := int32(0) // Default minReplicas
		if app.Spec.Scale.MinReplicas != nil {
			minReplicas = *app.Spec.Scale.MinReplicas
		}
		maxReplicas := int32(1) // Default maxReplicas
		if app.Spec.Scale.MaxReplicas != nil {
			maxReplicas = *app.Spec.Scale.MaxReplicas
		}
		if minReplicas > maxReplicas {
			maxReplicas = minReplicas // Ensure max is not less than min
		}

		ksvc.Annotations[autoscaling.MinScaleAnnotationKey] = strconv.Itoa(int(minReplicas))
		ksvc.Annotations[autoscaling.MaxScaleAnnotationKey] = strconv.Itoa(int(maxReplicas))
		ksvc.Annotations[autoscaling.InitialScaleAnnotationKey] = strconv.Itoa(int(minReplicas)) // Use minReplicas for initial scale

		metric := workloadv1alpha1.MetricConcurrency
		if app.Spec.Scale.Metric != "" {
			metric = app.Spec.Scale.Metric
		}
		ksvc.Annotations[autoscaling.MetricAnnotationKey] = string(metric)
		ksvc.Annotations[autoscaling.ClassAnnotationKey] = metric.GetClass() // Use GetClass method from Metric type

		if app.Spec.Scale.Target != nil {
			ksvc.Annotations[autoscaling.TargetAnnotationKey] = strconv.Itoa(int(*app.Spec.Scale.Target))
		}
		if app.Spec.Scale.TargetUtilizationPercentage != nil {
			ksvc.Annotations[autoscaling.TargetUtilizationPercentageKey] = strconv.Itoa(int(*app.Spec.Scale.TargetUtilizationPercentage))
		} else if metric == workloadv1alpha1.MetricCPU || metric == workloadv1alpha1.MetricMemory {
			// Set default target utilization only for HPA metrics if not specified
			defaultTarget := workloadv1alpha1.DefaultTargetUtilization
			ksvc.Annotations[autoscaling.TargetUtilizationPercentageKey] = strconv.Itoa(int(defaultTarget))
		}

		rolloutDuration := workloadv1alpha1.DefaultRolloutDuration.String() // Default rollout duration
		if app.Spec.RolloutDuration != nil {
			rolloutDuration = app.Spec.RolloutDuration.Duration.String() // Access Duration field
		}
		ksvc.Annotations[serving.RolloutDurationKey] = rolloutDuration

		// Configure the template spec
		ksvc.Spec.Template.Spec.ImagePullSecrets = app.Spec.ImagePullSecrets
		ksvc.Spec.Template.Spec.Containers = []corev1.Container{
			{
				Image:          app.Spec.Image,
				Env:            app.Spec.Env,
				Resources:      app.Spec.Resources,
				EnvFrom:        app.Spec.EnvFrom,
				LivenessProbe:  app.Spec.LivenessProbe,
				ReadinessProbe: app.Spec.ReadinessProbe,
				Command:        app.Spec.Command,
				Args:           app.Spec.Args,
				StartupProbe:   app.Spec.StartupProbe,
			},
		}
		// Ensure labels and annotations from the service are propagated to the template
		if ksvc.Spec.Template.ObjectMeta.Labels == nil {
			ksvc.Spec.Template.ObjectMeta.Labels = make(map[string]string)
		}
		for k, v := range ksvc.Labels { // Copy labels from service meta
			ksvc.Spec.Template.ObjectMeta.Labels[k] = v
		}
		if ksvc.Spec.Template.ObjectMeta.Annotations == nil {
			ksvc.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
		}
		for k, v := range ksvc.Annotations { // Copy annotations from service meta
			ksvc.Spec.Template.ObjectMeta.Annotations[k] = v
		}

		return nil
	}); err != nil {
		// Use embedded Status struct's SetCondition method
		app.Status.SetCondition(metav1.Condition{
			Type:    workloadv1alpha1.KnativeServiceReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  workloadv1alpha1.KnativeServiceCreationFailedReason,
			Message: fmt.Sprintf("Failed to reconcile Knative Service: %v", err),
		})
		return fmt.Errorf("failed to reconcile Knative Service: %w", err)
	}
	// Use embedded Status struct's SetCondition method
	app.Status.SetCondition(metav1.Condition{
		Type:    workloadv1alpha1.KnativeServiceReadyConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  workloadv1alpha1.KnativeServiceReadyReason, // Use ReadyReason constant
		Message: fmt.Sprintf("Knative Service %s created/updated", ksvc.Name),
	})

	// Reconcile DomainMapping only if app.Spec.Domain is set
	if app.Spec.Domain != "" {
		dm := &servingv1beta1.DomainMapping{
			ObjectMeta: metav1.ObjectMeta{
				Name:      app.Spec.Domain,
				Namespace: app.Namespace,
			},
			Spec: servingv1beta1.DomainMappingSpec{
				Ref: duckv1.KReference{
					APIVersion: servingv1.SchemeGroupVersion.String(),
					Kind:       "Service",
					Namespace:  ksvc.Namespace,
					Name:       ksvc.Name,
				},
			},
		}
		if err := r.reconcileOwnedResource(ctx, l, app, dm, func() error {
			// Check if the domain mapping already exists and is owned by a different application
			existingAppLabel, exists := dm.Labels[workloadv1alpha1.ApplicationLabel]
			if exists && existingAppLabel != app.Name {
				return fmt.Errorf("domain mapping %s already exists and is linked to a different application %s", dm.Name, existingAppLabel)
			}

			// Set annotations for Domain Mapping
			if dm.Annotations == nil {
				dm.Annotations = make(map[string]string)
			}
			enableTLS := workloadv1alpha1.DefaultEnableTLS
			if app.Spec.EnableTLS != nil {
				enableTLS = *app.Spec.EnableTLS
			}
			dm.Annotations[networking.DisableExternalDomainTLSAnnotationKey] = strconv.FormatBool(!enableTLS)
			return nil
		}); err != nil {
			// Use embedded Status struct's SetCondition method
			app.Status.SetCondition(metav1.Condition{
				Type:    workloadv1alpha1.DomainMappingReadyConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  workloadv1alpha1.DomainMappingCreationFailedReason,
				Message: fmt.Sprintf("Failed to reconcile DomainMapping: %v", err),
			})
			return fmt.Errorf("failed to reconcile DomainMapping: %w", err)
		}
		// Use embedded Status struct's SetCondition method
		app.Status.SetCondition(metav1.Condition{
			Type:    workloadv1alpha1.DomainMappingReadyConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  workloadv1alpha1.DomainMappingReadyReason, // Use ReadyReason constant
			Message: fmt.Sprintf("DomainMapping %s created/updated", dm.Name),
		})
	} else {
		// Domain is not specified in the spec. Clean up any existing DomainMapping owned by this Application.
		l.Info("Domain not specified in spec, ensuring any owned DomainMapping is deleted.")
		dmList := &servingv1beta1.DomainMappingList{}
		listOpts := []client.ListOption{
			client.InNamespace(app.Namespace),
			client.MatchingLabels{workloadv1alpha1.ApplicationLabel: app.Name},
		}
		if err := r.List(ctx, dmList, listOpts...); err != nil {
			l.Error(err, "Failed to list DomainMappings for cleanup check")
			// Don't block reconciliation, but log the error.
		} else {
			var deleteErrors []error
			for i := range dmList.Items {
				dm := dmList.Items[i] // Create a local copy for the closure/delete call
				// Check if the DomainMapping is owned by the current Application instance.
				if metav1.IsControlledBy(&dm, app) {
					l.Info("Deleting orphaned DomainMapping", "domainMapping", dm.Name)
					if err := r.Delete(ctx, &dm); err != nil && !apierrors.IsNotFound(err) {
						l.Error(err, "Failed to delete orphaned DomainMapping", "domainMapping", dm.Name)
						deleteErrors = append(deleteErrors, fmt.Errorf("failed to delete DomainMapping %s: %w", dm.Name, err))
					}
				} else {
					l.V(1).Info("Found DomainMapping with application label but different/no owner, skipping deletion.", "domainMapping", dm.Name)
				}
			}
			// If there were errors deleting DomainMappings, return an aggregated error
			if len(deleteErrors) > 0 {
				return kerrors.NewAggregate(deleteErrors)
			}
		}

		// Clear the DomainMappingReady condition if it was previously set true
		// Use embedded Status struct's SetCondition method
		app.Status.SetCondition(metav1.Condition{
			Type:    workloadv1alpha1.DomainMappingReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  workloadv1alpha1.DomainMappingNotConfiguredReason,
			Message: "DomainMapping is not configured in the Application spec.",
		})
	}

	// Update Status URLs based on Knative Service and DomainMapping
	if err := r.updateStatusURLs(ctx, l, app, ksvc); err != nil {
		l.Error(err, "Failed to update status URLs")
		// Consider setting a status condition for this failure
	}

	return nil
}

// reconcileOwnedResource handles the CreateOrUpdate logic for an owned resource.
func (r *ApplicationReconciler) reconcileOwnedResource(
	ctx context.Context,
	l logr.Logger,
	owner *workloadv1alpha1.Application,
	obj client.Object, // The object to reconcile (e.g., Knative Service, DomainMapping)
	mutateFn func() error,
) error {
	opRes, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
		// Apply common mutations: Label and Owner Reference
		if obj.GetLabels() == nil {
			obj.SetLabels(make(map[string]string))
		}
		obj.GetLabels()[workloadv1alpha1.ApplicationLabel] = owner.Name // Use ApplicationLabel

		if err := controllerutil.SetControllerReference(owner, obj, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}

		// Apply resource-specific mutations
		if mutateFn != nil {
			if err := mutateFn(); err != nil {
				// Wrap mutation error for better context
				return fmt.Errorf("mutation function failed: %w", err)
			}
		}
		return nil
	})

	if err != nil {
		l.Error(err, "unable to create or update resource",
			"resource", obj.GetObjectKind().GroupVersionKind().Kind,
			"name", obj.GetName(),
			"namespace", obj.GetNamespace())
		// Return the error to potentially trigger a requeue
		return fmt.Errorf("failed to create/update %s %s/%s: %w",
			obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName(), err)
	}

	if opRes != controllerutil.OperationResultNone {
		l.Info("Resource reconciled",
			"resource", obj.GetObjectKind().GroupVersionKind().Kind,
			"name", obj.GetName(),
			"namespace", obj.GetNamespace(),
			"operation", opRes)
	}
	return nil
}

// updateStatusURLs updates the Application status with the relevant URLs.
func (r *ApplicationReconciler) updateStatusURLs(ctx context.Context, l logr.Logger, app *workloadv1alpha1.Application, ksvc *servingv1.Service) error {
	urls := []string{}

	// Fetch the latest Knative Service status to get the URL
	latestKsvc := &servingv1.Service{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(ksvc), latestKsvc); err != nil {
		if apierrors.IsNotFound(err) {
			l.Info("Knative Service not found yet, cannot determine default URL", "service", ksvc.Name)
			// Clear URLs if the service is gone
			if !equality.Semantic.DeepEqual(app.Status.URLs, urls) {
				app.Status.URLs = urls
			}
			return nil // Not an error, just not ready
		}
		return fmt.Errorf("failed to get latest Knative Service status: %w", err)
	}

	// Add the default Knative Service URL if available
	if latestKsvc.Status.URL != nil {
		urls = append(urls, latestKsvc.Status.URL.String())
	} else {
		l.Info("Knative Service URL not available in status yet", "service", ksvc.Name)
	}

	// Add the custom domain URL if configured and DomainMapping is ready
	if app.Spec.Domain != "" {
		// Check the DomainMapping status condition we set earlier
		// Use embedded Status struct's GetCondition method
		dmCondition := app.Status.GetCondition(workloadv1alpha1.DomainMappingReadyConditionType)
		if dmCondition != nil && dmCondition.Status == metav1.ConditionTrue {
			scheme := "http"
			// Default EnableTLS to true if nil or not set
			enableTLS := workloadv1alpha1.DefaultEnableTLS
			if app.Spec.EnableTLS != nil {
				enableTLS = *app.Spec.EnableTLS
			}
			if enableTLS {
				scheme = "https"
			}
			urls = append(urls, fmt.Sprintf("%s://%s", scheme, app.Spec.Domain))
		} else {
			l.Info("DomainMapping not ready or not configured, skipping custom domain URL", "domain", app.Spec.Domain)
		}
	}

	// Update the status only if the URLs have changed (access URLs field directly on app.Status)
	if !equality.Semantic.DeepEqual(app.Status.URLs, urls) {
		l.Info("Updating status URLs", "oldURLs", app.Status.URLs, "newURLs", urls)
		app.Status.URLs = urls
	}

	return nil
}

// reconcileDeletion handles the cleanup when an Application is marked for deletion.
func (r *ApplicationReconciler) reconcileDeletion(ctx context.Context, l logr.Logger, app *workloadv1alpha1.Application) error {
	l.Info("Reconciling Application deletion", "application", app.Name)
	if controllerutil.ContainsFinalizer(app, workloadv1alpha1.ApplicationFinalizer) {
		l.Info("Removing finalizer")
		controllerutil.RemoveFinalizer(app, workloadv1alpha1.ApplicationFinalizer)
		if err := r.Update(ctx, app); err != nil {
			l.Error(err, "unable to remove finalizer")
			// Return error to retry finalizer removal
			return fmt.Errorf("failed to remove finalizer: %w", err)
		}
		l.Info("Finalizer removed")
	}

	// Stop reconciliation as the item is being deleted
	return nil // Return nil error on success
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Define a predicate to filter resources based on the application label.
	applicationLabelPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		labels := obj.GetLabels()
		if labels == nil {
			return false
		}
		_, exists := labels[workloadv1alpha1.ApplicationLabel]
		return exists
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&workloadv1alpha1.Application{}).
		// Use Owns with a predicate to watch resources specifically linked to an Application
		// via the label. This ensures the controller reconciles the Application if these
		// labeled resources change unexpectedly (e.g., manual modification or deletion outside GC).
		Owns(&servingv1.Service{}, builder.WithPredicates(applicationLabelPredicate)).
		Owns(&servingv1beta1.DomainMapping{}, builder.WithPredicates(applicationLabelPredicate)). // Watch DomainMapping too
		Named("workload-application").
		Complete(r)
}
