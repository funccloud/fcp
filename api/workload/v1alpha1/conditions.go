package v1alpha1

const (
	// ReadyConditionType is the condition type for the Ready condition.
	ReadyConditionType = "Ready"
	// KnativeServiceReadyConditionType is the condition type for the KnativeServiceReady condition.
	KnativeServiceReadyConditionType = "KnativeServiceReady"
	// DomainMappingReadyConditionType is the condition type for the DomainMappingReady condition.
	DomainMappingReadyConditionType = "DomainMappingReady"
)

const (
	// ReconciliationFailedReason is the reason when reconciliation fails.
	ReconciliationFailedReason = "ReconciliationFailed"
	// ResourcesCreatedReason is the reason when all resources are successfully created/updated.
	ResourcesCreatedReason = "ResourcesCreated"
	// KnativeServiceCreationFailedReason is the reason when Knative Service creation fails.
	KnativeServiceCreationFailedReason = "KnativeServiceCreationFailed"
	// KnativeServiceReadyReason is the reason when Knative Service is ready.
	KnativeServiceReadyReason = "KnativeServiceReady"
	// DomainMappingCreationFailedReason is the reason when DomainMapping creation fails.
	DomainMappingCreationFailedReason = "DomainMappingCreationFailed"
	// DomainMappingReadyReason is the reason when DomainMapping is ready.
	DomainMappingReadyReason = "DomainMappingReady"
	// DomainMappingNotConfiguredReason is the reason when DomainMapping is not configured in the spec.
	DomainMappingNotConfiguredReason = "DomainMappingNotConfigured"
)
