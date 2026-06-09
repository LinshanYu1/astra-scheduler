package policy

import (
	"testing"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNormalizeScoreDetailsPrefersPreferredCoverageBeforeHeadroom(t *testing.T) {
	p := NewSimplePolicy()

	decisions := p.NormalizeScoreDetails(map[string]NodeScoreDetail{
		"node-a": {
			NodeName:                    "node-a",
			PreferredMatched:            4,
			PreferredTotal:              4,
			KeyResource:                 "kvCacheGiB",
			KeyResourcePreferredMatched: true,
			KeyResourceHeadroom:         2,
			OtherHeadroomScore:          20,
		},
		"node-b": {
			NodeName:                    "node-b",
			PreferredMatched:            3,
			PreferredTotal:              4,
			KeyResource:                 "kvCacheGiB",
			KeyResourcePreferredMatched: true,
			KeyResourceHeadroom:         100,
			OtherHeadroomScore:          100,
		},
	})

	if decisions["node-a"].Score <= decisions["node-b"].Score {
		t.Fatalf("expected node-a to win by preferred coverage, got node-a=%d node-b=%d", decisions["node-a"].Score, decisions["node-b"].Score)
	}
}

func TestNormalizeScoreDetailsKeepsPreferredCoverageAsHardTier(t *testing.T) {
	p := NewSimplePolicy()

	decisions := p.NormalizeScoreDetails(map[string]NodeScoreDetail{
		"node-a": {
			NodeName:                    "node-a",
			PreferredMatched:            4,
			PreferredTotal:              4,
			KeyResource:                 "kvCacheGiB",
			KeyResourcePreferredMatched: true,
			RuntimeRiskPenalty:          15,
		},
		"node-b": {
			NodeName:                    "node-b",
			PreferredMatched:            3,
			PreferredTotal:              4,
			KeyResource:                 "kvCacheGiB",
			KeyResourcePreferredMatched: true,
			KeyResourceHeadroom:         100,
			OtherHeadroomScore:          100,
		},
	})

	if decisions["node-a"].Score <= decisions["node-b"].Score {
		t.Fatalf("expected higher preferred coverage to stay ahead, got node-a=%d node-b=%d", decisions["node-a"].Score, decisions["node-b"].Score)
	}
}

func TestNormalizeScoreDetailsUsesKeyHeadroomWhenCoverageTies(t *testing.T) {
	p := NewSimplePolicy()

	decisions := p.NormalizeScoreDetails(map[string]NodeScoreDetail{
		"node-a": {
			NodeName:                    "node-a",
			PreferredMatched:            4,
			PreferredTotal:              4,
			KeyResource:                 "kvCacheGiB",
			KeyResourcePreferredMatched: true,
			KeyResourceHeadroom:         2,
		},
		"node-b": {
			NodeName:                    "node-b",
			PreferredMatched:            4,
			PreferredTotal:              4,
			KeyResource:                 "kvCacheGiB",
			KeyResourcePreferredMatched: true,
			KeyResourceHeadroom:         8,
		},
	})

	if decisions["node-b"].Score <= decisions["node-a"].Score {
		t.Fatalf("expected node-b to win by key headroom, got node-a=%d node-b=%d", decisions["node-a"].Score, decisions["node-b"].Score)
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
