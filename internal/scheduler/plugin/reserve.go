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
	"k8s.io/kube-scheduler/framework"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	AllocationLabelManagedBy         = "astra.aiinfra.io/managed-by"
	AllocationLabelWorkloadNamespace = "astra.aiinfra.io/workload-namespace"
	AllocationLabelWorkloadName      = "astra.aiinfra.io/workload-name"
	AllocationLabelNodeName          = "astra.aiinfra.io/node-name"
	AllocationLabelPriority          = "astra.aiinfra.io/priority"
)

func (p *AstraScheduler) Reserve(ctx context.Context, state framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	if pod.Spec.SchedulerName != SchedulerName {
		return nil
	}

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
	allocationName := buildAllocationName(pod.Name, nodeName)
	allocation := &astrav1alpha1.AIResourceAllocation{}
	key := ctrlclient.ObjectKey{Namespace: pod.Namespace, Name: allocationName}

	if err := p.client.Get(ctx, key, allocation); err != nil {
		if !apierrors.IsNotFound(err) {
			return framework.NewStatus(framework.Error, fmt.Sprintf("get AIResourceAllocation failed: %v", err))
		}
		allocation = &astrav1alpha1.AIResourceAllocation{
			ObjectMeta: metav1.ObjectMeta{
				Name:      allocationName,
				Namespace: pod.Namespace,
			},
		}
		fillAllocation(allocation, pod, profile, nodeName, resources, p.policy.AllocationType(profile, resources), decision.Reason)
		if err := p.client.Create(ctx, allocation); err != nil {
			return framework.NewStatus(framework.Error, fmt.Sprintf("create AIResourceAllocation failed: %v", err))
		}
		return nil
	}

	fillAllocation(allocation, pod, profile, nodeName, resources, p.policy.AllocationType(profile, resources), decision.Reason)
	if err := p.client.Update(ctx, allocation); err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("update AIResourceAllocation failed: %v", err))
	}

	return nil
}

func (p *AstraScheduler) Unreserve(ctx context.Context, _ framework.CycleState, pod *corev1.Pod, nodeName string) {
	if pod.Spec.SchedulerName != SchedulerName {
		return
	}

	allocation := &astrav1alpha1.AIResourceAllocation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildAllocationName(pod.Name, nodeName),
			Namespace: pod.Namespace,
		},
	}
	if err := p.client.Delete(ctx, allocation); err != nil && !apierrors.IsNotFound(err) {
		return
	}
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

	allocation.Spec = astrav1alpha1.AIResourceAllocationSpec{
		WorkloadRef: astrav1alpha1.KubernetesObjectRef{
			APIVersion: astrav1alpha1.GroupVersion.String(),
			Kind:       "AIWorkloadProfile",
			Name:       profile.Name,
			Namespace:  profile.Namespace,
		},
		WorkloadType:    profile.Spec.WorkloadType,
		Priority:        profile.Spec.Priority,
		DemandShape:     profile.Spec.DemandShape,
		TimeWindows:     profile.Spec.TimeWindows,
		ResourceRequest: profile.Spec.Resources,
		NodeName:        nodeName,
		Resources:       &resources,
		AllocationType:  allocationType,
		Actions:         allocationActions(profile),
		Reason:          reason,
	}
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

func buildAllocationName(podName, nodeName string) string {
	hash := sha256.Sum256([]byte(nodeName))
	suffix := hex.EncodeToString(hash[:])[:8]
	base := sanitizeName(fmt.Sprintf("%s-%s", podName, suffix))
	if len(base) <= 253 {
		return base
	}
	return strings.Trim(base[:244], "-.") + "-" + suffix
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
