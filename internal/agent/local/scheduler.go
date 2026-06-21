package local

import (
	"context"
	"fmt"
	"strings"
	"time"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	"github.com/linshanyu/astra-scheduler/internal/agent/constraints"
	"github.com/linshanyu/astra-scheduler/internal/agent/resources"
)

const (
	ActionApply    = string(astrav1alpha1.AllocationActionApply)
	ActionThrottle = string(astrav1alpha1.AllocationActionThrottle)
	ActionPause    = string(astrav1alpha1.AllocationActionPause)
	ActionEvict    = string(astrav1alpha1.AllocationActionEvict)

	PlanPhasePendingApply = string(astrav1alpha1.AllocationPhasePendingApply)
	PlanPhaseApplied      = string(astrav1alpha1.AllocationPhaseApplied)
	PlanPhaseFailed       = string(astrav1alpha1.AllocationPhaseFailed)

	AllocationTierRequired  = string(astrav1alpha1.AllocationTierRequired)
	AllocationTierPreferred = string(astrav1alpha1.AllocationTierPreferred)
	AllocationTierMax       = string(astrav1alpha1.AllocationTierMax)
	AllocationTierDegraded  = string(astrav1alpha1.AllocationTierDegraded)
	AllocationTierRejected  = string(astrav1alpha1.AllocationTierRejected)
)

type Plan struct {
	Allocation         astrav1alpha1.AIResourceAllocation
	Action             string
	Reason             string
	TargetPhase        string
	AllocationTier     string
	RequestedResources astrav1alpha1.ResourceSummary
	AssignedResources  astrav1alpha1.ResourceSummary
	ActiveWindow       string
}

type Scheduler struct {
	resourceManagers []resources.Manager
	constraints      []constraints.Constraint
	actionPolicy     ActionPolicy
}

func NewScheduler(resourceManagers []resources.Manager, constraints []constraints.Constraint) *Scheduler {
	return &Scheduler{
		resourceManagers: resourceManagers,
		constraints:      constraints,
		actionPolicy:     DefaultActionPolicy{},
	}
}

func NewDefaultScheduler() *Scheduler {
	return NewScheduler(
		[]resources.Manager{
			resources.NewGPUManager(),
			resources.NewKVCacheManager(),
			resources.NewTokenThroughputManager(),
		},
		[]constraints.Constraint{
			constraints.NewTokenThroughputRequiresGPU(),
			constraints.NewRuntimeActionCapability(),
		},
	)
}

func (s *Scheduler) BuildPlans(
	ctx context.Context,
	node *astrav1alpha1.AINodeResourceProfile,
	allocations []astrav1alpha1.AIResourceAllocation,
) ([]Plan, error) {
	return s.BuildPlansAt(ctx, node, allocations, time.Now())
}

func (s *Scheduler) BuildPlansAt(
	ctx context.Context,
	node *astrav1alpha1.AINodeResourceProfile,
	allocations []astrav1alpha1.AIResourceAllocation,
	now time.Time,
) ([]Plan, error) {
	ordered := orderAllocationsByPriorityAndWindow(allocations, now)

	state, err := s.observe(ctx, node, ordered)
	if err != nil {
		return nil, err
	}

	ledger := newLedger(capacityFromState(state))
	plans := make([]Plan, 0, len(ordered))
	for _, group := range groupAllocationsByPriority(ordered) {
		groupHasActiveWindow := hasAnyActiveTimeWindow(group, now)
		for _, allocation := range group {
			plan := s.buildPlanForAllocation(ctx, node, state, ledger, allocation, now, groupHasActiveWindow)
			plans = append(plans, plan)
		}
	}

	return plans, nil
}

func (s *Scheduler) buildPlanForAllocation(
	ctx context.Context,
	node *astrav1alpha1.AINodeResourceProfile,
	state resources.NodeState,
	ledger *ledger,
	allocation astrav1alpha1.AIResourceAllocation,
	now time.Time,
	groupHasActiveWindow bool,
) Plan {
	action, reason := s.actionPolicy.ActionFor(allocation)
	activeWindow := activeTimeWindow(allocation.Spec.TimeWindows, now)
	required := requiredBudget(allocation)
	preferred := preferredBudget(allocation)
	maximum := maxBudget(allocation)
	target := preferred
	requiredForFit := required
	candidates := []budgetCandidate{{
		Tier:      AllocationTierPreferred,
		Resources: preferred,
	}}

	switch {
	case activeWindow != "" && !isZeroResource(maximum):
		target = maximum
		requiredForFit = maximum
		candidates = []budgetCandidate{{
			Tier:      AllocationTierMax,
			Resources: maximum,
		}}
		reason = fmt.Sprintf("%s; active time window %q requires max budget", reason, activeWindow)
	case groupHasActiveWindow && canYieldResources(allocation):
		target = required
		requiredForFit = required
		candidates = nil
		reason = fmt.Sprintf("%s; same-priority active workload exists, reclaimable workload keeps required budget", reason)
	case groupHasActiveWindow:
		target = required
		requiredForFit = required
		candidates = nil
		reason = fmt.Sprintf("%s; same-priority active workload exists, workload keeps required budget to match central time-window envelope", reason)
	case isZeroResource(preferred):
		target = required
		requiredForFit = required
		candidates = nil
	}

	assigned, assignedTier, ok := ledger.reserve(allocationKey(allocation), requiredForFit, candidates)
	if !ok {
		return Plan{
			Allocation:         allocation,
			Action:             s.actionPolicy.FallbackFor(allocation),
			Reason:             fmt.Sprintf("budget does not fit local ledger: required=%s remaining=%s", describeResource(requiredForFit), describeResource(ledger.Remaining)),
			TargetPhase:        PlanPhaseFailed,
			AllocationTier:     AllocationTierRejected,
			RequestedResources: requiredForFit,
			AssignedResources:  astrav1alpha1.ResourceSummary{},
			ActiveWindow:       activeWindow,
		}
	}

	plannedAllocation := allocation.DeepCopy()
	plannedAllocation.Spec.Resources = assigned.DeepCopy()
	if result := s.validateResources(ctx, state, *plannedAllocation); !result.Allowed {
		ledger.release(allocationKey(allocation))
		return Plan{
			Allocation:         allocation,
			Action:             s.actionPolicy.FallbackFor(allocation),
			Reason:             result.Reason,
			TargetPhase:        PlanPhaseFailed,
			AllocationTier:     AllocationTierDegraded,
			RequestedResources: target,
			AssignedResources:  assigned,
			ActiveWindow:       activeWindow,
		}
	}

	request := constraints.CheckRequest{
		Node:       node,
		Allocation: plannedAllocation,
		Action:     action,
		Snapshots:  state.Snapshots,
	}

	if result := s.checkConstraints(request); !result.Allowed {
		ledger.release(allocationKey(allocation))
		return Plan{
			Allocation:         allocation,
			Action:             s.actionPolicy.FallbackFor(allocation),
			Reason:             result.Reason,
			TargetPhase:        PlanPhaseFailed,
			AllocationTier:     AllocationTierDegraded,
			RequestedResources: target,
			AssignedResources:  assigned,
			ActiveWindow:       activeWindow,
		}
	}

	return Plan{
		Allocation:         allocation,
		Action:             action,
		Reason:             fmt.Sprintf("%s; assigned %s budget", reason, assignedTier),
		TargetPhase:        PlanPhasePendingApply,
		AllocationTier:     assignedTier,
		RequestedResources: target,
		AssignedResources:  assigned,
		ActiveWindow:       activeWindow,
	}
}

func (s *Scheduler) observe(
	ctx context.Context,
	node *astrav1alpha1.AINodeResourceProfile,
	allocations []astrav1alpha1.AIResourceAllocation,
) (resources.NodeState, error) {
	snapshots := make(map[string]resources.Snapshot, len(s.resourceManagers))
	for _, manager := range s.resourceManagers {
		snapshot, err := manager.Observe(ctx, node, allocations)
		if err != nil {
			return resources.NodeState{}, fmt.Errorf("observe %s: %w", manager.Name(), err)
		}
		snapshots[manager.Name()] = snapshot
	}
	return resources.NodeState{
		Node:        node,
		Allocations: allocations,
		Snapshots:   snapshots,
	}, nil
}

func (s *Scheduler) validateResources(
	ctx context.Context,
	state resources.NodeState,
	allocation astrav1alpha1.AIResourceAllocation,
) resources.ValidationResult {
	var reasons []string
	for _, manager := range s.resourceManagers {
		snapshot := state.Snapshots[manager.Name()]
		if allocation.Spec.Resources != nil {
			snapshot.Allocated = *allocation.Spec.Resources
			snapshot.Available = subtractResource(snapshot.Capacity, snapshot.Allocated)
		}
		result := manager.Validate(ctx, resources.ValidationRequest{
			Node:       state.Node,
			Allocation: &allocation,
			Snapshot:   snapshot,
		})
		if !result.Allowed {
			return result
		}
		if result.Reason != "" {
			reasons = append(reasons, result.Reason)
		}
	}
	return resources.ValidationResult{
		Allowed: true,
		Reason:  strings.Join(reasons, "; "),
	}
}

func (s *Scheduler) checkConstraints(request constraints.CheckRequest) constraints.CheckResult {
	var reasons []string
	for _, constraint := range s.constraints {
		result := constraint.Check(request)
		if !result.Allowed {
			return result
		}
		if result.Reason != "" {
			reasons = append(reasons, result.Reason)
		}
	}
	return constraints.CheckResult{
		Allowed: true,
		Reason:  strings.Join(reasons, "; "),
	}
}

func priorityRank(priority string) int {
	switch strings.ToLower(priority) {
	case string(astrav1alpha1.WorkloadPriorityCritical):
		return 4
	case string(astrav1alpha1.WorkloadPriorityHigh):
		return 3
	case string(astrav1alpha1.WorkloadPriorityMedium):
		return 2
	case string(astrav1alpha1.WorkloadPriorityLow):
		return 1
	default:
		return 0
	}
}
