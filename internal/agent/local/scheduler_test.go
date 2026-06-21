package local

import (
	"context"
	"testing"
	"time"

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

func TestBuildPlansUsesMaxBudgetInsideActiveWindow(t *testing.T) {
	scheduler := NewDefaultScheduler()
	node := fakeNode(astrav1alpha1.ResourceSummary{
		GPUCount:              2,
		GPUMemoryGiB:          160,
		KVCacheGiB:            64,
		DecodeTokensPerSecond: 4000,
	})
	allocation := fakeAllocation("live-chat", "high", astrav1alpha1.ResourceSummary{GPUCount: 1})
	allocation.Spec.TimeWindows = []astrav1alpha1.TimeWindow{{
		Name:     "evening-peak",
		Start:    "20:00",
		End:      "02:00",
		Timezone: "UTC",
	}}
	allocation.Spec.ResourceRequest = &astrav1alpha1.WorkloadResourceRequest{
		Required:  &astrav1alpha1.ResourceSummary{GPUCount: 1, DecodeTokensPerSecond: 1000},
		Preferred: &astrav1alpha1.ResourceSummary{GPUCount: 1, DecodeTokensPerSecond: 2500},
		Max:       &astrav1alpha1.ResourceSummary{GPUCount: 2, DecodeTokensPerSecond: 4000},
	}

	plans, err := scheduler.BuildPlansAt(
		context.Background(),
		node,
		[]astrav1alpha1.AIResourceAllocation{allocation},
		time.Date(2026, 6, 14, 21, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("BuildPlansAt returned error: %v", err)
	}
	if plans[0].AllocationTier != AllocationTierMax {
		t.Fatalf("expected max tier in active window, got %s", plans[0].AllocationTier)
	}
	if plans[0].AssignedResources.DecodeTokensPerSecond != 4000 {
		t.Fatalf("expected max decode budget, got %d", plans[0].AssignedResources.DecodeTokensPerSecond)
	}
}

func TestBuildPlansRejectsActiveWindowWhenMaxDoesNotFit(t *testing.T) {
	scheduler := NewDefaultScheduler()
	node := fakeNode(astrav1alpha1.ResourceSummary{
		GPUCount:              1,
		GPUMemoryGiB:          80,
		KVCacheGiB:            64,
		DecodeTokensPerSecond: 3000,
	})
	allocation := fakeAllocation("live-chat", "high", astrav1alpha1.ResourceSummary{GPUCount: 1})
	allocation.Spec.TimeWindows = []astrav1alpha1.TimeWindow{{
		Name:     "evening-peak",
		Start:    "20:00",
		End:      "02:00",
		Timezone: "UTC",
	}}
	allocation.Spec.ResourceRequest = &astrav1alpha1.WorkloadResourceRequest{
		Required:  &astrav1alpha1.ResourceSummary{GPUCount: 1, DecodeTokensPerSecond: 1000},
		Preferred: &astrav1alpha1.ResourceSummary{GPUCount: 1, DecodeTokensPerSecond: 2500},
		Max:       &astrav1alpha1.ResourceSummary{GPUCount: 2, DecodeTokensPerSecond: 4000},
	}

	plans, err := scheduler.BuildPlansAt(
		context.Background(),
		node,
		[]astrav1alpha1.AIResourceAllocation{allocation},
		time.Date(2026, 6, 14, 21, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("BuildPlansAt returned error: %v", err)
	}
	if plans[0].AllocationTier != AllocationTierRejected {
		t.Fatalf("expected rejected tier when active max does not fit, got %s", plans[0].AllocationTier)
	}
	if plans[0].TargetPhase != PlanPhaseFailed {
		t.Fatalf("expected failed phase when active max does not fit, got %s", plans[0].TargetPhase)
	}
}

func TestBuildPlansKeepsInactiveWorkloadAtRequiredDuringAnotherActiveWindow(t *testing.T) {
	scheduler := NewDefaultScheduler()
	node := fakeNode(astrav1alpha1.ResourceSummary{
		GPUCount:              3,
		GPUMemoryGiB:          160,
		KVCacheGiB:            96,
		DecodeTokensPerSecond: 6000,
	})
	active := fakeAllocation("live-chat", "high", astrav1alpha1.ResourceSummary{GPUCount: 1})
	active.Spec.TimeWindows = []astrav1alpha1.TimeWindow{{
		Name:     "evening-peak",
		Start:    "20:00",
		End:      "02:00",
		Timezone: "UTC",
	}}
	active.Spec.ResourceRequest = &astrav1alpha1.WorkloadResourceRequest{
		Required:  &astrav1alpha1.ResourceSummary{GPUCount: 1, DecodeTokensPerSecond: 1000},
		Preferred: &astrav1alpha1.ResourceSummary{GPUCount: 1, DecodeTokensPerSecond: 2000},
		Max:       &astrav1alpha1.ResourceSummary{GPUCount: 2, DecodeTokensPerSecond: 4000},
	}
	inactive := fakeAllocation("embedding", "low", astrav1alpha1.ResourceSummary{GPUCount: 1})
	inactive.Spec.Actions = &astrav1alpha1.AllocationActions{Throttleable: true}
	inactive.Spec.ResourceRequest = &astrav1alpha1.WorkloadResourceRequest{
		Required:  &astrav1alpha1.ResourceSummary{GPUCount: 1, DecodeTokensPerSecond: 500},
		Preferred: &astrav1alpha1.ResourceSummary{GPUCount: 2, DecodeTokensPerSecond: 2000},
	}

	plans, err := scheduler.BuildPlansAt(
		context.Background(),
		node,
		[]astrav1alpha1.AIResourceAllocation{inactive, active},
		time.Date(2026, 6, 14, 21, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("BuildPlansAt returned error: %v", err)
	}
	if plans[0].Allocation.Name != "live-chat" || plans[0].AllocationTier != AllocationTierMax {
		t.Fatalf("expected active high-priority workload to get max first, got %s tier=%s", plans[0].Allocation.Name, plans[0].AllocationTier)
	}
	if plans[1].Allocation.Name != "embedding" || plans[1].AllocationTier != AllocationTierRequired {
		t.Fatalf("expected inactive workload to stay at required during active window, got %s tier=%s", plans[1].Allocation.Name, plans[1].AllocationTier)
	}
	if plans[1].AssignedResources.DecodeTokensPerSecond != 500 {
		t.Fatalf("expected inactive workload required decode budget, got %d", plans[1].AssignedResources.DecodeTokensPerSecond)
	}
}

func TestBuildPlansAlignsWithCentralTimeWindowEnvelope(t *testing.T) {
	scheduler := NewDefaultScheduler()
	node := fakeNode(astrav1alpha1.ResourceSummary{
		DecodeTokensPerSecond: 45,
	})
	active := fakeAllocation("live-a", "high", astrav1alpha1.ResourceSummary{})
	active.Spec.TimeWindows = []astrav1alpha1.TimeWindow{{
		Name:     "a-peak",
		Start:    "20:00",
		End:      "08:00",
		Timezone: "UTC",
	}}
	active.Spec.ResourceRequest = &astrav1alpha1.WorkloadResourceRequest{
		Required:  &astrav1alpha1.ResourceSummary{DecodeTokensPerSecond: 10},
		Preferred: &astrav1alpha1.ResourceSummary{DecodeTokensPerSecond: 20},
		Max:       &astrav1alpha1.ResourceSummary{DecodeTokensPerSecond: 35},
	}
	inactive := fakeAllocation("live-b", "high", astrav1alpha1.ResourceSummary{})
	inactive.Spec.TimeWindows = []astrav1alpha1.TimeWindow{{
		Name:     "b-peak",
		Start:    "12:00",
		End:      "19:00",
		Timezone: "UTC",
	}}
	inactive.Spec.ResourceRequest = &astrav1alpha1.WorkloadResourceRequest{
		Required:  &astrav1alpha1.ResourceSummary{DecodeTokensPerSecond: 10},
		Preferred: &astrav1alpha1.ResourceSummary{DecodeTokensPerSecond: 25},
		Max:       &astrav1alpha1.ResourceSummary{DecodeTokensPerSecond: 35},
	}

	plans, err := scheduler.BuildPlansAt(
		context.Background(),
		node,
		[]astrav1alpha1.AIResourceAllocation{inactive, active},
		time.Date(2026, 6, 14, 21, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("BuildPlansAt returned error: %v", err)
	}
	if plans[0].Allocation.Name != "live-a" || plans[0].AllocationTier != AllocationTierMax {
		t.Fatalf("expected active workload to receive max budget first, got %s tier=%s", plans[0].Allocation.Name, plans[0].AllocationTier)
	}
	if plans[0].AssignedResources.DecodeTokensPerSecond != 35 {
		t.Fatalf("expected active workload max decode budget 35, got %d", plans[0].AssignedResources.DecodeTokensPerSecond)
	}
	if plans[1].Allocation.Name != "live-b" || plans[1].AllocationTier != AllocationTierRequired {
		t.Fatalf("expected inactive same-priority workload to receive required budget, got %s tier=%s", plans[1].Allocation.Name, plans[1].AllocationTier)
	}
	if plans[1].AssignedResources.DecodeTokensPerSecond != 10 {
		t.Fatalf("expected inactive workload required decode budget 10, got %d", plans[1].AssignedResources.DecodeTokensPerSecond)
	}
}

func TestBuildPlansOrdersSamePriorityByActiveWindowThenCreationTime(t *testing.T) {
	scheduler := NewDefaultScheduler()
	node := fakeNode(astrav1alpha1.ResourceSummary{
		GPUCount:              4,
		GPUMemoryGiB:          160,
		KVCacheGiB:            96,
		DecodeTokensPerSecond: 8000,
	})
	newerActive := fakeAllocation("newer-active", "medium", astrav1alpha1.ResourceSummary{GPUCount: 1})
	newerActive.CreationTimestamp = metav1.NewTime(time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC))
	newerActive.Spec.TimeWindows = []astrav1alpha1.TimeWindow{{
		Name:     "peak",
		Start:    "20:00",
		End:      "02:00",
		Timezone: "UTC",
	}}
	newerActive.Spec.ResourceRequest = &astrav1alpha1.WorkloadResourceRequest{
		Required: &astrav1alpha1.ResourceSummary{GPUCount: 1, DecodeTokensPerSecond: 500},
		Max:      &astrav1alpha1.ResourceSummary{GPUCount: 1, DecodeTokensPerSecond: 1000},
	}
	olderInactive := fakeAllocation("older-inactive", "medium", astrav1alpha1.ResourceSummary{GPUCount: 1})
	olderInactive.CreationTimestamp = metav1.NewTime(time.Date(2026, 6, 14, 8, 0, 0, 0, time.UTC))
	olderInactive.Spec.ResourceRequest = &astrav1alpha1.WorkloadResourceRequest{
		Required:  &astrav1alpha1.ResourceSummary{GPUCount: 1, DecodeTokensPerSecond: 500},
		Preferred: &astrav1alpha1.ResourceSummary{GPUCount: 1, DecodeTokensPerSecond: 1000},
	}
	newerInactive := fakeAllocation("newer-inactive", "medium", astrav1alpha1.ResourceSummary{GPUCount: 1})
	newerInactive.CreationTimestamp = metav1.NewTime(time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC))
	newerInactive.Spec.ResourceRequest = &astrav1alpha1.WorkloadResourceRequest{
		Required:  &astrav1alpha1.ResourceSummary{GPUCount: 1, DecodeTokensPerSecond: 500},
		Preferred: &astrav1alpha1.ResourceSummary{GPUCount: 1, DecodeTokensPerSecond: 1000},
	}

	plans, err := scheduler.BuildPlansAt(
		context.Background(),
		node,
		[]astrav1alpha1.AIResourceAllocation{newerInactive, newerActive, olderInactive},
		time.Date(2026, 6, 14, 21, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("BuildPlansAt returned error: %v", err)
	}
	got := []string{plans[0].Allocation.Name, plans[1].Allocation.Name, plans[2].Allocation.Name}
	want := []string{"newer-active", "older-inactive", "newer-inactive"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("unexpected order: got %v want %v", got, want)
		}
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
