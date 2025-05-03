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

	tenancyv1alpha1 "go.funccloud.dev/fcp/api/tenancy/v1alpha1"
	workloadv1alpha1 "go.funccloud.dev/fcp/api/workload/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// nolint:unused
// log is for logging in this package.
var applicationlog = logf.Log.WithName("application-resource")

// SetupApplicationWebhookWithManager registers the webhook for Application in the manager.
func SetupApplicationWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&workloadv1alpha1.Application{}).
		WithValidator(&ApplicationCustomValidator{
			Client: mgr.GetClient(),
		}).
		WithDefaulter(&ApplicationCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-workload-fcp-funccloud-com-v1alpha1-application,mutating=true,failurePolicy=fail,sideEffects=None,groups=workload.fcp.funccloud.com,resources=applications,verbs=create;update,versions=v1alpha1,name=mapplication-v1alpha1.kb.io,admissionReviewVersions=v1

// ApplicationCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Application when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type ApplicationCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &ApplicationCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Application.
func (d *ApplicationCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	application, ok := obj.(*workloadv1alpha1.Application)
	if !ok {
		return fmt.Errorf("expected an Application object but got %T", obj)
	}
	applicationlog.Info("Defaulting for Application", "name", application.GetName())
	if application.Labels == nil {
		application.Labels = make(map[string]string)
	}
	application.Labels[tenancyv1alpha1.WorkspaceLinkedResourceLabel] = application.Namespace
	if application.Spec.RolloutDuration == nil {
		application.Spec.RolloutDuration = &metav1.Duration{
			Duration: workloadv1alpha1.DefaultRolloutDuration,
		}
	}
	if application.Spec.EnableTLS == nil {
		application.Spec.EnableTLS = ptr.To(workloadv1alpha1.DefaultEnableTLS)
	}
	if application.Spec.Scale.Metric == "" {
		application.Spec.Scale.Metric = workloadv1alpha1.MetricConcurrency
	}
	if application.Spec.Scale.Target == nil && application.Spec.Scale.TargetUtilizationPercentage == nil {
		application.Spec.Scale.Target = ptr.To(workloadv1alpha1.DefaultTargetUtilization)
	}
	if application.Spec.Scale.MinReplicas == nil {
		application.Spec.Scale.MinReplicas = ptr.To(workloadv1alpha1.DefaultMinReplicas)
	}
	if application.Spec.Scale.MaxReplicas == nil {
		application.Spec.Scale.MaxReplicas = ptr.To(workloadv1alpha1.DefaultMaxReplicas)
	}
	if application.Spec.Ports == nil {
		application.Spec.Ports = []corev1.ContainerPort{{ContainerPort: 80}}
	}
	return nil
}

// +kubebuilder:webhook:path=/validate-workload-fcp-funccloud-com-v1alpha1-application,mutating=false,failurePolicy=fail,sideEffects=None,groups=workload.fcp.funccloud.com,resources=applications,verbs=create;update;delete,versions=v1alpha1,name=vapplication-v1alpha1.kb.io,admissionReviewVersions=v1

// ApplicationCustomValidator struct is responsible for validating the Application resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ApplicationCustomValidator struct {
	client.Client
}

var _ webhook.CustomValidator = &ApplicationCustomValidator{}

func (v *ApplicationCustomValidator) validate(ctx context.Context, application *workloadv1alpha1.Application) field.ErrorList {
	var errs field.ErrorList
	workspace := tenancyv1alpha1.Workspace{}
	if err := v.Get(ctx, client.ObjectKey{Name: application.Namespace}, &workspace); err != nil {
		if client.IgnoreNotFound(err) != nil {
			errs = append(errs, field.Invalid(field.NewPath("metadata").Child("namespace"),
				application.Namespace, err.Error()))
			return errs
		}
		errs = append(errs, field.Invalid(field.NewPath("metadata").Child("namespace"),
			application.Namespace, "workspace not found"))
	}
	if application.Spec.Image == "" {
		errs = append(errs, field.Required(field.NewPath("spec").Child("image"), "image is required"))
	}
	if application.Spec.Scale.MinReplicas == nil {
		errs = append(errs, field.Required(field.NewPath("spec").Child("scale").Child("minReplicas"),
			"minReplicas is required"))
	}
	if application.Spec.Scale.MaxReplicas == nil {
		errs = append(errs, field.Required(field.NewPath("spec").Child("scale").Child("maxReplicas"),
			"maxReplicas is required"))
	}
	if *application.Spec.Scale.MinReplicas > *application.Spec.Scale.MaxReplicas {
		errs = append(errs, field.Invalid(field.NewPath("spec", "scale", "minReplicas"), application.Spec.Scale.MinReplicas, "minReplicas must be less than or equal to maxReplicas"))
	}
	if application.Spec.Ports == nil {
		errs = append(errs, field.Required(field.NewPath("spec").Child("ports"),
			"ports is required"))
	}
	return errs
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Application.
func (v *ApplicationCustomValidator) ValidateCreate(
	ctx context.Context, obj runtime.Object,
) (admission.Warnings, error) {
	application, ok := obj.(*workloadv1alpha1.Application)
	if !ok {
		return nil, fmt.Errorf("expected a Application object but got %T", obj)
	}
	applicationlog.Info("Validation for Application upon creation", "name", application.GetName())

	errs := v.validate(ctx, application)
	// check if workspace exists and namespaces are the same nam
	if len(errs) > 0 {
		return nil, apierrors.NewInvalid(
			workloadv1alpha1.GroupVersion.WithKind("Application").GroupKind(),
			application.GetName(), errs)
	}
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Application.
func (v *ApplicationCustomValidator) ValidateUpdate(
	ctx context.Context, oldObj, newObj runtime.Object,
) (admission.Warnings, error) {
	application, ok := newObj.(*workloadv1alpha1.Application)
	if !ok {
		return nil, fmt.Errorf("expected a Application object for the newObj but got %T", newObj)
	}
	applicationlog.Info("Validation for Application upon update", "name", application.GetName())
	errs := v.validate(ctx, application)
	if len(errs) > 0 {
		return nil, apierrors.NewInvalid(
			workloadv1alpha1.GroupVersion.WithKind("Application").GroupKind(),
			application.GetName(), errs)
	}
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Application.
func (v *ApplicationCustomValidator) ValidateDelete(
	ctx context.Context, obj runtime.Object,
) (admission.Warnings, error) {
	application, ok := obj.(*workloadv1alpha1.Application)
	if !ok {
		return nil, fmt.Errorf("expected a Application object but got %T", obj)
	}
	applicationlog.Info("Validation for Application upon deletion", "name", application.GetName())

	return nil, nil
}
