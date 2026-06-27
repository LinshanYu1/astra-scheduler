/*
Copyright 2026.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AINodeResourceProfileSpec defines the desired AI resource model exposed by a node.
type AINodeResourceProfileSpec struct {
	// nodeName is the Kubernetes node represented by this profile.
	// +required
	NodeName string `json:"nodeName"`

	// backendType identifies the node-side backend that reports and applies resources.
	// Examples: fake, nvidia, runtime, cri.
	// +optional
	BackendType string `json:"backendType,omitempty"`

	// gpus describes static GPU device capability from the scheduler's view.
	// +optional
	// +listType=map
	// +listMapKey=id
	GPUs []GPUDeviceSpec `json:"gpus,omitempty"`

	// runtime describes AI runtime capacity and supported serving features.
	// +optional
	Runtime *RuntimeSpec `json:"runtime,omitempty"`

	// capabilities describes which node-side adjustment actions are supported.
	// +optional
	Capabilities *NodeCapabilities `json:"capabilities,omitempty"`
}

// GPUDeviceSpec describes static capability of one GPU device.
type GPUDeviceSpec struct {
	// id is a stable local identifier used by Astra.
	// +required
	ID string `json:"id"`

	// uuid is the vendor or backend reported GPU UUID.
	// +optional
	UUID string `json:"uuid,omitempty"`

	// product is the GPU product name, such as A100, H100, L40S, or fake-a100.
	// +optional
	Product string `json:"product,omitempty"`

	// memoryGiB is the physical GPU memory size.
	// +optional
	MemoryGiB int32 `json:"memoryGiB,omitempty"`

	// computeScore is a normalized compute capacity score used by the scheduler.
	// +optional
	ComputeScore int32 `json:"computeScore,omitempty"`

	// migEnabled indicates whether this GPU is currently managed as MIG-capable.
	// +optional
	MIGEnabled bool `json:"migEnabled,omitempty"`
}

// RuntimeSpec describes AI serving/runtime capability on this node.
type RuntimeSpec struct {
	// type identifies the runtime family, such as fake, vllm, sglang, tgi, or tensorrt-llm.
	// +optional
	Type string `json:"type,omitempty"`

	// supports lists runtime features usable by the scheduler and agent.
	// +optional
	Supports *RuntimeSupports `json:"supports,omitempty"`

	// capacity describes the runtime-level schedulable budget.
	// +optional
	Capacity *RuntimeCapacity `json:"capacity,omitempty"`
}

// RuntimeSupports describes AI runtime features.
type RuntimeSupports struct {
	// +optional
	KVCache bool `json:"kvCache,omitempty"`
	// +optional
	TokenThroughput bool `json:"tokenThroughput,omitempty"`
	// +optional
	PrefixCache bool `json:"prefixCache,omitempty"`
	// +optional
	ContinuousBatching bool `json:"continuousBatching,omitempty"`
	// +optional
	RequestThrottling bool `json:"requestThrottling,omitempty"`
	// +optional
	PauseResume bool `json:"pauseResume,omitempty"`
}

// RuntimeCapacity describes theoretical runtime capacity reported by the node agent.
type RuntimeCapacity struct {
	// kvCacheGiB is the KV-cache budget exposed to the scheduler.
	// +optional
	KVCacheGiB int32 `json:"kvCacheGiB,omitempty"`

	// maxConcurrentRequests is the approximate request concurrency limit.
	// +optional
	MaxConcurrentRequests int32 `json:"maxConcurrentRequests,omitempty"`

	// maxBatchTokens is the maximum token count the runtime can batch.
	// +optional
	MaxBatchTokens int32 `json:"maxBatchTokens,omitempty"`

	// tokenThroughput describes prefill/decode token budget.
	// +optional
	TokenThroughput *TokenThroughput `json:"tokenThroughput,omitempty"`
}

// TokenThroughput describes token processing capacity or allocation.
type TokenThroughput struct {
	// +optional
	PrefillTokensPerSecond int32 `json:"prefillTokensPerSecond,omitempty"`
	// +optional
	DecodeTokensPerSecond int32 `json:"decodeTokensPerSecond,omitempty"`
	// +optional
	TotalTokensPerSecond int32 `json:"totalTokensPerSecond,omitempty"`
}

// NodeCapabilities describes operations the node-side backend can apply.
type NodeCapabilities struct {
	// +optional
	Sharing bool `json:"sharing,omitempty"`
	// +optional
	Borrowing bool `json:"borrowing,omitempty"`
	// +optional
	Reclaim bool `json:"reclaim,omitempty"`
	// +optional
	Throttling bool `json:"throttling,omitempty"`
	// +optional
	PauseResume bool `json:"pauseResume,omitempty"`
	// +optional
	Eviction bool `json:"eviction,omitempty"`
	// +optional
	MIGResize bool `json:"migResize,omitempty"`
}

// AINodeResourceProfileStatus defines the observed state of AINodeResourceProfile.
type AINodeResourceProfileStatus struct {
	// phase is the high-level readiness phase reported by the node agent.
	// +optional
	Phase string `json:"phase,omitempty"`

	// observedAt is the last time this profile was refreshed by the agent.
	// +optional
	ObservedAt *metav1.Time `json:"observedAt,omitempty"`

	// gpus describes current GPU and MIG slice state.
	// +optional
	// +listType=map
	// +listMapKey=id
	GPUs []GPUDeviceStatus `json:"gpus,omitempty"`

	// allocatable summarizes schedulable capacity.
	// +optional
	Allocatable *ResourceSummary `json:"allocatable,omitempty"`

	// allocated summarizes currently assigned resources.
	// +optional
	Allocated *ResourceSummary `json:"allocated,omitempty"`

	// available summarizes remaining resources.
	// +optional
	Available *ResourceSummary `json:"available,omitempty"`

	// hourlyForecast summarizes the node-level expected resource envelope for
	// each hour of a day. resourceBuckets[0] is 00:00-01:00 and
	// resourceBuckets[23] is 23:00-24:00.
	// +optional
	HourlyForecast *HourlyResourceForecast `json:"hourlyForecast,omitempty"`

	// runtime describes runtime pressure and observed serving signals.
	// +optional
	Runtime *RuntimeStatus `json:"runtime,omitempty"`

	// allocations lists workload allocations currently applied on this node.
	// +optional
	// +listType=map
	// +listMapKey=name
	Allocations []NodeAllocationStatus `json:"allocations,omitempty"`

	// conditions represent the current state of the AINodeResourceProfile resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// GPUDeviceStatus describes observed GPU state.
type GPUDeviceStatus struct {
	// +required
	ID string `json:"id"`
	// +optional
	UUID string `json:"uuid,omitempty"`
	// +optional
	Allocated bool `json:"allocated,omitempty"`
	// +optional
	MIGEnabled bool `json:"migEnabled,omitempty"`
	// +optional
	AllocatedTo *AllocationRef `json:"allocatedTo,omitempty"`
	// migSlices describes concrete MIG instances available on this GPU.
	// +optional
	// +listType=map
	// +listMapKey=id
	MIGSlices []MIGSliceStatus `json:"migSlices,omitempty"`
}

// MIGSliceStatus describes one concrete schedulable MIG slice.
type MIGSliceStatus struct {
	// +required
	ID string `json:"id"`
	// +optional
	UUID string `json:"uuid,omitempty"`
	// profile is the MIG slice shape, such as 1g.10gb or 2g.20gb.
	// +optional
	Profile string `json:"profile,omitempty"`
	// +optional
	MemoryGiB int32 `json:"memoryGiB,omitempty"`
	// computeSlices is the relative compute partition count represented by this slice.
	// +optional
	ComputeSlices int32 `json:"computeSlices,omitempty"`
	// +optional
	Allocatable bool `json:"allocatable,omitempty"`
	// +optional
	Allocated bool `json:"allocated,omitempty"`
	// +optional
	AllocatedTo *AllocationRef `json:"allocatedTo,omitempty"`
}

// AllocationRef identifies the workload allocation using a resource.
type AllocationRef struct {
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// +optional
	WorkloadName string `json:"workloadName,omitempty"`
	// +optional
	AllocationName string `json:"allocationName,omitempty"`
}

// ResourceSummary summarizes GPU and runtime resources at node level.
type ResourceSummary struct {
	// +optional
	GPUCount int32 `json:"gpuCount,omitempty"`
	// +optional
	GPUMemoryGiB int32 `json:"gpuMemoryGiB,omitempty"`
	// +optional
	KVCacheGiB int32 `json:"kvCacheGiB,omitempty"`
	// +optional
	PrefillTokensPerSecond int32 `json:"prefillTokensPerSecond,omitempty"`
	// +optional
	DecodeTokensPerSecond int32 `json:"decodeTokensPerSecond,omitempty"`
	// +optional
	TotalTokensPerSecond int32 `json:"totalTokensPerSecond,omitempty"`
}

// RuntimeStatus describes observed runtime pressure.
type RuntimeStatus struct {
	// +optional
	Pressure *RuntimePressure `json:"pressure,omitempty"`
}

// RuntimePressure contains percent-based pressure and SLO-related runtime signals.
type RuntimePressure struct {
	// +optional
	GPUUtilizationPercent int32 `json:"gpuUtilizationPercent,omitempty"`
	// +optional
	GPUMemoryUtilizationPercent int32 `json:"gpuMemoryUtilizationPercent,omitempty"`
	// +optional
	KVCacheUtilizationPercent int32 `json:"kvCacheUtilizationPercent,omitempty"`
	// +optional
	PrefillUtilizationPercent int32 `json:"prefillUtilizationPercent,omitempty"`
	// +optional
	DecodeUtilizationPercent int32 `json:"decodeUtilizationPercent,omitempty"`
	// +optional
	QueueDepth int32 `json:"queueDepth,omitempty"`
	// +optional
	TTFTP95Ms int32 `json:"ttftP95Ms,omitempty"`
	// +optional
	TPOTP95Ms int32 `json:"tpotP95Ms,omitempty"`
	// +optional
	LatencyP99Ms int32 `json:"latencyP99Ms,omitempty"`
	// sloRisk is a coarse risk level, such as low, medium, or high.
	// +optional
	SLORisk string `json:"sloRisk,omitempty"`
}

// NodeAllocationStatus describes one workload allocation currently applied on the node.
type NodeAllocationStatus struct {
	// +required
	Name string `json:"name"`
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// +optional
	WorkloadName string `json:"workloadName,omitempty"`
	// +optional
	WorkloadType string `json:"workloadType,omitempty"`
	// +optional
	Priority string `json:"priority,omitempty"`
	// demandShape describes whether the workload is prefill-heavy, decode-heavy, kv-heavy, or mixed.
	// +optional
	// +kubebuilder:validation:Enum=mixed;kv_heavy;prefill_heavy;decode_heavy
	DemandShape DemandShape `json:"demandShape,omitempty"`
	// timeWindows describes the business windows copied from the workload profile.
	// +optional
	// +listType=map
	// +listMapKey=name
	TimeWindows []TimeWindow `json:"timeWindows,omitempty"`
	// resourceRequest describes the original required/preferred/max budgets copied
	// from the workload profile. It is separate from Resources, which describes
	// the concrete budget currently assigned by the scheduler.
	// +optional
	ResourceRequest *WorkloadResourceRequest `json:"resourceRequest,omitempty"`
	// +optional
	AssignedGPU *AssignedGPU `json:"assignedGPU,omitempty"`
	// +optional
	Resources *ResourceSummary `json:"resources,omitempty"`
	// current describes the resources the node agent actually applied most recently.
	// +optional
	Current *ResourceSummary `json:"current,omitempty"`
	// hourlyForecast describes the expected 24-hour resource envelope for this allocation.
	// +optional
	HourlyForecast *HourlyResourceForecast `json:"hourlyForecast,omitempty"`
	// +optional
	Actions *AllocationActions `json:"actions,omitempty"`
	// +optional
	Phase string `json:"phase,omitempty"`
}

// AssignedGPU identifies the physical GPU or MIG slice assigned to an allocation.
type AssignedGPU struct {
	// +optional
	GPUID string `json:"gpuID,omitempty"`
	// +optional
	MIGSliceID string `json:"migSliceID,omitempty"`
}

// AllocationActions describes what the scheduler/agent may do to this allocation.
type AllocationActions struct {
	// +optional
	Reclaimable bool `json:"reclaimable,omitempty"`
	// +optional
	Throttleable bool `json:"throttleable,omitempty"`
	// +optional
	Pauseable bool `json:"pauseable,omitempty"`
	// +optional
	Evictable bool `json:"evictable,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="Backend",type=string,JSONPath=`.spec.backendType`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="GPU",type=integer,JSONPath=`.status.available.gpuCount`
// +kubebuilder:printcolumn:name="KVCache",type=integer,JSONPath=`.status.available.kvCacheGiB`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AINodeResourceProfile is the Schema for the ainoderesourceprofiles API
type AINodeResourceProfile struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AINodeResourceProfile
	// +required
	Spec AINodeResourceProfileSpec `json:"spec"`

	// status defines the observed state of AINodeResourceProfile
	// +optional
	Status AINodeResourceProfileStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AINodeResourceProfileList contains a list of AINodeResourceProfile
type AINodeResourceProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AINodeResourceProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AINodeResourceProfile{}, &AINodeResourceProfileList{})
}
