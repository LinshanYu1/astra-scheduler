package constraints

import (
	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	"github.com/linshanyu/astra-scheduler/internal/agent/resources"
)

type CheckRequest struct {
	Node       *astrav1alpha1.AINodeResourceProfile
	Allocation *astrav1alpha1.AIResourceAllocation
	Action     string
	Snapshots  map[string]resources.Snapshot
}

type CheckResult struct {
	Allowed bool
	Reason  string
}

type Constraint interface {
	Name() string
	Check(CheckRequest) CheckResult
}
