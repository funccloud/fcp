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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// WorkspaceReconciler reconciles a Workspace object
type WorkspaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=*,resources=*,verbs=*

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Workspace object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, err error) {
	l := log.FromContext(ctx)
	l = l.WithValues("workspace", req.NamespacedName)
	l.Info("Reconciling Workspace")
	// Fetch the Workspace instance
	workspace := tenancyv1alpha1.Workspace{}
	if err := r.Get(ctx, req.NamespacedName, &workspace); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if workspace.Status.ObservedGeneration >= workspace.Generation {
		l.Info("Workspace status already up to date")
		return ctrl.Result{}, nil
	}
	// add finalizer if not present
	if !controllerutil.ContainsFinalizer(&workspace, tenancyv1alpha1.WorkspaceFinalizer) {
		controllerutil.AddFinalizer(&workspace, tenancyv1alpha1.WorkspaceFinalizer)
		if err := r.Update(ctx, &workspace); err != nil {
			return ctrl.Result{}, err
		}
		l.Info("Added finalizer to Workspace")
		return ctrl.Result{}, nil
	}
	obj := workspace.DeepCopy()
	defer func() {
		if !controllerutil.ContainsFinalizer(&workspace, tenancyv1alpha1.WorkspaceFinalizer) {
			// The object is being deleted
			return
		}
		errs := []error{}
		if err != nil {
			errs = append(errs, err)
		}
		workspace.Status.ObservedGeneration = workspace.Generation
		if uerr := r.Status().Update(ctx, &workspace); uerr != nil {
			l.Error(err, "unable to update Workspace status")
			errs = append(errs, uerr)
		}
		if uerr := r.Patch(ctx, &workspace, client.MergeFrom(obj)); uerr != nil {
			l.Error(err, "unable to patch Workspace")
			errs = append(errs, uerr)
		}
		if len(errs) > 0 {
			err = kerrors.NewAggregate(errs)
		}
		l.Info("Workspace status updated", "workspace", workspace.Name)
		l.Info("Workspace patched", "workspace", workspace.Name)
	}()
	// check if the workspace is marked for deletion
	if !workspace.DeletionTimestamp.IsZero() {
		// The object is being deleted
		return ctrl.Result{}, r.reconcileDeletion(ctx, l, &workspace)
	}
	// reconcile the workspace
	return ctrl.Result{}, r.reconcile(ctx, l, &workspace)
}

func (r *WorkspaceReconciler) reconcile(ctx context.Context, l logr.Logger, workspace *tenancyv1alpha1.Workspace) error {
	ns := corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: workspace.Name,
		},
	}
	res, err := controllerutil.CreateOrUpdate(ctx, r.Client, &ns, func() error {
		if ns.Labels == nil {
			ns.Labels = make(map[string]string)
		}
		ns.Labels[tenancyv1alpha1.WorkspaceLinkedResourceLabel] = workspace.Name
		ns.OwnerReferences = append(ns.OwnerReferences, metav1.OwnerReference{
			APIVersion: workspace.APIVersion,
			Kind:       workspace.Kind,
			Name:       workspace.Name,
			UID:        workspace.UID,
		})
		ns.OwnerReferences = sets.New(ns.OwnerReferences...).UnsortedList()
		return nil
	})
	if err != nil {
		l.Error(err, "unable to create or update namespace")
		workspace.Status.SetCondition(metav1.Condition{
			Type:    tenancyv1alpha1.NamespaceReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  tenancyv1alpha1.NamespaceCreatedReason,
			Message: err.Error(),
		})
		workspace.Status.SetCondition(metav1.Condition{
			Type:    tenancyv1alpha1.ReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  tenancyv1alpha1.NamespaceCreatedReason,
			Message: err.Error(),
		})
		return err
	}
	if res != controllerutil.OperationResultNone {
		l.Info("Created or updated namespace", "namespace", ns.Name, "operation", res)
	}
	workspace.Status.SetCondition(metav1.Condition{
		Type:    tenancyv1alpha1.NamespaceReadyConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  tenancyv1alpha1.NamespaceCreatedReason,
		Message: fmt.Sprintf("Namespace %s created", ns.Name),
	})
	workspace.Status.SetCondition(metav1.Condition{
		Type:    tenancyv1alpha1.ReadyConditionType,
		Status:  metav1.ConditionFalse,
		Reason:  tenancyv1alpha1.NamespaceCreatedReason,
		Message: fmt.Sprintf("Resources are being created in namespace %s", ns.Name),
	})

	roleOwner := rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: rbacv1.SchemeGroupVersion.String(),
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      workspace.Name,
			Namespace: ns.Name,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"*"},
				Resources: []string{"*"},
				Verbs:     []string{"*"},
			},
		},
	}
	res, err = controllerutil.CreateOrUpdate(ctx, r.Client, &roleOwner, func() error {
		if roleOwner.Labels == nil {
			roleOwner.Labels = make(map[string]string)
		}
		roleOwner.Labels[tenancyv1alpha1.WorkspaceLinkedResourceLabel] = workspace.Name
		roleOwner.OwnerReferences = append(roleOwner.OwnerReferences, metav1.OwnerReference{
			APIVersion: workspace.APIVersion,
			Kind:       workspace.Kind,
			Name:       workspace.Name,
			UID:        workspace.UID,
		})
		roleOwner.OwnerReferences = sets.New(roleOwner.OwnerReferences...).UnsortedList()
		return nil
	})
	if err != nil {
		workspace.Status.SetCondition(metav1.Condition{
			Type:    tenancyv1alpha1.RbacCreatedReason,
			Status:  metav1.ConditionFalse,
			Reason:  tenancyv1alpha1.RbacCreatedReason,
			Message: err.Error(),
		})
		workspace.Status.SetCondition(metav1.Condition{
			Type:    tenancyv1alpha1.ReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  tenancyv1alpha1.RbacCreatedReason,
			Message: err.Error(),
		})
		l.Error(err, "unable to create or update role")
		return err
	}
	if res != controllerutil.OperationResultNone {
		l.Info("Created or updated role", "role", roleOwner.Name, "operation", res)
	}
	roleBinding := rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: rbacv1.SchemeGroupVersion.String(),
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("fcp-ownership-%s", workspace.Name),
			Namespace: ns.Name,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.SchemeGroupVersion.Group,
			Kind:     "Role",
			Name:     roleOwner.Name,
		},
	}
	for _, owner := range workspace.Spec.Owners {
		roleBinding.Subjects = append(roleBinding.Subjects, rbacv1.Subject{
			APIGroup: owner.GroupVersionKind().Group,
			Kind:     owner.Kind,
			Name:     owner.Name,
		})
	}
	res, err = controllerutil.CreateOrUpdate(ctx, r.Client, &roleBinding, func() error {
		if roleBinding.Labels == nil {
			roleBinding.Labels = make(map[string]string)
		}
		roleBinding.Labels[tenancyv1alpha1.WorkspaceLinkedResourceLabel] = workspace.Name
		roleBinding.OwnerReferences = append(roleBinding.OwnerReferences, metav1.OwnerReference{
			APIVersion: workspace.APIVersion,
			Kind:       workspace.Kind,
			Name:       workspace.Name,
			UID:        workspace.UID,
		})
		roleBinding.OwnerReferences = sets.New(roleBinding.OwnerReferences...).UnsortedList()
		return nil
	})
	if err != nil {
		workspace.Status.SetCondition(metav1.Condition{
			Type:    tenancyv1alpha1.RbacCreatedReason,
			Status:  metav1.ConditionFalse,
			Reason:  tenancyv1alpha1.RbacCreatedReason,
			Message: err.Error(),
		})
		workspace.Status.SetCondition(metav1.Condition{
			Type:    tenancyv1alpha1.ReadyConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  tenancyv1alpha1.RbacCreatedReason,
			Message: err.Error(),
		})
		l.Error(err, "unable to create or update role binding")
		return err
	}
	if res != controllerutil.OperationResultNone {
		l.Info("Created or updated role binding", "roleBinding", roleBinding.Name, "operation", res)
	}
	workspace.Status.SetCondition(metav1.Condition{
		Type:    tenancyv1alpha1.RbacReadyConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  tenancyv1alpha1.NamespaceCreatedReason,
		Message: fmt.Sprintf("Role and RoleBinding created in namespace %s", ns.Name),
	})
	workspace.Status.SetCondition(metav1.Condition{
		Type:    tenancyv1alpha1.ReadyConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  tenancyv1alpha1.RbacCreatedReason,
		Message: fmt.Sprintf("Workspace %s is ready", workspace.Name),
	})
	// reconcile the workspace
	return nil
}

func (r *WorkspaceReconciler) reconcileDeletion(ctx context.Context, l logr.Logger, workspace *tenancyv1alpha1.Workspace) error {
	rolebindList := rbacv1.RoleBindingList{}
	err := r.Client.List(ctx, &rolebindList, client.MatchingLabels{
		tenancyv1alpha1.WorkspaceLinkedResourceLabel: workspace.Name,
	})
	if err != nil {
		l.Error(err, "unable to list role bindings")
		return err
	}
	for _, rolebind := range rolebindList.Items {
		err = r.Client.Delete(ctx, &rolebind)
		if err != nil {
			l.Error(err, "unable to delete role binding", "roleBinding", rolebind.Name)
			return err
		}
		l.Info("Deleted role binding", "roleBinding", rolebind.Name)
	}
	roleList := rbacv1.RoleList{}
	err = r.Client.List(ctx, &roleList, client.MatchingLabels{
		tenancyv1alpha1.WorkspaceLinkedResourceLabel: workspace.Name,
	})
	if err != nil {
		l.Error(err, "unable to list roles")
		return err
	}
	for _, role := range roleList.Items {
		err = r.Client.Delete(ctx, &role)
		if err != nil {
			l.Error(err, "unable to delete role", "role", role.Name)
			return err
		}
		l.Info("Deleted role", "role", role.Name)
	}
	ns := corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: workspace.Name,
		},
	}
	err = r.Client.Delete(ctx, &ns)
	if err != nil {
		l.Error(err, "unable to delete namespace", "namespace", ns.Name)
		return err
	}
	l.Info("Deleted namespace", "namespace", ns.Name)
	// Remove the finalizer from the list and update it
	if controllerutil.ContainsFinalizer(workspace, tenancyv1alpha1.WorkspaceFinalizer) {
		// Perform any necessary cleanup here
		// Remove the finalizer from the list and update it
		controllerutil.RemoveFinalizer(workspace, tenancyv1alpha1.WorkspaceFinalizer)
		if err := r.Update(ctx, workspace); err != nil {
			return err
		}
		l.Info("Removed finalizer from Workspace")
		// Stop reconciliation as the item is being deleted
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&tenancyv1alpha1.Workspace{}).
		Named("tenancy-workspace").
		Owns(&corev1.Namespace{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Complete(r)
}
