package v1alpha1

const (
	// ReadyConditionType indicates the overall readiness of the Application resource.
	ReadyConditionType = "Ready"
	// KnativeServiceReadyConditionType indicates the readiness of the underlying Knative Service.
	KnativeServiceReadyConditionType = "KnativeServiceReady"
	// DomainMappingReadyConditionType indicates the readiness of the Knative DomainMapping.
	DomainMappingReadyConditionType = "DomainMappingReady"
)

// Reasons for Condition Types
const (
	// --- Ready Condition Reasons ---
	ReconciliationFailedReason = "ReconciliationFailed"
	ResourcesCreatedReason     = "ResourcesCreated"

	// --- KnativeServiceReady Condition Reasons ---
	KnativeServiceCreationFailedReason    = "KnativeServiceCreationFailed"
	KnativeServiceStatusCheckFailedReason = "KnativeServiceStatusCheckFailed"
	KnativeServiceNotFoundReason          = "KnativeServiceNotFound"
	KnativeServiceNotReadyReason          = "KnativeServiceNotReady"
	KnativeServiceReadyReason             = "KnativeServiceReady"

	// --- DomainMappingReady Condition Reasons ---
	DomainMappingCheckFailedReason    = "DomainMappingCheckFailed" // Added
	DomainMappingConflictReason       = "DomainMappingConflict"    // Added
	DomainMappingCreationFailedReason = "DomainMappingCreationFailed"
	DomainMappingNotConfiguredReason  = "DomainMappingNotConfigured"
	DomainMappingReadyReason          = "DomainMappingReady"
	DomainMappingCleanupFailedReason  = "DomainMappingCleanupFailed" // Added
)
