package resources

import (
	"context"
	"fmt"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
)

const KVCacheManagerName = "kv-cache"

type KVCacheManager struct{}

func NewKVCacheManager() *KVCacheManager {
	return &KVCacheManager{}
}

func (m *KVCacheManager) Name() string {
	return KVCacheManagerName
}

func (m *KVCacheManager) Observe(_ context.Context, node *astrav1alpha1.AINodeResourceProfile, allocations []astrav1alpha1.AIResourceAllocation) (Snapshot, error) {
	capacity := astrav1alpha1.ResourceSummary{}
	if node != nil && node.Status.Allocatable != nil {
		capacity.KVCacheGiB = node.Status.Allocatable.KVCacheGiB
	}
	if capacity.KVCacheGiB == 0 && node != nil && node.Spec.Runtime != nil && node.Spec.Runtime.Capacity != nil {
		capacity.KVCacheGiB = node.Spec.Runtime.Capacity.KVCacheGiB
	}

	allocated := allocationSum(allocations)
	allocated = astrav1alpha1.ResourceSummary{KVCacheGiB: allocated.KVCacheGiB}

	return Snapshot{
		Name:      m.Name(),
		Capacity:  capacity,
		Allocated: allocated,
		Available: astrav1alpha1.ResourceSummary{
			KVCacheGiB: nonNegative(capacity.KVCacheGiB - allocated.KVCacheGiB),
		},
		Description: "runtime KV cache budget",
	}, nil
}

func (m *KVCacheManager) Validate(_ context.Context, req ValidationRequest) ValidationResult {
	if req.Snapshot.Allocated.KVCacheGiB > req.Snapshot.Capacity.KVCacheGiB {
		return ValidationResult{
			Allowed: false,
			Reason:  fmt.Sprintf("local KV cache over capacity: allocated=%dGiB capacity=%dGiB", req.Snapshot.Allocated.KVCacheGiB, req.Snapshot.Capacity.KVCacheGiB),
		}
	}
	return ValidationResult{Allowed: true, Reason: "KV cache desired budget fits local capacity"}
}
