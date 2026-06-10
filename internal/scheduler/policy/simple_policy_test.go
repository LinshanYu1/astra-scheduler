package policy

import (
	"testing"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNormalizeScoreDetailsUsesWeightedHeadroomScore(t *testing.T) {
	p := NewSimplePolicy()

	decisions := p.NormalizeScoreDetails(map[string]NodeScoreDetail{
		"node-a": {
			NodeName:              "node-a",
			PreferredMatched:      4,
			PreferredTotal:        4,
			WeightedHeadroomScore: 80,
		},
		"node-b": {
			NodeName:              "node-b",
			PreferredMatched:      4,
			PreferredTotal:        4,
			WeightedHeadroomScore: 30,
		},
	})

	if decisions["node-a"].Score <= decisions["node-b"].Score {
		t.Fatalf("expected node-a to win by weighted headroom, got node-a=%d node-b=%d", decisions["node-a"].Score, decisions["node-b"].Score)
	}
}

func TestNormalizeScoreDetailsSubtractsRuntimeRisk(t *testing.T) {
	p := NewSimplePolicy()

	decisions := p.NormalizeScoreDetails(map[string]NodeScoreDetail{
		"node-a": {
			NodeName:              "node-a",
			PreferredMatched:      4,
			PreferredTotal:        4,
			WeightedHeadroomScore: 70,
			RuntimeRiskPenalty:    15,
		},
		"node-b": {
			NodeName:              "node-b",
			PreferredMatched:      4,
			PreferredTotal:        4,
			WeightedHeadroomScore: 60,
		},
	})

	if decisions["node-a"].Score != 55 {
		t.Fatalf("expected runtime risk to reduce node-a score to 55, got %d", decisions["node-a"].Score)
	}
	if decisions["node-b"].Score != 60 {
		t.Fatalf("expected node-b score 60, got %d", decisions["node-b"].Score)
	}
}

func TestNormalizeScoreDetailsUsesWeightedHeadroomWhenCoverageTies(t *testing.T) {
	p := NewSimplePolicy()

	decisions := p.NormalizeScoreDetails(map[string]NodeScoreDetail{
		"node-a": {
			NodeName:              "node-a",
			PreferredMatched:      4,
			PreferredTotal:        4,
			WeightedHeadroomScore: 25,
		},
		"node-b": {
			NodeName:              "node-b",
			PreferredMatched:      4,
			PreferredTotal:        4,
			WeightedHeadroomScore: 75,
		},
	})

	if decisions["node-b"].Score <= decisions["node-a"].Score {
		t.Fatalf("expected node-b to win by weighted headroom, got node-a=%d node-b=%d", decisions["node-a"].Score, decisions["node-b"].Score)
	}
}

func TestWeightedHeadroomScoreUsesDemandShapeWeights(t *testing.T) {
	preferred := astrav1alpha1.ResourceSummary{
		KVCacheGiB:             16,
		DecodeTokensPerSecond:  1000,
		PrefillTokensPerSecond: 1000,
	}
	available := astrav1alpha1.ResourceSummary{
		KVCacheGiB:             32,
		DecodeTokensPerSecond:  1000,
		PrefillTokensPerSecond: 2000,
	}

	kvHeavyScore := weightedHeadroomScore("kvCacheGiB", preferred, available)
	decodeHeavyScore := weightedHeadroomScore("decodeTokensPerSecond", preferred, available)

	if kvHeavyScore <= decodeHeavyScore {
		t.Fatalf("expected kv-heavy score to be higher because KV has headroom, got kv=%d decode=%d", kvHeavyScore, decodeHeavyScore)
	}
}

func TestDemandShapeChangesBestNodeForSameNodeResources(t *testing.T) {
	p := NewSimplePolicy()
	nodes := []*astrav1alpha1.AINodeResourceProfile{
		scoreTestNode("node-kv-rich", "low", astrav1alpha1.ResourceSummary{
			GPUCount:               1,
			GPUMemoryGiB:           20,
			KVCacheGiB:             64,
			PrefillTokensPerSecond: 2000,
			DecodeTokensPerSecond:  2500,
		}),
		scoreTestNode("node-decode-rich", "low", astrav1alpha1.ResourceSummary{
			GPUCount:               1,
			GPUMemoryGiB:           20,
			KVCacheGiB:             32,
			PrefillTokensPerSecond: 2000,
			DecodeTokensPerSecond:  5000,
		}),
		scoreTestNode("node-prefill-rich", "low", astrav1alpha1.ResourceSummary{
			GPUCount:               1,
			GPUMemoryGiB:           20,
			KVCacheGiB:             32,
			PrefillTokensPerSecond: 4000,
			DecodeTokensPerSecond:  2500,
		}),
		scoreTestNode("node-risky-kv-rich", "medium", astrav1alpha1.ResourceSummary{
			GPUCount:               1,
			GPUMemoryGiB:           20,
			KVCacheGiB:             64,
			PrefillTokensPerSecond: 2000,
			DecodeTokensPerSecond:  2500,
		}),
	}

	tests := []struct {
		name         string
		demandShape  astrav1alpha1.DemandShape
		wantBestNode string
	}{
		{
			name:         "kv-heavy prefers KV headroom",
			demandShape:  astrav1alpha1.DemandShapeKVHeavy,
			wantBestNode: "node-kv-rich",
		},
		{
			name:         "decode-heavy prefers decode throughput headroom",
			demandShape:  astrav1alpha1.DemandShapeDecodeHeavy,
			wantBestNode: "node-decode-rich",
		},
		{
			name:         "prefill-heavy prefers prefill throughput headroom",
			demandShape:  astrav1alpha1.DemandShapePrefillHeavy,
			wantBestNode: "node-prefill-rich",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := scoreTestProfile(test.demandShape)
			decisions := p.NormalizeScoreDetails(scoreDetailsForNodes(p, profile, nodes))
			bestNode, bestScore := bestScoreDecision(decisions)

			if bestNode != test.wantBestNode {
				t.Fatalf("expected %s to win, got %s with score %d; decisions=%v", test.wantBestNode, bestNode, bestScore, decisions)
			}
			if decisions["node-risky-kv-rich"].Score >= decisions["node-kv-rich"].Score {
				t.Fatalf("expected medium-risk KV node to be scored below low-risk KV node, risky=%d lowRisk=%d", decisions["node-risky-kv-rich"].Score, decisions["node-kv-rich"].Score)
			}
		})
	}
}

func TestAllocationResourcesPreferPreferredWhenAvailable(t *testing.T) {
	p := NewSimplePolicy()
	profile := &astrav1alpha1.AIWorkloadProfile{
		Spec: astrav1alpha1.AIWorkloadProfileSpec{
			Resources: &astrav1alpha1.WorkloadResourceRequest{
				Required: &astrav1alpha1.ResourceSummary{GPUCount: 1, KVCacheGiB: 8},
				Preferred: &astrav1alpha1.ResourceSummary{
					GPUCount:              1,
					KVCacheGiB:            16,
					DecodeTokensPerSecond: 2000,
				},
			},
		},
	}
	node := &astrav1alpha1.AINodeResourceProfile{
		Spec: astrav1alpha1.AINodeResourceProfileSpec{
			Runtime: tokenAndKVRuntime(),
		},
		Status: astrav1alpha1.AINodeResourceProfileStatus{
			Phase: "Ready",
			Available: &astrav1alpha1.ResourceSummary{
				GPUCount:              1,
				KVCacheGiB:            20,
				DecodeTokensPerSecond: 1000,
			},
		},
	}

	resources := p.AllocationResources(profile, node)
	if resources.KVCacheGiB != 16 {
		t.Fatalf("expected available preferred KV cache allocation, got %d", resources.KVCacheGiB)
	}
	if resources.DecodeTokensPerSecond != 0 {
		t.Fatalf("expected decode allocation to fall back to required zero, got %d", resources.DecodeTokensPerSecond)
	}
}

func TestRequiredResourcesDoNotCountAsPreferredMatches(t *testing.T) {
	p := NewSimplePolicy()
	profile := &astrav1alpha1.AIWorkloadProfile{
		Spec: astrav1alpha1.AIWorkloadProfileSpec{
			Resources: &astrav1alpha1.WorkloadResourceRequest{
				Required: &astrav1alpha1.ResourceSummary{GPUCount: 1, KVCacheGiB: 8},
			},
		},
	}
	node := &astrav1alpha1.AINodeResourceProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Spec: astrav1alpha1.AINodeResourceProfileSpec{
			NodeName: "node-a",
			Runtime:  tokenAndKVRuntime(),
		},
		Status: astrav1alpha1.AINodeResourceProfileStatus{
			Phase: "Ready",
			Available: &astrav1alpha1.ResourceSummary{
				GPUCount:   2,
				KVCacheGiB: 24,
			},
		},
	}

	detail := p.ScoreDetail(profile, node)
	if detail.PreferredMatched != 0 || detail.PreferredTotal != 0 {
		t.Fatalf("expected required-only workload to have 0/0 preferred matches, got %d/%d", detail.PreferredMatched, detail.PreferredTotal)
	}

	resources := p.AllocationResources(profile, node)
	if resources.GPUCount != 1 || resources.KVCacheGiB != 8 {
		t.Fatalf("expected allocation to still use required resources, got gpu=%d kv=%d", resources.GPUCount, resources.KVCacheGiB)
	}
}

func TestScoreDetailExtractsKVHeavySignals(t *testing.T) {
	p := NewSimplePolicy()
	profile := &astrav1alpha1.AIWorkloadProfile{
		Spec: astrav1alpha1.AIWorkloadProfileSpec{
			DemandShape: "kv_heavy",
			Resources: &astrav1alpha1.WorkloadResourceRequest{
				Required: &astrav1alpha1.ResourceSummary{GPUCount: 1, KVCacheGiB: 8},
				Preferred: &astrav1alpha1.ResourceSummary{
					GPUCount:              1,
					KVCacheGiB:            16,
					DecodeTokensPerSecond: 1000,
				},
			},
		},
	}
	node := &astrav1alpha1.AINodeResourceProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Spec: astrav1alpha1.AINodeResourceProfileSpec{
			NodeName: "node-a",
			Runtime:  tokenAndKVRuntime(),
		},
		Status: astrav1alpha1.AINodeResourceProfileStatus{
			Phase: "Ready",
			Available: &astrav1alpha1.ResourceSummary{
				GPUCount:              2,
				KVCacheGiB:            24,
				DecodeTokensPerSecond: 1200,
			},
		},
	}

	detail := p.ScoreDetail(profile, node)
	if detail.PreferredMatched != 3 || detail.PreferredTotal != 3 {
		t.Fatalf("expected all preferred resources to match, got %d/%d", detail.PreferredMatched, detail.PreferredTotal)
	}
	if detail.KeyResource != "kvCacheGiB" {
		t.Fatalf("expected kvCacheGiB key resource, got %q", detail.KeyResource)
	}
	if detail.KeyResourceHeadroom != 8 {
		t.Fatalf("expected key headroom 8, got %d", detail.KeyResourceHeadroom)
	}
	if detail.WeightedHeadroomScore != 56 {
		t.Fatalf("expected weighted headroom score 56, got %d", detail.WeightedHeadroomScore)
	}
}

func TestFilterRejectsNodeWithoutKVCacheSupport(t *testing.T) {
	p := NewSimplePolicy()
	profile := &astrav1alpha1.AIWorkloadProfile{
		Spec: astrav1alpha1.AIWorkloadProfileSpec{
			Resources: &astrav1alpha1.WorkloadResourceRequest{
				Required: &astrav1alpha1.ResourceSummary{GPUCount: 1, KVCacheGiB: 8},
			},
		},
	}
	node := readyNodeWithAvailable(astrav1alpha1.ResourceSummary{
		GPUCount:   1,
		KVCacheGiB: 16,
	})

	decision := p.Filter(profile, node)
	if decision.Allowed {
		t.Fatalf("expected node without KV cache runtime support to be rejected")
	}
}

func TestFilterRejectsNodeWithoutTokenThroughputSupport(t *testing.T) {
	p := NewSimplePolicy()
	profile := &astrav1alpha1.AIWorkloadProfile{
		Spec: astrav1alpha1.AIWorkloadProfileSpec{
			Resources: &astrav1alpha1.WorkloadResourceRequest{
				Required: &astrav1alpha1.ResourceSummary{GPUCount: 1, DecodeTokensPerSecond: 500},
			},
		},
	}
	node := readyNodeWithAvailable(astrav1alpha1.ResourceSummary{
		GPUCount:              1,
		DecodeTokensPerSecond: 1000,
	})
	node.Spec.Runtime = &astrav1alpha1.RuntimeSpec{
		Supports: &astrav1alpha1.RuntimeSupports{KVCache: true},
	}

	decision := p.Filter(profile, node)
	if decision.Allowed {
		t.Fatalf("expected node without token throughput runtime support to be rejected")
	}
}

func TestFilterRejectsMissingRequiredNodeCapability(t *testing.T) {
	p := NewSimplePolicy()
	profile := &astrav1alpha1.AIWorkloadProfile{
		Spec: astrav1alpha1.AIWorkloadProfileSpec{
			Resources: &astrav1alpha1.WorkloadResourceRequest{
				Required: &astrav1alpha1.ResourceSummary{GPUCount: 1},
			},
			Policy: &astrav1alpha1.WorkloadPolicy{Pauseable: true},
		},
	}
	node := readyNodeWithAvailable(astrav1alpha1.ResourceSummary{GPUCount: 1})
	node.Spec.Runtime = tokenAndKVRuntime()
	node.Spec.Capabilities = &astrav1alpha1.NodeCapabilities{Throttling: true}

	decision := p.Filter(profile, node)
	if decision.Allowed {
		t.Fatalf("expected node without pause/resume capability to be rejected")
	}
}

func TestFilterAllowsRequiredRuntimeAndCapabilities(t *testing.T) {
	p := NewSimplePolicy()
	profile := &astrav1alpha1.AIWorkloadProfile{
		Spec: astrav1alpha1.AIWorkloadProfileSpec{
			Resources: &astrav1alpha1.WorkloadResourceRequest{
				Required: &astrav1alpha1.ResourceSummary{
					GPUCount:              1,
					KVCacheGiB:            8,
					DecodeTokensPerSecond: 500,
				},
			},
			Policy: &astrav1alpha1.WorkloadPolicy{
				AllowBorrowing: true,
				Throttleable:   true,
			},
		},
	}
	node := readyNodeWithAvailable(astrav1alpha1.ResourceSummary{
		GPUCount:              1,
		KVCacheGiB:            16,
		DecodeTokensPerSecond: 1000,
	})
	node.Spec.Runtime = tokenAndKVRuntime()
	node.Spec.Capabilities = &astrav1alpha1.NodeCapabilities{
		Borrowing:  true,
		Throttling: true,
	}

	decision := p.Filter(profile, node)
	if !decision.Allowed {
		t.Fatalf("expected node to pass hard constraints, got %q", decision.Reason)
	}
}

func readyNodeWithAvailable(available astrav1alpha1.ResourceSummary) *astrav1alpha1.AINodeResourceProfile {
	return &astrav1alpha1.AINodeResourceProfile{
		Status: astrav1alpha1.AINodeResourceProfileStatus{
			Phase:     "Ready",
			Available: &available,
		},
	}
}

func tokenAndKVRuntime() *astrav1alpha1.RuntimeSpec {
	return &astrav1alpha1.RuntimeSpec{
		Supports: &astrav1alpha1.RuntimeSupports{
			KVCache:         true,
			TokenThroughput: true,
		},
	}
}

func scoreTestProfile(demandShape astrav1alpha1.DemandShape) *astrav1alpha1.AIWorkloadProfile {
	return &astrav1alpha1.AIWorkloadProfile{
		Spec: astrav1alpha1.AIWorkloadProfileSpec{
			DemandShape: demandShape,
			Resources: &astrav1alpha1.WorkloadResourceRequest{
				Required: &astrav1alpha1.ResourceSummary{
					GPUCount:               1,
					GPUMemoryGiB:           10,
					KVCacheGiB:             16,
					PrefillTokensPerSecond: 1000,
					DecodeTokensPerSecond:  1000,
				},
				Preferred: &astrav1alpha1.ResourceSummary{
					GPUCount:               1,
					GPUMemoryGiB:           20,
					KVCacheGiB:             32,
					PrefillTokensPerSecond: 2000,
					DecodeTokensPerSecond:  2500,
				},
			},
		},
	}
}

func scoreTestNode(name string, sloRisk string, available astrav1alpha1.ResourceSummary) *astrav1alpha1.AINodeResourceProfile {
	node := readyNodeWithAvailable(available)
	node.ObjectMeta = metav1.ObjectMeta{Name: name}
	node.Spec.NodeName = name
	node.Spec.Runtime = tokenAndKVRuntime()
	node.Status.Runtime = &astrav1alpha1.RuntimeStatus{
		Pressure: &astrav1alpha1.RuntimePressure{SLORisk: sloRisk},
	}
	return node
}

func scoreDetailsForNodes(
	p *SimplePolicy,
	profile *astrav1alpha1.AIWorkloadProfile,
	nodes []*astrav1alpha1.AINodeResourceProfile,
) map[string]NodeScoreDetail {
	details := make(map[string]NodeScoreDetail, len(nodes))
	for _, node := range nodes {
		detail := p.ScoreDetail(profile, node)
		details[detail.NodeName] = detail
	}
	return details
}

func bestScoreDecision(decisions map[string]ScoreDecision) (string, int64) {
	var bestNode string
	var bestScore int64 = -1
	for nodeName, decision := range decisions {
		if decision.Score > bestScore {
			bestNode = nodeName
			bestScore = decision.Score
		}
	}
	return bestNode, bestScore
}
