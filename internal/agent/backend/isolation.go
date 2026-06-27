package backend

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
)

const (
	isolationCommandApply    = "ASTRA_ISOLATION_APPLY_COMMAND"
	isolationCommandThrottle = "ASTRA_ISOLATION_THROTTLE_COMMAND"
	isolationCommandPause    = "ASTRA_ISOLATION_PAUSE_COMMAND"
	isolationCommandEvict    = "ASTRA_ISOLATION_EVICT_COMMAND"
)

// ResourceIsolator is the node-side boundary for enforcing local AI resource
// budgets. Implementations may call NVIDIA tooling, DRA/device-plugin drivers,
// runtime APIs such as vLLM/SGLang/TGI, or local admission/queue controllers.
type ResourceIsolator interface {
	ApplyIsolation(context.Context, IsolationRequest) (IsolationResult, error)
}

type IsolationRequest struct {
	Node             *astrav1alpha1.AINodeResourceProfile
	Allocation       *astrav1alpha1.AIResourceAllocation
	Action           string
	DesiredResources *astrav1alpha1.ResourceSummary
	Reason           string
}

type IsolationResult struct {
	Applied bool
	Message string
}

// CommandResourceIsolator is a small production-facing adapter boundary. It
// lets a node operator plug in real isolation commands without coupling Astra
// to one GPU/runtime implementation too early.
type CommandResourceIsolator struct {
	commands map[string]string
}

func NewCommandResourceIsolator() *CommandResourceIsolator {
	return &CommandResourceIsolator{
		commands: map[string]string{
			string(astrav1alpha1.AllocationActionApply):    os.Getenv(isolationCommandApply),
			string(astrav1alpha1.AllocationActionThrottle): os.Getenv(isolationCommandThrottle),
			string(astrav1alpha1.AllocationActionPause):    os.Getenv(isolationCommandPause),
			string(astrav1alpha1.AllocationActionEvict):    os.Getenv(isolationCommandEvict),
		},
	}
}

func (i *CommandResourceIsolator) ApplyIsolation(ctx context.Context, req IsolationRequest) (IsolationResult, error) {
	if req.Allocation == nil {
		return IsolationResult{}, fmt.Errorf("missing allocation")
	}

	action := strings.ToLower(req.Action)
	command := i.commands[action]
	if command == "" {
		return IsolationResult{
			Applied: false,
			Message: fmt.Sprintf(
				"no isolation command configured for action %q; recorded desired resources only",
				req.Action,
			),
		}, nil
	}

	cmd := exec.CommandContext(ctx, command)
	cmd.Env = append(os.Environ(), isolationEnv(req)...)
	output, err := cmd.CombinedOutput()
	message := strings.TrimSpace(string(output))
	if err != nil {
		return IsolationResult{}, fmt.Errorf("run isolation command %s for action %q: %w output=%s", command, req.Action, err, message)
	}
	if message == "" {
		message = fmt.Sprintf("isolation command %s completed for action %q", command, req.Action)
	}
	return IsolationResult{
		Applied: true,
		Message: message,
	}, nil
}

func isolationEnv(req IsolationRequest) []string {
	allocation := req.Allocation
	resources := req.DesiredResources
	if resources == nil {
		resources = allocation.Spec.Resources
	}
	if resources == nil {
		resources = &astrav1alpha1.ResourceSummary{}
	}

	nodeName := ""
	if req.Node != nil {
		nodeName = req.Node.Spec.NodeName
		if nodeName == "" {
			nodeName = req.Node.Name
		}
	}

	workloadName := allocation.Spec.WorkloadRef.Name
	workloadNamespace := allocation.Spec.WorkloadRef.Namespace
	if workloadNamespace == "" {
		workloadNamespace = allocation.Namespace
	}

	gpuID := ""
	migSliceID := ""
	assigned := allocation.Spec.AssignedGPU
	if len(allocation.Spec.Workloads) > 0 && allocation.Spec.Workloads[0].AssignedGPU != nil {
		assigned = allocation.Spec.Workloads[0].AssignedGPU
	}
	if assigned != nil {
		gpuID = assigned.GPUID
		migSliceID = assigned.MIGSliceID
	}

	return []string{
		"ASTRA_NODE_NAME=" + nodeName,
		"ASTRA_ALLOCATION_NAMESPACE=" + allocation.Namespace,
		"ASTRA_ALLOCATION_NAME=" + allocation.Name,
		"ASTRA_WORKLOAD_NAMESPACE=" + workloadNamespace,
		"ASTRA_WORKLOAD_NAME=" + workloadName,
		"ASTRA_ACTION=" + req.Action,
		"ASTRA_REASON=" + req.Reason,
		"ASTRA_GPU_ID=" + gpuID,
		"ASTRA_MIG_SLICE_ID=" + migSliceID,
		fmt.Sprintf("ASTRA_GPU_COUNT=%d", resources.GPUCount),
		fmt.Sprintf("ASTRA_GPU_MEMORY_GIB=%d", resources.GPUMemoryGiB),
		fmt.Sprintf("ASTRA_KV_CACHE_GIB=%d", resources.KVCacheGiB),
		fmt.Sprintf("ASTRA_PREFILL_TOKENS_PER_SECOND=%d", resources.PrefillTokensPerSecond),
		fmt.Sprintf("ASTRA_DECODE_TOKENS_PER_SECOND=%d", resources.DecodeTokensPerSecond),
		fmt.Sprintf("ASTRA_TOTAL_TOKENS_PER_SECOND=%d", resources.TotalTokensPerSecond),
	}
}
