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

// AIWorkloadProfileSpec defines the scheduling intent of one AI workload.
type AIWorkloadProfileSpec struct {
	// workloadRef points to the Kubernetes workload represented by this profile.
	// +required
	WorkloadRef KubernetesObjectRef `json:"workloadRef"`

	// workloadType describes whether the workload is online, offline, or hybrid.
	// Examples: online, offline, hybrid.
	// +required
	WorkloadType string `json:"workloadType"`

	// priority is the business priority used by the policy engine.
	// Examples: high, medium, low.
	// +required
	Priority string `json:"priority"`

	// demandShape describes the dominant AI serving pressure.
	// Examples: decode_heavy, prefill_heavy, kv_heavy, mixed.
	// +optional
	// +kubebuilder:validation:Enum=mixed;kv_heavy;prefill_heavy;decode_heavy
	DemandShape DemandShape `json:"demandShape,omitempty"`

	// timeWindows describe business peak windows that should affect scheduling.
	// +optional
	// +listType=map
	// +listMapKey=name
	TimeWindows []TimeWindow `json:"timeWindows,omitempty"`

	// slo describes service-level objectives or offline deadlines.
	// +optional
	SLO *WorkloadSLO `json:"slo,omitempty"`

	// resources describes hard required resources and soft preferred resources.
	// +optional
	Resources *WorkloadResourceRequest `json:"resources,omitempty"`

	// policy describes which adjustment actions are allowed for this workload.
	// +optional
	Policy *WorkloadPolicy `json:"policy,omitempty"`
}

// KubernetesObjectRef identifies a Kubernetes object used by Astra CRDs.
type KubernetesObjectRef struct {
	// +required
	APIVersion string `json:"apiVersion"`
	// +required
	Kind string `json:"kind"`
	// +required
	Name string `json:"name"`
	// namespace is optional because some referenced objects can be cluster-scoped.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// TimeWindow describes a repeating daily business window.
type TimeWindow struct {
	// +required
	Name string `json:"name"`
	// start is a local wall-clock time in HH:MM format.
	// +required
	Start string `json:"start"`
	// end is a local wall-clock time in HH:MM format. It may be earlier than start for overnight windows.
	// +required
	End string `json:"end"`
	// timezone is an IANA timezone name, such as America/New_York.
	// +optional
	Timezone string `json:"timezone,omitempty"`
}

// WorkloadSLO describes latency or deadline objectives.
type WorkloadSLO struct {
	// ttftP95Ms is the p95 time-to-first-token target for online LLM serving.
	// +optional
	TTFTP95Ms int32 `json:"ttftP95Ms,omitempty"`

	// latencyP99Ms is the p99 end-to-end latency target.
	// +optional
	LatencyP99Ms int32 `json:"latencyP99Ms,omitempty"`

	// deadlineHours is mainly used by delay-tolerant offline jobs.
	// +optional
	DeadlineHours int32 `json:"deadlineHours,omitempty"`
}

// WorkloadResourceRequest describes required, preferred, and peak AI resource budgets.
type WorkloadResourceRequest struct {
	// required is the minimum resource budget needed to place this workload.
	// +optional
	Required *ResourceSummary `json:"required,omitempty"`

	// preferred is the target resource budget used for scoring and elastic growth.
	// +optional
	Preferred *ResourceSummary `json:"preferred,omitempty"`

	// max is the peak resource budget used for time-window feasibility checks.
	// It describes how much resource this workload may need during its active
	// business window. If omitted, preferred is used as the peak budget.
	// +optional
	Max *ResourceSummary `json:"max,omitempty"`
}

// WorkloadPolicy describes whether this workload can borrow or yield resources.
type WorkloadPolicy struct {
	// allowBorrowing means this workload may use idle capacity above its required budget.
	// +optional
	AllowBorrowing bool `json:"allowBorrowing,omitempty"`

	// allowReclaimFromLowerPriority means this workload can reclaim capacity from lower-priority workloads.
	// +optional
	AllowReclaimFromLowerPriority bool `json:"allowReclaimFromLowerPriority,omitempty"`

	// throttleable means the scheduler may ask the agent/runtime to reduce concurrency or token budget.
	// +optional
	Throttleable bool `json:"throttleable,omitempty"`

	// pauseable means the workload can be paused and resumed by the agent/runtime.
	// +optional
	Pauseable bool `json:"pauseable,omitempty"`

	// evictable means this workload can be terminated or moved as a last-resort action.
	// +optional
	Evictable bool `json:"evictable,omitempty"`
}

// AIWorkloadProfileStatus defines the observed state of AIWorkloadProfile.
type AIWorkloadProfileStatus struct {
	// phase is the high-level lifecycle phase observed by the scheduler.
	// +optional
	Phase string `json:"phase,omitempty"`

	// currentWindow is the active time window name, if any.
	// +optional
	CurrentWindow string `json:"currentWindow,omitempty"`

	// sloRisk is a coarse risk level inferred from metrics, such as low, medium, or high.
	// +optional
	SLORisk string `json:"sloRisk,omitempty"`

	// conditions represent the current state of the AIWorkloadProfile resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.workloadType`
// +kubebuilder:printcolumn:name="Priority",type=string,JSONPath=`.spec.priority`
// +kubebuilder:printcolumn:name="Shape",type=string,JSONPath=`.spec.demandShape`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AIWorkloadProfile is the Schema for the aiworkloadprofiles API.
type AIWorkloadProfile struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AIWorkloadProfile.
	// +required
	Spec AIWorkloadProfileSpec `json:"spec"`

	// status defines the observed state of AIWorkloadProfile.
	// +optional
	Status AIWorkloadProfileStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AIWorkloadProfileList contains a list of AIWorkloadProfile.
type AIWorkloadProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AIWorkloadProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIWorkloadProfile{}, &AIWorkloadProfileList{})
}
