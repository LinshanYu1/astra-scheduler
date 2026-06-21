package local

import (
	"fmt"
	"sort"
	"time"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	"github.com/linshanyu/astra-scheduler/internal/agent/resources"
)

type ledger struct {
	Capacity  astrav1alpha1.ResourceSummary
	Remaining astrav1alpha1.ResourceSummary
	Reserved  map[string]astrav1alpha1.ResourceSummary
}

type budgetCandidate struct {
	Tier      string
	Resources astrav1alpha1.ResourceSummary
}

func newLedger(capacity astrav1alpha1.ResourceSummary) *ledger {
	return &ledger{
		Capacity:  capacity,
		Remaining: capacity,
		Reserved:  map[string]astrav1alpha1.ResourceSummary{},
	}
}

func (l *ledger) reserve(
	key string,
	required astrav1alpha1.ResourceSummary,
	candidates []budgetCandidate,
) (astrav1alpha1.ResourceSummary, string, bool) {
	if !fits(required, l.Remaining) {
		return astrav1alpha1.ResourceSummary{}, AllocationTierRejected, false
	}

	assigned := required
	tier := AllocationTierRequired
	for _, candidate := range candidates {
		if isZeroResource(candidate.Resources) {
			continue
		}
		if fits(candidate.Resources, l.Remaining) {
			assigned = candidate.Resources
			tier = candidate.Tier
			break
		}
	}

	l.Remaining = subtractResource(l.Remaining, assigned)
	l.Reserved[key] = assigned
	return assigned, tier, true
}

func (l *ledger) release(key string) {
	reserved, ok := l.Reserved[key]
	if !ok {
		return
	}
	l.Remaining = addResource(l.Remaining, reserved)
	delete(l.Reserved, key)
}

func capacityFromState(state resources.NodeState) astrav1alpha1.ResourceSummary {
	capacity := astrav1alpha1.ResourceSummary{}
	for _, snapshot := range state.Snapshots {
		capacity = maxResource(capacity, snapshot.Capacity)
	}
	return capacity
}

func requiredBudget(allocation astrav1alpha1.AIResourceAllocation) astrav1alpha1.ResourceSummary {
	if allocation.Spec.ResourceRequest != nil && allocation.Spec.ResourceRequest.Required != nil {
		return *allocation.Spec.ResourceRequest.Required
	}
	if allocation.Spec.Resources != nil {
		return *allocation.Spec.Resources
	}
	return astrav1alpha1.ResourceSummary{}
}

func preferredBudget(allocation astrav1alpha1.AIResourceAllocation) astrav1alpha1.ResourceSummary {
	if allocation.Spec.ResourceRequest != nil && allocation.Spec.ResourceRequest.Preferred != nil {
		return *allocation.Spec.ResourceRequest.Preferred
	}
	return requiredBudget(allocation)
}

func maxBudget(allocation astrav1alpha1.AIResourceAllocation) astrav1alpha1.ResourceSummary {
	if allocation.Spec.ResourceRequest != nil && allocation.Spec.ResourceRequest.Max != nil {
		return *allocation.Spec.ResourceRequest.Max
	}
	return preferredBudget(allocation)
}

func activeTimeWindow(windows []astrav1alpha1.TimeWindow, now time.Time) string {
	for _, window := range windows {
		location := now.Location()
		if window.Timezone != "" {
			if loaded, err := time.LoadLocation(window.Timezone); err == nil {
				location = loaded
			}
		}
		localNow := now.In(location)
		start, ok := parseMinuteOfDay(window.Start)
		if !ok {
			continue
		}
		end, ok := parseMinuteOfDay(window.End)
		if !ok {
			continue
		}
		current := localNow.Hour()*60 + localNow.Minute()
		if minuteInWindow(current, start, end) {
			return window.Name
		}
	}
	return ""
}

func hasAnyActiveTimeWindow(allocations []astrav1alpha1.AIResourceAllocation, now time.Time) bool {
	for _, allocation := range allocations {
		if activeTimeWindow(allocation.Spec.TimeWindows, now) != "" {
			return true
		}
	}
	return false
}

func orderAllocationsByPriorityAndWindow(
	allocations []astrav1alpha1.AIResourceAllocation,
	now time.Time,
) []astrav1alpha1.AIResourceAllocation {
	ordered := append([]astrav1alpha1.AIResourceAllocation(nil), allocations...)
	sort.SliceStable(ordered, func(i, j int) bool {
		leftPriority := priorityRank(priorityOf(ordered[i]))
		rightPriority := priorityRank(priorityOf(ordered[j]))
		if leftPriority != rightPriority {
			return leftPriority > rightPriority
		}

		leftActive := activeTimeWindow(ordered[i].Spec.TimeWindows, now) != ""
		rightActive := activeTimeWindow(ordered[j].Spec.TimeWindows, now) != ""
		if leftActive != rightActive {
			return leftActive
		}

		leftCreated := ordered[i].CreationTimestamp
		rightCreated := ordered[j].CreationTimestamp
		if !leftCreated.IsZero() && !rightCreated.IsZero() && !leftCreated.Equal(&rightCreated) {
			return leftCreated.Before(&rightCreated)
		}
		if !leftCreated.IsZero() && rightCreated.IsZero() {
			return true
		}
		if leftCreated.IsZero() && !rightCreated.IsZero() {
			return false
		}

		return ordered[i].Name < ordered[j].Name
	})
	return ordered
}

func groupAllocationsByPriority(allocations []astrav1alpha1.AIResourceAllocation) [][]astrav1alpha1.AIResourceAllocation {
	if len(allocations) == 0 {
		return nil
	}

	groups := [][]astrav1alpha1.AIResourceAllocation{}
	currentPriority := priorityOf(allocations[0])
	current := []astrav1alpha1.AIResourceAllocation{}
	for _, allocation := range allocations {
		priority := priorityOf(allocation)
		if priority != currentPriority {
			groups = append(groups, current)
			currentPriority = priority
			current = []astrav1alpha1.AIResourceAllocation{}
		}
		current = append(current, allocation)
	}
	groups = append(groups, current)
	return groups
}

func priorityOf(allocation astrav1alpha1.AIResourceAllocation) string {
	if allocation.Spec.Priority != "" {
		return allocation.Spec.Priority
	}
	return allocation.Labels["astra.aiinfra.io/priority"]
}

func canYieldResources(allocation astrav1alpha1.AIResourceAllocation) bool {
	if allocation.Spec.Actions == nil {
		return false
	}
	return allocation.Spec.Actions.Reclaimable ||
		allocation.Spec.Actions.Throttleable ||
		allocation.Spec.Actions.Pauseable ||
		allocation.Spec.Actions.Evictable
}

func parseMinuteOfDay(value string) (int, bool) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, false
	}
	return parsed.Hour()*60 + parsed.Minute(), true
}

func minuteInWindow(current, start, end int) bool {
	if start == end {
		return true
	}
	if start < end {
		return current >= start && current < end
	}
	return current >= start || current < end
}

func allocationKey(allocation astrav1alpha1.AIResourceAllocation) string {
	if allocation.Namespace == "" {
		return allocation.Name
	}
	return allocation.Namespace + "/" + allocation.Name
}

func fits(required, available astrav1alpha1.ResourceSummary) bool {
	return required.GPUCount <= available.GPUCount &&
		required.GPUMemoryGiB <= available.GPUMemoryGiB &&
		required.KVCacheGiB <= available.KVCacheGiB &&
		required.PrefillTokensPerSecond <= available.PrefillTokensPerSecond &&
		required.DecodeTokensPerSecond <= available.DecodeTokensPerSecond &&
		required.TotalTokensPerSecond <= available.TotalTokensPerSecond
}

func isZeroResource(value astrav1alpha1.ResourceSummary) bool {
	return value.GPUCount == 0 &&
		value.GPUMemoryGiB == 0 &&
		value.KVCacheGiB == 0 &&
		value.PrefillTokensPerSecond == 0 &&
		value.DecodeTokensPerSecond == 0 &&
		value.TotalTokensPerSecond == 0
}

func addResource(left, right astrav1alpha1.ResourceSummary) astrav1alpha1.ResourceSummary {
	return astrav1alpha1.ResourceSummary{
		GPUCount:               left.GPUCount + right.GPUCount,
		GPUMemoryGiB:           left.GPUMemoryGiB + right.GPUMemoryGiB,
		KVCacheGiB:             left.KVCacheGiB + right.KVCacheGiB,
		PrefillTokensPerSecond: left.PrefillTokensPerSecond + right.PrefillTokensPerSecond,
		DecodeTokensPerSecond:  left.DecodeTokensPerSecond + right.DecodeTokensPerSecond,
		TotalTokensPerSecond:   left.TotalTokensPerSecond + right.TotalTokensPerSecond,
	}
}

func subtractResource(left, right astrav1alpha1.ResourceSummary) astrav1alpha1.ResourceSummary {
	return astrav1alpha1.ResourceSummary{
		GPUCount:               nonNegative(left.GPUCount - right.GPUCount),
		GPUMemoryGiB:           nonNegative(left.GPUMemoryGiB - right.GPUMemoryGiB),
		KVCacheGiB:             nonNegative(left.KVCacheGiB - right.KVCacheGiB),
		PrefillTokensPerSecond: nonNegative(left.PrefillTokensPerSecond - right.PrefillTokensPerSecond),
		DecodeTokensPerSecond:  nonNegative(left.DecodeTokensPerSecond - right.DecodeTokensPerSecond),
		TotalTokensPerSecond:   nonNegative(left.TotalTokensPerSecond - right.TotalTokensPerSecond),
	}
}

func maxResource(left, right astrav1alpha1.ResourceSummary) astrav1alpha1.ResourceSummary {
	return astrav1alpha1.ResourceSummary{
		GPUCount:               maxInt32(left.GPUCount, right.GPUCount),
		GPUMemoryGiB:           maxInt32(left.GPUMemoryGiB, right.GPUMemoryGiB),
		KVCacheGiB:             maxInt32(left.KVCacheGiB, right.KVCacheGiB),
		PrefillTokensPerSecond: maxInt32(left.PrefillTokensPerSecond, right.PrefillTokensPerSecond),
		DecodeTokensPerSecond:  maxInt32(left.DecodeTokensPerSecond, right.DecodeTokensPerSecond),
		TotalTokensPerSecond:   maxInt32(left.TotalTokensPerSecond, right.TotalTokensPerSecond),
	}
}

func maxInt32(left, right int32) int32 {
	if left > right {
		return left
	}
	return right
}

func nonNegative(value int32) int32 {
	if value < 0 {
		return 0
	}
	return value
}

func describeResource(value astrav1alpha1.ResourceSummary) string {
	return fmt.Sprintf(
		"gpu=%d gpuMemoryGiB=%d kvCacheGiB=%d prefillTPS=%d decodeTPS=%d totalTPS=%d",
		value.GPUCount,
		value.GPUMemoryGiB,
		value.KVCacheGiB,
		value.PrefillTokensPerSecond,
		value.DecodeTokensPerSecond,
		value.TotalTokensPerSecond,
	)
}
