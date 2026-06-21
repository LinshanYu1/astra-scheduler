package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	"github.com/linshanyu/astra-scheduler/internal/agent/backend"
	"github.com/linshanyu/astra-scheduler/internal/agent/local"
	"github.com/linshanyu/astra-scheduler/internal/scheduler/plugin"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	nodePhaseReady = "Ready"

	defaultRequeueAfter = time.Minute
)

type Config struct {
	NodeName                string
	NodeProfileNamespace    string
	FakeResourceConfigMap   string
	FakeResourceConfigMapNS string
}

type Reconciler struct {
	client  client.Client
	config  Config
	backend backend.Backend
	local   *local.Scheduler
	status  *statusRecorder
}

func NewReconciler(config Config, client client.Client, backend backend.Backend) *Reconciler {
	return &Reconciler{
		client:  client,
		config:  config,
		backend: backend,
		local:   local.NewDefaultScheduler(),
		status:  newStatusRecorder(client),
	}
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	nodePredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		if object == nil {
			return false
		}
		return object.GetLabels()[plugin.AllocationLabelNodeName] == r.config.NodeName
	})
	nodeProfilePredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		profile, ok := object.(*astrav1alpha1.AINodeResourceProfile)
		if !ok || profile == nil {
			return false
		}
		return profile.Spec.NodeName == r.config.NodeName || profile.Name == r.config.NodeName
	})
	fakeConfigMapPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		if object == nil || r.config.FakeResourceConfigMap == "" {
			return false
		}
		return object.GetName() == r.config.FakeResourceConfigMap &&
			(r.config.FakeResourceConfigMapNS == "" || object.GetNamespace() == r.config.FakeResourceConfigMapNS)
	})
	nodeRequest := handler.EnqueueRequestsFromMapFunc(func(_ context.Context, _ client.Object) []reconcile.Request {
		return []reconcile.Request{{NamespacedName: client.ObjectKey{
			Namespace: r.config.NodeProfileNamespace,
			Name:      r.config.NodeName,
		}}}
	})

	allocationChangePredicate := predicate.And(nodePredicate, predicate.GenerationChangedPredicate{})
	nodeProfileChangePredicate := predicate.And(nodeProfilePredicate, predicate.GenerationChangedPredicate{})

	return ctrl.NewControllerManagedBy(mgr).
		For(&astrav1alpha1.AIResourceAllocation{}, builder.WithPredicates(allocationChangePredicate)).
		Watches(&astrav1alpha1.AINodeResourceProfile{}, nodeRequest, builder.WithPredicates(nodeProfileChangePredicate)).
		Watches(&corev1.ConfigMap{}, nodeRequest, builder.WithPredicates(fakeConfigMapPredicate)).
		Complete(r)
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	allocation := &astrav1alpha1.AIResourceAllocation{}
	if err := r.client.Get(ctx, req.NamespacedName, allocation); err != nil {
		if apierrors.IsNotFound(err) {
			return r.reconcileNode(ctx)
		}
		return ctrl.Result{}, err
	}
	if allocation.Spec.NodeName != r.config.NodeName {
		return ctrl.Result{}, nil
	}

	return r.reconcileNode(ctx)
}

func (r *Reconciler) reconcileNode(ctx context.Context) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("astra-agent").WithValues("node", r.config.NodeName)

	nodeProfile, err := r.getNodeProfile(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	allocations, err := r.listNodeAllocations(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.syncNodeStatus(ctx, nodeProfile, allocations); err != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
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

	return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
}

func (r *Reconciler) applyPlan(
	ctx context.Context,
	nodeProfile *astrav1alpha1.AINodeResourceProfile,
	plan local.Plan,
) error {
	allocation := plan.Allocation
	if allocation.Status.Phase == string(astrav1alpha1.AllocationPhaseApplied) &&
		allocation.Status.ObservedResources != nil &&
		apiequality.Semantic.DeepEqual(*allocation.Status.ObservedResources, plan.AssignedResources) {
		return nil
	}
	if plan.TargetPhase == local.PlanPhaseFailed {
		if plan.Action == local.ActionEvict {
			return r.handleMigrationPlan(ctx, &allocation, plan)
		}
		return r.status.markPlanFailed(ctx, &allocation, plan)
	}
	if err := r.status.markPendingApply(ctx, &allocation, plan); err != nil {
		return err
	}
	if err := r.status.markApplying(ctx, &allocation, plan); err != nil {
		return err
	}

	result, err := r.backend.Apply(ctx, backend.ApplyRequest{
		Node:             nodeProfile,
		Allocation:       &allocation,
		Action:           plan.Action,
		DesiredResources: plan.AssignedResources.DeepCopy(),
		AllocationTier:   plan.AllocationTier,
		TargetPhase:      plan.TargetPhase,
		Reason:           plan.Reason,
	})
	if err != nil {
		r.status.markFailed(ctx, &allocation, plan, err)
		return err
	}

	return r.status.markApplied(ctx, &allocation, result)
}

func (r *Reconciler) handleMigrationPlan(
	ctx context.Context,
	allocation *astrav1alpha1.AIResourceAllocation,
	plan local.Plan,
) error {
	if allocation.Status.Phase != string(astrav1alpha1.AllocationPhaseMigrating) {
		if err := r.status.markMigrating(ctx, allocation, plan); err != nil {
			return err
		}
	}

	sourcePod, err := r.sourcePodForAllocation(ctx, allocation)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.status.markReleased(ctx, allocation, "source pod is already gone; allocation released")
		}
		return err
	}

	replacement, err := r.ensureReplacementPod(ctx, allocation, sourcePod)
	if err != nil {
		return err
	}
	if replacement.Status.Phase != corev1.PodRunning {
		return nil
	}

	if sourcePod.DeletionTimestamp == nil {
		if err := r.client.Delete(ctx, sourcePod); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	return r.status.markReleased(
		ctx,
		allocation,
		fmt.Sprintf("replacement pod %s/%s is running; source pod released", replacement.Namespace, replacement.Name),
	)
}

func (r *Reconciler) sourcePodForAllocation(ctx context.Context, allocation *astrav1alpha1.AIResourceAllocation) (*corev1.Pod, error) {
	podName := allocation.Annotations[plugin.AllocationAnnotationSourcePodName]
	if podName == "" {
		podName = allocation.Labels[plugin.AllocationLabelWorkloadName]
	}
	if podName == "" {
		return nil, fmt.Errorf("allocation %s/%s has no source pod reference", allocation.Namespace, allocation.Name)
	}

	pod := &corev1.Pod{}
	if err := r.client.Get(ctx, client.ObjectKey{Namespace: allocation.Namespace, Name: podName}, pod); err != nil {
		return nil, err
	}
	return pod, nil
}

func (r *Reconciler) ensureReplacementPod(
	ctx context.Context,
	allocation *astrav1alpha1.AIResourceAllocation,
	sourcePod *corev1.Pod,
) (*corev1.Pod, error) {
	name := replacementPodName(sourcePod.Name, allocation.Name)
	key := client.ObjectKey{Namespace: sourcePod.Namespace, Name: name}
	replacement := &corev1.Pod{}
	if err := r.client.Get(ctx, key, replacement); err == nil {
		return replacement, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, err
	}

	replacement = sourcePod.DeepCopy()
	replacement.ObjectMeta = metav1.ObjectMeta{
		Name:        name,
		Namespace:   sourcePod.Namespace,
		Labels:      copyStringMap(sourcePod.Labels),
		Annotations: copyStringMap(sourcePod.Annotations),
	}
	if replacement.Labels == nil {
		replacement.Labels = map[string]string{}
	}
	replacement.Labels["astra.aiinfra.io/replacement-for-allocation"] = allocation.Name
	replacement.Labels["astra.aiinfra.io/replacement-for-pod"] = sourcePod.Name
	if replacement.Annotations == nil {
		replacement.Annotations = map[string]string{}
	}
	replacement.Annotations["astra.aiinfra.io/replacement-reason"] = "node-local-migration"
	replacement.Spec.NodeName = ""
	replacement.Spec.SchedulerName = plugin.SchedulerName

	if err := r.client.Create(ctx, replacement); err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, err
	}
	if err := r.client.Get(ctx, key, replacement); err != nil {
		return nil, err
	}
	return replacement, nil
}

func (r *Reconciler) syncNodeStatus(
	ctx context.Context,
	nodeProfile *astrav1alpha1.AINodeResourceProfile,
	allocations []astrav1alpha1.AIResourceAllocation,
) error {
	snapshot, err := r.backend.Snapshot(ctx, backend.SnapshotRequest{
		Node:        nodeProfile,
		Allocations: allocations,
	})
	if err != nil {
		return err
	}

	now := metav1.Now()
	status := snapshot.Status
	status.Phase = nodePhaseReady
	status.ObservedAt = &now
	return r.updateNodeProfileStatus(ctx, nodeProfile, status)
}

func (r *Reconciler) refreshNodeStatus(ctx context.Context, nodeProfile *astrav1alpha1.AINodeResourceProfile) error {
	allocations, err := r.listNodeAllocations(ctx)
	if err != nil {
		return err
	}

	applied := make([]astrav1alpha1.AIResourceAllocation, 0, len(allocations))
	for _, allocation := range allocations {
		if allocation.Status.Phase == string(astrav1alpha1.AllocationPhaseApplied) {
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
	status := snapshot.Status
	status.Phase = nodePhaseReady
	status.ObservedAt = &now
	return r.updateNodeProfileStatus(ctx, nodeProfile, status)
}

func (r *Reconciler) updateNodeProfileStatus(
	ctx context.Context,
	nodeProfile *astrav1alpha1.AINodeResourceProfile,
	status astrav1alpha1.AINodeResourceProfileStatus,
) error {
	key := client.ObjectKeyFromObject(nodeProfile)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &astrav1alpha1.AINodeResourceProfile{}
		if err := r.client.Get(ctx, key, latest); err != nil {
			return err
		}
		latest.Status = *status.DeepCopy()
		return r.client.Status().Update(ctx, latest)
	})
}

func planStatusMessage(plan local.Plan) string {
	message := fmt.Sprintf(
		"plan action=%s tier=%s phase=%s requested=(%s) assigned=(%s)",
		plan.Action,
		plan.AllocationTier,
		plan.TargetPhase,
		describeResource(plan.RequestedResources),
		describeResource(plan.AssignedResources),
	)
	if plan.ActiveWindow != "" {
		message += fmt.Sprintf(" activeWindow=%s", plan.ActiveWindow)
	}
	if plan.Reason != "" {
		message += "; " + plan.Reason
	}
	return message
}

func describeResource(value astrav1alpha1.ResourceSummary) string {
	return fmt.Sprintf(
		"gpu=%d gpuMemoryGiB=%d kvCacheGiB=%d prefillTPS=%d decodeTPS=%d totalTPS=%d",
		value.GPUCount,
		value.GPUMemoryGiB,
		value.KVCacheGiB,
		value.PrefillTokensPerSecond,
		value.DecodeTokensPerSecond,
		value.TotalTokensPerSecond,
	)
}

func (r *Reconciler) getNodeProfile(ctx context.Context) (*astrav1alpha1.AINodeResourceProfile, error) {
	nodeProfile := &astrav1alpha1.AINodeResourceProfile{}
	key := client.ObjectKey{
		Namespace: r.config.NodeProfileNamespace,
		Name:      r.config.NodeName,
	}
	if err := r.client.Get(ctx, key, nodeProfile); err == nil {
		return nodeProfile, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, err
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

	nodeProfile = &astrav1alpha1.AINodeResourceProfile{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.config.NodeProfileNamespace,
			Name:      r.config.NodeName,
		},
		Spec: astrav1alpha1.AINodeResourceProfileSpec{
			NodeName:    r.config.NodeName,
			BackendType: "fake",
		},
	}
	if err := r.client.Create(ctx, nodeProfile); err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("create AINodeResourceProfile for node %q: %w", r.config.NodeName, err)
	}
	if err := r.client.Get(ctx, key, nodeProfile); err != nil {
		return nil, err
	}
	return nodeProfile, nil
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
		if allocation.Status.Phase == string(astrav1alpha1.AllocationPhaseReleased) {
			continue
		}
		allocations = append(allocations, allocation)
	}

	return allocations, nil
}

func replacementPodName(sourcePodName, allocationName string) string {
	hash := sha256.Sum256([]byte(allocationName))
	suffix := hex.EncodeToString(hash[:])[:8]
	base := sanitizeDNSLabel(sourcePodName)
	maxBaseLen := 63 - len("-astra-") - len(suffix)
	if len(base) > maxBaseLen {
		base = strings.Trim(base[:maxBaseLen], "-")
	}
	if base == "" {
		base = "pod"
	}
	return fmt.Sprintf("%s-astra-%s", base, suffix)
}

func sanitizeDNSLabel(value string) string {
	value = strings.ToLower(value)
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			builder.WriteRune(char)
			continue
		}
		builder.WriteRune('-')
	}
	return strings.Trim(builder.String(), "-")
}

func copyStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	target := make(map[string]string, len(source))
	for key, value := range source {
		target[key] = value
	}
	return target
}
