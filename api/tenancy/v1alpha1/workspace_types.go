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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WorkspaceType string

const (
	// WorkspaceTypePersonal is a workspace type for personal use.
	WorkspaceTypePersonal WorkspaceType = "personal"
	// WorkspaceTypeOrganization is a workspace type for organization use.
	WorkspaceTypeOrganization WorkspaceType = "organization"
)

// WorkspaceSpec defines the desired state of Workspace.
type WorkspaceSpec struct {
	// Type is the type of the workspace.
	// +kubebuilder:validation:Enum=personal;organization
	// kubebuilder:validation:Required
	Type WorkspaceType `json:"type,omitempty"`
	// Owner is the owner of the workspace.
	// +kubebuilder:validation:MinItems=1
	// kubebuilder:validation:Required
	Owners []corev1.ObjectReference `json:"owners,omitempty"`
}

// WorkspaceStatus defines the observed state of Workspace.
type WorkspaceStatus struct {
	// Conditions the latest available observations of a resource's current state.
	Status `json:",inline"`
	// LinkedResources is the list of resources linked to this workspace.
	LinkedResources []corev1.ObjectReference `json:"linkedResources,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName="ws"
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type",description="The type of the workspace"
// +kubebuilder:printcolumn:name="Owners",type="string",JSONPath=".spec.owners[*].name",description="The owners of the workspace"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].message",description="The status of the workspace"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status",description="The status of the workspace"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="The age of the workspace"
// Workspace is the Schema for the workspaces API.
type Workspace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkspaceSpec   `json:"spec,omitempty"`
	Status WorkspaceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkspaceList contains a list of Workspace.
type WorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workspace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Workspace{}, &WorkspaceList{})
}
