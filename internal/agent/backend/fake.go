package backend

import (
	"context"
	"fmt"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
)

type FakeBackend struct{}

func NewFakeBackend() *FakeBackend {
	return &FakeBackend{}
}

func (b *FakeBackend) Apply(_ context.Context, req ApplyRequest) (ApplyResult, error) {
	if req.Allocation == nil {
		return ApplyResult{}, fmt.Errorf("missing allocation")
	}
	return ApplyResult{
		ObservedResources: req.Allocation.Spec.Resources,
		Message:           fmt.Sprintf("fake backend accepted action %q", req.Action),
	}, nil
}

func (b *FakeBackend) Snapshot(_ context.Context, req SnapshotRequest) (SnapshotResult, error) {
	if req.Node == nil {
		return SnapshotResult{}, fmt.Errorf("missing node profile")
	}

	status := req.Node.Status
	status.Allocatable = allocatableFromNode(req.Node)
	status.Allocated = allocatedFromAllocations(req.Allocations)
	status.Available = subtractResources(status.Allocatable, status.Allocated)
	status.Allocations = nodeAllocationStatuses(req.Allocations)

	if status.Runtime == nil {
		status.Runtime = &astrav1alpha1.RuntimeStatus{}
	}
	if status.Runtime.Pressure == nil {
		status.Runtime.Pressure = fakePressure(status.Allocatable, status.Allocated)
	}

	return SnapshotResult{Status: status}, nil
}

func allocatableFromNode(node *astrav1alpha1.AINodeResourceProfile) *astrav1alpha1.ResourceSummary {
	summary := &astrav1alpha1.ResourceSummary{}
	summary.GPUCount = int32(len(node.Spec.GPUs))
	for _, gpu := range node.Spec.GPUs {
		summary.GPUMemoryGiB += gpu.MemoryGiB
	}

	if node.Spec.Runtime != nil && node.Spec.Runtime.Capacity != nil {
		capacity := node.Spec.Runtime.Capacity
		summary.KVCacheGiB = capacity.KVCacheGiB
		if capacity.TokenThroughput != nil {
			summary.PrefillTokensPerSecond = capacity.TokenThroughput.PrefillTokensPerSecond
			summary.DecodeTokensPerSecond = capacity.TokenThroughput.DecodeTokensPerSecond
			summary.TotalTokensPerSecond = capacity.TokenThroughput.TotalTokensPerSecond
		}
	}

	return summary
}

func allocatedFromAllocations(allocations []astrav1alpha1.AIResourceAllocation) *astrav1alpha1.ResourceSummary {
	summary := &astrav1alpha1.ResourceSummary{}
	for _, allocation := range allocations {
		if allocation.Spec.Resources == nil {
			continue
		}
		addResources(summary, allocation.Spec.Resources)
	}
	return summary
}

func nodeAllocationStatuses(allocations []astrav1alpha1.AIResourceAllocation) []astrav1alpha1.NodeAllocationStatus {
	statuses := make([]astrav1alpha1.NodeAllocationStatus, 0, len(allocations))
	for _, allocation := range allocations {
		workloadName := allocation.Spec.WorkloadRef.Name
		statuses = append(statuses, astrav1alpha1.NodeAllocationStatus{
			Name:            allocation.Name,
			Namespace:       allocation.Namespace,
			WorkloadName:    workloadName,
			WorkloadType:    allocation.Spec.WorkloadType,
			Priority:        allocation.Spec.Priority,
			DemandShape:     allocation.Spec.DemandShape,
			TimeWindows:     allocation.Spec.TimeWindows,
			ResourceRequest: allocation.Spec.ResourceRequest,
			AssignedGPU:     allocation.Spec.AssignedGPU,
			Resources:       allocation.Spec.Resources,
			Actions:         allocation.Spec.Actions,
			Phase:           allocation.Status.Phase,
		})
	}
	return statuses
}

func addResources(target *astrav1alpha1.ResourceSummary, value *astrav1alpha1.ResourceSummary) {
	target.GPUCount += value.GPUCount
	target.GPUMemoryGiB += value.GPUMemoryGiB
	target.KVCacheGiB += value.KVCacheGiB
	target.PrefillTokensPerSecond += value.PrefillTokensPerSecond
	target.DecodeTokensPerSecond += value.DecodeTokensPerSecond
	target.TotalTokensPerSecond += value.TotalTokensPerSecond
}

func subtractResources(total, used *astrav1alpha1.ResourceSummary) *astrav1alpha1.ResourceSummary {
	if total == nil {
		return &astrav1alpha1.ResourceSummary{}
	}
	if used == nil {
		copy := *total
		return &copy
	}
	return &astrav1alpha1.ResourceSummary{
		GPUCount:               nonNegative(total.GPUCount - used.GPUCount),
		GPUMemoryGiB:           nonNegative(total.GPUMemoryGiB - used.GPUMemoryGiB),
		KVCacheGiB:             nonNegative(total.KVCacheGiB - used.KVCacheGiB),
		PrefillTokensPerSecond: nonNegative(total.PrefillTokensPerSecond - used.PrefillTokensPerSecond),
		DecodeTokensPerSecond:  nonNegative(total.DecodeTokensPerSecond - used.DecodeTokensPerSecond),
		TotalTokensPerSecond:   nonNegative(total.TotalTokensPerSecond - used.TotalTokensPerSecond),
	}
}

func fakePressure(total, used *astrav1alpha1.ResourceSummary) *astrav1alpha1.RuntimePressure {
	return &astrav1alpha1.RuntimePressure{
		GPUUtilizationPercent:       percent(used.GPUCount, total.GPUCount),
		GPUMemoryUtilizationPercent: percent(used.GPUMemoryGiB, total.GPUMemoryGiB),
		KVCacheUtilizationPercent:   percent(used.KVCacheGiB, total.KVCacheGiB),
		PrefillUtilizationPercent:   percent(used.PrefillTokensPerSecond, total.PrefillTokensPerSecond),
		DecodeUtilizationPercent:    percent(used.DecodeTokensPerSecond, total.DecodeTokensPerSecond),
		SLORisk:                     "low",
	}
}

func percent(value, total int32) int32 {
	if total <= 0 {
		return 0
	}
	return nonNegative(value * 100 / total)
}

func nonNegative(value int32) int32 {
	if value < 0 {
		return 0
	}
	return value
}
