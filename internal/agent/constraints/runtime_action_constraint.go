package constraints

import (
	"strings"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
)

type RuntimeActionCapability struct{}

func NewRuntimeActionCapability() *RuntimeActionCapability {
	return &RuntimeActionCapability{}
}

func (c *RuntimeActionCapability) Name() string {
	return "runtime-action-capability"
}

func (c *RuntimeActionCapability) Check(req CheckRequest) CheckResult {
	if req.Node == nil || req.Node.Spec.Capabilities == nil {
		return CheckResult{Allowed: true}
	}
	capabilities := req.Node.Spec.Capabilities

	switch strings.ToLower(req.Action) {
	case string(astrav1alpha1.AllocationActionThrottle):
		if !capabilities.Throttling {
			return CheckResult{Allowed: false, Reason: "node backend does not support throttling action"}
		}
	case string(astrav1alpha1.AllocationActionPause):
		if !capabilities.PauseResume {
			return CheckResult{Allowed: false, Reason: "node backend does not support pause action"}
		}
	case string(astrav1alpha1.AllocationActionEvict):
		if !capabilities.Eviction {
			return CheckResult{Allowed: false, Reason: "node backend does not support eviction action"}
		}
	}

	return CheckResult{Allowed: true, Reason: "runtime action capability is satisfied"}
}
