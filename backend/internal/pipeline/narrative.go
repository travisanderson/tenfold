package pipeline

import (
	"fmt"
	"sort"
	"time"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/model"
)

// maxCitations caps the event IDs on one sentence. A beacon can rest on
// hundreds of requests; the first and last few are enough to check it.
const maxCitations = 200

// templateDraft writes an incident's account from fixed templates: one
// sentence per detector that fired, each citing the events it rests on.
// It is the fallback when no model runs or a model's citations fail the
// check, so it must always be true to the evidence.
func templateDraft(in NarrateInput) Draft {
	// One anomaly speaks for each detector: the one resting on most events.
	best := map[string]model.Anomaly{}
	for _, a := range in.Anomalies {
		if cur, ok := best[a.Detector]; !ok || len(a.LineNos) > len(cur.LineNos) {
			best[a.Detector] = a
		}
	}
	chosen := make([]model.Anomaly, 0, len(best))
	for _, a := range best {
		chosen = append(chosen, a)
	}
	sort.SliceStable(chosen, func(i, j int) bool {
		ri, rj := detectorRank[api.DetectorType(chosen[i].Detector)], detectorRank[api.DetectorType(chosen[j].Detector)]
		return ri < rj
	})

	var d Draft
	steps := map[string]bool{}
	addStep := func(step string) {
		if !steps[step] {
			steps[step] = true
			d.NextSteps = append(d.NextSteps, step)
		}
	}
	for _, a := range chosen {
		lines := a.LineNos
		if len(lines) > maxCitations {
			lines = lines[:maxCitations]
		}
		d.Sentences = append(d.Sentences, api.NarrativeSentence{Text: a.Reason, Citations: model.EventIDs(lines)})

		switch api.DetectorType(a.Detector) {
		case api.DetectorTypeExfilVolume:
			addStep(fmt.Sprintf("Ask %s whether the transfer to %s was expected.", a.User, a.Host))
			addStep(fmt.Sprintf("Check who else has reached %s in earlier uploads.", a.Host))
		case api.DetectorTypeBeaconing:
			addStep(fmt.Sprintf("Find the process on %s that calls %s.", orUser(a), a.Host))
		case api.DetectorTypeThreatHit:
			addStep(fmt.Sprintf("Confirm with %s that nothing was entered on %s.", a.User, a.Host))
		case api.DetectorTypeBlockedBurst:
			addStep(fmt.Sprintf("Ask %s what they were trying to reach.", a.User))
		case api.DetectorTypeNewSourceIp:
			addStep(fmt.Sprintf("Check whether %s was working from the unusual address.", a.User))
		}
	}
	if d.NextSteps == nil {
		d.NextSteps = []string{}
	}

	// The summary is the first sentence or two, as written.
	for i, s := range d.Sentences {
		if i == 2 {
			break
		}
		if i > 0 {
			d.Summary += " "
		}
		d.Summary += s.Text
	}
	return d
}

func orUser(a model.Anomaly) string {
	if a.Device != "" {
		return a.Device
	}
	return a.User + "'s machine"
}

// checkCitations reports whether every sentence cites at least one event
// and every cited event belongs to the incident's evidence. A model that
// cites an event outside the evidence has made something up.
func checkCitations(d Draft, evidence []int) error {
	allowed := make(map[string]bool, len(evidence))
	for _, n := range evidence {
		allowed[model.EventID(n)] = true
	}
	if len(d.Sentences) == 0 {
		return fmt.Errorf("the narrative has no sentences")
	}
	for i, s := range d.Sentences {
		if len(s.Citations) == 0 {
			return fmt.Errorf("sentence %d cites no events", i+1)
		}
		for _, id := range s.Citations {
			if !allowed[id] {
				return fmt.Errorf("sentence %d cites %s, which is not in the evidence", i+1, id)
			}
		}
	}
	return nil
}

// narrative turns a checked draft into the stored narrative.
func narrative(d Draft, status api.NarrativeStatus, provider api.DecisionProvider, at time.Time) api.Narrative {
	return api.Narrative{
		Status:      status,
		Provider:    &provider,
		Sentences:   d.Sentences,
		NextSteps:   d.NextSteps,
		GeneratedAt: &at,
	}
}
