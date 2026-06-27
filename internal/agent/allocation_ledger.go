package agent

import (
	"strings"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	"github.com/linshanyu/astra-scheduler/internal/scheduler/plugin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func expandNodeAllocationLedgers(ledgers []astrav1alpha1.AIResourceAllocation) []astrav1alpha1.AIResourceAllocation {
	expanded := make([]astrav1alpha1.AIResourceAllocation, 0, len(ledgers))
	for _, ledger := range ledgers {
		if len(ledger.Spec.Workloads) == 0 {
			expanded = append(expanded, ledger)
			continue
		}
		for _, workload := range ledger.Spec.Workloads {
			allocation := ledger.DeepCopy()
			allocation.Spec.Workloads = []astrav1alpha1.AllocationWorkloadSpec{workload}
			allocation.Spec.WorkloadRef = workload.WorkloadRef
			allocation.Spec.WorkloadType = workload.WorkloadType
			allocation.Spec.Priority = workload.Priority
			allocation.Spec.DemandShape = workload.DemandShape
			allocation.Spec.TimeWindows = workload.TimeWindows
			allocation.Spec.ResourceRequest = workload.ResourceRequest
			allocation.Spec.AssignedGPU = workload.AssignedGPU
			allocation.Spec.Resources = firstResourceSummary(workload.Desired, workload.Resources)
			allocation.Spec.AllocationType = workload.AllocationType
			allocation.Spec.Actions = workload.Actions
			allocation.Spec.Reason = workload.Reason
			if allocation.Annotations == nil {
				allocation.Annotations = map[string]string{}
			}
			if workload.SourcePodName != "" {
				allocation.Annotations[plugin.AllocationAnnotationSourcePodName] = workload.SourcePodName
			}
			if workload.SourcePodUID != "" {
				allocation.Annotations[plugin.AllocationAnnotationSourcePodUID] = workload.SourcePodUID
			}
			if status, ok := workloadStatusByName(ledger.Status.Workloads, workload.Name); ok {
				allocation.Status.Phase = status.Phase
				allocation.Status.ObservedResources = status.Applied
				allocation.Status.Current = status.Applied
				allocation.Status.HourlyForecast = status.HourlyForecast
				allocation.Status.Message = status.Message
			} else {
				allocation.Status = astrav1alpha1.AIResourceAllocationStatus{}
			}
			expanded = append(expanded, *allocation)
		}
	}
	return expanded
}

func firstResourceSummary(values ...*astrav1alpha1.ResourceSummary) *astrav1alpha1.ResourceSummary {
	for _, value := range values {
		if value != nil {
			return value.DeepCopy()
		}
	}
	return nil
}

func workloadEntryNameFromAllocation(allocation *astrav1alpha1.AIResourceAllocation) string {
	if allocation == nil {
		return ""
	}
	if len(allocation.Spec.Workloads) == 1 && allocation.Spec.Workloads[0].Name != "" {
		return allocation.Spec.Workloads[0].Name
	}
	namespace := allocation.Spec.WorkloadRef.Namespace
	if namespace == "" {
		namespace = allocation.Namespace
	}
	name := allocation.Annotations[plugin.AllocationAnnotationSourcePodName]
	if name == "" {
		name = allocation.Labels[plugin.AllocationLabelWorkloadName]
	}
	if name == "" {
		name = allocation.Spec.WorkloadRef.Name
	}
	return sanitizeLedgerKey(namespace + "/" + name)
}

func workloadStatusFromAllocation(
	allocation *astrav1alpha1.AIResourceAllocation,
	desired *astrav1alpha1.ResourceSummary,
	applied *astrav1alpha1.ResourceSummary,
	action string,
	message string,
) astrav1alpha1.AllocationWorkloadStatus {
	status := astrav1alpha1.AllocationWorkloadStatus{
		Name:           workloadEntryNameFromAllocation(allocation),
		WorkloadRef:    objectRefCopy(allocation.Spec.WorkloadRef),
		SourcePodName:  allocation.Annotations[plugin.AllocationAnnotationSourcePodName],
		Phase:          allocation.Status.Phase,
		Priority:       allocation.Spec.Priority,
		DemandShape:    allocation.Spec.DemandShape,
		Desired:        desired,
		Applied:        applied,
		HourlyForecast: allocation.Status.HourlyForecast,
		AssignedGPU:    allocation.Spec.AssignedGPU,
		Action:         action,
		Message:        message,
	}
	if status.WorkloadRef != nil && status.WorkloadRef.Name == "" {
		status.WorkloadRef = nil
	}
	return status
}

func mergeWorkloadStatus(
	statuses []astrav1alpha1.AllocationWorkloadStatus,
	entry astrav1alpha1.AllocationWorkloadStatus,
) []astrav1alpha1.AllocationWorkloadStatus {
	for index := range statuses {
		if statuses[index].Name == entry.Name {
			statuses[index] = entry
			return statuses
		}
	}
	return append(statuses, entry)
}

func workloadStatusByName(
	statuses []astrav1alpha1.AllocationWorkloadStatus,
	name string,
) (astrav1alpha1.AllocationWorkloadStatus, bool) {
	for _, status := range statuses {
		if status.Name == name {
			return status, true
		}
	}
	return astrav1alpha1.AllocationWorkloadStatus{}, false
}

func allocationLedgerSummary(statuses []astrav1alpha1.AllocationWorkloadStatus) *astrav1alpha1.NodeAllocationLedger {
	allocated := &astrav1alpha1.ResourceSummary{}
	for _, status := range statuses {
		if status.Applied == nil || strings.EqualFold(status.Phase, string(astrav1alpha1.AllocationPhaseReleased)) {
			continue
		}
		allocated.GPUCount += status.Applied.GPUCount
		allocated.GPUMemoryGiB += status.Applied.GPUMemoryGiB
		allocated.KVCacheGiB += status.Applied.KVCacheGiB
		allocated.PrefillTokensPerSecond += status.Applied.PrefillTokensPerSecond
		allocated.DecodeTokensPerSecond += status.Applied.DecodeTokensPerSecond
		allocated.TotalTokensPerSecond += status.Applied.TotalTokensPerSecond
	}
	return &astrav1alpha1.NodeAllocationLedger{
		Allocated: allocated,
	}
}

func aggregateHourlyForecastFromStatuses(
	statuses []astrav1alpha1.AllocationWorkloadStatus,
) *astrav1alpha1.HourlyResourceForecast {
	buckets := make([]astrav1alpha1.ResourceSummary, 24)
	var generatedAt *metav1.Time
	timezone := ""

	for _, status := range statuses {
		if strings.EqualFold(status.Phase, string(astrav1alpha1.AllocationPhaseReleased)) {
			continue
		}
		if status.HourlyForecast == nil {
			continue
		}
		if generatedAt == nil {
			generatedAt = status.HourlyForecast.GeneratedAt
		}
		if timezone == "" {
			timezone = status.HourlyForecast.Timezone
		}
		for hour := 0; hour < 24 && hour < len(status.HourlyForecast.ResourceBuckets); hour++ {
			buckets[hour] = addResourceSummary(buckets[hour], status.HourlyForecast.ResourceBuckets[hour])
		}
	}

	return &astrav1alpha1.HourlyResourceForecast{
		GeneratedAt:     generatedAt,
		Timezone:        timezone,
		ResourceBuckets: buckets,
	}
}

func addResourceSummary(left, right astrav1alpha1.ResourceSummary) astrav1alpha1.ResourceSummary {
	return astrav1alpha1.ResourceSummary{
		GPUCount:               left.GPUCount + right.GPUCount,
		GPUMemoryGiB:           left.GPUMemoryGiB + right.GPUMemoryGiB,
		KVCacheGiB:             left.KVCacheGiB + right.KVCacheGiB,
		PrefillTokensPerSecond: left.PrefillTokensPerSecond + right.PrefillTokensPerSecond,
		DecodeTokensPerSecond:  left.DecodeTokensPerSecond + right.DecodeTokensPerSecond,
		TotalTokensPerSecond:   left.TotalTokensPerSecond + right.TotalTokensPerSecond,
	}
}

func sanitizeLedgerKey(value string) string {
	value = strings.ToLower(value)
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' || char == '.' {
			builder.WriteRune(char)
			continue
		}
		builder.WriteRune('-')
	}
	return strings.Trim(builder.String(), "-.")
}

func objectRefCopy(ref astrav1alpha1.KubernetesObjectRef) *astrav1alpha1.KubernetesObjectRef {
	if ref.Name == "" {
		return nil
	}
	copy := ref
	return &copy
}
