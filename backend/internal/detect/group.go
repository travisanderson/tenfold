package detect

import (
	"path"
	"sort"
	"strings"
	"time"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/model"
)

// Group is a set of anomalies that rest on some of the same events, and so
// tell one story. A group that scores at or above the threshold can become
// an incident.
type Group struct {
	Anomalies []*model.Anomaly // in time order
	Signals   []api.Signal     // one per signal name, heaviest first
	RiskScore int
	LineNos   []int // every event behind the group, ascending
	Start     time.Time
	End       time.Time
}

// GroupAnomalies links anomalies that share at least one event. Two
// detectors that flag the same requests are describing the same activity;
// anomalies with no event in common stay apart, however close in time.
//
// It skips anomalies a lesson has set aside. The result is in time order.
func GroupAnomalies(anomalies []model.Anomaly) []Group {
	// Union-find over the anomalies, joined through the events they share.
	parent := make([]int, len(anomalies))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	firstWithEvent := map[int]int{}
	for i, a := range anomalies {
		if a.Disposition == model.DispositionLesson {
			continue
		}
		for _, lineNo := range a.LineNos {
			if j, seen := firstWithEvent[lineNo]; seen {
				parent[find(i)] = find(j)
			} else {
				firstWithEvent[lineNo] = i
			}
		}
	}

	members := map[int][]*model.Anomaly{}
	var roots []int
	for i := range anomalies {
		if anomalies[i].Disposition == model.DispositionLesson {
			continue
		}
		root := find(i)
		if members[root] == nil {
			roots = append(roots, root)
		}
		members[root] = append(members[root], &anomalies[i])
	}

	groups := make([]Group, 0, len(roots))
	for _, root := range roots {
		groups = append(groups, newGroup(members[root]))
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Start.Before(groups[j].Start) })
	return groups
}

func newGroup(anomalies []*model.Anomaly) Group {
	sort.SliceStable(anomalies, func(i, j int) bool { return anomalies[i].FirstSeen.Before(anomalies[j].FirstSeen) })
	g := Group{Anomalies: anomalies, Start: anomalies[0].FirstSeen, End: anomalies[0].LastSeen}

	lines := map[int]bool{}
	var all []api.Signal
	for _, a := range anomalies {
		all = append(all, a.Signals...)
		for _, n := range a.LineNos {
			lines[n] = true
		}
		if a.FirstSeen.Before(g.Start) {
			g.Start = a.FirstSeen
		}
		if a.LastSeen.After(g.End) {
			g.End = a.LastSeen
		}
	}
	for n := range lines {
		g.LineNos = append(g.LineNos, n)
	}
	sort.Ints(g.LineNos)
	g.Signals = MergeSignals(all)
	g.RiskScore = Score(g.Signals)
	return g
}

// MergeSignals keeps one signal per name: the heaviest, with the event IDs
// of all of them. The result is heaviest first, so the weights shown add up
// to the risk score.
func MergeSignals(signals []api.Signal) []api.Signal {
	index := map[string]int{}
	var merged []api.Signal
	for _, s := range signals {
		i, seen := index[s.Name]
		if !seen {
			index[s.Name] = len(merged)
			s.EventIds = append([]string(nil), s.EventIds...)
			merged = append(merged, s)
			continue
		}
		ids := unionIDs(merged[i].EventIds, s.EventIds)
		if s.Weight > merged[i].Weight {
			merged[i] = s
		}
		merged[i].EventIds = ids
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].Weight > merged[j].Weight })
	return merged
}

// unionIDs joins two lists of event IDs, in line order, without repeats.
func unionIDs(a, b []string) []string {
	seen := map[int]bool{}
	var lines []int
	for _, id := range append(append([]string(nil), a...), b...) {
		if n, err := model.ParseEventID(id); err == nil && !seen[n] {
			seen[n] = true
			lines = append(lines, n)
		}
	}
	sort.Ints(lines)
	return model.EventIDs(lines)
}

// MatchesLesson reports whether a lesson's scope covers an anomaly.
//
//   - user: the anomaly is about that user.
//   - domain: every host behind the anomaly is that domain or under it.
//   - pattern: every host matches the glob, such as "*.backup-vendor.com".
//
// "Every host" is deliberate. A lesson about one domain must not hide an
// anomaly that also touches another.
func MatchesLesson(a model.Anomaly, scopeType, scopeValue string) bool {
	value := strings.ToLower(strings.TrimSpace(scopeValue))
	switch api.LessonScopeType(scopeType) {
	case api.LessonScopeTypeUser:
		return strings.EqualFold(a.User, value)
	case api.LessonScopeTypeDomain:
		return allHosts(a, func(host string) bool {
			return host == value || strings.HasSuffix(host, "."+value)
		})
	case api.LessonScopeTypePattern:
		return allHosts(a, func(host string) bool {
			ok, err := path.Match(value, host)
			return err == nil && ok
		})
	}
	return false
}

func allHosts(a model.Anomaly, match func(string) bool) bool {
	if len(a.Hosts) == 0 {
		return false
	}
	for _, host := range a.Hosts {
		if !match(strings.ToLower(host)) {
			return false
		}
	}
	return true
}
