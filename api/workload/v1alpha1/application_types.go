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
	tenancyv1alpha1 "go.funccloud.dev/fcp/api/tenancy/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApplicationSpec defines the desired state of Application.
type ApplicationSpec struct {
	// Workspace is the name of the workspace where the application is deployed
	// +kubebuilder:validation:Required
	Workspace string `json:"workspace,omitempty"`
	// Image is the image of the application
	// +kubebuilder:validation:Required
	Image string `json:"image,omitempty"`
	// Scale is the scale of the application
	Scale Scale `json:"scale,omitempty"`
	// Resources is the resources of the application
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// Env is the environment variables of the application
	Env []corev1.EnvVar `json:"env,omitempty"`
	// EnvFrom is the environment variables from the secret of the application
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`
	// Command is the command of the application
	Command []string `json:"command,omitempty"`
	// Args is the arguments of the application
	Args []string `json:"args,omitempty"`
	// LivenessProbe is the liveness probe of the application
	LivenessProbe *corev1.Probe `json:"livenessProbe,omitempty"`
	// ReadinessProbe is the readiness probe of the application
	ReadinessProbe *corev1.Probe `json:"readinessProbe,omitempty"`
	// StartupProbe is the startup probe of the application
	StartupProbe *corev1.Probe `json:"startupProbe,omitempty"`
	// ImagePullSecrets is the image pull secrets of the application
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// RolloutDuration is the rollout duration of the application
	// +kubebuilder:validation:Required
	RolloutDuration *metav1.Duration `json:"rolloutDuration,omitempty"`
	// EnableTLS indicates whether to enable TLS for the application
	// +kubebuilder:validation:Required
	EnableTLS *bool `json:"enableTLS,omitempty"`
	// Domain is the custom domain of the application
	Domain string `json:"domain,omitempty"`
}

type Scale struct {
	// MinReplicas is the minimum number of replicas for the application
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=0
	MinReplicas *int32 `json:"minReplicas,omitempty"`
	// MaxReplicas is the maximum number of replicas for the application
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`
}

// ApplicationStatus defines the observed state of Application.
type ApplicationStatus struct {
	tenancyv1alpha1.Status `json:",inline"`
	// URLs is the list of URLs of the application
	URLs []string `json:"urls,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName="app"
// +kubebuilder:printcolumn:name="Workspace",type="string",JSONPath=".spec.workspace",description="The workspace of the application"
// +kubebuilder:printcolumn:name="URLs",type="string",JSONPath=".status.urls",description="The URLs of the application"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].message",description="The status of the workspace"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status",description="The status of the workspace"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="The age of the workspace"
// Application is the Schema for the applications API.
type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationSpec   `json:"spec,omitempty"`
	Status ApplicationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ApplicationList contains a list of Application.
type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Application `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Application{}, &ApplicationList{})
}
