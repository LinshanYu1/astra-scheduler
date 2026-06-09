package local

import (
	"context"
	"fmt"
	"sort"
	"strings"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	"github.com/linshanyu/astra-scheduler/internal/agent/constraints"
	"github.com/linshanyu/astra-scheduler/internal/agent/resources"
)

const (
	ActionApply    = "apply"
	ActionThrottle = "throttle"
	ActionPause    = "pause"
	ActionEvict    = "evict"
)

type Plan struct {
	Allocation astrav1alpha1.AIResourceAllocation
	Action     string
	Reason     string
}

type Scheduler struct {
	resourceManagers []resources.Manager
	constraints      []constraints.Constraint
}

func NewScheduler(resourceManagers []resources.Manager, constraints []constraints.Constraint) *Scheduler {
	return &Scheduler{
		resourceManagers: resourceManagers,
		constraints:      constraints,
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
	ordered := append([]astrav1alpha1.AIResourceAllocation(nil), allocations...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return priorityRank(ordered[i].Labels["astra.aiinfra.io/priority"]) > priorityRank(ordered[j].Labels["astra.aiinfra.io/priority"])
	})

	state, err := s.observe(ctx, node, ordered)
	if err != nil {
		return nil, err
	}

	plans := make([]Plan, 0, len(ordered))
	for _, allocation := range ordered {
		action, reason := s.actionForAllocation(allocation)
		request := constraints.CheckRequest{
			Node:       node,
			Allocation: &allocation,
			Action:     action,
			Snapshots:  state.Snapshots,
		}

		if result := s.validateResources(ctx, state, allocation); !result.Allowed {
			plans = append(plans, Plan{
				Allocation: allocation,
				Action:     fallbackAction(allocation),
				Reason:     result.Reason,
			})
			continue
		}
		if result := s.checkConstraints(request); !result.Allowed {
			plans = append(plans, Plan{
				Allocation: allocation,
				Action:     fallbackAction(allocation),
				Reason:     result.Reason,
			})
			continue
		}

		plans = append(plans, Plan{
			Allocation: allocation,
			Action:     action,
			Reason:     reason,
		})
	}

	return plans, nil
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

func (s *Scheduler) actionForAllocation(allocation astrav1alpha1.AIResourceAllocation) (string, string) {
	return ActionApply, "local scheduler accepted central allocation"
}

func fallbackAction(allocation astrav1alpha1.AIResourceAllocation) string {
	if allocation.Spec.Actions != nil {
		if allocation.Spec.Actions.Throttleable {
			return ActionThrottle
		}
		if allocation.Spec.Actions.Pauseable {
			return ActionPause
		}
		if allocation.Spec.Actions.Evictable {
			return ActionEvict
		}
	}
	return ActionApply
}

func priorityRank(priority string) int {
	switch priority {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}
