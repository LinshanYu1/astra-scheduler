package resources

import (
	"context"
	"fmt"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
)

const TokenThroughputManagerName = "token-throughput"

type TokenThroughputManager struct{}

func NewTokenThroughputManager() *TokenThroughputManager {
	return &TokenThroughputManager{}
}

func (m *TokenThroughputManager) Name() string {
	return TokenThroughputManagerName
}

func (m *TokenThroughputManager) Observe(_ context.Context, node *astrav1alpha1.AINodeResourceProfile, allocations []astrav1alpha1.AIResourceAllocation) (Snapshot, error) {
	capacity := astrav1alpha1.ResourceSummary{}
	if node != nil && node.Status.Allocatable != nil {
		capacity.PrefillTokensPerSecond = node.Status.Allocatable.PrefillTokensPerSecond
		capacity.DecodeTokensPerSecond = node.Status.Allocatable.DecodeTokensPerSecond
		capacity.TotalTokensPerSecond = node.Status.Allocatable.TotalTokensPerSecond
	}
	if node != nil && node.Spec.Runtime != nil && node.Spec.Runtime.Capacity != nil && node.Spec.Runtime.Capacity.TokenThroughput != nil {
		tokenThroughput := node.Spec.Runtime.Capacity.TokenThroughput
		if capacity.PrefillTokensPerSecond == 0 {
			capacity.PrefillTokensPerSecond = tokenThroughput.PrefillTokensPerSecond
		}
		if capacity.DecodeTokensPerSecond == 0 {
			capacity.DecodeTokensPerSecond = tokenThroughput.DecodeTokensPerSecond
		}
		if capacity.TotalTokensPerSecond == 0 {
			capacity.TotalTokensPerSecond = tokenThroughput.TotalTokensPerSecond
		}
	}

	allocated := allocationSum(allocations)
	allocated = astrav1alpha1.ResourceSummary{
		PrefillTokensPerSecond: allocated.PrefillTokensPerSecond,
		DecodeTokensPerSecond:  allocated.DecodeTokensPerSecond,
		TotalTokensPerSecond:   allocated.TotalTokensPerSecond,
	}

	return Snapshot{
		Name:      m.Name(),
		Capacity:  capacity,
		Allocated: allocated,
		Available: astrav1alpha1.ResourceSummary{
			PrefillTokensPerSecond: nonNegative(capacity.PrefillTokensPerSecond - allocated.PrefillTokensPerSecond),
			DecodeTokensPerSecond:  nonNegative(capacity.DecodeTokensPerSecond - allocated.DecodeTokensPerSecond),
			TotalTokensPerSecond:   nonNegative(capacity.TotalTokensPerSecond - allocated.TotalTokensPerSecond),
		},
		Description: "prefill/decode token throughput budget",
	}, nil
}

func (m *TokenThroughputManager) Validate(_ context.Context, req ValidationRequest) ValidationResult {
	if req.Snapshot.Allocated.PrefillTokensPerSecond > req.Snapshot.Capacity.PrefillTokensPerSecond {
		return ValidationResult{
			Allowed: false,
			Reason: fmt.Sprintf(
				"local prefill token throughput over capacity: allocated=%d capacity=%d",
				req.Snapshot.Allocated.PrefillTokensPerSecond,
				req.Snapshot.Capacity.PrefillTokensPerSecond,
			),
		}
	}
	if req.Snapshot.Allocated.DecodeTokensPerSecond > req.Snapshot.Capacity.DecodeTokensPerSecond {
		return ValidationResult{
			Allowed: false,
			Reason: fmt.Sprintf(
				"local decode token throughput over capacity: allocated=%d capacity=%d",
				req.Snapshot.Allocated.DecodeTokensPerSecond,
				req.Snapshot.Capacity.DecodeTokensPerSecond,
			),
		}
	}
	if req.Snapshot.Allocated.TotalTokensPerSecond > req.Snapshot.Capacity.TotalTokensPerSecond {
		return ValidationResult{
			Allowed: false,
			Reason: fmt.Sprintf(
				"local total token throughput over capacity: allocated=%d capacity=%d",
				req.Snapshot.Allocated.TotalTokensPerSecond,
				req.Snapshot.Capacity.TotalTokensPerSecond,
			),
		}
	}
	return ValidationResult{Allowed: true, Reason: "token throughput desired budget fits local capacity"}
}
