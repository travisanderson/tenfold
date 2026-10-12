package pipeline

import (
	"context"
	"time"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/detect"
	"tenfold/backend/internal/model"
)

// Judge gives the fast AI decision on one anomaly. It sees the anomaly, the
// group it belongs to and a sample of the events behind it. It cannot
// change the risk score.
type Judge interface {
	Decide(ctx context.Context, in JudgeInput) (api.Decision, error)
}

// JudgeInput is what a judge is shown.
type JudgeInput struct {
	Anomaly  model.Anomaly
	Group    detect.Group
	Evidence []model.Event // a sample of the anomaly's events, in time order
}

// Narrator writes the account of one incident. Every sentence must cite
// events from the incident's evidence; the pipeline checks.
type Narrator interface {
	Narrate(ctx context.Context, in NarrateInput) (Draft, error)
	Provider() api.DecisionProvider
}

// NarrateInput is what a narrator is shown.
type NarrateInput struct {
	Incident  model.Incident
	Anomalies []model.Anomaly
	Evidence  []model.Event // a sample of the incident's events, in time order
}

// Draft is a narrator's output, before its citations are checked.
type Draft struct {
	Summary   string
	Sentences []api.NarrativeSentence
	NextSteps []string
}

// AI is the set of providers a job uses. Judge and Narrator both nil means
// no AI: the job still scores, groups and reports, and says the AI did not
// run.
//
// Shadow is an optional second judge. Its decisions are saved beside the
// judge's for the scorecard to compare. They never change a result, and a
// shadow that fails is skipped.
type AI struct {
	Judge    Judge
	Narrator Narrator
	Shadow   Judge
}

// NewAI returns the offline providers for a name: "mock" or "none". The
// server picks a provider for each role instead; see cmd/api.
func NewAI(provider string) AI {
	if provider == "mock" {
		return AI{Judge: MockJudge{}, Narrator: MockNarrator{}}
	}
	return AI{}
}

// Mock score bands. A stand-in has to decide somehow; it reads the group's
// risk score and nothing else.
const (
	mockMaliciousAt    = 65
	mockInconclusiveAt = 40
)

// MockJudge is the offline stand-in for the decision model, for tests and
// demos. It labels by the group's risk score alone, so its answers are
// repeatable and cost nothing.
type MockJudge struct{}

func (MockJudge) Decide(_ context.Context, in JudgeInput) (api.Decision, error) {
	started := time.Now()
	d := api.Decision{Provider: api.DecisionProviderMock}
	switch score := in.Group.RiskScore; {
	case score >= mockMaliciousAt:
		d.Label, d.Confidence = api.AiVerdictLikelyMalicious, 0.9
	case score >= mockInconclusiveAt:
		d.Label, d.Confidence = api.AiVerdictInconclusive, 0.6
	default:
		d.Label, d.Confidence = api.AiVerdictLikelyBenign, 0.7
	}
	d.LatencyMs = int(time.Since(started).Milliseconds())
	return d, nil
}

// MockNarrator is the offline stand-in for the narrative model. It returns
// the same deterministic text as the template fallback.
type MockNarrator struct{}

func (MockNarrator) Provider() api.DecisionProvider { return api.DecisionProviderMock }

func (MockNarrator) Narrate(_ context.Context, in NarrateInput) (Draft, error) {
	return templateDraft(in), nil
}

// groupVerdict is the AI's reading of a whole group: the most serious
// label any of its anomalies received. The second result is false when no
// anomaly in the group has a decision.
func groupVerdict(g detect.Group) (api.AiVerdict, bool) {
	rank := map[api.AiVerdict]int{
		api.AiVerdictLikelyBenign:    1,
		api.AiVerdictInconclusive:    2,
		api.AiVerdictLikelyMalicious: 3,
	}
	var verdict api.AiVerdict
	for _, a := range g.Anomalies {
		if a.Decision != nil && rank[a.Decision.Label] > rank[verdict] {
			verdict = a.Decision.Label
		}
	}
	return verdict, verdict != ""
}
