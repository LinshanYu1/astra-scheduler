package backend

import (
	"context"
	"fmt"
	"strings"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
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
	operation map[string]RealOperationSupport
}

func NewRealBackend(name string) *RealBackend {
	if name == "" {
		name = "real"
	}
	return &RealBackend{
		name:      name,
		operation: defaultRealOperationSupport(),
	}
}

func (b *RealBackend) Apply(_ context.Context, req ApplyRequest) (ApplyResult, error) {
	if req.Allocation == nil {
		return ApplyResult{}, fmt.Errorf("missing allocation")
	}

	action := strings.ToLower(req.Action)
	switch action {
	case string(astrav1alpha1.AllocationActionApply):
		return ApplyResult{}, b.notImplemented(
			RealOperationUpdatePodResource,
			"real apply requires integration with Kubernetes DRA/device-plugin/CRI or inference-runtime APIs",
		)
	case string(astrav1alpha1.AllocationActionThrottle):
		return ApplyResult{}, b.notImplemented(
			RealOperationThrottleRuntime,
			"real throttle requires runtime-specific concurrency or token-budget APIs such as vLLM/SGLang/TGI adapters",
		)
	case string(astrav1alpha1.AllocationActionPause):
		return ApplyResult{}, b.notImplemented(
			RealOperationPauseRuntime,
			"real pause/resume requires workload-specific queue or job-control APIs",
		)
	case string(astrav1alpha1.AllocationActionEvict):
		return ApplyResult{}, b.notImplemented(
			RealOperationEvictWorkload,
			"real eviction should be implemented through Kubernetes eviction/delete APIs with policy guards",
		)
	default:
		return ApplyResult{}, fmt.Errorf("real backend does not recognize action %q", req.Action)
	}
}

func (b *RealBackend) Snapshot(_ context.Context, req SnapshotRequest) (SnapshotResult, error) {
	if req.Node == nil {
		return SnapshotResult{}, fmt.Errorf("missing node profile")
	}

	status := req.Node.Status
	if status.Runtime == nil {
		status.Runtime = &astrav1alpha1.RuntimeStatus{}
	}
	if status.Runtime.Pressure == nil {
		status.Runtime.Pressure = &astrav1alpha1.RuntimePressure{
			SLORisk: "unknown",
		}
	}
	if status.Phase == "" {
		status.Phase = "Ready"
	}

	return SnapshotResult{Status: status}, nil
}

func (b *RealBackend) OperationSupport() []RealOperationSupport {
	operations := make([]RealOperationSupport, 0, len(b.operation))
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
