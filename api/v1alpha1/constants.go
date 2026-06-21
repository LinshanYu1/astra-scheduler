package v1alpha1

// DemandShape describes the dominant runtime pressure of an AI workload.
type DemandShape string

const (
	DemandShapeMixed        DemandShape = "mixed"
	DemandShapeKVHeavy      DemandShape = "kv_heavy"
	DemandShapePrefillHeavy DemandShape = "prefill_heavy"
	DemandShapeDecodeHeavy  DemandShape = "decode_heavy"
)

type WorkloadType string

const (
	WorkloadTypeOnline  WorkloadType = "online"
	WorkloadTypeOffline WorkloadType = "offline"
	WorkloadTypeHybrid  WorkloadType = "hybrid"
	WorkloadTypeBatch   WorkloadType = "batch"
)

type WorkloadPriority string

const (
	WorkloadPriorityCritical WorkloadPriority = "critical"
	WorkloadPriorityHigh     WorkloadPriority = "high"
	WorkloadPriorityMedium   WorkloadPriority = "medium"
	WorkloadPriorityLow      WorkloadPriority = "low"
)

type SLORiskLevel string

const (
	SLORiskLow    SLORiskLevel = "low"
	SLORiskMedium SLORiskLevel = "medium"
	SLORiskHigh   SLORiskLevel = "high"
)

type AllocationAction string

const (
	AllocationActionApply    AllocationAction = "apply"
	AllocationActionThrottle AllocationAction = "throttle"
	AllocationActionPause    AllocationAction = "pause"
	AllocationActionEvict    AllocationAction = "evict"
)

type AllocationPhase string

const (
	AllocationPhasePending      AllocationPhase = "Pending"
	AllocationPhasePendingApply AllocationPhase = "PendingApply"
	AllocationPhaseApplying     AllocationPhase = "Applying"
	AllocationPhaseApplied      AllocationPhase = "Applied"
	AllocationPhaseMigrating    AllocationPhase = "Migrating"
	AllocationPhaseReleased     AllocationPhase = "Released"
	AllocationPhaseFailed       AllocationPhase = "Failed"
)

type AllocationTier string

const (
	AllocationTierRequired  AllocationTier = "required"
	AllocationTierPreferred AllocationTier = "preferred"
	AllocationTierMax       AllocationTier = "max"
	AllocationTierDegraded  AllocationTier = "degraded"
	AllocationTierRejected  AllocationTier = "rejected"
)

// ResourceName is the policy-facing name of a schedulable AI resource.
type ResourceName string

const (
	ResourceGPUCount              ResourceName = "gpuCount"
	ResourceGPUMemoryGiB          ResourceName = "gpuMemoryGiB"
	ResourceKVCacheGiB            ResourceName = "kvCacheGiB"
	ResourcePrefillTokensPerSec   ResourceName = "prefillTokensPerSecond"
	ResourceDecodeTokensPerSec    ResourceName = "decodeTokensPerSecond"
	ResourceTotalTokensPerSecond  ResourceName = "totalTokensPerSecond"
	ResourceBalancedResourceFocus ResourceName = "balanced"
)
