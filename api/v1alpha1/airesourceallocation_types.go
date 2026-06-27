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

// AIResourceAllocationSpec defines the node-level resource allocation ledger.
//
// In the node-level design, one AIResourceAllocation represents one node's
// desired allocation plan. Workload-level entries are stored in Workloads.
// The legacy single-workload fields are kept temporarily so the current
// scheduler/agent implementation can be migrated incrementally.
type AIResourceAllocationSpec struct {
	// nodeName is the target Kubernetes node represented by this allocation ledger.
	// +required
	NodeName string `json:"nodeName"`

	// workloads describes desired workload allocations on this node.
	// This is the primary field for the node-level allocation design.
	// +optional
	// +listType=map
	// +listMapKey=name
	Workloads []AllocationWorkloadSpec `json:"workloads,omitempty"`

	// reason is a human-readable explanation from the scheduler or agent.
	// +optional
	Reason string `json:"reason,omitempty"`

	// workloadRef points to the AIWorkloadProfile that owns this allocation.
	// Deprecated: use workloads[].workloadRef for the node-level allocation ledger.
	// +optional
	WorkloadRef KubernetesObjectRef `json:"workloadRef,omitempty"`

	// workloadType describes whether the workload is online, offline, or hybrid.
	// It is copied from AIWorkloadProfile so node agents can reason about local mix.
	// Deprecated: use workloads[].workloadType.
	// +optional
	WorkloadType string `json:"workloadType,omitempty"`

	// priority is copied from AIWorkloadProfile for node-local resource ordering.
	// Deprecated: use workloads[].priority.
	// +optional
	Priority string `json:"priority,omitempty"`

	// demandShape is copied from AIWorkloadProfile for workload-mix-aware scheduling.
	// Deprecated: use workloads[].demandShape.
	// +optional
	// +kubebuilder:validation:Enum=mixed;kv_heavy;prefill_heavy;decode_heavy
	DemandShape DemandShape `json:"demandShape,omitempty"`

	// timeWindows is copied from AIWorkloadProfile for time-aware node scoring.
	// Deprecated: use workloads[].timeWindows.
	// +optional
	// +listType=map
	// +listMapKey=name
	TimeWindows []TimeWindow `json:"timeWindows,omitempty"`

	// resourceRequest is copied from AIWorkloadProfile so the scheduler and
	// agent can reason about required/preferred/max budgets after placement.
	// Deprecated: use workloads[].resourceRequest.
	// +optional
	ResourceRequest *WorkloadResourceRequest `json:"resourceRequest,omitempty"`

	// assignedGPU identifies the physical GPU or MIG slice selected by the scheduler.
	// Deprecated: use workloads[].assignedGPU.
	// +optional
	AssignedGPU *AssignedGPU `json:"assignedGPU,omitempty"`

	// resources describes the concrete resource budget assigned by the scheduler.
	// Deprecated: use workloads[].resources.
	// +optional
	Resources *ResourceSummary `json:"resources,omitempty"`

	// allocationType describes the resource ownership model.
	// Examples: guaranteed, borrowed, reclaimed.
	// Deprecated: use workloads[].allocationType.
	// +optional
	AllocationType string `json:"allocationType,omitempty"`

	// actions describes what the agent may do to this allocation later.
	// Deprecated: use workloads[].actions.
	// +optional
	Actions *AllocationActions `json:"actions,omitempty"`
}

// AllocationWorkloadSpec describes one workload entry inside a node allocation ledger.
type AllocationWorkloadSpec struct {
	// name is a stable key for this workload entry inside the node ledger.
	// It is usually derived from namespace/name of the workload profile.
	// +required
	Name string `json:"name"`

	// workloadRef points to the AIWorkloadProfile represented by this entry.
	// +required
	WorkloadRef KubernetesObjectRef `json:"workloadRef"`

	// sourcePodName is the Kubernetes Pod that caused this workload entry.
	// It lets the node agent migrate or release the concrete Pod without
	// relying on node-level object annotations.
	// +optional
	SourcePodName string `json:"sourcePodName,omitempty"`

	// sourcePodUID is the UID of sourcePodName.
	// +optional
	SourcePodUID string `json:"sourcePodUID,omitempty"`

	// workloadType describes whether the workload is online, offline, or hybrid.
	// +optional
	WorkloadType string `json:"workloadType,omitempty"`

	// priority is copied from AIWorkloadProfile for node-local resource ordering.
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

	// resourceRequest describes required/preferred/max budgets copied from the workload profile.
	// +optional
	ResourceRequest *WorkloadResourceRequest `json:"resourceRequest,omitempty"`

	// assignedGPU identifies the physical GPU or MIG slice selected for this workload.
	// +optional
	AssignedGPU *AssignedGPU `json:"assignedGPU,omitempty"`

	// resources describes the concrete resource budget desired for this workload.
	// +optional
	Resources *ResourceSummary `json:"resources,omitempty"`

	// desired describes the node-local target budget for this workload.
	// It may be required, preferred, max, or a throttled budget depending on
	// priority, time window, and local resource pressure.
	// +optional
	Desired *ResourceSummary `json:"desired,omitempty"`

	// allocationType describes the ownership model, such as guaranteed, borrowed, or reclaimed.
	// +optional
	AllocationType string `json:"allocationType,omitempty"`

	// actions describes what the node agent may do to this workload.
	// +optional
	Actions *AllocationActions `json:"actions,omitempty"`

	// reason is a human-readable explanation from the scheduler or node agent.
	// +optional
	Reason string `json:"reason,omitempty"`
}

// AIResourceAllocationStatus defines the observed state of AIResourceAllocation.
type AIResourceAllocationStatus struct {
	// phase is the high-level execution phase reported by the node agent.
	// Examples: Pending, Applying, Applied, Failed, Releasing, Released.
	// +optional
	Phase string `json:"phase,omitempty"`

	// observedAt is the last time this allocation was observed by the agent.
	// +optional
	ObservedAt *metav1.Time `json:"observedAt,omitempty"`

	// appliedAt is the time when the agent successfully applied this allocation.
	// +optional
	AppliedAt *metav1.Time `json:"appliedAt,omitempty"`

	// nodeLedger describes the node-level resource ledger observed by the agent.
	// It is the primary status field for the node-level allocation design.
	// +optional
	NodeLedger *NodeAllocationLedger `json:"nodeLedger,omitempty"`

	// workloads describes observed per-workload allocation states on this node.
	// +optional
	// +listType=map
	// +listMapKey=name
	Workloads []AllocationWorkloadStatus `json:"workloads,omitempty"`

	// observedResources describes the runtime resources observed after applying the allocation.
	// Deprecated: use nodeLedger.allocated or workloads[].applied.
	// +optional
	ObservedResources *ResourceSummary `json:"observedResources,omitempty"`

	// current describes the resources that the node agent actually applied most recently.
	// It is the runtime truth used for node-local accounting.
	// Deprecated: use nodeLedger.allocated or workloads[].applied.
	// +optional
	Current *ResourceSummary `json:"current,omitempty"`

	// hourlyForecast describes the expected resource envelope for each hour of a day.
	// resourceBuckets[0] is 00:00-01:00, resourceBuckets[23] is 23:00-24:00.
	// It is generated by the node agent from the allocation profile snapshot.
	// +optional
	HourlyForecast *HourlyResourceForecast `json:"hourlyForecast,omitempty"`

	// message is a short human-readable status message.
	// +optional
	Message string `json:"message,omitempty"`

	// conditions represent the current state of the AIResourceAllocation resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// NodeAllocationLedger summarizes one node's resource allocation state.
type NodeAllocationLedger struct {
	// total is the node capacity used by the allocation ledger.
	// +optional
	Total *ResourceSummary `json:"total,omitempty"`

	// allocated is the sum of resources currently applied to workloads.
	// +optional
	Allocated *ResourceSummary `json:"allocated,omitempty"`

	// available is total minus allocated from the ledger view.
	// +optional
	Available *ResourceSummary `json:"available,omitempty"`
}

// AllocationWorkloadStatus describes one workload's observed state inside a node allocation ledger.
type AllocationWorkloadStatus struct {
	// name is the stable key matching spec.workloads[].name.
	// +required
	Name string `json:"name"`

	// workloadRef points to the workload profile represented by this status entry.
	// +optional
	WorkloadRef *KubernetesObjectRef `json:"workloadRef,omitempty"`

	// sourcePodName is the Kubernetes Pod represented by this status entry.
	// +optional
	SourcePodName string `json:"sourcePodName,omitempty"`

	// phase is this workload entry's node-local phase.
	// Examples: PendingApply, Applied, Throttled, Paused, Migrating, Released, Failed.
	// +optional
	Phase string `json:"phase,omitempty"`

	// priority is copied from the workload profile for easier inspection.
	// +optional
	Priority string `json:"priority,omitempty"`

	// demandShape is copied from the workload profile for easier inspection.
	// +optional
	// +kubebuilder:validation:Enum=mixed;kv_heavy;prefill_heavy;decode_heavy
	DemandShape DemandShape `json:"demandShape,omitempty"`

	// activeWindow indicates whether this workload is currently inside a peak time window.
	// +optional
	ActiveWindow bool `json:"activeWindow,omitempty"`

	// desired is the resource target chosen by the local scheduler for this workload.
	// +optional
	Desired *ResourceSummary `json:"desired,omitempty"`

	// applied is the resource budget actually applied by the backend.
	// +optional
	Applied *ResourceSummary `json:"applied,omitempty"`

	// hourlyForecast describes this workload's expected 24-hour resource envelope.
	// +optional
	HourlyForecast *HourlyResourceForecast `json:"hourlyForecast,omitempty"`

	// assignedGPU identifies the physical GPU or MIG slice used by this workload.
	// +optional
	AssignedGPU *AssignedGPU `json:"assignedGPU,omitempty"`

	// action is the latest local action, such as keep, throttle, pause, evict, or migrate.
	// +optional
	Action string `json:"action,omitempty"`

	// message explains the current workload-level allocation state.
	// +optional
	Message string `json:"message,omitempty"`
}

// HourlyResourceForecast is the agent-generated daily resource envelope.
type HourlyResourceForecast struct {
	// generatedAt is the time when the forecast was generated.
	// +optional
	GeneratedAt *metav1.Time `json:"generatedAt,omitempty"`

	// timezone describes the business timezone used to interpret the 0-23 buckets.
	// +optional
	Timezone string `json:"timezone,omitempty"`

	// resourceBuckets contains 24 resource summaries ordered by hour of day.
	// +optional
	// +kubebuilder:validation:MaxItems=24
	ResourceBuckets []ResourceSummary `json:"resourceBuckets,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="GPU",type=integer,JSONPath=`.status.nodeLedger.available.gpuCount`
// +kubebuilder:printcolumn:name="KVCache",type=integer,JSONPath=`.status.nodeLedger.available.kvCacheGiB`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AIResourceAllocation is the Schema for the airesourceallocations API.
type AIResourceAllocation struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AIResourceAllocation.
	// +required
	Spec AIResourceAllocationSpec `json:"spec"`

	// status defines the observed state of AIResourceAllocation.
	// +optional
	Status AIResourceAllocationStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AIResourceAllocationList contains a list of AIResourceAllocation.
type AIResourceAllocationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AIResourceAllocation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIResourceAllocation{}, &AIResourceAllocationList{})
}
