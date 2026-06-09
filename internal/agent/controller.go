package agent

import (
	"context"
	"fmt"
	"time"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	"github.com/linshanyu/astra-scheduler/internal/agent/backend"
	"github.com/linshanyu/astra-scheduler/internal/agent/local"
	"github.com/linshanyu/astra-scheduler/internal/scheduler/plugin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	allocationPhasePending = "Pending"
	allocationPhaseApplied = "Applied"
	allocationPhaseFailed  = "Failed"

	nodePhaseReady = "Ready"
)

type Config struct {
	NodeName             string
	NodeProfileNamespace string
}

type Reconciler struct {
	client  client.Client
	config  Config
	backend backend.Backend
	local   *local.Scheduler
}

func NewReconciler(config Config, client client.Client, backend backend.Backend) *Reconciler {
	return &Reconciler{
		client:  client,
		config:  config,
		backend: backend,
		local:   local.NewDefaultScheduler(),
	}
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	nodePredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		if object == nil {
			return false
		}
		return object.GetLabels()[plugin.AllocationLabelNodeName] == r.config.NodeName
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&astrav1alpha1.AIResourceAllocation{}).
		WithEventFilter(nodePredicate).
		Complete(r)
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("astra-agent").WithValues(
		"node", r.config.NodeName,
		"allocation", req.NamespacedName.String(),
	)

	allocation := &astrav1alpha1.AIResourceAllocation{}
	if err := r.client.Get(ctx, req.NamespacedName, allocation); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if allocation.Spec.NodeName != r.config.NodeName {
		return ctrl.Result{}, nil
	}

	nodeProfile, err := r.getNodeProfile(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	allocations, err := r.listNodeAllocations(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	plans, err := r.local.BuildPlans(ctx, nodeProfile, allocations)
	if err != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}
	for _, plan := range plans {
		if err := r.applyPlan(ctx, nodeProfile, plan); err != nil {
			logger.Error(err, "Failed to apply local plan")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, err
		}
	}

	if err := r.refreshNodeStatus(ctx, nodeProfile); err != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) applyPlan(
	ctx context.Context,
	nodeProfile *astrav1alpha1.AINodeResourceProfile,
	plan local.Plan,
) error {
	allocation := plan.Allocation
	now := metav1.Now()

	result, err := r.backend.Apply(ctx, backend.ApplyRequest{
		Node:       nodeProfile,
		Allocation: &allocation,
		Action:     plan.Action,
	})
	if err != nil {
		allocation.Status.Phase = allocationPhaseFailed
		allocation.Status.ObservedAt = &now
		allocation.Status.Message = err.Error()
		_ = r.client.Status().Update(ctx, &allocation)
		return err
	}

	allocation.Status.Phase = allocationPhaseApplied
	allocation.Status.ObservedAt = &now
	allocation.Status.AppliedAt = &now
	allocation.Status.ObservedResources = result.ObservedResources
	allocation.Status.Message = result.Message
	if err := r.client.Status().Update(ctx, &allocation); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("update allocation status %s/%s: %w", allocation.Namespace, allocation.Name, err)
	}

	return nil
}

func (r *Reconciler) refreshNodeStatus(ctx context.Context, nodeProfile *astrav1alpha1.AINodeResourceProfile) error {
	allocations, err := r.listNodeAllocations(ctx)
	if err != nil {
		return err
	}

	applied := make([]astrav1alpha1.AIResourceAllocation, 0, len(allocations))
	for _, allocation := range allocations {
		if allocation.Status.Phase == allocationPhaseApplied {
			applied = append(applied, allocation)
		}
	}

	snapshot, err := r.backend.Snapshot(ctx, backend.SnapshotRequest{
		Node:        nodeProfile,
		Allocations: applied,
	})
	if err != nil {
		return err
	}

	now := metav1.Now()
	nodeProfile.Status = snapshot.Status
	nodeProfile.Status.Phase = nodePhaseReady
	nodeProfile.Status.ObservedAt = &now
	return r.client.Status().Update(ctx, nodeProfile)
}

func (r *Reconciler) getNodeProfile(ctx context.Context) (*astrav1alpha1.AINodeResourceProfile, error) {
	nodeProfile := &astrav1alpha1.AINodeResourceProfile{}
	key := client.ObjectKey{
		Namespace: r.config.NodeProfileNamespace,
		Name:      r.config.NodeName,
	}
	if err := r.client.Get(ctx, key, nodeProfile); err == nil {
		return nodeProfile, nil
	}

	list := &astrav1alpha1.AINodeResourceProfileList{}
	if err := r.client.List(ctx, list); err != nil {
		return nil, err
	}
	for index := range list.Items {
		item := &list.Items[index]
		if item.Spec.NodeName == r.config.NodeName || item.Name == r.config.NodeName {
			return item, nil
		}
	}

	return nil, fmt.Errorf("AINodeResourceProfile for node %q not found", r.config.NodeName)
}

func (r *Reconciler) listNodeAllocations(ctx context.Context) ([]astrav1alpha1.AIResourceAllocation, error) {
	list := &astrav1alpha1.AIResourceAllocationList{}
	if err := r.client.List(ctx, list, client.MatchingLabels{
		plugin.AllocationLabelManagedBy: "astra-scheduler",
		plugin.AllocationLabelNodeName:  r.config.NodeName,
	}); err != nil {
		return nil, err
	}

	allocations := make([]astrav1alpha1.AIResourceAllocation, 0, len(list.Items))
	for _, allocation := range list.Items {
		if allocation.Spec.NodeName != r.config.NodeName {
			continue
		}
		allocations = append(allocations, allocation)
	}

	return allocations, nil
}
