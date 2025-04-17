// Copyright 2025
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1alpha1

const (
	// ReadyConditionType is the condition type for the Ready condition.
	ReadyConditionType = "Ready"
	// NamespaceReadyConditionType is the condition type for the NamespaceReady condition.
	NamespaceReadyConditionType = "NamespaceReady"
	// RbacReadyConditionType is the condition type for the RbacReady condition.
	RbacReadyConditionType = "RbacReady"
)

const (
	// NamespaceCreatedReason is the reason for the NamespaceCreated condition.
	NamespaceCreatedReason = "NamespaceCreated"
	// NamespaceCreationFailedReason is the reason when namespace creation fails.
	NamespaceCreationFailedReason = "NamespaceCreationFailed"
	// RbacCreatedReason is the reason for the RbacCreated condition.
	RbacCreatedReason = "RbacCreated"
	// RbacCreationFailedReason is the reason when RBAC resource creation fails.
	RbacCreationFailedReason = "RbacCreationFailed"
	// ResourcesCreatedReason is the reason when all resources are successfully created/updated.
	ResourcesCreatedReason = "ResourcesCreated"
)
