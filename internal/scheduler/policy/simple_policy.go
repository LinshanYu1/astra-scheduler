package policy

import (
	"fmt"
	"strings"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
)

const MaxNodeScore int64 = 100

type FilterDecision struct {
	Allowed bool
	Reason  string
}

type ScoreDecision struct {
	Score  int64
	Reason string
}

type NodeScoreDetail struct {
	NodeName                    string
	PreferredMatched            int
	PreferredTotal              int
	KeyResource                 string
	KeyResourcePreferredMatched bool
	KeyResourceHeadroom         int32
	OtherHeadroomScore          int64
	RuntimeRiskPenalty          int64
	Reason                      string
}

type SimplePolicy struct{}

func NewSimplePolicy() *SimplePolicy {
	return &SimplePolicy{}
}

func (p *SimplePolicy) Filter(profile *astrav1alpha1.AIWorkloadProfile, node *astrav1alpha1.AINodeResourceProfile) FilterDecision {
	if profile == nil {
		return FilterDecision{Allowed: false, Reason: "missing AIWorkloadProfile"}
	}
	if node == nil {
		return FilterDecision{Allowed: false, Reason: "missing AINodeResourceProfile"}
	}
	if !nodeIsReady(node) {
		return FilterDecision{Allowed: false, Reason: fmt.Sprintf("node AI resource profile is not ready: phase=%s", node.Status.Phase)}
	}
	if node.Status.Available == nil {
		return FilterDecision{Allowed: false, Reason: "node has no available resource summary"}
	}

	required := requiredResources(profile)
	available := node.Status.Available
	if decision := validateRuntimeHardConstraints(required, profile, node); !decision.Allowed {
		return decision
	}
	if decision := validateCapabilityHardConstraints(profile, node); !decision.Allowed {
		return decision
	}

	if required.GPUCount > available.GPUCount {
		return reject("gpuCount", required.GPUCount, available.GPUCount)
	}
	if required.GPUMemoryGiB > available.GPUMemoryGiB {
		return reject("gpuMemoryGiB", required.GPUMemoryGiB, available.GPUMemoryGiB)
	}
	if required.KVCacheGiB > available.KVCacheGiB {
		return reject("kvCacheGiB", required.KVCacheGiB, available.KVCacheGiB)
	}
	if required.PrefillTokensPerSecond > available.PrefillTokensPerSecond {
		return reject("prefillTokensPerSecond", required.PrefillTokensPerSecond, available.PrefillTokensPerSecond)
	}
	if required.DecodeTokensPerSecond > available.DecodeTokensPerSecond {
		return reject("decodeTokensPerSecond", required.DecodeTokensPerSecond, available.DecodeTokensPerSecond)
	}
	if required.TotalTokensPerSecond > available.TotalTokensPerSecond {
		return reject("totalTokensPerSecond", required.TotalTokensPerSecond, available.TotalTokensPerSecond)
	}

	return FilterDecision{Allowed: true, Reason: "required AI resources fit node availability"}
}

func nodeIsReady(node *astrav1alpha1.AINodeResourceProfile) bool {
	if node == nil {
		return false
	}
	return node.Status.Phase == "" || strings.EqualFold(node.Status.Phase, "Ready")
}

func validateRuntimeHardConstraints(
	required astrav1alpha1.ResourceSummary,
	profile *astrav1alpha1.AIWorkloadProfile,
	node *astrav1alpha1.AINodeResourceProfile,
) FilterDecision {
	supports := runtimeSupports(node)
	if requiresKVCache(required, profile) && !supports.KVCache {
		return FilterDecision{Allowed: false, Reason: "node runtime does not support KV cache"}
	}
	if requiresTokenThroughput(required, profile) && !supports.TokenThroughput {
		return FilterDecision{Allowed: false, Reason: "node runtime does not support token throughput control"}
	}
	return FilterDecision{Allowed: true, Reason: "runtime hard constraints satisfied"}
}

func validateCapabilityHardConstraints(
	profile *astrav1alpha1.AIWorkloadProfile,
	node *astrav1alpha1.AINodeResourceProfile,
) FilterDecision {
	if profile == nil || profile.Spec.Policy == nil {
		return FilterDecision{Allowed: true, Reason: "no workload policy hard constraints"}
	}
	capabilities := nodeCapabilities(node)
	policy := profile.Spec.Policy

	if policy.AllowBorrowing && !capabilities.Borrowing {
		return FilterDecision{Allowed: false, Reason: "node does not support resource borrowing"}
	}
	if policy.AllowReclaimFromLowerPriority && !capabilities.Reclaim {
		return FilterDecision{Allowed: false, Reason: "node does not support resource reclaim"}
	}
	if policy.Throttleable && !capabilities.Throttling {
		return FilterDecision{Allowed: false, Reason: "node does not support throttling"}
	}
	if policy.Pauseable && !capabilities.PauseResume {
		return FilterDecision{Allowed: false, Reason: "node does not support pause/resume"}
	}
	if policy.Evictable && !capabilities.Eviction {
		return FilterDecision{Allowed: false, Reason: "node does not support eviction"}
	}

	return FilterDecision{Allowed: true, Reason: "node capability hard constraints satisfied"}
}

func requiresKVCache(required astrav1alpha1.ResourceSummary, profile *astrav1alpha1.AIWorkloadProfile) bool {
	return required.KVCacheGiB > 0 || strings.EqualFold(profile.Spec.DemandShape, "kv_heavy")
}

func requiresTokenThroughput(required astrav1alpha1.ResourceSummary, profile *astrav1alpha1.AIWorkloadProfile) bool {
	return required.PrefillTokensPerSecond > 0 ||
		required.DecodeTokensPerSecond > 0 ||
		required.TotalTokensPerSecond > 0 ||
		strings.EqualFold(profile.Spec.DemandShape, "prefill_heavy") ||
		strings.EqualFold(profile.Spec.DemandShape, "decode_heavy")
}

func runtimeSupports(node *astrav1alpha1.AINodeResourceProfile) astrav1alpha1.RuntimeSupports {
	if node == nil || node.Spec.Runtime == nil || node.Spec.Runtime.Supports == nil {
		return astrav1alpha1.RuntimeSupports{}
	}
	return *node.Spec.Runtime.Supports
}

func nodeCapabilities(node *astrav1alpha1.AINodeResourceProfile) astrav1alpha1.NodeCapabilities {
	if node == nil || node.Spec.Capabilities == nil {
		return astrav1alpha1.NodeCapabilities{}
	}
	return *node.Spec.Capabilities
}

func (p *SimplePolicy) Score(profile *astrav1alpha1.AIWorkloadProfile, node *astrav1alpha1.AINodeResourceProfile) ScoreDecision {
	detail := p.ScoreDetail(profile, node)
	return ScoreDecision{
		Score:  int64(detail.PreferredMatched),
		Reason: detail.Reason,
	}
}

func (p *SimplePolicy) ScoreDetail(profile *astrav1alpha1.AIWorkloadProfile, node *astrav1alpha1.AINodeResourceProfile) NodeScoreDetail {
	detail := NodeScoreDetail{}
	if node != nil {
		detail.NodeName = node.Spec.NodeName
		if detail.NodeName == "" {
			detail.NodeName = node.Name
		}
	}

	filter := p.Filter(profile, node)
	if !filter.Allowed {
		detail.Reason = filter.Reason
		return detail
	}

	available := *node.Status.Available
	preferred := preferredResources(profile)
	detail.PreferredMatched, detail.PreferredTotal = preferredMatchCount(preferred, available)
	detail.KeyResource = keyResourceName(profile.Spec.DemandShape)
	detail.KeyResourcePreferredMatched = keyResourceMatched(detail.KeyResource, preferred, available)
	detail.KeyResourceHeadroom = keyResourceHeadroom(detail.KeyResource, preferred, available)
	detail.OtherHeadroomScore = otherHeadroomScore(detail.KeyResource, preferred, available)
	detail.RuntimeRiskPenalty = runtimeRiskPenalty(node)
	detail.Reason = fmt.Sprintf(
		"preferredMatched=%d/%d keyResource=%s keyPreferredMatched=%t keyHeadroom=%d otherHeadroom=%d runtimeRiskPenalty=%d",
		detail.PreferredMatched,
		detail.PreferredTotal,
		detail.KeyResource,
		detail.KeyResourcePreferredMatched,
		detail.KeyResourceHeadroom,
		detail.OtherHeadroomScore,
		detail.RuntimeRiskPenalty,
	)

	return detail
}

func (p *SimplePolicy) NormalizeScoreDetails(details map[string]NodeScoreDetail) map[string]ScoreDecision {
	decisions := make(map[string]ScoreDecision, len(details))
	if len(details) == 0 {
		return decisions
	}

	maxPreferredMatched := 0
	for _, detail := range details {
		if detail.PreferredMatched > maxPreferredMatched {
			maxPreferredMatched = detail.PreferredMatched
		}
	}

	var maxKeyHeadroom int32
	var maxOtherHeadroom int64
	for _, detail := range details {
		if detail.PreferredMatched != maxPreferredMatched {
			continue
		}
		if detail.KeyResourceHeadroom > maxKeyHeadroom {
			maxKeyHeadroom = detail.KeyResourceHeadroom
		}
		if detail.OtherHeadroomScore > maxOtherHeadroom {
			maxOtherHeadroom = detail.OtherHeadroomScore
		}
	}

	for nodeName, detail := range details {
		score := preferredCoverageScore(detail, maxPreferredMatched)
		reasonParts := []string{
			detail.Reason,
			fmt.Sprintf("maxPreferredMatched=%d", maxPreferredMatched),
		}

		if detail.PreferredMatched == maxPreferredMatched {
			if detail.KeyResourcePreferredMatched {
				score += 15
				reasonParts = append(reasonParts, "keyPreferredBonus=15")
			}
			keyHeadroomBonus := proportionalBonus(int64(detail.KeyResourceHeadroom), int64(maxKeyHeadroom), 20)
			otherHeadroomBonus := proportionalBonus(detail.OtherHeadroomScore, maxOtherHeadroom, 10)
			score += keyHeadroomBonus + otherHeadroomBonus
			reasonParts = append(
				reasonParts,
				fmt.Sprintf("keyHeadroomBonus=%d", keyHeadroomBonus),
				fmt.Sprintf("otherHeadroomBonus=%d", otherHeadroomBonus),
			)
		}

		score -= detail.RuntimeRiskPenalty
		if detail.RuntimeRiskPenalty > 0 {
			reasonParts = append(reasonParts, fmt.Sprintf("runtimeRiskPenalty=%d", detail.RuntimeRiskPenalty))
		}

		decisions[nodeName] = ScoreDecision{
			Score:  clampScore(score),
			Reason: strings.Join(reasonParts, "; "),
		}
	}

	return decisions
}

func (p *SimplePolicy) AllocationResources(profile *astrav1alpha1.AIWorkloadProfile, node *astrav1alpha1.AINodeResourceProfile) astrav1alpha1.ResourceSummary {
	required := requiredResources(profile)
	if profile == nil || node == nil || node.Status.Available == nil {
		return required
	}

	preferred := preferredResources(profile)
	available := *node.Status.Available

	return astrav1alpha1.ResourceSummary{
		GPUCount:               allocationResourceValue(required.GPUCount, preferred.GPUCount, available.GPUCount),
		GPUMemoryGiB:           allocationResourceValue(required.GPUMemoryGiB, preferred.GPUMemoryGiB, available.GPUMemoryGiB),
		KVCacheGiB:             allocationResourceValue(required.KVCacheGiB, preferred.KVCacheGiB, available.KVCacheGiB),
		PrefillTokensPerSecond: allocationResourceValue(required.PrefillTokensPerSecond, preferred.PrefillTokensPerSecond, available.PrefillTokensPerSecond),
		DecodeTokensPerSecond:  allocationResourceValue(required.DecodeTokensPerSecond, preferred.DecodeTokensPerSecond, available.DecodeTokensPerSecond),
		TotalTokensPerSecond:   allocationResourceValue(required.TotalTokensPerSecond, preferred.TotalTokensPerSecond, available.TotalTokensPerSecond),
	}
}

func (p *SimplePolicy) AllocationType(profile *astrav1alpha1.AIWorkloadProfile, resources astrav1alpha1.ResourceSummary) string {
	required := requiredResources(profile)
	if resources.GPUCount > required.GPUCount ||
		resources.GPUMemoryGiB > required.GPUMemoryGiB ||
		resources.KVCacheGiB > required.KVCacheGiB ||
		resources.PrefillTokensPerSecond > required.PrefillTokensPerSecond ||
		resources.DecodeTokensPerSecond > required.DecodeTokensPerSecond ||
		resources.TotalTokensPerSecond > required.TotalTokensPerSecond {
		return "preferred"
	}
	return "guaranteed"
}

func requiredResources(profile *astrav1alpha1.AIWorkloadProfile) astrav1alpha1.ResourceSummary {
	if profile == nil {
		return astrav1alpha1.ResourceSummary{}
	}
	if profile.Spec.Resources == nil || profile.Spec.Resources.Required == nil {
		return astrav1alpha1.ResourceSummary{}
	}
	return *profile.Spec.Resources.Required
}

func preferredResources(profile *astrav1alpha1.AIWorkloadProfile) astrav1alpha1.ResourceSummary {
	if profile == nil {
		return astrav1alpha1.ResourceSummary{}
	}
	if profile.Spec.Resources == nil {
		return astrav1alpha1.ResourceSummary{}
	}
	if profile.Spec.Resources.Preferred != nil {
		return *profile.Spec.Resources.Preferred
	}
	if profile.Spec.Resources.Required != nil {
		return *profile.Spec.Resources.Required
	}
	return astrav1alpha1.ResourceSummary{}
}

func preferredMatchCount(preferred, available astrav1alpha1.ResourceSummary) (int, int) {
	checks := []struct {
		preferred int32
		available int32
	}{
		{preferred: preferred.GPUCount, available: available.GPUCount},
		{preferred: preferred.GPUMemoryGiB, available: available.GPUMemoryGiB},
		{preferred: preferred.KVCacheGiB, available: available.KVCacheGiB},
		{preferred: preferred.PrefillTokensPerSecond, available: available.PrefillTokensPerSecond},
		{preferred: preferred.DecodeTokensPerSecond, available: available.DecodeTokensPerSecond},
		{preferred: preferred.TotalTokensPerSecond, available: available.TotalTokensPerSecond},
	}

	var matched int
	var total int
	for _, check := range checks {
		if check.preferred <= 0 {
			continue
		}
		total++
		if check.available >= check.preferred {
			matched++
		}
	}

	return matched, total
}

func keyResourceName(demandShape string) string {
	switch strings.ToLower(demandShape) {
	case "decode_heavy":
		return "decodeTokensPerSecond"
	case "prefill_heavy":
		return "prefillTokensPerSecond"
	case "kv_heavy":
		return "kvCacheGiB"
	default:
		return "balanced"
	}
}

func keyResourceMatched(key string, preferred, available astrav1alpha1.ResourceSummary) bool {
	preferredValue, availableValue := keyResourceValues(key, preferred, available)
	if preferredValue <= 0 {
		return false
	}
	return availableValue >= preferredValue
}

func keyResourceHeadroom(key string, preferred, available astrav1alpha1.ResourceSummary) int32 {
	if key == "balanced" {
		return int32(otherHeadroomScore(key, preferred, available))
	}
	preferredValue, availableValue := keyResourceValues(key, preferred, available)
	if preferredValue <= 0 || availableValue <= preferredValue {
		return 0
	}
	return availableValue - preferredValue
}

func keyResourceValues(key string, preferred, available astrav1alpha1.ResourceSummary) (int32, int32) {
	switch key {
	case "decodeTokensPerSecond":
		return preferred.DecodeTokensPerSecond, available.DecodeTokensPerSecond
	case "prefillTokensPerSecond":
		return preferred.PrefillTokensPerSecond, available.PrefillTokensPerSecond
	case "kvCacheGiB":
		return preferred.KVCacheGiB, available.KVCacheGiB
	default:
		return 0, 0
	}
}

func otherHeadroomScore(key string, preferred, available astrav1alpha1.ResourceSummary) int64 {
	resources := []struct {
		name      string
		preferred int32
		available int32
	}{
		{name: "gpuCount", preferred: preferred.GPUCount, available: available.GPUCount},
		{name: "gpuMemoryGiB", preferred: preferred.GPUMemoryGiB, available: available.GPUMemoryGiB},
		{name: "kvCacheGiB", preferred: preferred.KVCacheGiB, available: available.KVCacheGiB},
		{name: "prefillTokensPerSecond", preferred: preferred.PrefillTokensPerSecond, available: available.PrefillTokensPerSecond},
		{name: "decodeTokensPerSecond", preferred: preferred.DecodeTokensPerSecond, available: available.DecodeTokensPerSecond},
		{name: "totalTokensPerSecond", preferred: preferred.TotalTokensPerSecond, available: available.TotalTokensPerSecond},
	}

	var total int64
	var count int64
	for _, resource := range resources {
		if resource.name == key || resource.preferred <= 0 || resource.available <= resource.preferred {
			continue
		}
		total += cappedHeadroomPercent(resource.available, resource.preferred)
		count++
	}

	if count == 0 {
		return 0
	}
	return total / count
}

func runtimeRiskPenalty(node *astrav1alpha1.AINodeResourceProfile) int64 {
	if node == nil || node.Status.Runtime == nil || node.Status.Runtime.Pressure == nil {
		return 0
	}

	switch strings.ToLower(node.Status.Runtime.Pressure.SLORisk) {
	case "high":
		return 15
	case "medium":
		return 8
	default:
		return 0
	}
}

func preferredCoverageScore(detail NodeScoreDetail, maxPreferredMatched int) int64 {
	if maxPreferredMatched == 0 {
		return 70
	}
	if detail.PreferredMatched < maxPreferredMatched {
		return int64(detail.PreferredMatched) * 49 / int64(maxPreferredMatched)
	}
	return 70
}

func proportionalBonus(value, maxValue, maxBonus int64) int64 {
	if value <= 0 || maxValue <= 0 || maxBonus <= 0 {
		return 0
	}
	if value >= maxValue {
		return maxBonus
	}
	return value * maxBonus / maxValue
}

func reject(resource string, required, available int32) FilterDecision {
	return FilterDecision{
		Allowed: false,
		Reason:  fmt.Sprintf("insufficient %s: required=%d available=%d", resource, required, available),
	}
}

func allocationResourceValue(required, preferred, available int32) int32 {
	target := preferred
	if target <= 0 {
		target = required
	}
	if available >= target {
		return target
	}
	return required
}

func cappedHeadroomPercent(available, preferred int32) int64 {
	if preferred <= 0 || available <= preferred {
		return 0
	}
	score := int64(available-preferred) * 100 / int64(preferred)
	if score > 100 {
		return 100
	}
	return score
}

func clampScore(score int64) int64 {
	if score < 0 {
		return 0
	}
	if score > MaxNodeScore {
		return MaxNodeScore
	}
	return score
}
