package constraints

import (
	"github.com/linshanyu/astra-scheduler/internal/agent/resources"
)

type TokenThroughputRequiresGPU struct{}

func NewTokenThroughputRequiresGPU() *TokenThroughputRequiresGPU {
	return &TokenThroughputRequiresGPU{}
}

func (c *TokenThroughputRequiresGPU) Name() string {
	return "token-throughput-requires-gpu"
}

func (c *TokenThroughputRequiresGPU) Check(req CheckRequest) CheckResult {
	if req.Allocation == nil || req.Allocation.Spec.Resources == nil {
		return CheckResult{Allowed: true}
	}
	requested := req.Allocation.Spec.Resources
	needsTokenBudget := requested.PrefillTokensPerSecond > 0 ||
		requested.DecodeTokensPerSecond > 0 ||
		requested.TotalTokensPerSecond > 0
	if !needsTokenBudget {
		return CheckResult{Allowed: true}
	}

	gpuSnapshot := req.Snapshots[resources.GPUManagerName]
	if gpuSnapshot.Capacity.GPUCount <= 0 {
		return CheckResult{
			Allowed: false,
			Reason:  "token throughput allocation requires GPU capacity on the node",
		}
	}
	return CheckResult{Allowed: true, Reason: "token throughput has GPU capacity backing"}
}
