package local

import (
	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
)

type ActionPolicy interface {
	ActionFor(allocation astrav1alpha1.AIResourceAllocation) (string, string)
	FallbackFor(allocation astrav1alpha1.AIResourceAllocation) string
}

type DefaultActionPolicy struct{}

func (DefaultActionPolicy) ActionFor(_ astrav1alpha1.AIResourceAllocation) (string, string) {
	return ActionApply, "local scheduler accepted central allocation"
}

func (DefaultActionPolicy) FallbackFor(allocation astrav1alpha1.AIResourceAllocation) string {
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
