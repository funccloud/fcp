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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/serving/pkg/apis/autoscaling"
)

const (
	// ApplicationFinalizer is the finalizer for the Application
	ApplicationFinalizer = "application.fcp.funccloud.com/finalizer"
	// ApplicationLabel is the label for the Application
	ApplicationLabel = "fcp.funccloud.com/application"
	// DefaultRolloutDuration is the default rollout duration for the Application
	DefaultRolloutDuration = 5 * time.Minute
	// DefaultEnableTLS is the default enable TLS for the Application
	DefaultEnableTLS = true
	// DefaultTargetUtilization is the default target utilization for the Application
	DefaultTargetUtilization = int32(80)
	// DefaultMinReplicas is the default minimum replicas for the Application
	DefaultMinReplicas = int32(0)
	// DefaultMaxReplicas is the default maximum replicas for the Application
	DefaultMaxReplicas = int32(1)
)

type Metric string

const (
	// MetricCPU is the CPU metric
	MetricCPU Metric = "cpu"
	// MetricMemory is the Memory metric
	MetricMemory Metric = "memory"
	// MetricConcurrency is the Concurrency metric
	MetricConcurrency Metric = "concurrency"
	// MetricRPS is the Requests per second metric
	MetricRPS Metric = "rps"
)

func (m Metric) GetClass() string {
	switch m {
	case MetricCPU, MetricMemory:
		return autoscaling.HPA
	case MetricConcurrency, MetricRPS:
		return autoscaling.KPA
	}
	return autoscaling.HPA
}

// ApplicationSpec defines the desired state of Application.
type ApplicationSpec struct {
	// Containers is the list of containers of the application
	Containers []corev1.Container `json:"containers,omitempty"`
	// +kubebuilder:validation:Required
	// Scale is the scale of the application
	Scale Scale `json:"scale,omitempty"`
	// ImagePullSecrets is the image pull secrets of the application
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// RolloutDuration is the rollout duration of the application
	// +kubebuilder:validation:Required
	RolloutDuration *metav1.Duration `json:"rolloutDuration,omitempty"`
	// EnableTLS indicates whether to enable TLS for the application
	// +kubebuilder:validation:Required
	EnableTLS *bool `json:"enableTLS,omitempty"`
	// Domains is the custom domains of the application
	Domains []string `json:"domains,omitempty"`
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
	// TargetUtilizationPercentage is the target  utilization percentage for the application
	TargetUtilizationPercentage *int32 `json:"targetUtilizationPercentage,omitempty"`
	// Target is the target of the application
	Target *int32 `json:"target,omitempty"`
	// Metric is the metric of the application
	Metric Metric `json:"metric,omitempty"`
}

// ApplicationStatus defines the observed state of Application.
type ApplicationStatus struct {
	Status `json:",inline"` // Embed workload status
	// URLs is the list of URLs of the application
	URLs []string `json:"urls,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName="app"
// +kubebuilder:printcolumn:name="Workspace",type="string",JSONPath=".metadata.namespace",description="The workspace of the application"
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
