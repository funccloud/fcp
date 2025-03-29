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

package v1alpha1

import (
	"context"
	"fmt"
	"reflect"

	tenancyv1alpha1 "go.funccloud.dev/fcp/api/tenancy/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// nolint:unused
// log is for logging in this package.
var workspacelog = logf.Log.WithName("workspace-resource")

// SetupWorkspaceWebhookWithManager registers the webhook for Workspace in the manager.
func SetupWorkspaceWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&tenancyv1alpha1.Workspace{}).
		WithValidator(&WorkspaceCustomValidator{}).
		WithDefaulter(&WorkspaceCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-tenancy-fcp-funccloud-com-v1alpha1-workspace,mutating=true,failurePolicy=fail,sideEffects=None,groups=tenancy.fcp.funccloud.com,resources=workspaces,verbs=create;update,versions=v1alpha1,name=mworkspace-v1alpha1.kb.io,admissionReviewVersions=v1

// WorkspaceCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Workspace when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type WorkspaceCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &WorkspaceCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Workspace.
func (d *WorkspaceCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	workspace, ok := obj.(*tenancyv1alpha1.Workspace)
	if !ok {
		return fmt.Errorf("expected an Workspace object but got %T", obj)
	}
	workspacelog.Info("Defaulting for Workspace", "name", workspace.GetName())
	if len(workspace.Spec.Owners) > 1 {
		workspace.Spec.Owners = sets.New(workspace.Spec.Owners...).UnsortedList()
	}
	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-tenancy-fcp-funccloud-com-v1alpha1-workspace,mutating=false,failurePolicy=fail,sideEffects=None,groups=tenancy.fcp.funccloud.com,resources=workspaces,verbs=create;update;delete,versions=v1alpha1,name=vworkspace-v1alpha1.kb.io,admissionReviewVersions=v1

// WorkspaceCustomValidator struct is responsible for validating the Workspace resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type WorkspaceCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &WorkspaceCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Workspace.
func (v *WorkspaceCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	workspace, ok := obj.(*tenancyv1alpha1.Workspace)
	if !ok {
		return nil, fmt.Errorf("expected a Workspace object but got %T", obj)
	}
	workspacelog.Info("Validation for Workspace upon creation", "name", workspace.GetName())
	errs := validateWorkspace(workspace)
	if len(errs) > 0 {
		return nil, errors.NewAggregate(errs)
	}
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Workspace.
func (v *WorkspaceCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	workspace, ok := newObj.(*tenancyv1alpha1.Workspace)
	if !ok {
		return nil, fmt.Errorf("expected a Workspace object for the newObj but got %T", newObj)
	}
	workspacelog.Info("Validation for Workspace upon update", "name", workspace.GetName())
	workspaceOld, ok := oldObj.(*tenancyv1alpha1.Workspace)
	if !ok {
		return nil, fmt.Errorf("expected a Workspace object for the oldObj but got %T", newObj)
	}
	errs := validateWorkspace(workspace)
	if workspaceOld.Spec.Type != workspace.Spec.Type {
		errs = append(errs, fmt.Errorf("workspaceType is immutable"))
	}
	if workspace.Spec.Type == tenancyv1alpha1.WorkspaceTypePersonal &&
		!reflect.DeepEqual(workspace.Spec.Owners, workspaceOld.Spec.Owners) {
		errs = append(errs, fmt.Errorf("owners is immutable for personal workspaces"))
	}
	if len(errs) > 0 {
		return nil, errors.NewAggregate(errs)
	}
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Workspace.
func (v *WorkspaceCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	workspace, ok := obj.(*tenancyv1alpha1.Workspace)
	if !ok {
		return nil, fmt.Errorf("expected a Workspace object but got %T", obj)
	}
	workspacelog.Info("Validation for Workspace upon deletion", "name", workspace.GetName())
	return nil, nil
}

func validateWorkspace(workspace *tenancyv1alpha1.Workspace) []error {
	errs := make([]error, 0)
	if workspace.Spec.Type == "" {
		errs = append(errs, fmt.Errorf("workspaceType is required"))
	}
	if workspace.Spec.Type != tenancyv1alpha1.WorkspaceTypePersonal &&
		workspace.Spec.Type != tenancyv1alpha1.WorkspaceTypeOrganization {
		errs = append(errs, fmt.Errorf("workspaceType must be either personal or organization"))
	}
	if len(workspace.Spec.Owners) == 0 {
		errs = append(errs, fmt.Errorf("owners is required"))
	}
	if workspace.Spec.Type == tenancyv1alpha1.WorkspaceTypePersonal &&
		len(workspace.Spec.Owners) > 1 {
		errs = append(errs, fmt.Errorf("owners must be a single owner for personal workspaces"))
	}
	if workspace.Spec.Type == tenancyv1alpha1.WorkspaceTypePersonal &&
		workspace.Spec.Owners[0].Kind != "User" {
		errs = append(errs, fmt.Errorf("owners must be a User for personal workspaces"))
	}
	if workspace.Spec.Type == tenancyv1alpha1.WorkspaceTypePersonal &&
		workspace.Spec.Owners[0].Name != workspace.Name {
		errs = append(errs, fmt.Errorf("owners must be the same as the workspace name for personal workspaces"))
	}
	return errs
}
