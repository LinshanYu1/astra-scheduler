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

// AIResourceAllocationSpec defines one scheduler decision for one AI workload.
type AIResourceAllocationSpec struct {
	// workloadRef points to the AIWorkloadProfile that owns this allocation.
	// The referenced profile is usually in the same namespace as this allocation.
	// +required
	WorkloadRef KubernetesObjectRef `json:"workloadRef"`

	// workloadType describes whether the workload is online, offline, or hybrid.
	// It is copied from AIWorkloadProfile so node agents can reason about local mix.
	// +optional
	WorkloadType string `json:"workloadType,omitempty"`

	// priority is copied from AIWorkloadProfile for node-local resource ordering.
	// +optional
	Priority string `json:"priority,omitempty"`

	// demandShape is copied from AIWorkloadProfile for workload-mix-aware scheduling.
	// +optional
	// +kubebuilder:validation:Enum=mixed;kv_heavy;prefill_heavy;decode_heavy
	DemandShape DemandShape `json:"demandShape,omitempty"`

	// timeWindows is copied from AIWorkloadProfile for time-aware node scoring.
	// +optional
	// +listType=map
	// +listMapKey=name
	TimeWindows []TimeWindow `json:"timeWindows,omitempty"`

	// resourceRequest is copied from AIWorkloadProfile so the scheduler and
	// agent can reason about required/preferred/max budgets after placement.
	// +optional
	ResourceRequest *WorkloadResourceRequest `json:"resourceRequest,omitempty"`

	// nodeName is the target Kubernetes node chosen by the scheduler.
	// +required
	NodeName string `json:"nodeName"`

	// assignedGPU identifies the physical GPU or MIG slice selected by the scheduler.
	// +optional
	AssignedGPU *AssignedGPU `json:"assignedGPU,omitempty"`

	// resources describes the concrete resource budget assigned by the scheduler.
	// +optional
	Resources *ResourceSummary `json:"resources,omitempty"`

	// allocationType describes the resource ownership model.
	// Examples: guaranteed, borrowed, reclaimed.
	// +optional
	AllocationType string `json:"allocationType,omitempty"`

	// actions describes what the agent may do to this allocation later.
	// +optional
	Actions *AllocationActions `json:"actions,omitempty"`

	// reason is a human-readable explanation from the scheduler.
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

	// observedResources describes the runtime resources observed after applying the allocation.
	// +optional
	ObservedResources *ResourceSummary `json:"observedResources,omitempty"`

	// message is a short human-readable status message.
	// +optional
	Message string `json:"message,omitempty"`

	// conditions represent the current state of the AIResourceAllocation resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Workload",type=string,JSONPath=`.spec.workloadRef.name`
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.spec.workloadRef.namespace`
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.allocationType`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
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
