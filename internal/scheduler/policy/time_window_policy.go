package policy

import (
	"strings"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
)

func comparableTimeWindowAllocation(profile *astrav1alpha1.AIWorkloadProfile, allocation astrav1alpha1.NodeAllocationStatus) bool {
	if profile == nil || !isOnlineWorkload(allocation.WorkloadType) || len(allocation.TimeWindows) == 0 {
		return false
	}
	return strings.EqualFold(profile.Spec.Priority, allocation.Priority)
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

func nodeForecastResourceForHour(
	node *astrav1alpha1.AINodeResourceProfile,
	hour int,
) astrav1alpha1.ResourceSummary {
	if hour < 0 || hour > 23 {
		return astrav1alpha1.ResourceSummary{}
	}
	if node == nil || node.Status.HourlyForecast == nil || len(node.Status.HourlyForecast.ResourceBuckets) <= hour {
		return astrav1alpha1.ResourceSummary{}
	}
	return node.Status.HourlyForecast.ResourceBuckets[hour]
}

func reclaimableResourceForHour(
	profile *astrav1alpha1.AIWorkloadProfile,
	node *astrav1alpha1.AINodeResourceProfile,
	hour int,
) astrav1alpha1.ResourceSummary {
	if profile == nil || node == nil || hour < 0 || hour > 23 {
		return astrav1alpha1.ResourceSummary{}
	}

	total := astrav1alpha1.ResourceSummary{}
	for _, allocation := range node.Status.Allocations {
		if !canCurrentWorkloadReclaimFrom(profile, allocation) {
			continue
		}
		total = addResourceSummary(total, allocationResourceForHour(allocation, hour))
	}

	return total
}

func allocationResourceForHour(
	allocation astrav1alpha1.NodeAllocationStatus,
	hour int,
) astrav1alpha1.ResourceSummary {
	if hour < 0 || hour > 23 {
		return astrav1alpha1.ResourceSummary{}
	}
	if allocation.HourlyForecast != nil && len(allocation.HourlyForecast.ResourceBuckets) > hour {
		return allocation.HourlyForecast.ResourceBuckets[hour]
	}
	if allocation.ResourceRequest != nil {
		return resourcesForHour(
			allocation.TimeWindows,
			requiredResourcesFromRequest(allocation.ResourceRequest),
			maxResourcesFromRequest(allocation.ResourceRequest),
			hour,
		)
	}
	if allocation.Current != nil {
		return *allocation.Current
	}
	if allocation.Resources != nil {
		return *allocation.Resources
	}
	return astrav1alpha1.ResourceSummary{}
}

func resourcesForHour(
	windows []astrav1alpha1.TimeWindow,
	required astrav1alpha1.ResourceSummary,
	max astrav1alpha1.ResourceSummary,
	hour int,
) astrav1alpha1.ResourceSummary {
	if hourInTimeWindows(windows, hour) && !resourceSummaryIsZero(max) {
		return max
	}
	return required
}

func hourInTimeWindows(windows []astrav1alpha1.TimeWindow, hour int) bool {
	if hour < 0 || hour > 23 {
		return false
	}
	minute := hour * 60

	for _, window := range windows {
		intervals, ok := timeWindowIntervals(window)
		if !ok {
			continue
		}
		for _, interval := range intervals {
			if minute >= interval.start && minute < interval.end {
				return true
			}
		}
	}

	return false
}
