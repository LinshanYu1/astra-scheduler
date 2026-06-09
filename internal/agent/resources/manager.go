package resources

import (
	"context"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
)

type NodeState struct {
	Node        *astrav1alpha1.AINodeResourceProfile
	Allocations []astrav1alpha1.AIResourceAllocation
	Snapshots   map[string]Snapshot
}

type Snapshot struct {
	Name        string
	Capacity    astrav1alpha1.ResourceSummary
	Allocated   astrav1alpha1.ResourceSummary
	Available   astrav1alpha1.ResourceSummary
	Description string
}

type ValidationRequest struct {
	Node       *astrav1alpha1.AINodeResourceProfile
	Allocation *astrav1alpha1.AIResourceAllocation
	Snapshot   Snapshot
}

type ValidationResult struct {
	Allowed bool
	Reason  string
}

type Manager interface {
	Name() string
	Observe(context.Context, *astrav1alpha1.AINodeResourceProfile, []astrav1alpha1.AIResourceAllocation) (Snapshot, error)
	Validate(context.Context, ValidationRequest) ValidationResult
}

func allocationSum(allocations []astrav1alpha1.AIResourceAllocation) astrav1alpha1.ResourceSummary {
	summary := astrav1alpha1.ResourceSummary{}
	for _, allocation := range allocations {
		if allocation.Spec.Resources == nil {
			continue
		}
		summary.GPUCount += allocation.Spec.Resources.GPUCount
		summary.GPUMemoryGiB += allocation.Spec.Resources.GPUMemoryGiB
		summary.KVCacheGiB += allocation.Spec.Resources.KVCacheGiB
		summary.PrefillTokensPerSecond += allocation.Spec.Resources.PrefillTokensPerSecond
		summary.DecodeTokensPerSecond += allocation.Spec.Resources.DecodeTokensPerSecond
		summary.TotalTokensPerSecond += allocation.Spec.Resources.TotalTokensPerSecond
	}
	return summary
}

func nonNegative(value int32) int32 {
	if value < 0 {
		return 0
	}
	return value
}
