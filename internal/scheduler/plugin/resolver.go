package plugin

import (
	"context"
	"fmt"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/kube-scheduler/framework"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (p *AstraScheduler) resolveProfile(ctx context.Context, pod *corev1.Pod) (*astrav1alpha1.AIWorkloadProfile, *framework.Status) {
	profileName := pod.Annotations[WorkloadProfileAnnotation]
	if profileName == "" {
		return nil, framework.NewStatus(
			framework.UnschedulableAndUnresolvable,
			fmt.Sprintf("missing annotation %s", WorkloadProfileAnnotation),
		)
	}

	profile := &astrav1alpha1.AIWorkloadProfile{}
	key := ctrlclient.ObjectKey{Namespace: pod.Namespace, Name: profileName}
	if err := p.client.Get(ctx, key, profile); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, framework.NewStatus(
				framework.UnschedulableAndUnresolvable,
				fmt.Sprintf("AIWorkloadProfile %s/%s not found", pod.Namespace, profileName),
			)
		}
		return nil, framework.NewStatus(framework.Error, fmt.Sprintf("get AIWorkloadProfile failed: %v", err))
	}

	return profile, nil
}

func (p *AstraScheduler) profileForPod(ctx context.Context, state framework.CycleState, pod *corev1.Pod) (*astrav1alpha1.AIWorkloadProfile, *framework.Status) {
	schedulingState, status := readSchedulingState(state)
	if status == nil {
		if profile := schedulingState.getProfile(); profile != nil {
			return profile, nil
		}
	}

	profile, status := p.resolveProfile(ctx, pod)
	if status != nil {
		return nil, status
	}
	if schedulingState != nil {
		schedulingState.setProfile(profile)
	}

	return profile, nil
}

func (p *AstraScheduler) resolveNodeResource(ctx context.Context, nodeName string) (*astrav1alpha1.AINodeResourceProfile, *framework.Status) {
	if nodeName == "" {
		return nil, framework.NewStatus(framework.Error, "candidate node name is empty")
	}

	nodeResource := &astrav1alpha1.AINodeResourceProfile{}
	key := ctrlclient.ObjectKey{Namespace: p.nodeResourceNamespace, Name: nodeName}
	if err := p.client.Get(ctx, key, nodeResource); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, framework.NewStatus(
				framework.Unschedulable,
				fmt.Sprintf("AINodeResourceProfile %s/%s not found", p.nodeResourceNamespace, nodeName),
			)
		}
		return nil, framework.NewStatus(framework.Error, fmt.Sprintf("get AINodeResourceProfile failed: %v", err))
	}

	return nodeResource, nil
}

func (p *AstraScheduler) nodeResourceForNode(ctx context.Context, state framework.CycleState, nodeName string) (*astrav1alpha1.AINodeResourceProfile, *framework.Status) {
	schedulingState, status := readSchedulingState(state)
	if status == nil {
		if nodeResource := schedulingState.getNodeResource(nodeName); nodeResource != nil {
			return nodeResource, nil
		}
	}

	nodeResource, status := p.resolveNodeResource(ctx, nodeName)
	if status != nil {
		return nil, status
	}
	if schedulingState != nil {
		schedulingState.setNodeResource(nodeName, nodeResource)
	}

	return nodeResource, nil
}
