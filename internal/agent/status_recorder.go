package agent

import (
	"context"
	"fmt"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	"github.com/linshanyu/astra-scheduler/internal/agent/backend"
	"github.com/linshanyu/astra-scheduler/internal/agent/local"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
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
	allocation.Status.Current = plan.AssignedResources.DeepCopy()
	allocation.Status.HourlyForecast = buildHourlyForecast(allocation, &now)
	allocation.Status.Message = planStatusMessage(plan)
	setWorkloadStatusFromPlan(allocation, plan, plan.Action, allocation.Status.Message)
	return r.updateAllocationStatus(ctx, allocation, "mark allocation pending apply")
}

func (r *statusRecorder) markApplying(ctx context.Context, allocation *astrav1alpha1.AIResourceAllocation, plan local.Plan) error {
	now := metav1.Now()
	allocation.Status.Phase = string(astrav1alpha1.AllocationPhaseApplying)
	allocation.Status.ObservedAt = &now
	allocation.Status.Message = planStatusMessage(plan)
	setWorkloadStatusFromPlan(allocation, plan, plan.Action, allocation.Status.Message)
	return r.updateAllocationStatus(ctx, allocation, "mark allocation applying")
}

func (r *statusRecorder) markApplied(ctx context.Context, allocation *astrav1alpha1.AIResourceAllocation, plan local.Plan, result backend.ApplyResult) error {
	now := metav1.Now()
	allocation.Status.Phase = string(astrav1alpha1.AllocationPhaseApplied)
	allocation.Status.ObservedAt = &now
	allocation.Status.AppliedAt = &now
	allocation.Status.ObservedResources = result.ObservedResources
	allocation.Status.Current = result.ObservedResources
	allocation.Status.HourlyForecast = buildHourlyForecast(allocation, &now)
	allocation.Status.Message = result.Message
	setWorkloadStatus(
		allocation,
		plan.AssignedResources.DeepCopy(),
		allocation.Status.Current,
		plan.Action,
		allocation.Status.Message,
	)
	return r.updateAllocationStatus(ctx, allocation, "mark allocation applied")
}

func (r *statusRecorder) markMigrating(ctx context.Context, allocation *astrav1alpha1.AIResourceAllocation, plan local.Plan) error {
	now := metav1.Now()
	allocation.Status.Phase = string(astrav1alpha1.AllocationPhaseMigrating)
	allocation.Status.ObservedAt = &now
	allocation.Status.ObservedResources = plan.AssignedResources.DeepCopy()
	allocation.Status.Current = plan.AssignedResources.DeepCopy()
	allocation.Status.HourlyForecast = buildHourlyForecast(allocation, &now)
	allocation.Status.Message = planStatusMessage(plan)
	setWorkloadStatusFromPlan(allocation, plan, local.ActionEvict, allocation.Status.Message)
	return r.updateAllocationStatus(ctx, allocation, "mark allocation migrating")
}

func (r *statusRecorder) markReleased(ctx context.Context, allocation *astrav1alpha1.AIResourceAllocation, message string) error {
	now := metav1.Now()
	allocation.Status.Phase = string(astrav1alpha1.AllocationPhaseReleased)
	allocation.Status.ObservedAt = &now
	allocation.Status.ObservedResources = (&astrav1alpha1.ResourceSummary{}).DeepCopy()
	allocation.Status.Current = (&astrav1alpha1.ResourceSummary{}).DeepCopy()
	allocation.Status.HourlyForecast = emptyHourlyForecast(&now)
	allocation.Status.Message = message
	setWorkloadStatus(
		allocation,
		allocation.Status.Current,
		allocation.Status.Current,
		local.ActionEvict,
		message,
	)
	return r.updateAllocationStatus(ctx, allocation, "mark allocation released")
}

func (r *statusRecorder) markPlanFailed(ctx context.Context, allocation *astrav1alpha1.AIResourceAllocation, plan local.Plan) error {
	now := metav1.Now()
	allocation.Status.Phase = string(astrav1alpha1.AllocationPhaseFailed)
	allocation.Status.ObservedAt = &now
	allocation.Status.ObservedResources = plan.AssignedResources.DeepCopy()
	allocation.Status.Current = plan.AssignedResources.DeepCopy()
	allocation.Status.HourlyForecast = buildHourlyForecast(allocation, &now)
	allocation.Status.Message = planStatusMessage(plan)
	setWorkloadStatusFromPlan(allocation, plan, plan.Action, allocation.Status.Message)
	return r.updateAllocationStatus(ctx, allocation, "mark allocation plan failed")
}

func (r *statusRecorder) markFailed(ctx context.Context, allocation *astrav1alpha1.AIResourceAllocation, plan local.Plan, err error) {
	now := metav1.Now()
	allocation.Status.Phase = string(astrav1alpha1.AllocationPhaseFailed)
	allocation.Status.ObservedAt = &now
	allocation.Status.ObservedResources = plan.AssignedResources.DeepCopy()
	allocation.Status.Current = plan.AssignedResources.DeepCopy()
	allocation.Status.HourlyForecast = buildHourlyForecast(allocation, &now)
	allocation.Status.Message = err.Error()
	setWorkloadStatusFromPlan(allocation, plan, plan.Action, allocation.Status.Message)
	_ = r.updateAllocationStatus(ctx, allocation, "mark allocation failed")
}

func (r *statusRecorder) updateAllocationStatus(ctx context.Context, allocation *astrav1alpha1.AIResourceAllocation, action string) error {
	if len(allocation.Status.Workloads) == 0 {
		key := client.ObjectKeyFromObject(allocation)
		return retry.RetryOnConflict(retry.DefaultRetry, func() error {
			latest := &astrav1alpha1.AIResourceAllocation{}
			if err := r.client.Get(ctx, key, latest); err != nil {
				if apierrors.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("%s %s/%s: %w", action, allocation.Namespace, allocation.Name, err)
			}
			latest.Status = *allocation.Status.DeepCopy()
			if err := r.client.Status().Update(ctx, latest); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("%s %s/%s: %w", action, allocation.Namespace, allocation.Name, err)
			}
			return nil
		})
	}

	key := client.ObjectKeyFromObject(allocation)
	entry := allocation.Status.Workloads[0]
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &astrav1alpha1.AIResourceAllocation{}
		if err := r.client.Get(ctx, key, latest); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("%s %s/%s: %w", action, allocation.Namespace, allocation.Name, err)
		}

		latest.Status.Phase = allocation.Status.Phase
		latest.Status.ObservedAt = allocation.Status.ObservedAt
		latest.Status.AppliedAt = allocation.Status.AppliedAt
		latest.Status.Message = allocation.Status.Message
		latest.Status.Workloads = mergeWorkloadStatus(latest.Status.Workloads, entry)
		latest.Status.NodeLedger = allocationLedgerSummary(latest.Status.Workloads)
		latest.Status.ObservedResources = latest.Status.NodeLedger.Allocated
		latest.Status.Current = latest.Status.NodeLedger.Allocated
		latest.Status.HourlyForecast = aggregateHourlyForecastFromStatuses(latest.Status.Workloads)

		if err := r.client.Status().Update(ctx, latest); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("%s %s/%s: %w", action, allocation.Namespace, allocation.Name, err)
		}
		return nil
	})
}

func setWorkloadStatusFromPlan(
	allocation *astrav1alpha1.AIResourceAllocation,
	plan local.Plan,
	action string,
	message string,
) {
	setWorkloadStatus(
		allocation,
		plan.AssignedResources.DeepCopy(),
		plan.AssignedResources.DeepCopy(),
		action,
		message,
	)
}

func setWorkloadStatus(
	allocation *astrav1alpha1.AIResourceAllocation,
	desired *astrav1alpha1.ResourceSummary,
	applied *astrav1alpha1.ResourceSummary,
	action string,
	message string,
) {
	entry := workloadStatusFromAllocation(allocation, desired, applied, action, message)
	allocation.Status.Workloads = mergeWorkloadStatus(allocation.Status.Workloads, entry)
	allocation.Status.NodeLedger = allocationLedgerSummary(allocation.Status.Workloads)
}
