package policy

import (
	"fmt"
	"strings"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
)

func (TimeWindowEnvelopeConstraint) check(
	profile *astrav1alpha1.AIWorkloadProfile,
	node *astrav1alpha1.AINodeResourceProfile,
) FilterDecision {
	if profile == nil || node == nil {
		return FilterDecision{Allowed: false, Reason: "missing profile or node for time-window check"}
	}
	if !isOnlineWorkload(profile.Spec.WorkloadType) || len(profile.Spec.TimeWindows) == 0 {
		return FilterDecision{Allowed: true, Reason: "time-window hard check not required"}
	}

	capacity, ok := nodeCapacityForEnvelope(node)
	if !ok {
		return FilterDecision{Allowed: true, Reason: "node has no allocatable capacity for time-window envelope check"}
	}

	currentRequired := requiredResources(profile)
	currentMax := maxResources(profile)
	if resourceSummaryIsZero(currentMax) {
		return FilterDecision{Allowed: true, Reason: "workload has no max resource envelope"}
	}

	for _, allocation := range node.Status.Allocations {
		if !comparableTimeWindowAllocation(profile, allocation) {
			continue
		}

		existingRequired, existingMax := allocationEnvelopeResources(allocation)
		if resourceSummaryIsZero(existingMax) {
			continue
		}

		if timeWindowsOverlap(profile.Spec.TimeWindows, allocation.TimeWindows) {
			totalPeak := addResourceSummary(currentMax, existingMax)
			if fits, reason := resourceSummaryFits(totalPeak, capacity); !fits {
				return FilterDecision{
					Allowed: false,
					Reason: fmt.Sprintf(
						"time-window envelope rejected: workload=%s existing=%s overlap=true reason=%s",
						profile.Name,
						allocation.Name,
						reason,
					),
				}
			}
			continue
		}

		currentPeak := addResourceSummary(currentMax, existingRequired)
		if fits, reason := resourceSummaryFits(currentPeak, capacity); !fits {
			return FilterDecision{
				Allowed: false,
				Reason: fmt.Sprintf(
					"time-window envelope rejected: workload=%s peak with existing required=%s reason=%s",
					profile.Name,
					allocation.Name,
					reason,
				),
			}
		}

		existingPeak := addResourceSummary(currentRequired, existingMax)
		if fits, reason := resourceSummaryFits(existingPeak, capacity); !fits {
			return FilterDecision{
				Allowed: false,
				Reason: fmt.Sprintf(
					"time-window envelope rejected: existing=%s peak with workload required=%s reason=%s",
					allocation.Name,
					profile.Name,
					reason,
				),
			}
		}
	}

	return FilterDecision{Allowed: true, Reason: "time-window resource envelopes fit"}
}

func comparableTimeWindowAllocation(profile *astrav1alpha1.AIWorkloadProfile, allocation astrav1alpha1.NodeAllocationStatus) bool {
	if profile == nil || !isOnlineWorkload(allocation.WorkloadType) || len(allocation.TimeWindows) == 0 {
		return false
	}
	return strings.EqualFold(profile.Spec.Priority, allocation.Priority)
}

func allocationEnvelopeResources(allocation astrav1alpha1.NodeAllocationStatus) (astrav1alpha1.ResourceSummary, astrav1alpha1.ResourceSummary) {
	if allocation.ResourceRequest != nil {
		return requiredResourcesFromRequest(allocation.ResourceRequest), maxResourcesFromRequest(allocation.ResourceRequest)
	}
	if allocation.Resources != nil {
		return *allocation.Resources, *allocation.Resources
	}
	return astrav1alpha1.ResourceSummary{}, astrav1alpha1.ResourceSummary{}
}

func nodeCapacityForEnvelope(node *astrav1alpha1.AINodeResourceProfile) (astrav1alpha1.ResourceSummary, bool) {
	if node == nil {
		return astrav1alpha1.ResourceSummary{}, false
	}
	if node.Status.Allocatable != nil {
		return *node.Status.Allocatable, true
	}
	if node.Status.Available != nil && node.Status.Allocated != nil {
		return addResourceSummary(*node.Status.Available, *node.Status.Allocated), true
	}
	return astrav1alpha1.ResourceSummary{}, false
}

func resourceSummaryIsZero(summary astrav1alpha1.ResourceSummary) bool {
	return summary.GPUCount == 0 &&
		summary.GPUMemoryGiB == 0 &&
		summary.KVCacheGiB == 0 &&
		summary.PrefillTokensPerSecond == 0 &&
		summary.DecodeTokensPerSecond == 0 &&
		summary.TotalTokensPerSecond == 0
}
