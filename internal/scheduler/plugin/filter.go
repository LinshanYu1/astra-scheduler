package plugin

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kube-scheduler/framework"
)

const (
	SchedulerName             = "astra-scheduler"
	WorkloadProfileAnnotation = "astra.aiinfra.io/workload-profile"
)

func (p *AstraScheduler) PreFilter(ctx context.Context, state framework.CycleState, pod *corev1.Pod, _ []framework.NodeInfo) (*framework.PreFilterResult, *framework.Status) {
	if pod.Spec.SchedulerName != SchedulerName {
		return nil, framework.NewStatus(framework.Skip)
	}

	profile, status := p.resolveProfile(ctx, pod)
	if status != nil {
		return nil, status
	}

	state.Write(schedulingStateKey, newSchedulingState(profile))
	return nil, nil
}

func (p *AstraScheduler) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

func (p *AstraScheduler) Filter(ctx context.Context, state framework.CycleState, pod *corev1.Pod, nodeInfo framework.NodeInfo) *framework.Status {
	if pod.Spec.SchedulerName != SchedulerName {
		return nil
	}

	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "nodeInfo has nil node")
	}

	profile, status := p.profileForPod(ctx, state, pod)
	if status != nil {
		return status
	}

	nodeResource, status := p.nodeResourceForNode(ctx, state, node.Name)
	if status != nil {
		return status
	}

	decision := p.policy.Filter(profile, nodeResource)
	klog.V(4).InfoS("AstraScheduler Filter", "pod", klog.KObj(pod), "node", node.Name, "allowed", decision.Allowed, "reason", decision.Reason)
	if !decision.Allowed {
		return framework.NewStatus(framework.Unschedulable, decision.Reason)
	}

	return nil
}
