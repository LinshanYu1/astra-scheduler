package plugin

import (
	"context"
	"fmt"

	"github.com/linshanyu/astra-scheduler/internal/scheduler/policy"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/kube-scheduler/framework"
)

func (p *AstraScheduler) PreScore(_ context.Context, state framework.CycleState, pod *corev1.Pod, _ []framework.NodeInfo) *framework.Status {
	if pod.Spec.SchedulerName != SchedulerName {
		return framework.NewStatus(framework.Skip)
	}

	if _, status := readSchedulingState(state); status != nil {
		state.Write(schedulingStateKey, newSchedulingState(nil))
	}
	return nil
}

func (p *AstraScheduler) Score(ctx context.Context, state framework.CycleState, pod *corev1.Pod, nodeInfo framework.NodeInfo) (int64, *framework.Status) {
	if pod.Spec.SchedulerName != SchedulerName {
		return 0, nil
	}

	node := nodeInfo.Node()
	if node == nil {
		return 0, framework.NewStatus(framework.Error, "nodeInfo has nil node")
	}

	profile, status := p.profileForPod(ctx, state, pod)
	if status != nil {
		return 0, status
	}

	nodeResource, status := p.nodeResourceForNode(ctx, state, node.Name)
	if status != nil {
		return 0, status
	}

	detail := p.policy.ScoreDetail(profile, nodeResource)
	detail.NodeName = node.Name

	schedulingState, status := readSchedulingState(state)
	if status != nil {
		return 0, status
	}
	initialScore := schedulingState.recordScoreDetail(node.Name, detail)
	klog.V(4).InfoS("AstraScheduler Score", "pod", klog.KObj(pod), "node", node.Name, "initialScore", initialScore, "preferredMatched", detail.PreferredMatched, "reason", detail.Reason)

	// The final score is assigned in NormalizeScore. This initial score only
	// preserves the preferred-count tier while all candidate nodes are still
	// being visited by the scheduler framework.
	return initialScore, nil
}

func (p *AstraScheduler) ScoreExtensions() framework.ScoreExtensions {
	return p
}

func (p *AstraScheduler) NormalizeScore(_ context.Context, state framework.CycleState, pod *corev1.Pod, scores framework.NodeScoreList) *framework.Status {
	if pod.Spec.SchedulerName != SchedulerName {
		return nil
	}

	schedulingState, status := readSchedulingState(state)
	if status != nil {
		return status
	}

	allDetails := schedulingState.scoreDetailSnapshot()
	bestDetails := schedulingState.bestScoreDetailSnapshot()
	scoreByNode := p.policy.NormalizeScoreDetails(bestDetails)
	loserScore := nonBestPreferredTierScore(scoreByNode)
	scoredNodes := sets.New[string]()

	for i := range scores {
		decision, ok := scoreByNode[scores[i].Name]
		if !ok {
			if detail, found := allDetails[scores[i].Name]; found {
				decision = policy.ScoreDecision{
					Score:  loserScore,
					Reason: fmt.Sprintf("%s; not in best preferred-match tier", detail.Reason),
				}
				scores[i].Score = decision.Score
				schedulingState.setScoreDecision(scores[i].Name, decision)
			} else {
				scores[i].Score = 0
			}
			continue
		}
		scores[i].Score = decision.Score
		schedulingState.setScoreDecision(scores[i].Name, decision)
		scoredNodes.Insert(scores[i].Name)
	}

	if scoredNodes.Len() == 0 && len(scores) > 0 {
		return framework.NewStatus(framework.Error, "no Astra node score details were recorded")
	}

	return nil
}

func nonBestPreferredTierScore(bestDecisions map[string]policy.ScoreDecision) int64 {
	if len(bestDecisions) == 0 {
		return 0
	}

	minScore := int64(framework.MaxNodeScore)
	for _, decision := range bestDecisions {
		if decision.Score < minScore {
			minScore = decision.Score
		}
	}
	if minScore <= 0 {
		return 0
	}
	return minScore / 2
}
