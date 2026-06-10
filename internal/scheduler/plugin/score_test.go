package plugin

import (
	"testing"

	"github.com/linshanyu/astra-scheduler/internal/scheduler/policy"
)

func TestNonBestPreferredTierScoreUsesHalfOfWorstBestNode(t *testing.T) {
	score := nonBestPreferredTierScore(map[string]policy.ScoreDecision{
		"node-a": {Score: 90},
		"node-b": {Score: 80},
	})

	if score != 40 {
		t.Fatalf("expected non-best tier score to be half of the worst best-node score, got %d", score)
	}
}
