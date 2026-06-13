package v1alpha1

// DemandShape describes the dominant runtime pressure of an AI workload.
type DemandShape string

const (
	DemandShapeMixed        DemandShape = "mixed"
	DemandShapeKVHeavy      DemandShape = "kv_heavy"
	DemandShapePrefillHeavy DemandShape = "prefill_heavy"
	DemandShapeDecodeHeavy  DemandShape = "decode_heavy"
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
