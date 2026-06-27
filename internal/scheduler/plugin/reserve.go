package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	"github.com/linshanyu/astra-scheduler/internal/scheduler/policy"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/kube-scheduler/framework"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	AllocationLabelManagedBy         = "astra.aiinfra.io/managed-by"
	AllocationLabelWorkloadNamespace = "astra.aiinfra.io/workload-namespace"
	AllocationLabelWorkloadName      = "astra.aiinfra.io/workload-name"
	AllocationLabelNodeName          = "astra.aiinfra.io/node-name"
	AllocationLabelPriority          = "astra.aiinfra.io/priority"

	AllocationAnnotationSourcePodName = "astra.aiinfra.io/source-pod-name"
	AllocationAnnotationSourcePodUID  = "astra.aiinfra.io/source-pod-uid"
)

func (p *AstraScheduler) Reserve(ctx context.Context, state framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	if pod.Spec.SchedulerName != SchedulerName {
		return nil
	}
	klog.InfoS("AstraScheduler Reserve", "pod", klog.KObj(pod), "node", nodeName)

	if nodeName == "" {
		return framework.NewStatus(framework.Error, "reserve received empty nodeName")
	}

	profile, status := p.profileForPod(ctx, state, pod)
	if status != nil {
		return status
	}
	nodeResource, status := p.nodeResourceForNode(ctx, state, nodeName)
	if status != nil {
		return status
	}

	resources := p.policy.AllocationResources(profile, nodeResource)
	decision := policyDecisionForNode(state, nodeName)
	allocationName := buildAllocationName(nodeName)
	key := ctrlclient.ObjectKey{Namespace: p.nodeResourceNamespace, Name: allocationName}

	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		allocation := &astrav1alpha1.AIResourceAllocation{}
		if err := p.client.Get(ctx, key, allocation); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			allocation = &astrav1alpha1.AIResourceAllocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      allocationName,
					Namespace: p.nodeResourceNamespace,
				},
			}
			fillAllocation(allocation, pod, profile, nodeName, resources, p.policy.AllocationType(profile, resources), decision.Reason)
			if err := p.client.Create(ctx, allocation); err != nil {
				if apierrors.IsAlreadyExists(err) {
					return apierrors.NewConflict(allocationGroupResource(), allocationName, err)
				}
				return err
			}
			klog.InfoS("AstraScheduler created allocation ledger", "allocation", klog.KObj(allocation), "node", nodeName, "pod", klog.KObj(pod))
			return nil
		}

		fillAllocation(allocation, pod, profile, nodeName, resources, p.policy.AllocationType(profile, resources), decision.Reason)
		if err := p.client.Update(ctx, allocation); err != nil {
			return err
		}
		klog.InfoS("AstraScheduler updated allocation ledger", "allocation", klog.KObj(allocation), "node", nodeName, "pod", klog.KObj(pod))
		return nil
	}); err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("reserve AIResourceAllocation failed: %v", err))
	}

	return nil
}

func (p *AstraScheduler) Unreserve(ctx context.Context, _ framework.CycleState, pod *corev1.Pod, nodeName string) {
	if pod.Spec.SchedulerName != SchedulerName {
		return
	}

	key := ctrlclient.ObjectKey{Namespace: p.nodeResourceNamespace, Name: buildAllocationName(nodeName)}
	entryName := workloadEntryName(pod, nil)

	_ = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		allocation := &astrav1alpha1.AIResourceAllocation{}
		if err := p.client.Get(ctx, key, allocation); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}

		allocation.Spec.Workloads = removeAllocationWorkload(allocation.Spec.Workloads, entryName)
		if len(allocation.Spec.Workloads) == 0 {
			err := p.client.Delete(ctx, allocation)
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		return p.client.Update(ctx, allocation)
	})
}

func fillAllocation(
	allocation *astrav1alpha1.AIResourceAllocation,
	pod *corev1.Pod,
	profile *astrav1alpha1.AIWorkloadProfile,
	nodeName string,
	resources astrav1alpha1.ResourceSummary,
	allocationType string,
	reason string,
) {
	allocation.Labels = mergeLabels(allocation.Labels, map[string]string{
		AllocationLabelManagedBy:         "astra-scheduler",
		AllocationLabelWorkloadNamespace: labelValue(pod.Namespace),
		AllocationLabelWorkloadName:      labelValue(pod.Name),
		AllocationLabelNodeName:          labelValue(nodeName),
		AllocationLabelPriority:          labelValue(profile.Spec.Priority),
	})
	allocation.Annotations = mergeLabels(allocation.Annotations, map[string]string{
		AllocationAnnotationSourcePodName: pod.Name,
		AllocationAnnotationSourcePodUID:  string(pod.UID),
	})

	entry := allocationWorkloadSpec(pod, profile, resources, allocationType, reason)
	allocation.Spec.NodeName = nodeName
	allocation.Spec.Reason = "Node-level allocation ledger maintained by AstraScheduler."
	allocation.Spec.Workloads = upsertAllocationWorkload(allocation.Spec.Workloads, entry)

	// Keep legacy single-workload fields populated with the latest reserved
	// workload until the agent path is fully migrated to spec.workloads[].
	allocation.Spec.WorkloadRef = entry.WorkloadRef
	allocation.Spec.WorkloadType = entry.WorkloadType
	allocation.Spec.Priority = entry.Priority
	allocation.Spec.DemandShape = entry.DemandShape
	allocation.Spec.TimeWindows = entry.TimeWindows
	allocation.Spec.ResourceRequest = entry.ResourceRequest
	allocation.Spec.NodeName = nodeName
	allocation.Spec.Resources = entry.Resources
	allocation.Spec.AllocationType = entry.AllocationType
	allocation.Spec.Actions = entry.Actions
	allocation.Spec.Reason = reason
}

func allocationWorkloadSpec(
	pod *corev1.Pod,
	profile *astrav1alpha1.AIWorkloadProfile,
	resources astrav1alpha1.ResourceSummary,
	allocationType string,
	reason string,
) astrav1alpha1.AllocationWorkloadSpec {
	return astrav1alpha1.AllocationWorkloadSpec{
		Name: workloadEntryName(pod, profile),
		WorkloadRef: astrav1alpha1.KubernetesObjectRef{
			APIVersion: astrav1alpha1.GroupVersion.String(),
			Kind:       "AIWorkloadProfile",
			Name:       profile.Name,
			Namespace:  profile.Namespace,
		},
		SourcePodName:   pod.Name,
		SourcePodUID:    string(pod.UID),
		WorkloadType:    profile.Spec.WorkloadType,
		Priority:        profile.Spec.Priority,
		DemandShape:     profile.Spec.DemandShape,
		TimeWindows:     profile.Spec.TimeWindows,
		ResourceRequest: profile.Spec.Resources,
		Resources:       &resources,
		Desired:         &resources,
		AllocationType:  allocationType,
		Actions:         allocationActions(profile),
		Reason:          reason,
	}
}

func upsertAllocationWorkload(
	workloads []astrav1alpha1.AllocationWorkloadSpec,
	entry astrav1alpha1.AllocationWorkloadSpec,
) []astrav1alpha1.AllocationWorkloadSpec {
	for index := range workloads {
		if workloads[index].Name == entry.Name {
			workloads[index] = entry
			return workloads
		}
	}
	return append(workloads, entry)
}

func removeAllocationWorkload(
	workloads []astrav1alpha1.AllocationWorkloadSpec,
	name string,
) []astrav1alpha1.AllocationWorkloadSpec {
	result := workloads[:0]
	for _, workload := range workloads {
		if workload.Name != name {
			result = append(result, workload)
		}
	}
	return result
}

func workloadEntryName(pod *corev1.Pod, profile *astrav1alpha1.AIWorkloadProfile) string {
	namespace := pod.Namespace
	name := pod.Name
	if namespace == "" && profile != nil {
		namespace = profile.Namespace
	}
	if name == "" && profile != nil {
		name = profile.Name
	}
	return sanitizeName(namespace + "/" + name)
}

func policyDecisionForNode(state framework.CycleState, nodeName string) policy.ScoreDecision {
	schedulingState, status := readSchedulingState(state)
	if status != nil {
		return policy.ScoreDecision{Reason: "scheduled by AstraScheduler"}
	}
	decision, ok := schedulingState.getScoreDecision(nodeName)
	if !ok || decision.Reason == "" {
		return policy.ScoreDecision{Reason: "scheduled by AstraScheduler"}
	}
	return decision
}

func allocationActions(profile *astrav1alpha1.AIWorkloadProfile) *astrav1alpha1.AllocationActions {
	if profile == nil || profile.Spec.Policy == nil {
		return &astrav1alpha1.AllocationActions{}
	}
	return &astrav1alpha1.AllocationActions{
		Reclaimable:  profile.Spec.Policy.AllowBorrowing,
		Throttleable: profile.Spec.Policy.Throttleable,
		Pauseable:    profile.Spec.Policy.Pauseable,
		Evictable:    profile.Spec.Policy.Evictable,
	}
}

func buildAllocationName(nodeName string) string {
	hash := sha256.Sum256([]byte(nodeName))
	suffix := hex.EncodeToString(hash[:])[:8]
	base := sanitizeName(fmt.Sprintf("%s-allocation", nodeName))
	if len(base) <= 253 {
		return base
	}
	return strings.Trim(base[:244], "-.") + "-" + suffix
}

func allocationGroupResource() schema.GroupResource {
	return schema.GroupResource{
		Group:    astrav1alpha1.GroupVersion.Group,
		Resource: "airesourceallocations",
	}
}

func sanitizeName(value string) string {
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

func labelValue(value string) string {
	value = sanitizeName(value)
	if len(value) <= 63 {
		return value
	}
	return strings.Trim(value[:63], "-.")
}

func mergeLabels(existing map[string]string, labels map[string]string) map[string]string {
	if existing == nil {
		existing = map[string]string{}
	}
	for key, value := range labels {
		existing[key] = value
	}
	return existing
}
