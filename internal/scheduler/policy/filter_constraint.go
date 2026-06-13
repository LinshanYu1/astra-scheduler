package policy

import (
	"fmt"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
)

type FilterConstraint interface {
	Name() string
	Check(profile *astrav1alpha1.AIWorkloadProfile, node *astrav1alpha1.AINodeResourceProfile) FilterDecision
}

var filterConstraints *FilterConstraints

func init() {
	filterConstraints = &FilterConstraints{constraints: []FilterConstraint{}}
	filterConstraints.register(
		BaseFilterConstraint{},
		RuntimeHardConstraint{},
		NodeCapabilityHardConstraint{},
		RequiredResourceFitConstraint{},
		TimeWindowEnvelopeConstraint{},
	)
}

func GetFilterConstraints() *FilterConstraints {
	return filterConstraints
}

type FilterConstraints struct {
	constraints []FilterConstraint
}

func (r *FilterConstraints) register(constraints ...FilterConstraint) {
	r.constraints = append(r.constraints, constraints...)
}

func (r *FilterConstraints) RunAllConstraints(
	profile *astrav1alpha1.AIWorkloadProfile,
	node *astrav1alpha1.AINodeResourceProfile,
) FilterDecision {
	for _, constraint := range r.constraints {
		decision := constraint.Check(profile, node)
		if !decision.Allowed {
			return FilterDecision{
				Allowed: false,
				Reason:  fmt.Sprintf("%s rejected: %s", constraint.Name(), decision.Reason),
			}
		}
	}

	return FilterDecision{Allowed: true, Reason: "all filter constraints passed"}
}

type BaseFilterConstraint struct{}

func (BaseFilterConstraint) Name() string {
	return "BaseFilterConstraint"
}

func (BaseFilterConstraint) Check(
	profile *astrav1alpha1.AIWorkloadProfile,
	node *astrav1alpha1.AINodeResourceProfile,
) FilterDecision {
	if profile == nil {
		return FilterDecision{Allowed: false, Reason: "missing AIWorkloadProfile"}
	}
	if node == nil {
		return FilterDecision{Allowed: false, Reason: "missing AINodeResourceProfile"}
	}
	if !nodeIsReady(node) {
		return FilterDecision{Allowed: false, Reason: fmt.Sprintf("node AI resource profile is not ready: phase=%s", node.Status.Phase)}
	}
	if node.Status.Available == nil {
		return FilterDecision{Allowed: false, Reason: "node has no available resource summary"}
	}

	return FilterDecision{Allowed: true, Reason: "base filter constraints satisfied"}
}

type RuntimeHardConstraint struct{}

func (RuntimeHardConstraint) Name() string {
	return "RuntimeHardConstraint"
}

func (RuntimeHardConstraint) Check(
	profile *astrav1alpha1.AIWorkloadProfile,
	node *astrav1alpha1.AINodeResourceProfile,
) FilterDecision {
	required := requiredResources(profile)
	supports := runtimeSupports(node)
	if requiresKVCache(required, profile) && !supports.KVCache {
		return FilterDecision{Allowed: false, Reason: "node runtime does not support KV cache"}
	}
	if requiresTokenThroughput(required, profile) && !supports.TokenThroughput {
		return FilterDecision{Allowed: false, Reason: "node runtime does not support token throughput control"}
	}
	return FilterDecision{Allowed: true, Reason: "runtime hard constraints satisfied"}
}

type NodeCapabilityHardConstraint struct{}

func (NodeCapabilityHardConstraint) Name() string {
	return "NodeCapabilityHardConstraint"
}

func (NodeCapabilityHardConstraint) Check(
	profile *astrav1alpha1.AIWorkloadProfile,
	node *astrav1alpha1.AINodeResourceProfile,
) FilterDecision {
	if profile == nil || profile.Spec.Policy == nil {
		return FilterDecision{Allowed: true, Reason: "no workload policy hard constraints"}
	}
	capabilities := nodeCapabilities(node)
	policy := profile.Spec.Policy

	if policy.AllowBorrowing && !capabilities.Borrowing {
		return FilterDecision{Allowed: false, Reason: "node does not support resource borrowing"}
	}
	if policy.AllowReclaimFromLowerPriority && !capabilities.Reclaim {
		return FilterDecision{Allowed: false, Reason: "node does not support resource reclaim"}
	}
	if policy.Throttleable && !capabilities.Throttling {
		return FilterDecision{Allowed: false, Reason: "node does not support throttling"}
	}
	if policy.Pauseable && !capabilities.PauseResume {
		return FilterDecision{Allowed: false, Reason: "node does not support pause/resume"}
	}
	if policy.Evictable && !capabilities.Eviction {
		return FilterDecision{Allowed: false, Reason: "node does not support eviction"}
	}

	return FilterDecision{Allowed: true, Reason: "node capability hard constraints satisfied"}
}

type RequiredResourceFitConstraint struct{}

func (RequiredResourceFitConstraint) Name() string {
	return "RequiredResourceFitConstraint"
}

func (RequiredResourceFitConstraint) Check(
	profile *astrav1alpha1.AIWorkloadProfile,
	node *astrav1alpha1.AINodeResourceProfile,
) FilterDecision {
	required := requiredResources(profile)
	available := node.Status.Available

	if required.GPUCount > available.GPUCount {
		return reject("gpuCount", required.GPUCount, available.GPUCount)
	}
	if required.GPUMemoryGiB > available.GPUMemoryGiB {
		return reject("gpuMemoryGiB", required.GPUMemoryGiB, available.GPUMemoryGiB)
	}
	if required.KVCacheGiB > available.KVCacheGiB {
		return reject("kvCacheGiB", required.KVCacheGiB, available.KVCacheGiB)
	}
	if required.PrefillTokensPerSecond > available.PrefillTokensPerSecond {
		return reject("prefillTokensPerSecond", required.PrefillTokensPerSecond, available.PrefillTokensPerSecond)
	}
	if required.DecodeTokensPerSecond > available.DecodeTokensPerSecond {
		return reject("decodeTokensPerSecond", required.DecodeTokensPerSecond, available.DecodeTokensPerSecond)
	}
	if required.TotalTokensPerSecond > available.TotalTokensPerSecond {
		return reject("totalTokensPerSecond", required.TotalTokensPerSecond, available.TotalTokensPerSecond)
	}

	return FilterDecision{Allowed: true, Reason: "required AI resources fit node availability"}
}

type TimeWindowEnvelopeConstraint struct{}

func (TimeWindowEnvelopeConstraint) Name() string {
	return "TimeWindowEnvelopeConstraint"
}

func (c TimeWindowEnvelopeConstraint) Check(
	profile *astrav1alpha1.AIWorkloadProfile,
	node *astrav1alpha1.AINodeResourceProfile,
) FilterDecision {
	return c.check(profile, node)
}
