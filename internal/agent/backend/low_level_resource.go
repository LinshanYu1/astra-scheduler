package backend

import (
	"context"
	"fmt"
	"os"
	"strings"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
)

// LowLevelResourceDriver hides vendor/runtime-specific resource operations
// from the agent backend. Astra depends on this capability contract instead of
// depending directly on NVIDIA, AMD, DRA, CRI, vLLM, or another implementation.
type LowLevelResourceDriver interface {
	Observe(context.Context, LowLevelObserveRequest) (LowLevelObserveResult, error)
	Apply(context.Context, LowLevelApplyRequest) (LowLevelApplyResult, error)
	Capabilities() []RealOperationSupport
}

type LowLevelObserveRequest struct {
	Node        *astrav1alpha1.AINodeResourceProfile
	Allocations []astrav1alpha1.AIResourceAllocation
}

type LowLevelObserveResult struct {
	GPUs        []observedGPU
	Allocatable *astrav1alpha1.ResourceSummary
	Runtime     *astrav1alpha1.RuntimeStatus
	Message     string
}

type LowLevelApplyRequest struct {
	Node             *astrav1alpha1.AINodeResourceProfile
	Allocation       *astrav1alpha1.AIResourceAllocation
	Action           string
	DesiredResources *astrav1alpha1.ResourceSummary
	Reason           string
}

type LowLevelApplyResult struct {
	Applied bool
	Message string
}

// CommonLowLevelResourceDriver is the first concrete feasibility driver. It
// observes common NVIDIA nodes through nvidia-smi and delegates enforcement to
// pluggable commands. This proves the Astra contract can map to real node-side
// capabilities without hardcoding one runtime.
type CommonLowLevelResourceDriver struct {
	runner    commandRunner
	isolator  ResourceIsolator
	operation map[string]RealOperationSupport
}

func NewCommonLowLevelResourceDriver(runner commandRunner, isolator ResourceIsolator) *CommonLowLevelResourceDriver {
	if runner == nil {
		runner = osCommandRunner{}
	}
	if isolator == nil {
		isolator = NewCommandResourceIsolator()
	}
	return &CommonLowLevelResourceDriver{
		runner:    runner,
		isolator:  isolator,
		operation: defaultRealOperationSupport(),
	}
}

func (d *CommonLowLevelResourceDriver) Observe(ctx context.Context, req LowLevelObserveRequest) (LowLevelObserveResult, error) {
	if req.Node == nil {
		return LowLevelObserveResult{}, fmt.Errorf("missing node profile")
	}

	gpus, err := d.queryNvidiaSMI(ctx)
	if err != nil {
		return LowLevelObserveResult{}, err
	}

	return LowLevelObserveResult{
		GPUs:        gpus,
		Allocatable: observedAllocatable(req.Node, gpus),
		Runtime:     observedRuntimeStatus(req.Node, gpus, req.Allocations),
		Message:     "observed GPU resources through nvidia-smi",
	}, nil
}

func (d *CommonLowLevelResourceDriver) Apply(ctx context.Context, req LowLevelApplyRequest) (LowLevelApplyResult, error) {
	if req.Allocation == nil {
		return LowLevelApplyResult{}, fmt.Errorf("missing allocation")
	}
	if d.isolator == nil {
		return LowLevelApplyResult{
			Applied: false,
			Message: "no resource isolator configured; desired resources recorded only",
		}, nil
	}

	result, err := d.isolator.ApplyIsolation(ctx, IsolationRequest{
		Node:             req.Node,
		Allocation:       req.Allocation,
		Action:           req.Action,
		DesiredResources: req.DesiredResources,
		Reason:           req.Reason,
	})
	if err != nil {
		return LowLevelApplyResult{}, err
	}
	return LowLevelApplyResult{
		Applied: result.Applied,
		Message: result.Message,
	}, nil
}

func (d *CommonLowLevelResourceDriver) Capabilities() []RealOperationSupport {
	operations := make([]RealOperationSupport, 0, len(d.operation))
	for _, support := range d.operation {
		operations = append(operations, support)
	}
	return operations
}

func (d *CommonLowLevelResourceDriver) queryNvidiaSMI(ctx context.Context) ([]observedGPU, error) {
	path := os.Getenv("NVIDIA_SMI_PATH")
	if path == "" {
		var err error
		path, err = d.runner.LookPath("nvidia-smi")
		if err != nil {
			return nil, fmt.Errorf("nvidia-smi not found: %w", err)
		}
	}

	output, err := d.runner.Run(
		ctx,
		path,
		"--query-gpu=index,uuid,name,memory.total,memory.used,utilization.gpu",
		"--format=csv,noheader,nounits",
	)
	if err != nil {
		return nil, fmt.Errorf("run nvidia-smi query: %w output=%s", err, strings.TrimSpace(string(output)))
	}
	return parseNvidiaSMI(output)
}
