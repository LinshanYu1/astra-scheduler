package plugin

import (
	"fmt"
	"sync"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	"github.com/linshanyu/astra-scheduler/internal/scheduler/policy"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kube-scheduler/framework"
)

const schedulingStateKey framework.StateKey = "AstraSchedulerSchedulingState"

type schedulingState struct {
	mu                   sync.RWMutex
	profile              *astrav1alpha1.AIWorkloadProfile
	nodeResources        map[string]*astrav1alpha1.AINodeResourceProfile
	scoreDetails         map[string]policy.NodeScoreDetail
	scoreDecisions       map[string]policy.ScoreDecision
	bestPreferredMatched int
	bestPreferredScore   int64
	bestPreferredNodes   sets.Set[string]
}

func newSchedulingState(profile *astrav1alpha1.AIWorkloadProfile) *schedulingState {
	return &schedulingState{
		profile:        profile,
		nodeResources:  map[string]*astrav1alpha1.AINodeResourceProfile{},
		scoreDetails:   map[string]policy.NodeScoreDetail{},
		scoreDecisions: map[string]policy.ScoreDecision{},
		// PreFilter creates this per-Pod scheduling state before any node is scored.
		// PreferredMatched starts below the minimum valid match count, so the first
		// scored node always becomes the best tier. Score starts at 1, so that first
		// best tier gets score 2.
		bestPreferredMatched: -1,
		bestPreferredScore:   1,
		bestPreferredNodes:   sets.New[string](),
	}
}

func (s *schedulingState) Clone() framework.StateData {
	if s == nil {
		return newSchedulingState(nil)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	copied := newSchedulingState(s.profile)
	for nodeName, nodeResource := range s.nodeResources {
		copied.nodeResources[nodeName] = nodeResource
	}
	for nodeName, detail := range s.scoreDetails {
		copied.scoreDetails[nodeName] = detail
	}
	for nodeName, decision := range s.scoreDecisions {
		copied.scoreDecisions[nodeName] = decision
	}
	copied.bestPreferredMatched = s.bestPreferredMatched
	copied.bestPreferredScore = s.bestPreferredScore
	copied.bestPreferredNodes = s.bestPreferredNodes.Clone()

	return copied
}

func (s *schedulingState) getProfile() *astrav1alpha1.AIWorkloadProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.profile
}

func (s *schedulingState) setProfile(profile *astrav1alpha1.AIWorkloadProfile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profile = profile
}

func (s *schedulingState) getNodeResource(nodeName string) *astrav1alpha1.AINodeResourceProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nodeResources[nodeName]
}

func (s *schedulingState) setNodeResource(nodeName string, nodeResource *astrav1alpha1.AINodeResourceProfile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodeResources[nodeName] = nodeResource
}

func (s *schedulingState) recordScoreDetail(nodeName string, detail policy.NodeScoreDetail) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.scoreDetails[nodeName] = detail

	// A higher preferred-resource match count replaces the current best tier.
	// The new tier's intermediate score is one point above the previous best tier.
	if detail.PreferredMatched > s.bestPreferredMatched {
		s.bestPreferredMatched = detail.PreferredMatched
		s.bestPreferredScore++
		s.bestPreferredNodes = sets.New[string](nodeName)
		return s.bestPreferredScore
	}

	if detail.PreferredMatched == s.bestPreferredMatched {
		s.bestPreferredNodes.Insert(nodeName)
		return s.bestPreferredScore
	}

	return s.bestPreferredScore - 1
}

func (s *schedulingState) scoreDetailSnapshot() map[string]policy.NodeScoreDetail {
	s.mu.RLock()
	defer s.mu.RUnlock()

	copied := make(map[string]policy.NodeScoreDetail, len(s.scoreDetails))
	for nodeName, detail := range s.scoreDetails {
		copied[nodeName] = detail
	}

	return copied
}

func (s *schedulingState) bestScoreDetailSnapshot() map[string]policy.NodeScoreDetail {
	s.mu.RLock()
	defer s.mu.RUnlock()

	copied := make(map[string]policy.NodeScoreDetail, s.bestPreferredNodes.Len())
	for nodeName := range s.bestPreferredNodes {
		if detail, ok := s.scoreDetails[nodeName]; ok {
			copied[nodeName] = detail
		}
	}

	return copied
}

func (s *schedulingState) setScoreDecision(nodeName string, decision policy.ScoreDecision) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scoreDecisions[nodeName] = decision
}

func (s *schedulingState) getScoreDecision(nodeName string) (policy.ScoreDecision, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	decision, ok := s.scoreDecisions[nodeName]
	return decision, ok
}

func readSchedulingState(state framework.CycleState) (*schedulingState, *framework.Status) {
	data, err := state.Read(schedulingStateKey)
	if err != nil {
		return nil, framework.NewStatus(framework.Error, fmt.Sprintf("read Astra scheduling state failed: %v", err))
	}

	schedulingState, ok := data.(*schedulingState)
	if !ok {
		return nil, framework.NewStatus(framework.Error, "Astra scheduling state has unexpected type")
	}

	return schedulingState, nil
}
