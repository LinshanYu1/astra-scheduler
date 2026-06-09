package resources

import (
	"context"
	"fmt"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
)

const GPUManagerName = "gpu"

type GPUManager struct{}

func NewGPUManager() *GPUManager {
	return &GPUManager{}
}

func (m *GPUManager) Name() string {
	return GPUManagerName
}

func (m *GPUManager) Observe(_ context.Context, node *astrav1alpha1.AINodeResourceProfile, allocations []astrav1alpha1.AIResourceAllocation) (Snapshot, error) {
	capacity := astrav1alpha1.ResourceSummary{}
	if node != nil && node.Status.Allocatable != nil {
		capacity.GPUCount = node.Status.Allocatable.GPUCount
		capacity.GPUMemoryGiB = node.Status.Allocatable.GPUMemoryGiB
	}
	if capacity.GPUCount == 0 && node != nil {
		capacity.GPUCount = int32(len(node.Spec.GPUs))
	}
	if capacity.GPUMemoryGiB == 0 && node != nil {
		for _, gpu := range node.Spec.GPUs {
			capacity.GPUMemoryGiB += gpu.MemoryGiB
		}
	}

	allocated := allocationSum(allocations)
	allocated = astrav1alpha1.ResourceSummary{
		GPUCount:     allocated.GPUCount,
		GPUMemoryGiB: allocated.GPUMemoryGiB,
	}

	return Snapshot{
		Name:      m.Name(),
		Capacity:  capacity,
		Allocated: allocated,
		Available: astrav1alpha1.ResourceSummary{
			GPUCount:     nonNegative(capacity.GPUCount - allocated.GPUCount),
			GPUMemoryGiB: nonNegative(capacity.GPUMemoryGiB - allocated.GPUMemoryGiB),
		},
		Description: "GPU count and memory budget",
	}, nil
}

func (m *GPUManager) Validate(_ context.Context, req ValidationRequest) ValidationResult {
	if req.Snapshot.Allocated.GPUCount > req.Snapshot.Capacity.GPUCount {
		return ValidationResult{
			Allowed: false,
			Reason:  fmt.Sprintf("local GPU count over capacity: allocated=%d capacity=%d", req.Snapshot.Allocated.GPUCount, req.Snapshot.Capacity.GPUCount),
		}
	}
	if req.Snapshot.Allocated.GPUMemoryGiB > req.Snapshot.Capacity.GPUMemoryGiB {
		return ValidationResult{
			Allowed: false,
			Reason:  fmt.Sprintf("local GPU memory over capacity: allocated=%dGiB capacity=%dGiB", req.Snapshot.Allocated.GPUMemoryGiB, req.Snapshot.Capacity.GPUMemoryGiB),
		}
	}
	return ValidationResult{Allowed: true, Reason: "GPU desired budget fits local capacity"}
}
