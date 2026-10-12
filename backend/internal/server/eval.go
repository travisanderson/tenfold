package server

import (
	"context"
	"strings"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/samples"
)

// caughtShare is the scoring rule from the samples README: a planted
// attack counts as caught when one incident's evidence covers at least
// half of the attack's lines.
const caughtShare = 0.5

// GetEvalReport marks the incidents of a sample upload against the
// sample's answer key. It is arithmetic over stored results.
func (s *Server) GetEvalReport(ctx context.Context, req api.GetEvalReportRequestObject) (api.GetEvalReportResponseObject, error) {
	u, err := s.upload(ctx, req.UploadId)
	if err != nil {
		return nil, err
	}
	if u.SampleName == nil {
		return nil, notFound() // an analyst's own file has no answer key
	}
	key, err := s.samples.AnswerKey(ctx, *u.SampleName)
	if err != nil {
		return nil, err
	}
	incidents, err := s.store.ListIncidents(ctx, u.UserID, u.ID)
	if err != nil {
		return nil, err
	}
	anomalies, err := s.store.LoadAnomalies(ctx, u.ID)
	if err != nil {
		return nil, err
	}

	var attacks []samples.Scenario
	for _, scenario := range key.Scenarios {
		if scenario.Malicious() {
			attacks = append(attacks, scenario)
		}
	}

	report := api.EvalReport{
		UploadId:      u.ID,
		SampleName:    *u.SampleName,
		RunAt:         u.CreatedAt.UTC(),
		LineCount:     orZero(u.LineCount),
		AnomalyCount:  orZero(u.AnomalyCount),
		IncidentCount: len(incidents),
		Scoresheet:    []api.ScoresheetRow{},
		Providers:     []api.ProviderScore{},
	}
	if u.CompletedAt != nil {
		report.RunAt = u.CompletedAt.UTC()
	}

	// One row per incident raised.
	caught := make([]bool, len(attacks))
	for _, inc := range incidents {
		row := api.ScoresheetRow{
			Title:      inc.Title,
			Subtitle:   subtitle(orZero(inc.PrincipalUser), mitreIDs(inc.Mitre)),
			IncidentId: ptr(inc.ID),
			Layers:     layers(inc.Detail.Signals),
			RiskScore:  ptr(inc.RiskScore),
			AnswerKey:  api.AnswerKeyLabelBenign,
			Mark:       api.ScoreMarkFalsePositive,
		}
		if inc.AIVerdict != nil {
			row.AiVerdict = ptr(api.AiVerdict(*inc.AIVerdict))
		}
		for i, attack := range attacks {
			if coverage(inc.LineNos, attack.LineNos) >= caughtShare {
				caught[i] = true
				row.AnswerKey, row.Mark = api.AnswerKeyLabelAttack, api.ScoreMarkCaught
			}
		}
		if row.Mark == api.ScoreMarkFalsePositive {
			report.FalsePositives++
		}
		report.Scoresheet = append(report.Scoresheet, row)
	}
	// Then one row per planted attack that no incident caught.
	for i, attack := range attacks {
		if caught[i] {
			report.TruePositives++
			continue
		}
		report.FalseNegatives++
		report.Scoresheet = append(report.Scoresheet, api.ScoresheetRow{
			Title:     humanize(attack.Type),
			Subtitle:  subtitle(first(attack.Users), attack.Mitre),
			Layers:    []api.SignalLayer{},
			AnswerKey: api.AnswerKeyLabelAttack,
			Mark:      api.ScoreMarkMissed,
		})
	}
	report.Precision = ratio(report.TruePositives, report.TruePositives+report.FalsePositives)
	report.Recall = ratio(report.TruePositives, len(attacks))

	// How the AI judges did, anomaly by anomaly.
	attackLines := map[int]bool{}
	for _, attack := range attacks {
		for _, n := range attack.LineNos {
			attackLines[n] = true
		}
	}
	inUse, shadow := map[api.DecisionProvider]*tally{}, map[api.DecisionProvider]*tally{}
	for _, a := range anomalies {
		isAttack := coverageOf(a.LineNos, attackLines) >= caughtShare
		if a.Decision != nil {
			count(inUse, *a.Decision, isAttack)
		}
		if a.ShadowDecision != nil {
			count(shadow, *a.ShadowDecision, isAttack)
		}
		if a.Decision != nil && a.ShadowDecision != nil {
			report.JudgeAgreement.Total++
			if a.Decision.Label == a.ShadowDecision.Label {
				report.JudgeAgreement.Agreed++
			}
		}
	}
	for role, tallies := range map[api.ProviderRole]map[api.DecisionProvider]*tally{
		api.ProviderRoleInUse: inUse, api.ProviderRoleShadow: shadow,
	} {
		for provider, t := range tallies {
			score := api.ProviderScore{
				Provider: provider, Label: providerLabels[provider], Role: role,
				Decisions: t.decisions, AttacksTotal: len(attacks),
				AgreementWithAnswerKey: ratio(t.agreed, t.decisions),
				AvgLatencyMs:           float64(t.latencyMs) / float64(t.decisions),
				CostPer1kUsd:           providerCostPer1k[provider],
			}
			// Only the provider in use decided which incidents exist, so
			// only it can be scored on them.
			if role == api.ProviderRoleInUse {
				score.AttacksCaught, score.FalsePositives = report.TruePositives, report.FalsePositives
			}
			report.Providers = append(report.Providers, score)
		}
	}
	return api.GetEvalReport200JSONResponse(report), nil
}

var providerLabels = map[api.DecisionProvider]string{
	api.DecisionProviderMock:   "Offline stand-in",
	api.DecisionProviderRules:  "Rules only",
	api.DecisionProviderClaude: "Claude",
	api.DecisionProviderJev:    "Fast typed-decision model",
}

// providerCostPer1k is the cost in US dollars per 1,000 decisions. The
// stand-ins cost nothing; real providers fill this in when they arrive.
var providerCostPer1k = map[api.DecisionProvider]float64{}

// tally adds up one provider's decisions.
type tally struct {
	decisions, agreed, latencyMs int
}

// count adds one decision to its provider's tally. A decision agrees with
// the answer key when it does not contradict it: "inconclusive" sends the
// anomaly to a person either way, so it never counts as wrong.
func count(tallies map[api.DecisionProvider]*tally, d api.Decision, isAttack bool) {
	t := tallies[d.Provider]
	if t == nil {
		t = &tally{}
		tallies[d.Provider] = t
	}
	t.decisions++
	t.latencyMs += d.LatencyMs
	wrong := (isAttack && d.Label == api.AiVerdictLikelyBenign) || (!isAttack && d.Label == api.AiVerdictLikelyMalicious)
	if !wrong {
		t.agreed++
	}
}

// coverage is the share of an attack's lines found in some evidence.
func coverage(evidence, attack []int) float64 {
	in := make(map[int]bool, len(evidence))
	for _, n := range evidence {
		in[n] = true
	}
	return coverageOf(attack, in)
}

// coverageOf is the share of lines that are in the set.
func coverageOf(lines []int, set map[int]bool) float64 {
	if len(lines) == 0 {
		return 0
	}
	hit := 0
	for _, n := range lines {
		if set[n] {
			hit++
		}
	}
	return float64(hit) / float64(len(lines))
}

// ratio divides, and calls 0 of 0 a perfect score: nothing was asked, so
// nothing was missed.
func ratio(part, whole int) float64 {
	if whole == 0 {
		return 1
	}
	return float64(part) / float64(whole)
}

func layers(signals []api.Signal) []api.SignalLayer {
	out := []api.SignalLayer{}
	seen := map[api.SignalLayer]bool{}
	for _, s := range signals {
		if !seen[s.Layer] {
			seen[s.Layer] = true
			out = append(out, s.Layer)
		}
	}
	return out
}

func mitreIDs(techniques []api.MitreTechnique) []string {
	ids := make([]string, len(techniques))
	for i, t := range techniques {
		ids[i] = t.Id
	}
	return ids
}

// subtitle is "dana.k · T1567", or whichever half is known.
func subtitle(user string, mitre []string) string {
	parts := []string{}
	if user != "" {
		parts = append(parts, user)
	}
	if len(mitre) > 0 {
		parts = append(parts, mitre[0])
	}
	return strings.Join(parts, " · ")
}

// humanize turns "policy_evasion_burst" into "Policy evasion burst".
func humanize(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
