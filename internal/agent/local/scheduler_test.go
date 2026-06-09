package local

import (
	"context"
	"testing"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildPlansOrdersAllocationsByPriority(t *testing.T) {
	scheduler := NewDefaultScheduler()
	node := fakeNode(astrav1alpha1.ResourceSummary{
		GPUCount:              4,
		GPUMemoryGiB:          160,
		KVCacheGiB:            64,
		DecodeTokensPerSecond: 8000,
	})

	plans, err := scheduler.BuildPlans(context.Background(), node, []astrav1alpha1.AIResourceAllocation{
		fakeAllocation("low-job", "low", astrav1alpha1.ResourceSummary{GPUCount: 1}),
		fakeAllocation("high-job", "high", astrav1alpha1.ResourceSummary{GPUCount: 1}),
	})
	if err != nil {
		t.Fatalf("BuildPlans returned error: %v", err)
	}
	if len(plans) != 2 {
		t.Fatalf("expected 2 plans, got %d", len(plans))
	}
	if plans[0].Allocation.Name != "high-job" {
		t.Fatalf("expected high priority allocation first, got %s", plans[0].Allocation.Name)
	}
	if plans[0].Action != ActionApply {
		t.Fatalf("expected apply action, got %s", plans[0].Action)
	}
}

func TestBuildPlansFallsBackToThrottleWhenDesiredBudgetExceedsCapacity(t *testing.T) {
	scheduler := NewDefaultScheduler()
	node := fakeNode(astrav1alpha1.ResourceSummary{
		GPUCount:              1,
		GPUMemoryGiB:          80,
		KVCacheGiB:            16,
		DecodeTokensPerSecond: 1000,
	})
	allocation := fakeAllocation("batch-job", "low", astrav1alpha1.ResourceSummary{
		GPUCount:              1,
		KVCacheGiB:            32,
		DecodeTokensPerSecond: 1000,
	})
	allocation.Spec.Actions = &astrav1alpha1.AllocationActions{Throttleable: true}

	plans, err := scheduler.BuildPlans(context.Background(), node, []astrav1alpha1.AIResourceAllocation{allocation})
	if err != nil {
		t.Fatalf("BuildPlans returned error: %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("expected 1 plan, got %d", len(plans))
	}
	if plans[0].Action != ActionThrottle {
		t.Fatalf("expected throttle action, got %s", plans[0].Action)
	}
}

func fakeNode(capacity astrav1alpha1.ResourceSummary) *astrav1alpha1.AINodeResourceProfile {
	return &astrav1alpha1.AINodeResourceProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Spec: astrav1alpha1.AINodeResourceProfileSpec{
			NodeName: "node-a",
			GPUs: []astrav1alpha1.GPUDeviceSpec{
				{ID: "gpu-0", MemoryGiB: 80},
				{ID: "gpu-1", MemoryGiB: 80},
			},
			Runtime: &astrav1alpha1.RuntimeSpec{
				Capacity: &astrav1alpha1.RuntimeCapacity{
					KVCacheGiB: capacity.KVCacheGiB,
					TokenThroughput: &astrav1alpha1.TokenThroughput{
						PrefillTokensPerSecond: capacity.PrefillTokensPerSecond,
						DecodeTokensPerSecond:  capacity.DecodeTokensPerSecond,
						TotalTokensPerSecond:   capacity.TotalTokensPerSecond,
					},
				},
			},
			Capabilities: &astrav1alpha1.NodeCapabilities{Throttling: true, PauseResume: true},
		},
		Status: astrav1alpha1.AINodeResourceProfileStatus{
			Allocatable: &capacity,
		},
	}
}

func fakeAllocation(name string, priority string, resources astrav1alpha1.ResourceSummary) astrav1alpha1.AIResourceAllocation {
	return astrav1alpha1.AIResourceAllocation{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"astra.aiinfra.io/priority": priority,
			},
		},
		Spec: astrav1alpha1.AIResourceAllocationSpec{
			NodeName:  "node-a",
			Resources: &resources,
		},
	}
}
