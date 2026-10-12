// Package detect turns events into anomalies. Everything here is
// deterministic: fixed rules and statistics measured from the file itself.
// No model runs in this package, and nothing here talks to the database.
//
// This is the first pass. Each detector is one small function with its
// thresholds named at the top of its file, so they are easy to find and tune.
package detect

import (
	"sort"
	"time"

	"github.com/google/uuid"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/model"
)

// Weights are the points each signal adds to a risk score. A risk score is
// the sum of the weights of the distinct signals that fired, capped at 100.
// It measures how unusual the activity is, not the odds of an attack.
const (
	WeightBytesOut        = 34 // outbound bytes far above the user's baseline
	WeightRegularFast     = 34 // requests to one host at a regular, fast interval
	WeightRegularSlow     = 20 // regular, but slow enough to be routine polling
	WeightSustained       = 16 // regular requests kept up for hours
	WeightThreatAllowed   = 60 // the proxy named a threat and let it through
	WeightThreatBlocked   = 35 // the proxy named a threat and blocked it
	WeightBlockedBurst    = 30 // many blocked requests in a short window
	WeightNewSourceIP     = 22 // requests from an address the user rarely uses
	WeightRareDestination = 19 // a host no other user reached
	WeightOffHours        = 16 // large transfers in the file's quiet hours
)

// Signal names. The grouping step adds up one weight per name.
const (
	SignalBytesOut        = "bytes_out_vs_baseline"
	SignalRegularInterval = "regular_interval"
	SignalSustained       = "sustained_polling"
	SignalThreat          = "proxy_threat"
	SignalBlockedBurst    = "blocked_burst"
	SignalNewSourceIP     = "new_source_ip"
	SignalRareDestination = "rare_destination"
	SignalOffHours        = "off_hours_transfer"
)

// Run runs every detector over an upload's events, which must be in time
// order. The anomalies it returns are pending: no lesson, threshold or
// judge has been applied yet.
func Run(uploadID uuid.UUID, events []model.Event) []model.Anomaly {
	if len(events) == 0 {
		return nil
	}
	p := newProfile(events)

	var found []model.Anomaly
	for _, detector := range []func(*profile) []model.Anomaly{
		threatHits,
		blockedBursts,
		exfilVolume,
		beaconing,
		rareDestinations,
		newSourceIPs,
		offHours,
	} {
		found = append(found, detector(p)...)
	}

	for i := range found {
		a := &found[i]
		a.ID = uuid.New()
		a.UploadID = uploadID
		a.Disposition = model.DispositionPending
		a.RiskScore = Score(a.Signals)
	}
	// A stable order, so two runs over the same file give the same list.
	sort.SliceStable(found, func(i, j int) bool {
		if !found[i].FirstSeen.Equal(found[j].FirstSeen) {
			return found[i].FirstSeen.Before(found[j].FirstSeen)
		}
		if found[i].Detector != found[j].Detector {
			return found[i].Detector < found[j].Detector
		}
		return found[i].LineNos[0] < found[j].LineNos[0]
	})
	return found
}

// Score adds up one weight per signal name, capped at 100. When the same
// signal fired more than once, only its heaviest instance counts.
func Score(signals []api.Signal) int {
	heaviest := map[string]int{}
	for _, s := range signals {
		if s.Weight > heaviest[s.Name] {
			heaviest[s.Name] = s.Weight
		}
	}
	total := 0
	for _, w := range heaviest {
		total += w
	}
	return min(total, 100)
}

// Severity maps a risk score to a severity. The bands are fixed.
func Severity(riskScore int) api.Severity {
	switch {
	case riskScore >= 95:
		return api.SeverityCritical
	case riskScore >= 65:
		return api.SeverityHigh
	case riskScore >= 40:
		return api.SeverityMedium
	default:
		return api.SeverityLow
	}
}

// anomaly builds one finding from the events it rests on, which must be in
// time order and not empty.
func anomaly(detector api.DetectorType, events []model.Event, reason, label string, signals ...api.Signal) model.Anomaly {
	a := model.Anomaly{
		Detector:  string(detector),
		Reason:    reason,
		Label:     label,
		Signals:   signals,
		User:      events[0].User,
		Device:    events[0].Device,
		FirstSeen: events[0].TS,
		LastSeen:  events[len(events)-1].TS,
	}
	// The main destination is the one with the most events.
	counts := map[string]int{}
	for _, e := range events {
		a.LineNos = append(a.LineNos, e.LineNo)
		if counts[e.Host] == 0 {
			a.Hosts = append(a.Hosts, e.Host)
		}
		counts[e.Host]++
		if counts[e.Host] > counts[a.Host] {
			a.Host = e.Host
		}
	}
	sort.Strings(a.Hosts)
	return a
}

// eventIDs lists the API IDs of some events.
func eventIDs(events []model.Event) []string {
	ids := make([]string, len(events))
	for i, e := range events {
		ids[i] = model.EventID(e.LineNo)
	}
	return ids
}

// span is the time between the first and last of some ordered events.
func span(events []model.Event) time.Duration {
	return events[len(events)-1].TS.Sub(events[0].TS)
}

func ptr[T any](v T) *T { return &v }
