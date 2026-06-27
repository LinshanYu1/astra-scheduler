package backend

import (
	"context"
	"fmt"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ApplyRequest struct {
	Node             *astrav1alpha1.AINodeResourceProfile
	Allocation       *astrav1alpha1.AIResourceAllocation
	Action           string
	DesiredResources *astrav1alpha1.ResourceSummary
	AllocationTier   string
	TargetPhase      string
	Reason           string
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

type Options struct {
	Client             client.Reader
	NodeName           string
	ConfigMapNamespace string
	ConfigMapName      string
}

func New(name string) (Backend, error) {
	return NewWithOptions(name, Options{})
}

func NewWithOptions(name string, options Options) (Backend, error) {
	switch name {
	case "", "fake":
		return NewFakeBackend(options), nil
	case "real", "nvidia", "runtime", "cri":
		return NewRealBackend(name, options), nil
	default:
		return nil, fmt.Errorf("unsupported agent backend %q", name)
	}
}
