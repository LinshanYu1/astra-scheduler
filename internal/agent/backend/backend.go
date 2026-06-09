package backend

import (
	"context"
	"fmt"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
)

type ApplyRequest struct {
	Node       *astrav1alpha1.AINodeResourceProfile
	Allocation *astrav1alpha1.AIResourceAllocation
	Action     string
}

type ApplyResult struct {
	ObservedResources *astrav1alpha1.ResourceSummary
	Message           string
}

type SnapshotRequest struct {
	Node        *astrav1alpha1.AINodeResourceProfile
	Allocations []astrav1alpha1.AIResourceAllocation
}

type SnapshotResult struct {
	Status astrav1alpha1.AINodeResourceProfileStatus
}

type Backend interface {
	Apply(context.Context, ApplyRequest) (ApplyResult, error)
	Snapshot(context.Context, SnapshotRequest) (SnapshotResult, error)
}

func New(name string) (Backend, error) {
	switch name {
	case "", "fake":
		return NewFakeBackend(), nil
	default:
		return nil, fmt.Errorf("unsupported agent backend %q", name)
	}
}
