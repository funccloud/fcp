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

package tenancy

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	tenancyv1alpha1 "go.funccloud.dev/fcp/api/tenancy/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors" // Import apierrors
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder" // Import builder
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate" // Import predicate
)

// WorkspaceReconciler reconciles a Workspace object
type WorkspaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=*,resources=*,verbs=*

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	l := log.FromContext(ctx).WithValues("workspace", req.NamespacedName)
	l.Info("Reconciling Workspace")

	// Fetch the Workspace instance
	workspace := &tenancyv1alpha1.Workspace{}
	if err := r.Get(ctx, req.NamespacedName, workspace); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Initialize status if necessary
	if workspace.Status.Conditions == nil {
		workspace.Status.Conditions = []metav1.Condition{}
	}

	// Defer status update
	defer func() {
		// Re-fetch the workspace to ensure we have the latest version for status update
		latestWorkspace := &tenancyv1alpha1.Workspace{}
		// Use the original workspace name/namespace from the request,
		// as the 'workspace' variable might be modified or become stale.
		if getErr := r.Get(ctx, req.NamespacedName, latestWorkspace); getErr != nil {
			// If the workspace is not found, it might have been deleted concurrently.
			if client.IgnoreNotFound(getErr) == nil {
				l.Info("Workspace not found during deferred status update, likely deleted.", "workspace", req.NamespacedName)
				// Don't treat 'not found' as an error for the deferred update,
				// as the object might have been deleted as part of the reconciliation or concurrently.
				// The main reconcile loop error (if any) will be returned.
				return
			}
			// For other errors during re-fetch, log and aggregate the error.
			l.Error(getErr, "unable to re-fetch Workspace for status update", "workspace", req.NamespacedName)
			// Aggregate the fetch error with any error from the main reconcile logic.
			err = kerrors.NewAggregate([]error{err, fmt.Errorf("failed to re-fetch workspace for status update: %w", getErr)})
			return
		}

		// Check if the status we computed actually differs from the latest status.
		// This avoids unnecessary updates.
		// Note: Comparing complex structs might require a more sophisticated check,
		// but for typical status updates, this can prevent no-op updates.
		// Consider using equality.Semantic.DeepEqual if available and necessary.
		// For simplicity, we'll proceed with the update if fetched successfully.
		// TODO: Implement a deep comparison if needed to avoid unnecessary updates.

		// Apply the status changes calculated during the reconcile loop
		// onto the 'latestWorkspace' object before updating.
		latestWorkspace.Status = workspace.Status // workspace.Status holds the computed status

		if updateErr := r.Status().Update(ctx, latestWorkspace); updateErr != nil {
			// Ignore conflicts on update, as they should trigger a new reconcile anyway.
			if apierrors.IsConflict(updateErr) { // Use apierrors.IsConflict
				l.Info("Conflict during status update, requeueing.", "workspace", req.NamespacedName)
				// Set result to requeue if not already set by the main reconcile logic
				if err == nil && result.IsZero() {
					result = ctrl.Result{Requeue: true}
				}
				return // Don't aggregate conflict errors, let requeue handle it.
			}
			l.Error(updateErr, "unable to update Workspace status", "workspace", req.NamespacedName)
			err = kerrors.NewAggregate([]error{err, fmt.Errorf("failed to update workspace status: %w", updateErr)}) // Combine original error with status update error
		}
	}()

	// Handle deletion
	if !workspace.DeletionTimestamp.IsZero() {
		// Call reconcileDeletion and return its error directly.
		// The result is implicitly ctrl.Result{} when an error occurs or deletion is finished.
		return ctrl.Result{}, r.reconcileDeletion(ctx, l, workspace)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(workspace, tenancyv1alpha1.WorkspaceFinalizer) {
		l.Info("Adding finalizer")
		controllerutil.AddFinalizer(workspace, tenancyv1alpha1.WorkspaceFinalizer)
		if err := r.Update(ctx, workspace); err != nil {
			l.Error(err, "unable to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil // Requeue to process after finalizer is added
	}

	// Reconcile the workspace resources
	err = r.reconcileResources(ctx, l, workspace)
	if err != nil {
		workspace.Status.SetCondition(metav1.Condition{
			Type:    tenancyv1alpha1.ReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  "ReconciliationFailed",
			Message: fmt.Sprintf("Failed to reconcile resources: %v", err),
		})
		return ctrl.Result{}, err // Return error to requeue
	}

	// Update status to Ready
	workspace.Status.SetCondition(metav1.Condition{
		Type:    tenancyv1alpha1.ReadyConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  tenancyv1alpha1.ResourcesCreatedReason,
		Message: fmt.Sprintf("Workspace %s is ready", workspace.Name),
	})
	workspace.Status.ObservedGeneration = workspace.Generation
	l.Info("Workspace reconciled successfully")
	return ctrl.Result{}, nil
}

func (r *WorkspaceReconciler) reconcileResources(ctx context.Context, l logr.Logger,
	workspace *tenancyv1alpha1.Workspace) error {

	// Reconcile Namespace
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: workspace.Name}}
	// Remove ownerRef from the call to reconcileOwnedResource
	if err := r.reconcileOwnedResource(ctx, l, workspace, ns, func() error {
		// No specific mutations needed beyond labels/ownerRefs for Namespace
		return nil
	}); err != nil {
		workspace.Status.SetCondition(metav1.Condition{
			Type:    tenancyv1alpha1.NamespaceReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  tenancyv1alpha1.NamespaceCreationFailedReason,
			Message: err.Error(),
		})
		return fmt.Errorf("failed to reconcile namespace: %w", err)
	}
	workspace.Status.SetCondition(metav1.Condition{
		Type:    tenancyv1alpha1.NamespaceReadyConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  tenancyv1alpha1.NamespaceCreatedReason,
		Message: fmt.Sprintf("Namespace %s created/updated", ns.Name),
	})

	// Reconcile Role
	role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: workspace.Name, Namespace: workspace.Name}}
	if err := r.reconcileOwnedResource(ctx, l, workspace, role, func() error {
		role.Rules = []rbacv1.PolicyRule{
			{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"*"}},
		}
		return nil
	}); err != nil {
		workspace.Status.SetCondition(metav1.Condition{
			Type:    tenancyv1alpha1.RbacReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  tenancyv1alpha1.RbacCreationFailedReason,
			Message: fmt.Sprintf("Failed to reconcile Role: %v", err),
		})
		return fmt.Errorf("failed to reconcile role: %w", err)
	}

	// Reconcile RoleBinding
	roleBindingName := fmt.Sprintf("fcp-ownership-%s", workspace.Name)
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: workspace.Name,
		},
	}
	// Remove ownerRef from the call to reconcileOwnedResource
	if err := r.reconcileOwnedResource(ctx, l, workspace, roleBinding, func() error {
		roleBinding.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacv1.SchemeGroupVersion.Group,
			Kind:     "Role",
			Name:     role.Name,
		}
		subjects := []rbacv1.Subject{}
		for _, owner := range workspace.Spec.Owners {
			// Ensure Group is set correctly for core K8s kinds like User/Group/ServiceAccount
			apiGroup := owner.APIVersion // Use APIVersion from ObjectReference first
			if apiGroup == "" {          // If APIVersion is empty, infer based on Kind
				if owner.Kind == "User" || owner.Kind == "Group" {
					apiGroup = rbacv1.GroupName // Explicitly set for core RBAC kinds
				} else if owner.Kind == "ServiceAccount" {
					apiGroup = "" // Core API group
				}
				// Add more Kind checks if needed
			}

			subjects = append(subjects, rbacv1.Subject{
				APIGroup: apiGroup,
				Kind:     owner.Kind,
				Name:     owner.Name,
			})
		}
		roleBinding.Subjects = subjects
		return nil
	}); err != nil {
		workspace.Status.SetCondition(metav1.Condition{
			Type:    tenancyv1alpha1.RbacReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  tenancyv1alpha1.RbacCreationFailedReason,
			Message: fmt.Sprintf("Failed to reconcile RoleBinding: %v", err),
		})
		return fmt.Errorf("failed to reconcile role binding: %w", err)
	}

	workspace.Status.SetCondition(metav1.Condition{
		Type:    tenancyv1alpha1.RbacReadyConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  tenancyv1alpha1.RbacCreatedReason,
		Message: fmt.Sprintf("Role and RoleBinding created/updated in namespace %s", ns.Name),
	})

	return nil
}

// reconcileOwnedResource handles the CreateOrUpdate logic for an owned resource.
// Remove the unused ownerRef parameter.
func (r *WorkspaceReconciler) reconcileOwnedResource(
	ctx context.Context,
	l logr.Logger,
	owner *tenancyv1alpha1.Workspace,
	obj client.Object, // The object to reconcile (e.g., Namespace, Role, RoleBinding)
	// ownerRef metav1.OwnerReference, // Removed unused parameter
	mutateFn func() error, // Function to apply specific mutations
) error {
	opRes, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
		// Apply common mutations
		if obj.GetLabels() == nil {
			obj.SetLabels(make(map[string]string))
		}
		obj.GetLabels()[tenancyv1alpha1.WorkspaceLinkedResourceLabel] = owner.Name

		if err := controllerutil.SetControllerReference(owner, obj, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}

		// Apply resource-specific mutations
		if mutateFn != nil {
			if err := mutateFn(); err != nil {
				return fmt.Errorf("mutation failed: %w", err)
			}
		}
		return nil
	})

	if err != nil {
		l.Error(err, "unable to create or update resource",
			"resource", obj.GetObjectKind().GroupVersionKind().Kind,
			"name", obj.GetName())
		return err
	}

	if opRes != controllerutil.OperationResultNone {
		l.Info("Resource reconciled",
			"resource", obj.GetObjectKind().GroupVersionKind().Kind,
			"name", obj.GetName(),
			"operation", opRes)
	}
	return nil
}

// reconcileDeletion handles the cleanup when a Workspace is marked for deletion.
// It now only returns an error, as the Result is always empty.
func (r *WorkspaceReconciler) reconcileDeletion(ctx context.Context, l logr.Logger,
	workspace *tenancyv1alpha1.Workspace) error {
	l.Info("Reconciling Workspace deletion")

	// Resources (Namespace, Role, RoleBinding) should be garbage collected automatically
	// by Kubernetes because we set the OwnerReference with Controller=true.
	// We just need to remove the finalizer.

	if controllerutil.ContainsFinalizer(workspace, tenancyv1alpha1.WorkspaceFinalizer) {
		l.Info("Removing finalizer")
		controllerutil.RemoveFinalizer(workspace, tenancyv1alpha1.WorkspaceFinalizer)
		if err := r.Update(ctx, workspace); err != nil {
			l.Error(err, "unable to remove finalizer")
			return err // Return only the error
		}
		l.Info("Finalizer removed")
	}

	// Stop reconciliation as the item is being deleted
	return nil // Return nil error on success
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Define a predicate to filter resources based on the workspace label.
	workspaceLabelPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		labels := obj.GetLabels()
		if labels == nil {
			return false
		}
		_, exists := labels[tenancyv1alpha1.WorkspaceLinkedResourceLabel]
		return exists
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&tenancyv1alpha1.Workspace{}).
		// Use Owns with a predicate to watch resources specifically linked to a Workspace
		// via the label. This ensures the controller reconciles the Workspace if these
		// labeled resources change unexpectedly (e.g., manual modification or deletion outside GC).
		// Note: OwnerReferences with Controller=true already trigger reconciliation on deletion.
		// Owns() primarily helps if the owned object is modified or deleted in a way that bypasses GC.
		Owns(&corev1.Namespace{}, builder.WithPredicates(workspaceLabelPredicate)).
		Owns(&rbacv1.Role{}, builder.WithPredicates(workspaceLabelPredicate)).
		Owns(&rbacv1.RoleBinding{}, builder.WithPredicates(workspaceLabelPredicate)).
		Named("tenancy-workspace").
		Complete(r)
}
