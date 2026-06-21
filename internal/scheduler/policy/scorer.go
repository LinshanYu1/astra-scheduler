package policy

import (
	"fmt"
	"strings"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
)

type ScoringContext struct {
	Profile   *astrav1alpha1.AIWorkloadProfile
	Node      *astrav1alpha1.AINodeResourceProfile
	Preferred astrav1alpha1.ResourceSummary
	Available astrav1alpha1.ResourceSummary
	Key       string
}

type ScoreContribution struct {
	Name       string
	Score      int64
	OtherScore int64
	Multiplier float64
	Penalty    int64
	Reason     string
}

type Scorer interface {
	Name() string
	Score(ScoringContext) ScoreContribution
}

type ScoreComposer struct {
	scorers            []Scorer
	demandShapeMapping DemandShapeResourceMapping
}

func NewScoreComposer(scorers ...Scorer) *ScoreComposer {
	return NewScoreComposerWithOptions(ScoreComposerOptions{Scorers: scorers})
}

type ScoreComposerOptions struct {
	Scorers            []Scorer
	DemandShapeMapping DemandShapeResourceMapping
}

func NewScoreComposerWithOptions(options ScoreComposerOptions) *ScoreComposer {
	scorers := options.Scorers
	if len(scorers) == 0 {
		scorers = DefaultScorers()
	}
	mapping := options.DemandShapeMapping
	if len(mapping) == 0 {
		mapping = DefaultDemandShapeResourceMapping()
	}
	return &ScoreComposer{
		scorers:            scorers,
		demandShapeMapping: mapping,
	}
}

func DefaultScorers() []Scorer {
	return []Scorer{
		PreferredCoverageScorer{},
		KeyResourceHeadroomScorer{},
		TimeWindowScorer{},
		WorkloadMixScorer{},
		SLORiskScorer{},
	}
}

func (c *ScoreComposer) BuildDetail(profile *astrav1alpha1.AIWorkloadProfile, node *astrav1alpha1.AINodeResourceProfile) NodeScoreDetail {
	detail := NodeScoreDetail{}
	if node != nil {
		detail.NodeName = node.Spec.NodeName
		if detail.NodeName == "" {
			detail.NodeName = node.Name
		}
	}

	if profile == nil || node == nil || node.Status.Available == nil {
		detail.Reason = "missing profile, node, or node availability"
		return detail
	}

	available := *node.Status.Available
	preferred := preferredResources(profile)
	key := string(c.demandShapeMapping.FocusedResource(profile.Spec.DemandShape))
	ctx := ScoringContext{
		Profile:   profile,
		Node:      node,
		Preferred: preferred,
		Available: available,
		Key:       key,
	}

	detail.TimeWindowMultiplier = 1.0
	detail.MixBalanceMultiplier = 1.0
	detail.ExtraMultiplier = 1.0

	var reasons []string
	for _, scorer := range c.scorers {
		contribution := scorer.Score(ctx)
		if contribution.Name == "" {
			contribution.Name = scorer.Name()
		}
		c.applyContribution(&detail, contribution)
		if contribution.Reason != "" {
			reasons = append(reasons, fmt.Sprintf("%s=%s", contribution.Name, contribution.Reason))
		}
	}

	detail.Reason = fmt.Sprintf(
		"preferredMatched=%d/%d keyResource=%s keyPreferredMatched=%t keyHeadroom=%d otherHeadroom=%d weightedHeadroom=%d timeMultiplier=%.2f timeReason=%s mixMultiplier=%.2f mixReason=%s runtimeRiskPenalty=%d extraScore=%d extraMultiplier=%.2f extraPenalty=%d contributions=[%s]",
		detail.PreferredMatched,
		detail.PreferredTotal,
		detail.KeyResource,
		detail.KeyResourcePreferredMatched,
		detail.KeyResourceHeadroom,
		detail.OtherHeadroomScore,
		detail.WeightedHeadroomScore,
		detail.TimeWindowMultiplier,
		detail.TimeWindowReason,
		detail.MixBalanceMultiplier,
		detail.MixBalanceReason,
		detail.RuntimeRiskPenalty,
		detail.ExtraScore,
		detail.ExtraMultiplier,
		detail.ExtraPenalty,
		strings.Join(reasons, "; "),
	)

	return detail
}

func (c *ScoreComposer) Normalize(details map[string]NodeScoreDetail) map[string]ScoreDecision {
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

	for nodeName, detail := range details {
		score := applyMultiplier(detail.WeightedHeadroomScore, detail.TimeWindowMultiplier)
		score = applyMultiplier(score, detail.MixBalanceMultiplier)
		score += detail.ExtraScore
		score = applyMultiplier(score, detail.ExtraMultiplier)
		reasonParts := []string{
			detail.Reason,
			fmt.Sprintf("maxPreferredMatched=%d", maxPreferredMatched),
			fmt.Sprintf("weightedHeadroomScore=%d", detail.WeightedHeadroomScore),
			fmt.Sprintf("adjustedScore=%d", score),
		}

		if detail.PreferredMatched < maxPreferredMatched {
			score = score / 2
			reasonParts = append(reasonParts, "belowBestPreferredTier=true")
		}

		totalPenalty := detail.RuntimeRiskPenalty + detail.ExtraPenalty
		score -= totalPenalty
		if totalPenalty > 0 {
			reasonParts = append(reasonParts, fmt.Sprintf("totalPenalty=%d", totalPenalty))
		}

		decisions[nodeName] = ScoreDecision{
			Score:  clampScore(score),
			Reason: strings.Join(reasonParts, "; "),
		}
	}

	return decisions
}

func (c *ScoreComposer) applyContribution(detail *NodeScoreDetail, contribution ScoreContribution) {
	switch contribution.Name {
	case PreferredCoverageScorer{}.Name():
		detail.PreferredMatched = int(contribution.Score)
		detail.PreferredTotal = int(contribution.Penalty)
	case KeyResourceHeadroomScorer{}.Name():
		detail.KeyResource = contribution.Reason
		detail.KeyResourcePreferredMatched = contribution.Multiplier > 0
		detail.KeyResourceHeadroom = int32(contribution.Penalty)
		detail.WeightedHeadroomScore = contribution.Score
		detail.OtherHeadroomScore = contribution.OtherScore
	case TimeWindowScorer{}.Name():
		detail.TimeWindowMultiplier = contribution.Multiplier
		detail.TimeWindowReason = contribution.Reason
	case WorkloadMixScorer{}.Name():
		detail.MixBalanceMultiplier = contribution.Multiplier
		detail.MixBalanceReason = contribution.Reason
	case SLORiskScorer{}.Name():
		detail.RuntimeRiskPenalty = contribution.Penalty
	default:
		detail.ExtraScore += contribution.Score
		if contribution.Multiplier > 0 {
			if detail.ExtraMultiplier <= 0 {
				detail.ExtraMultiplier = 1.0
			}
			detail.ExtraMultiplier *= contribution.Multiplier
		}
		detail.ExtraPenalty += contribution.Penalty
	}
}

type PreferredCoverageScorer struct{}

func (PreferredCoverageScorer) Name() string {
	return "PreferredCoverageScorer"
}

func (PreferredCoverageScorer) Score(ctx ScoringContext) ScoreContribution {
	matched, total := preferredMatchCount(ctx.Preferred, ctx.Available)
	return ScoreContribution{
		Name:    PreferredCoverageScorer{}.Name(),
		Score:   int64(matched),
		Penalty: int64(total),
		Reason:  fmt.Sprintf("preferredMatched=%d/%d", matched, total),
	}
}

type KeyResourceHeadroomScorer struct{}

func (KeyResourceHeadroomScorer) Name() string {
	return "KeyResourceHeadroomScorer"
}

func (KeyResourceHeadroomScorer) Score(ctx ScoringContext) ScoreContribution {
	matched := keyResourceMatched(ctx.Key, ctx.Preferred, ctx.Available)
	headroom := keyResourceHeadroom(ctx.Key, ctx.Preferred, ctx.Available)
	other := otherHeadroomScore(ctx.Key, ctx.Preferred, ctx.Available)
	weighted := weightedHeadroomScore(ctx.Key, ctx.Preferred, ctx.Available)
	if ctx.Key == "" {
		ctx.Key = string(astrav1alpha1.ResourceBalancedResourceFocus)
	}
	return ScoreContribution{
		Name:       KeyResourceHeadroomScorer{}.Name(),
		Score:      weighted,
		OtherScore: other,
		Multiplier: boolMultiplier(matched),
		Penalty:    int64(headroom),
		Reason:     ctx.Key,
	}
}

type TimeWindowScorer struct{}

func (TimeWindowScorer) Name() string {
	return "TimeWindowScorer"
}

func (TimeWindowScorer) Score(ctx ScoringContext) ScoreContribution {
	multiplier, reason := timeWindowMultiplier(ctx.Profile, ctx.Node)
	return ScoreContribution{
		Name:       TimeWindowScorer{}.Name(),
		Multiplier: multiplier,
		Reason:     reason,
	}
}

type WorkloadMixScorer struct{}

func (WorkloadMixScorer) Name() string {
	return "WorkloadMixScorer"
}

func (WorkloadMixScorer) Score(ctx ScoringContext) ScoreContribution {
	multiplier, reason := mixBalanceMultiplier(ctx.Profile, ctx.Node)
	return ScoreContribution{
		Name:       WorkloadMixScorer{}.Name(),
		Multiplier: multiplier,
		Reason:     reason,
	}
}

type SLORiskScorer struct{}

func (SLORiskScorer) Name() string {
	return "SLORiskScorer"
}

func (SLORiskScorer) Score(ctx ScoringContext) ScoreContribution {
	penalty := runtimeRiskPenalty(ctx.Node)
	return ScoreContribution{
		Name:    SLORiskScorer{}.Name(),
		Penalty: penalty,
		Reason:  fmt.Sprintf("runtimeRiskPenalty=%d", penalty),
	}
}

func boolMultiplier(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
