package agent

import (
	"context"
	"fmt"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	"github.com/linshanyu/astra-scheduler/internal/agent/backend"
	"github.com/linshanyu/astra-scheduler/internal/agent/local"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type statusRecorder struct {
	client client.Client
}

func newStatusRecorder(client client.Client) *statusRecorder {
	return &statusRecorder{client: client}
}

func (r *statusRecorder) markPendingApply(ctx context.Context, allocation *astrav1alpha1.AIResourceAllocation, plan local.Plan) error {
	now := metav1.Now()
	allocation.Status.Phase = string(astrav1alpha1.AllocationPhasePendingApply)
	allocation.Status.ObservedAt = &now
	allocation.Status.ObservedResources = plan.AssignedResources.DeepCopy()
	allocation.Status.Message = planStatusMessage(plan)
	return r.updateAllocationStatus(ctx, allocation, "mark allocation pending apply")
}

func (r *statusRecorder) markApplying(ctx context.Context, allocation *astrav1alpha1.AIResourceAllocation, plan local.Plan) error {
	now := metav1.Now()
	allocation.Status.Phase = string(astrav1alpha1.AllocationPhaseApplying)
	allocation.Status.ObservedAt = &now
	allocation.Status.Message = planStatusMessage(plan)
	return r.updateAllocationStatus(ctx, allocation, "mark allocation applying")
}

func (r *statusRecorder) markApplied(ctx context.Context, allocation *astrav1alpha1.AIResourceAllocation, result backend.ApplyResult) error {
	now := metav1.Now()
	allocation.Status.Phase = string(astrav1alpha1.AllocationPhaseApplied)
	allocation.Status.ObservedAt = &now
	allocation.Status.AppliedAt = &now
	allocation.Status.ObservedResources = result.ObservedResources
	allocation.Status.Message = result.Message
	return r.updateAllocationStatus(ctx, allocation, "mark allocation applied")
}

func (r *statusRecorder) markMigrating(ctx context.Context, allocation *astrav1alpha1.AIResourceAllocation, plan local.Plan) error {
	now := metav1.Now()
	allocation.Status.Phase = string(astrav1alpha1.AllocationPhaseMigrating)
	allocation.Status.ObservedAt = &now
	allocation.Status.ObservedResources = plan.AssignedResources.DeepCopy()
	allocation.Status.Message = planStatusMessage(plan)
	return r.updateAllocationStatus(ctx, allocation, "mark allocation migrating")
}

func (r *statusRecorder) markReleased(ctx context.Context, allocation *astrav1alpha1.AIResourceAllocation, message string) error {
	now := metav1.Now()
	allocation.Status.Phase = string(astrav1alpha1.AllocationPhaseReleased)
	allocation.Status.ObservedAt = &now
	allocation.Status.ObservedResources = (&astrav1alpha1.ResourceSummary{}).DeepCopy()
	allocation.Status.Message = message
	return r.updateAllocationStatus(ctx, allocation, "mark allocation released")
}

func (r *statusRecorder) markPlanFailed(ctx context.Context, allocation *astrav1alpha1.AIResourceAllocation, plan local.Plan) error {
	now := metav1.Now()
	allocation.Status.Phase = string(astrav1alpha1.AllocationPhaseFailed)
	allocation.Status.ObservedAt = &now
	allocation.Status.ObservedResources = plan.AssignedResources.DeepCopy()
	allocation.Status.Message = planStatusMessage(plan)
	return r.updateAllocationStatus(ctx, allocation, "mark allocation plan failed")
}

func (r *statusRecorder) markFailed(ctx context.Context, allocation *astrav1alpha1.AIResourceAllocation, plan local.Plan, err error) {
	now := metav1.Now()
	allocation.Status.Phase = string(astrav1alpha1.AllocationPhaseFailed)
	allocation.Status.ObservedAt = &now
	allocation.Status.ObservedResources = plan.AssignedResources.DeepCopy()
	allocation.Status.Message = err.Error()
	_ = r.client.Status().Update(ctx, allocation)
}

func (r *statusRecorder) updateAllocationStatus(ctx context.Context, allocation *astrav1alpha1.AIResourceAllocation, action string) error {
	if err := r.client.Status().Update(ctx, allocation); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("%s %s/%s: %w", action, allocation.Namespace, allocation.Name, err)
	}
	return nil
}
