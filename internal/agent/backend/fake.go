package backend

import (
	"context"
	"fmt"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

type FakeBackend struct {
	options Options
}

type FakeNodeResourceConfig struct {
	Allocatable *astrav1alpha1.ResourceSummary `json:"allocatable,omitempty"`
	Runtime     *astrav1alpha1.RuntimeStatus   `json:"runtime,omitempty"`
}

func NewFakeBackend(options Options) *FakeBackend {
	return &FakeBackend{options: options}
}

func (b *FakeBackend) Apply(_ context.Context, req ApplyRequest) (ApplyResult, error) {
	if req.Allocation == nil {
		return ApplyResult{}, fmt.Errorf("missing allocation")
	}
	observed := req.Allocation.Spec.Resources
	if req.DesiredResources != nil {
		observed = req.DesiredResources
	}
	return ApplyResult{
		ObservedResources: observed,
		Message: fmt.Sprintf(
			"fake backend accepted action %q tier=%q targetPhase=%q reason=%s",
			req.Action,
			req.AllocationTier,
			req.TargetPhase,
			req.Reason,
		),
	}, nil
}

func (b *FakeBackend) Snapshot(ctx context.Context, req SnapshotRequest) (SnapshotResult, error) {
	if req.Node == nil {
		return SnapshotResult{}, fmt.Errorf("missing node profile")
	}

	status := req.Node.Status
	config, err := b.nodeResourceConfig(ctx, req.Node)
	if err != nil {
		return SnapshotResult{}, err
	}
	status.Allocatable = allocatableFromNode(req.Node)
	if config != nil && config.Allocatable != nil {
		status.Allocatable = config.Allocatable.DeepCopy()
	}
	status.Allocated = allocatedFromAllocations(req.Allocations)
	status.Available = subtractResources(status.Allocatable, status.Allocated)
	status.Allocations = nodeAllocationStatuses(req.Allocations)

	if status.Runtime == nil {
		status.Runtime = &astrav1alpha1.RuntimeStatus{}
	}
	if config != nil && config.Runtime != nil {
		status.Runtime = config.Runtime.DeepCopy()
	}
	if status.Runtime.Pressure == nil {
		status.Runtime.Pressure = fakePressure(status.Allocatable, status.Allocated)
	}

	return SnapshotResult{Status: status}, nil
}

func (b *FakeBackend) nodeResourceConfig(ctx context.Context, node *astrav1alpha1.AINodeResourceProfile) (*FakeNodeResourceConfig, error) {
	if b.options.Client == nil || b.options.ConfigMapName == "" {
		return nil, nil
	}

	namespace := b.options.ConfigMapNamespace
	if namespace == "" {
		namespace = node.Namespace
	}
	if namespace == "" {
		namespace = "astra-system"
	}

	configMap := &corev1.ConfigMap{}
	if err := b.options.Client.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      b.options.ConfigMapName,
	}, configMap); err != nil {
		return nil, fmt.Errorf("get fake node resource ConfigMap %s/%s: %w", namespace, b.options.ConfigMapName, err)
	}

	nodeName := b.options.NodeName
	if nodeName == "" {
		nodeName = node.Spec.NodeName
	}
	if nodeName == "" {
		nodeName = node.Name
	}

	value, ok := nodeConfigMapValue(configMap.Data, nodeName)
	if !ok {
		return nil, nil
	}

	config := &FakeNodeResourceConfig{}
	if err := yaml.Unmarshal([]byte(value), config); err != nil {
		return nil, fmt.Errorf("parse fake node resource config for node %q: %w", nodeName, err)
	}
	return config, nil
}

func nodeConfigMapValue(data map[string]string, nodeName string) (string, bool) {
	for _, key := range []string{
		nodeName + ".yaml",
		nodeName + ".yml",
		nodeName,
		"resources.yaml",
		"resources.yml",
	} {
		if value, ok := data[key]; ok {
			return value, true
		}
	}
	return "", false
}

func allocatableFromNode(node *astrav1alpha1.AINodeResourceProfile) *astrav1alpha1.ResourceSummary {
	summary := &astrav1alpha1.ResourceSummary{}
	summary.GPUCount = int32(len(node.Spec.GPUs))
	for _, gpu := range node.Spec.GPUs {
		summary.GPUMemoryGiB += gpu.MemoryGiB
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

func allocatedFromAllocations(allocations []astrav1alpha1.AIResourceAllocation) *astrav1alpha1.ResourceSummary {
	summary := &astrav1alpha1.ResourceSummary{}
	for _, allocation := range allocations {
		resources := allocation.Status.ObservedResources
		if resources == nil {
			resources = allocation.Spec.Resources
		}
		if resources == nil {
			continue
		}
		addResources(summary, resources)
	}
	return summary
}

func nodeAllocationStatuses(allocations []astrav1alpha1.AIResourceAllocation) []astrav1alpha1.NodeAllocationStatus {
	statuses := make([]astrav1alpha1.NodeAllocationStatus, 0, len(allocations))
	for _, allocation := range allocations {
		workloadName := allocation.Spec.WorkloadRef.Name
		statuses = append(statuses, astrav1alpha1.NodeAllocationStatus{
			Name:            allocation.Name,
			Namespace:       allocation.Namespace,
			WorkloadName:    workloadName,
			WorkloadType:    allocation.Spec.WorkloadType,
			Priority:        allocation.Spec.Priority,
			DemandShape:     allocation.Spec.DemandShape,
			TimeWindows:     allocation.Spec.TimeWindows,
			ResourceRequest: allocation.Spec.ResourceRequest,
			AssignedGPU:     allocation.Spec.AssignedGPU,
			Resources:       allocationResources(allocation),
			Actions:         allocation.Spec.Actions,
			Phase:           allocation.Status.Phase,
		})
	}
	return statuses
}

func allocationResources(allocation astrav1alpha1.AIResourceAllocation) *astrav1alpha1.ResourceSummary {
	if allocation.Status.ObservedResources != nil {
		return allocation.Status.ObservedResources
	}
	return allocation.Spec.Resources
}

func addResources(target *astrav1alpha1.ResourceSummary, value *astrav1alpha1.ResourceSummary) {
	target.GPUCount += value.GPUCount
	target.GPUMemoryGiB += value.GPUMemoryGiB
	target.KVCacheGiB += value.KVCacheGiB
	target.PrefillTokensPerSecond += value.PrefillTokensPerSecond
	target.DecodeTokensPerSecond += value.DecodeTokensPerSecond
	target.TotalTokensPerSecond += value.TotalTokensPerSecond
}

func subtractResources(total, used *astrav1alpha1.ResourceSummary) *astrav1alpha1.ResourceSummary {
	if total == nil {
		return &astrav1alpha1.ResourceSummary{}
	}
	if used == nil {
		copy := *total
		return &copy
	}
	return &astrav1alpha1.ResourceSummary{
		GPUCount:               nonNegative(total.GPUCount - used.GPUCount),
		GPUMemoryGiB:           nonNegative(total.GPUMemoryGiB - used.GPUMemoryGiB),
		KVCacheGiB:             nonNegative(total.KVCacheGiB - used.KVCacheGiB),
		PrefillTokensPerSecond: nonNegative(total.PrefillTokensPerSecond - used.PrefillTokensPerSecond),
		DecodeTokensPerSecond:  nonNegative(total.DecodeTokensPerSecond - used.DecodeTokensPerSecond),
		TotalTokensPerSecond:   nonNegative(total.TotalTokensPerSecond - used.TotalTokensPerSecond),
	}
}

func fakePressure(total, used *astrav1alpha1.ResourceSummary) *astrav1alpha1.RuntimePressure {
	return &astrav1alpha1.RuntimePressure{
		GPUUtilizationPercent:       percent(used.GPUCount, total.GPUCount),
		GPUMemoryUtilizationPercent: percent(used.GPUMemoryGiB, total.GPUMemoryGiB),
		KVCacheUtilizationPercent:   percent(used.KVCacheGiB, total.KVCacheGiB),
		PrefillUtilizationPercent:   percent(used.PrefillTokensPerSecond, total.PrefillTokensPerSecond),
		DecodeUtilizationPercent:    percent(used.DecodeTokensPerSecond, total.DecodeTokensPerSecond),
		SLORisk:                     "low",
	}
}

func percent(value, total int32) int32 {
	if total <= 0 {
		return 0
	}
	return nonNegative(value * 100 / total)
}

func nonNegative(value int32) int32 {
	if value < 0 {
		return 0
	}
	return value
}
