package backend

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	RealOperationObserveGPU        = "observe_gpu"
	RealOperationObserveMIG        = "observe_mig"
	RealOperationConfigureMIG      = "configure_mig"
	RealOperationObserveRuntime    = "observe_runtime"
	RealOperationThrottleRuntime   = "throttle_runtime"
	RealOperationPauseRuntime      = "pause_runtime"
	RealOperationEvictWorkload     = "evict_workload"
	RealOperationObserveCRI        = "observe_cri"
	RealOperationUpdatePodResource = "update_pod_resource"
)

type RealOperationSupport struct {
	Operation string
	Layer     string
	Backend   string
	ReadOnly  bool
	Supported bool
	Notes     string
}

type RealBackend struct {
	name      string
	options   Options
	operation map[string]RealOperationSupport
	driver    LowLevelResourceDriver
}

type commandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
	LookPath(string) (string, error)
}

type osCommandRunner struct{}

type observedGPU struct {
	Index             string
	UUID              string
	Product           string
	MemoryTotalMiB    int32
	MemoryUsedMiB     int32
	UtilizationGPUPct int32
}

func NewRealBackend(name string, options Options) *RealBackend {
	if name == "" {
		name = "real"
	}
	return &RealBackend{
		name:      name,
		options:   options,
		operation: defaultRealOperationSupport(),
		driver:    NewCommonLowLevelResourceDriver(osCommandRunner{}, NewCommandResourceIsolator()),
	}
}

func (osCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func (osCommandRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

func (b *RealBackend) Apply(ctx context.Context, req ApplyRequest) (ApplyResult, error) {
	if req.Allocation == nil {
		return ApplyResult{}, fmt.Errorf("missing allocation")
	}

	action := strings.ToLower(req.Action)
	switch action {
	case string(astrav1alpha1.AllocationActionApply):
		return b.applyWithIsolation(ctx, req, RealOperationUpdatePodResource)
	case string(astrav1alpha1.AllocationActionThrottle):
		return b.applyWithIsolation(ctx, req, RealOperationThrottleRuntime)
	case string(astrav1alpha1.AllocationActionPause):
		return b.applyWithIsolation(ctx, req, RealOperationPauseRuntime)
	case string(astrav1alpha1.AllocationActionEvict):
		return b.applyWithIsolation(ctx, req, RealOperationEvictWorkload)
	default:
		return ApplyResult{}, fmt.Errorf("real backend does not recognize action %q", req.Action)
	}
}

func (b *RealBackend) Snapshot(ctx context.Context, req SnapshotRequest) (SnapshotResult, error) {
	if req.Node == nil {
		return SnapshotResult{}, fmt.Errorf("missing node profile")
	}

	status := req.Node.Status
	now := metav1.Now()
	status.ObservedAt = &now

	var observed LowLevelObserveResult
	var observeErr error
	if b.driver == nil {
		observeErr = fmt.Errorf("no low-level resource driver configured")
	} else {
		observed, observeErr = b.driver.Observe(ctx, LowLevelObserveRequest{
			Node:        req.Node,
			Allocations: req.Allocations,
		})
	}
	if observeErr == nil && len(observed.GPUs) > 0 {
		status.Phase = "Ready"
		status.GPUs = observedGPUStatuses(observed.GPUs)
		status.Allocatable = observed.Allocatable
		status.Runtime = observed.Runtime
	} else {
		status.Phase = "Degraded"
		status.Allocatable = allocatableFromNode(req.Node)
		status.Runtime = fallbackRuntimeStatus(req.Node, req.Allocations)
	}

	status.Allocated = allocatedFromAllocations(req.Allocations)
	status.Available = subtractResources(status.Allocatable, status.Allocated)
	status.Allocations = nodeAllocationStatuses(req.Allocations)
	status.HourlyForecast = hourlyForecastFromAllocations(req.Allocations)

	if status.Runtime == nil {
		status.Runtime = &astrav1alpha1.RuntimeStatus{}
	}
	if status.Runtime.Pressure == nil {
		status.Runtime.Pressure = &astrav1alpha1.RuntimePressure{
			SLORisk: "unknown",
		}
	}
	if observeErr != nil {
		status.Runtime.Pressure.SLORisk = "unknown"
	}

	return SnapshotResult{Status: status}, nil
}

func (b *RealBackend) OperationSupport() []RealOperationSupport {
	operations := make([]RealOperationSupport, 0, len(b.operation))
	if b.driver != nil {
		return b.driver.Capabilities()
	}
	for _, support := range b.operation {
		operations = append(operations, support)
	}
	return operations
}

func (b *RealBackend) notImplemented(operation string, message string) error {
	support, ok := b.operation[operation]
	if !ok {
		return fmt.Errorf("%s backend operation %q is not mapped: %s", b.name, operation, message)
	}
	return fmt.Errorf(
		"%s backend operation %q is mapped to %s/%s but not implemented yet: %s; notes=%s",
		b.name,
		operation,
		support.Layer,
		support.Backend,
		message,
		support.Notes,
	)
}

func (b *RealBackend) recordDesiredResources(req ApplyRequest, operation string) ApplyResult {
	observed := req.Allocation.Spec.Resources
	if req.DesiredResources != nil {
		observed = req.DesiredResources
	}

	support := b.operation[operation]
	message := fmt.Sprintf(
		"real backend recorded action %q tier=%q targetPhase=%q reason=%s",
		req.Action,
		req.AllocationTier,
		req.TargetPhase,
		req.Reason,
	)
	if support.Operation != "" && !support.ReadOnly {
		message = fmt.Sprintf(
			"%s; lower-level adapter=%s/%s",
			message,
			support.Layer,
			support.Backend,
		)
	}
	return ApplyResult{
		ObservedResources: observed,
		Message:           message,
	}
}

func (b *RealBackend) applyWithIsolation(ctx context.Context, req ApplyRequest, operation string) (ApplyResult, error) {
	result := b.recordDesiredResources(req, operation)
	if b.driver == nil {
		return result, nil
	}

	isolation, err := b.driver.Apply(ctx, LowLevelApplyRequest{
		Node:             req.Node,
		Allocation:       req.Allocation,
		Action:           req.Action,
		DesiredResources: req.DesiredResources,
		Reason:           req.Reason,
	})
	if err != nil {
		return ApplyResult{}, err
	}
	if isolation.Message != "" {
		result.Message = result.Message + "; isolation=" + isolation.Message
	}
	if isolation.Applied {
		result.Message = result.Message + "; isolationApplied=true"
	}
	return result, nil
}

func parseNvidiaSMI(output []byte) ([]observedGPU, error) {
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	gpus := make([]observedGPU, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) != 6 {
			return nil, fmt.Errorf("unexpected nvidia-smi csv line %q", line)
		}
		for index := range parts {
			parts[index] = strings.TrimSpace(parts[index])
		}

		total, err := parseInt32(parts[3])
		if err != nil {
			return nil, fmt.Errorf("parse GPU memory.total from %q: %w", line, err)
		}
		used, err := parseInt32(parts[4])
		if err != nil {
			return nil, fmt.Errorf("parse GPU memory.used from %q: %w", line, err)
		}
		utilization, err := parseInt32(parts[5])
		if err != nil {
			return nil, fmt.Errorf("parse GPU utilization from %q: %w", line, err)
		}

		gpus = append(gpus, observedGPU{
			Index:             parts[0],
			UUID:              parts[1],
			Product:           parts[2],
			MemoryTotalMiB:    total,
			MemoryUsedMiB:     used,
			UtilizationGPUPct: utilization,
		})
	}
	if len(gpus) == 0 {
		return nil, fmt.Errorf("nvidia-smi returned no GPU rows")
	}
	return gpus, nil
}

func parseInt32(value string) (int32, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(parsed), nil
}

func observedGPUStatuses(gpus []observedGPU) []astrav1alpha1.GPUDeviceStatus {
	statuses := make([]astrav1alpha1.GPUDeviceStatus, 0, len(gpus))
	for _, gpu := range gpus {
		statuses = append(statuses, astrav1alpha1.GPUDeviceStatus{
			ID:   gpu.Index,
			UUID: gpu.UUID,
		})
	}
	return statuses
}

func observedAllocatable(node *astrav1alpha1.AINodeResourceProfile, gpus []observedGPU) *astrav1alpha1.ResourceSummary {
	summary := &astrav1alpha1.ResourceSummary{
		GPUCount: int32(len(gpus)),
	}
	for _, gpu := range gpus {
		summary.GPUMemoryGiB += mibToGiB(gpu.MemoryTotalMiB)
	}

	if node.Spec.Runtime != nil && node.Spec.Runtime.Capacity != nil {
		capacity := node.Spec.Runtime.Capacity
		summary.KVCacheGiB = capacity.KVCacheGiB
		if capacity.TokenThroughput != nil {
			summary.PrefillTokensPerSecond = capacity.TokenThroughput.PrefillTokensPerSecond
			summary.DecodeTokensPerSecond = capacity.TokenThroughput.DecodeTokensPerSecond
			summary.TotalTokensPerSecond = capacity.TokenThroughput.TotalTokensPerSecond
		}
	}

	return summary
}

func observedRuntimeStatus(
	node *astrav1alpha1.AINodeResourceProfile,
	gpus []observedGPU,
	allocations []astrav1alpha1.AIResourceAllocation,
) *astrav1alpha1.RuntimeStatus {
	totalMemory := int32(0)
	usedMemory := int32(0)
	totalUtilization := int32(0)
	for _, gpu := range gpus {
		totalMemory += gpu.MemoryTotalMiB
		usedMemory += gpu.MemoryUsedMiB
		totalUtilization += gpu.UtilizationGPUPct
	}

	allocatable := observedAllocatable(node, gpus)
	allocated := allocatedFromAllocations(allocations)
	pressure := &astrav1alpha1.RuntimePressure{
		GPUUtilizationPercent:       average(totalUtilization, int32(len(gpus))),
		GPUMemoryUtilizationPercent: percent(usedMemory, totalMemory),
		KVCacheUtilizationPercent:   percent(allocated.KVCacheGiB, allocatable.KVCacheGiB),
		PrefillUtilizationPercent:   percent(allocated.PrefillTokensPerSecond, allocatable.PrefillTokensPerSecond),
		DecodeUtilizationPercent:    percent(allocated.DecodeTokensPerSecond, allocatable.DecodeTokensPerSecond),
		SLORisk:                     observedSLORisk(average(totalUtilization, int32(len(gpus))), percent(usedMemory, totalMemory)),
	}
	return &astrav1alpha1.RuntimeStatus{Pressure: pressure}
}

func fallbackRuntimeStatus(
	node *astrav1alpha1.AINodeResourceProfile,
	allocations []astrav1alpha1.AIResourceAllocation,
) *astrav1alpha1.RuntimeStatus {
	return &astrav1alpha1.RuntimeStatus{
		Pressure: fakePressure(allocatableFromNode(node), allocatedFromAllocations(allocations)),
	}
}

func observedSLORisk(gpuUtilizationPercent int32, memoryUtilizationPercent int32) string {
	switch {
	case gpuUtilizationPercent >= 90 || memoryUtilizationPercent >= 90:
		return string(astrav1alpha1.SLORiskHigh)
	case gpuUtilizationPercent >= 70 || memoryUtilizationPercent >= 75:
		return string(astrav1alpha1.SLORiskMedium)
	default:
		return string(astrav1alpha1.SLORiskLow)
	}
}

func mibToGiB(value int32) int32 {
	if value <= 0 {
		return 0
	}
	return (value + 1023) / 1024
}

func average(total int32, count int32) int32 {
	if count <= 0 {
		return 0
	}
	return total / count
}

func defaultRealOperationSupport() map[string]RealOperationSupport {
	return map[string]RealOperationSupport{
		RealOperationObserveGPU: {
			Operation: RealOperationObserveGPU,
			Layer:     "node-metrics",
			Backend:   "NVML/DCGM/NVIDIA device plugin",
			ReadOnly:  true,
			Supported: true,
			Notes:     "GPU UUID, memory, utilization, and health can be observed through NVML/DCGM or Kubernetes device plugin state.",
		},
		RealOperationObserveMIG: {
			Operation: RealOperationObserveMIG,
			Layer:     "node-metrics",
			Backend:   "NVML/NVIDIA device plugin",
			ReadOnly:  true,
			Supported: true,
			Notes:     "MIG device identity and capacity can be observed through NVIDIA tooling and device-plugin advertised resources.",
		},
		RealOperationConfigureMIG: {
			Operation: RealOperationConfigureMIG,
			Layer:     "gpu-management",
			Backend:   "NVML/nvidia-smi/GPU Operator",
			ReadOnly:  false,
			Supported: true,
			Notes:     "Dynamic MIG reconfiguration is hardware and cluster-policy dependent, so it should be guarded and usually handled outside the hot scheduling path.",
		},
		RealOperationObserveRuntime: {
			Operation: RealOperationObserveRuntime,
			Layer:     "inference-runtime",
			Backend:   "vLLM/SGLang/TGI/Triton metrics",
			ReadOnly:  true,
			Supported: true,
			Notes:     "Queue depth, TTFT, TPOT, KV cache usage, and token throughput are usually available through runtime metrics or Prometheus exporters.",
		},
		RealOperationThrottleRuntime: {
			Operation: RealOperationThrottleRuntime,
			Layer:     "inference-runtime",
			Backend:   "runtime-specific adapter",
			ReadOnly:  false,
			Supported: true,
			Notes:     "Throttle can be implemented by controlling concurrency, request admission, token budget, or gateway routing, but APIs are runtime-specific.",
		},
		RealOperationPauseRuntime: {
			Operation: RealOperationPauseRuntime,
			Layer:     "workload-control",
			Backend:   "job queue/runtime adapter",
			ReadOnly:  false,
			Supported: true,
			Notes:     "Pause/resume is realistic for batch, embedding, evaluation, and fine-tuning jobs, but not for most online inference requests.",
		},
		RealOperationEvictWorkload: {
			Operation: RealOperationEvictWorkload,
			Layer:     "kubernetes-control",
			Backend:   "Kubernetes eviction/delete APIs",
			ReadOnly:  false,
			Supported: true,
			Notes:     "Eviction is supported by Kubernetes, but should be used as a last-resort policy action.",
		},
		RealOperationObserveCRI: {
			Operation: RealOperationObserveCRI,
			Layer:     "container-runtime",
			Backend:   "CRI/containerd",
			ReadOnly:  true,
			Supported: true,
			Notes:     "CRI/containerd can observe container and sandbox state; fine-grained GPU sharing is usually delegated to device plugins/runtime hooks.",
		},
		RealOperationUpdatePodResource: {
			Operation: RealOperationUpdatePodResource,
			Layer:     "kubernetes-resource",
			Backend:   "DRA/device-plugin/runtime adapter",
			ReadOnly:  false,
			Supported: true,
			Notes:     "Post-scheduling resource updates require integration with Kubernetes DRA, device plugin semantics, or runtime-level admission rather than plain Pod spec mutation.",
		},
	}
}
