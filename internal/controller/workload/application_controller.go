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
	"time"

	"github.com/go-logr/logr"
	workloadv1alpha1 "go.funccloud.dev/fcp/api/workload/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"knative.dev/networking/pkg/apis/networking"
	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/serving/pkg/apis/autoscaling"
	"knative.dev/serving/pkg/apis/serving"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	servingv1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil" // Ensure controllerutil is imported
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
	requeueNeeded, reconcileErr := r.reconcileResources(ctx, l, app)
	if reconcileErr != nil {
		// Use embedded Status struct's SetCondition method
		app.Status.SetCondition(metav1.Condition{
			Type:    workloadv1alpha1.ReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  workloadv1alpha1.ReconciliationFailedReason,
			Message: fmt.Sprintf("Failed to reconcile resources: %v", reconcileErr),
		})
		// Return error to requeue, even if requeueNeeded is true, error takes precedence
		return ctrl.Result{}, reconcileErr
	}
	if requeueNeeded {
		l.Info("Requeueing reconciliation as Knative Service is not ready yet.")
		// Don't update ObservedGeneration yet
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil // Requeue requested by reconcileResources
	}

	// Update status to Ready only if all components are ready
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
// Returns true if requeue is needed (e.g., waiting for ksvc), error if reconciliation failed.
func (r *ApplicationReconciler) reconcileResources(
	ctx context.Context,
	l logr.Logger,
	app *workloadv1alpha1.Application,
) (requeueNeeded bool, err error) {
	l.Info("Reconciling Application resources", "application", app.Name)

	// 1. Reconcile Knative Service
	ksvc, requeueNeeded, err := r.reconcileKnativeService(ctx, l, app)
	if err != nil {
		return false, fmt.Errorf("failed to reconcile Knative Service: %w", err)
	}

	// 2. Reconcile Domain Mapping
	dm, err := r.reconcileDomainMapping(ctx, l, app, ksvc)
	if err != nil {
		return false, fmt.Errorf("failed to reconcile Domain Mapping: %w", err)
	}

	// 3. Update Status URLs
	r.updateStatusURLs(l, app, ksvc)

	if !requeueNeeded && *app.Spec.EnableTLS {
		_, err = controllerutil.CreateOrUpdate(ctx, r.Client, ksvc, func() error {
			ksvc.Annotations[networking.HTTPProtocolAnnotationKey] = string(netv1alpha1.HTTPOptionRedirected)
			return nil
		})
		if err != nil {
			return true, fmt.Errorf("failed to force TLS redirect: %w", err)
		}
		_, err = controllerutil.CreateOrUpdate(ctx, r.Client, dm, func() error {
			dm.Annotations[networking.HTTPProtocolAnnotationKey] = string(netv1alpha1.HTTPOptionRedirected)
			return nil
		})
		if err != nil {
			return true, fmt.Errorf("failed to force TLS redirect: %w", err)
		}
	}
	// If we reached here without returning, no requeue is needed and no error occurred
	return requeueNeeded, nil
}

// reconcileKnativeService handles the reconciliation of the Knative Service for the Application.
// It returns the reconciled Service, a boolean indicating if requeue is needed, and an error if any.
func (r *ApplicationReconciler) reconcileKnativeService(
	ctx context.Context,
	l logr.Logger,
	app *workloadv1alpha1.Application,
) (*servingv1.Service, bool, error) {
	l = l.WithValues("resource", "KnativeService")
	l.Info("Reconciling")

	ksvc := &servingv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
		},
	}

	// Use controllerutil.CreateOrUpdate
	opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, ksvc, func() error {
		// Set the application label
		if ksvc.Labels == nil {
			ksvc.Labels = make(map[string]string)
		}
		ksvc.Labels[workloadv1alpha1.ApplicationLabel] = app.Name

		// Apply mutations from the Application spec
		r.mutateKnativeService(app, ksvc)

		// Set the controller reference
		return controllerutil.SetControllerReference(app, ksvc, r.Scheme)
	})

	if err != nil {
		app.Status.SetCondition(metav1.Condition{
			Type:    workloadv1alpha1.KnativeServiceReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  workloadv1alpha1.KnativeServiceCreationFailedReason,
			Message: fmt.Sprintf("Failed to reconcile Knative Service: %v", err),
		})
		return nil, false, fmt.Errorf("failed to reconcile Knative Service: %w", err)
	}
	if opResult != controllerutil.OperationResultNone {
		l.Info("Knative Service reconciled", "operation", opResult)
	}

	// --- Check Knative Service Readiness ---
	// No need to re-fetch immediately after CreateOrUpdate unless status is critical
	// The existing logic below handles fetching the latest status
	latestKsvc := &servingv1.Service{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(ksvc), latestKsvc); err != nil {
		if apierrors.IsNotFound(err) {
			l.Info("Knative Service not found after reconcile, requeueing.", "service", ksvc.Name)
			app.Status.SetCondition(metav1.Condition{
				Type:    workloadv1alpha1.KnativeServiceReadyConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  workloadv1alpha1.KnativeServiceNotFoundReason,
				Message: "Knative Service not found, waiting for creation.",
			})
			return nil, true, nil // Requeue needed
		}
		l.Error(err, "Failed to get Knative Service status after reconcile", "service", ksvc.Name)
		app.Status.SetCondition(metav1.Condition{
			Type:    workloadv1alpha1.KnativeServiceReadyConditionType,
			Status:  metav1.ConditionUnknown,
			Reason:  workloadv1alpha1.KnativeServiceStatusCheckFailedReason,
			Message: fmt.Sprintf("Failed to get Knative Service status: %v", err),
		})
		return nil, false, fmt.Errorf("failed to get Knative Service status %s: %w", ksvc.Name, err)
	}

	ksvcReadyCond := latestKsvc.Status.GetCondition(servingv1.ServiceConditionReady)
	if ksvcReadyCond == nil || ksvcReadyCond.Status != corev1.ConditionTrue {
		l.Info("Knative Service is not ready yet, requeueing.", "service", ksvc.Name)
		conds := latestKsvc.Status.GetConditions()
		if len(conds) > 0 {
			latestCond := conds[len(conds)-1]
			cond := metav1.Condition{
				Type:    workloadv1alpha1.KnativeServiceReadyConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  workloadv1alpha1.KnativeServiceNotReadyReason,
				Message: latestCond.Message,
			}
			app.Status.SetCondition(cond)
		}
		return latestKsvc, true, nil // Requeue needed, return the latest ksvc
	}
	// Knative Service is Ready
	l.Info("Knative Service is Ready", "service", ksvc.Name)
	return latestKsvc, false, nil // Return the ready ksvc, no requeue, no error
}

// mutateKnativeService applies the desired state from the Application spec to the Knative Service.
// No error is returned as the operations are straightforward assignments.
func (r *ApplicationReconciler) mutateKnativeService(app *workloadv1alpha1.Application, ksvc *servingv1.Service) {
	// Set the annotations for the Knative Service
	if ksvc.Annotations == nil {
		ksvc.Annotations = make(map[string]string)
	}
	if ksvc.Spec.Template.ObjectMeta.Annotations == nil {
		ksvc.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}
	// Default EnableTLS to true if nil or not set
	enableTLS := workloadv1alpha1.DefaultEnableTLS
	if app.Spec.EnableTLS != nil {
		enableTLS = *app.Spec.EnableTLS
	}
	ksvc.Annotations[networking.DisableExternalDomainTLSAnnotationKey] = strconv.FormatBool(!enableTLS)
	ksvc.Annotations[networking.HTTPProtocolAnnotationKey] = string(netv1alpha1.HTTPOptionEnabled)
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

	ksvc.Spec.Template.ObjectMeta.Annotations[autoscaling.MinScaleAnnotationKey] = strconv.Itoa(int(minReplicas))
	ksvc.Spec.Template.ObjectMeta.Annotations[autoscaling.MaxScaleAnnotationKey] = strconv.Itoa(int(maxReplicas))
	ksvc.Spec.Template.ObjectMeta.Annotations[autoscaling.InitialScaleAnnotationKey] = strconv.Itoa(int(minReplicas))
	metric := workloadv1alpha1.MetricConcurrency
	if app.Spec.Scale.Metric != "" {
		metric = app.Spec.Scale.Metric
	}
	ksvc.Spec.Template.ObjectMeta.Annotations[autoscaling.MetricAnnotationKey] = string(metric)
	ksvc.Spec.Template.ObjectMeta.Annotations[autoscaling.ClassAnnotationKey] = metric.GetClass()

	if app.Spec.Scale.Target != nil {
		ksvc.Spec.Template.ObjectMeta.Annotations[autoscaling.TargetAnnotationKey] = strconv.Itoa(int(*app.Spec.Scale.Target))
	}
	if app.Spec.Scale.TargetUtilizationPercentage != nil {
		ksvc.Spec.Template.ObjectMeta.Annotations[autoscaling.TargetUtilizationPercentageKey] =
			strconv.Itoa(int(*app.Spec.Scale.TargetUtilizationPercentage))
	} else if metric == workloadv1alpha1.MetricCPU || metric == workloadv1alpha1.MetricMemory {
		defaultTarget := workloadv1alpha1.DefaultTargetUtilization
		ksvc.Spec.Template.ObjectMeta.Annotations[autoscaling.TargetUtilizationPercentageKey] =
			strconv.Itoa(int(defaultTarget))
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
			Ports:          app.Spec.Ports,
		},
	}
	// Ensure labels and annotations from the service are propagated to the template
	if ksvc.Spec.Template.ObjectMeta.Labels == nil {
		ksvc.Spec.Template.ObjectMeta.Labels = make(map[string]string)
	}
	for k, v := range ksvc.Labels { // Copy labels from service meta
		ksvc.Spec.Template.ObjectMeta.Labels[k] = v
	}
	for k, v := range ksvc.Annotations { // Copy annotations from service meta
		ksvc.Spec.Template.ObjectMeta.Annotations[k] = v
	}
}

// reconcileDomainMapping handles the reconciliation of the DomainMapping for the Application.
func (r *ApplicationReconciler) reconcileDomainMapping(
	ctx context.Context,
	l logr.Logger,
	app *workloadv1alpha1.Application,
	ksvc *servingv1.Service, // Knative Service must be ready or reconciled before this
) (*servingv1beta1.DomainMapping, error) {
	l = l.WithValues("resource", "DomainMapping")
	if app.Spec.Domain == "" {
		l.Info("Domain not specified in spec, ensuring any owned DomainMapping is deleted.")
		return nil, r.cleanupOwnedDomainMappings(ctx, l, app)
	}
	if ksvc == nil {
		// Should not happen if reconcileResources calls this correctly, but defensive check
		l.Info("Knative Service is nil, cannot reconcile DomainMapping yet.")
		app.Status.SetCondition(metav1.Condition{
			Type:    workloadv1alpha1.DomainMappingReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  workloadv1alpha1.KnativeServiceNotReadyReason,
			Message: "Waiting for Knative Service before reconciling DomainMapping.",
		})
		return nil, fmt.Errorf("knative service is nil, cannot proceed with domain mapping reconciliation")
	}

	l = l.WithValues("domain", app.Spec.Domain)
	l.Info("Reconciling")

	// --- Check for conflicting DomainMapping before CreateOrUpdate ---
	existingDM := &servingv1beta1.DomainMapping{}
	err := r.Get(ctx, client.ObjectKey{Name: app.Spec.Domain, Namespace: app.Namespace}, existingDM)

	if err != nil {
		// Handle errors other than NotFound
		if !apierrors.IsNotFound(err) {
			l.Error(err, "Failed to check for existing DomainMapping")
			app.Status.SetCondition(metav1.Condition{
				Type:    workloadv1alpha1.DomainMappingReadyConditionType,
				Status:  metav1.ConditionUnknown,
				Reason:  workloadv1alpha1.DomainMappingCheckFailedReason,
				Message: fmt.Sprintf("Failed to check for existing DomainMapping: %v", err),
			})
			return nil, fmt.Errorf("failed to check for existing DomainMapping %s: %w", app.Spec.Domain, err)
		}
		// If err is NotFound, we can proceed to CreateOrUpdate below.
	} else {
		// DomainMapping exists, check if it belongs to a different app
		existingAppLabel, exists := existingDM.Labels[workloadv1alpha1.ApplicationLabel]
		if exists && existingAppLabel != app.Name {
			conflictErr := fmt.Errorf("domain mapping %s already exists and is linked to a different application %s",
				existingDM.Name, existingAppLabel)
			l.Error(conflictErr, "DomainMapping conflict detected")
			app.Status.SetCondition(metav1.Condition{
				Type:    workloadv1alpha1.DomainMappingReadyConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  workloadv1alpha1.DomainMappingConflictReason,
				Message: conflictErr.Error(),
			})
			return nil, conflictErr // Early return on conflict
		}
	}

	// --- Reconcile the DomainMapping using CreateOrUpdate ---
	dm := &servingv1beta1.DomainMapping{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Spec.Domain,
			Namespace: app.Namespace,
		},
	}

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, dm, func() error {
		// Set the application label
		if dm.Labels == nil {
			dm.Labels = make(map[string]string)
		}
		dm.Labels[workloadv1alpha1.ApplicationLabel] = app.Name

		// Set annotations for Domain Mapping
		if dm.Annotations == nil {
			dm.Annotations = make(map[string]string)
		}
		enableTLS := workloadv1alpha1.DefaultEnableTLS
		if app.Spec.EnableTLS != nil {
			enableTLS = *app.Spec.EnableTLS
		}
		dm.Annotations[networking.DisableExternalDomainTLSAnnotationKey] = strconv.FormatBool(!enableTLS)
		dm.Annotations[networking.HTTPProtocolAnnotationKey] = string(netv1alpha1.HTTPOptionEnabled)

		// Set the reference to the Knative Service
		dm.Spec.Ref = duckv1.KReference{
			APIVersion: servingv1.SchemeGroupVersion.String(),
			Kind:       "Service",
			Namespace:  ksvc.Namespace, // Use namespace from the ready ksvc
			Name:       ksvc.Name,      // Use name from the ready ksvc
		}

		// Set the controller reference
		return controllerutil.SetControllerReference(app, dm, r.Scheme)
	})

	if err != nil {
		// Error from CreateOrUpdate
		app.Status.SetCondition(metav1.Condition{
			Type:    workloadv1alpha1.DomainMappingReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  workloadv1alpha1.DomainMappingCreationFailedReason,
			Message: fmt.Sprintf("Failed to reconcile DomainMapping: %v", err),
		})
		return nil, fmt.Errorf("failed to reconcile DomainMapping %s: %w", dm.Name, err)
	}
	if opResult != controllerutil.OperationResultNone {
		l.Info("DomainMapping reconciled", "operation", opResult)
	}

	// Success - Update status (consider checking DM readiness if needed, but for now assume success)
	app.Status.SetCondition(metav1.Condition{
		Type:    workloadv1alpha1.DomainMappingReadyConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  workloadv1alpha1.DomainMappingReadyReason,
		Message: fmt.Sprintf("DomainMapping %s created/updated", dm.Name),
	})
	return dm, nil
}

// cleanupOwnedDomainMappings deletes any DomainMapping resources owned by the Application.
func (r *ApplicationReconciler) cleanupOwnedDomainMappings(
	ctx context.Context,
	l logr.Logger,
	app *workloadv1alpha1.Application,
) error {
	dmList := &servingv1beta1.DomainMappingList{}
	listOpts := []client.ListOption{
		client.InNamespace(app.Namespace),
		client.MatchingLabels{workloadv1alpha1.ApplicationLabel: app.Name},
	}
	if err := r.List(ctx, dmList, listOpts...); err != nil {
		l.Error(err, "Failed to list DomainMappings for cleanup check")
		// Don't block reconciliation, but log the error and return it.
		return fmt.Errorf("failed to list DomainMappings for cleanup: %w", err)
	}

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
			l.V(1).Info("Found DomainMapping with application label but different/no owner, skipping deletion.",
				"domainMapping", dm.Name)
		}
	}

	// Clear the DomainMappingReady condition if it was previously set true
	app.Status.SetCondition(metav1.Condition{
		Type:    workloadv1alpha1.DomainMappingReadyConditionType,
		Status:  metav1.ConditionFalse,
		Reason:  workloadv1alpha1.DomainMappingNotConfiguredReason,
		Message: "DomainMapping is not configured in the Application spec.",
	})

	// If there were errors deleting DomainMappings, return an aggregated error
	if len(deleteErrors) > 0 {
		return kerrors.NewAggregate(deleteErrors)
	}
	return nil
}

// updateStatusURLs updates the Application status with the relevant URLs.
func (r *ApplicationReconciler) updateStatusURLs(
	l logr.Logger,
	app *workloadv1alpha1.Application,
	ksvc *servingv1.Service,
) { // Removed error return type
	urls := []string{}

	// Add the default Knative Service URL if available
	// No need to re-fetch, use the ksvc passed in which is confirmed ready or latest fetched
	if ksvc != nil && ksvc.Status.URL != nil {
		urls = append(urls, ksvc.Status.URL.String())
	} else if ksvc != nil {
		l.Info("Knative Service URL not available in status yet", "service", ksvc.Name)
	} else {
		l.Info("Knative Service object is nil, cannot determine default URL")
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
}

// reconcileDeletion handles the cleanup when an Application is marked for deletion.
func (r *ApplicationReconciler) reconcileDeletion(
	ctx context.Context,
	l logr.Logger,
	app *workloadv1alpha1.Application,
) error {
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
		// Owns Knative Service - Reconcile Application if owned Service changes
		// We also need to trigger reconcile if the *status* of the owned service changes (specifically Ready condition)
		// This requires watching the Service directly and enqueuing requests for the owner.
		Owns(
			&servingv1.Service{},
			builder.WithPredicates(applicationLabelPredicate), // Trigger on status changes too
		).
		// Owns DomainMapping - Reconcile Application if owned DomainMapping changes
		Owns(&servingv1beta1.DomainMapping{}, builder.WithPredicates(applicationLabelPredicate)). // Watch DomainMapping too
		Named("workload-application").
		Complete(r)
}
