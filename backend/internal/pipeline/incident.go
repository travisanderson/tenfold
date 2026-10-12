package pipeline

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/detect"
	"tenfold/backend/internal/model"
)

// Limits on what an incident carries. They keep a payload small enough to
// draw; the events themselves stay one query away.
const (
	maxGraphDomains    = 8
	maxNeighbours      = 5
	maxNeighbourEvents = 50
	chartPadding       = 10 * time.Minute
	dataNodeMinBytes   = 1 << 20 // the graph shows a "bytes out" node from 1 MB
)

// detectorRank picks an incident's lead detector when several fired: the
// one that says the most about what happened.
var detectorRank = map[api.DetectorType]int{
	api.DetectorTypeThreatHit:       0,
	api.DetectorTypeExfilVolume:     1,
	api.DetectorTypeBeaconing:       2,
	api.DetectorTypeBlockedBurst:    3,
	api.DetectorTypeNewSourceIp:     4,
	api.DetectorTypeRareDestination: 5,
	api.DetectorTypeOffHours:        6,
}

// fileView is what the incident builder needs to know about the whole file.
type fileView struct {
	byLine map[int]model.Event
	byUser map[string][]model.Event // time order
	counts map[string]int           // "user:x", "host:x", "domain:x", "ip:x" -> events in the upload
}

func newFileView(events []model.Event) *fileView {
	v := &fileView{
		byLine: make(map[int]model.Event, len(events)),
		byUser: map[string][]model.Event{},
		counts: map[string]int{},
	}
	for _, e := range events {
		v.byLine[e.LineNo] = e
		v.byUser[e.User] = append(v.byUser[e.User], e)
		v.counts["user:"+e.User]++
		v.counts["domain:"+e.Host]++
		v.counts["ip:"+e.SrcIP]++
		if e.Device != "" {
			v.counts["host:"+e.Device]++
		}
	}
	return v
}

// buildIncident turns one group of anomalies into an incident. Everything
// here is a fixed template over measured values; the narrative is added
// later, by the narrating stage.
func buildIncident(uploadID, userID uuid.UUID, g detect.Group, file *fileView) model.Incident {
	evidence := make([]model.Event, 0, len(g.LineNos))
	for _, n := range g.LineNos {
		if e, ok := file.byLine[n]; ok {
			evidence = append(evidence, e)
		}
	}
	sort.SliceStable(evidence, func(i, j int) bool { return evidence[i].TS.Before(evidence[j].TS) })

	lead := leadAnomaly(g)
	user := lead.User
	device := mostCommon(evidence, func(e model.Event) string { return e.Device })

	inc := model.Incident{
		ID:            uuid.New(),
		UploadID:      uploadID,
		UserID:        userID,
		Title:         title(lead, evidence),
		Severity:      string(detect.Severity(g.RiskScore)),
		RiskScore:     g.RiskScore,
		PrincipalUser: optional(user),
		PrincipalHost: optional(device),
		TimeStart:     g.Start,
		TimeEnd:       g.End,
		EventCount:    len(evidence),
		AnomalyCount:  len(g.Anomalies),
		Mitre:         mitre(g, evidence),
		LineNos:       g.LineNos,
		AnalystStatus: string(api.AnalystStatusOpen),
	}
	inc.MarkerLabel, inc.MarkerDetail = marker(g.Signals[0])
	if verdict, ok := groupVerdict(g); ok {
		inc.AIVerdict = ptr(string(verdict))
	}

	detectors := map[api.DetectorType]bool{}
	for _, a := range g.Anomalies {
		detectors[api.DetectorType(a.Detector)] = true
	}
	inc.Detail = model.IncidentDetail{
		Narrative:         unavailableNarrative(),
		Signals:           g.Signals,
		Timeline:          timeline(g, file),
		Graph:             graph(g, evidence, file),
		Indicators:        indicators(evidence, file),
		Chart:             chart(g, user, file),
		LessonSuggestions: lessonSuggestions(user, evidence),
	}
	for d := range detectors {
		inc.Detail.Detectors = append(inc.Detail.Detectors, d)
	}
	sort.Slice(inc.Detail.Detectors, func(i, j int) bool {
		return detectorRank[inc.Detail.Detectors[i]] < detectorRank[inc.Detail.Detectors[j]]
	})
	return inc
}

// leadAnomaly is the anomaly an incident is named after.
func leadAnomaly(g detect.Group) *model.Anomaly {
	lead := g.Anomalies[0]
	for _, a := range g.Anomalies[1:] {
		if detectorRank[api.DetectorType(a.Detector)] < detectorRank[api.DetectorType(lead.Detector)] {
			lead = a
		}
	}
	return lead
}

// title names the incident for what was observed. It never states intent:
// a large upload is a "large outbound transfer", not "exfiltration".
func title(lead *model.Anomaly, evidence []model.Event) string {
	switch api.DetectorType(lead.Detector) {
	case api.DetectorTypeThreatHit:
		return "Request to a site the proxy flagged: " + lead.Host
	case api.DetectorTypeExfilVolume:
		return "Large outbound transfer to " + lead.Host
	case api.DetectorTypeBeaconing:
		return "Beacon-like requests to " + lead.Host
	case api.DetectorTypeBlockedBurst:
		category := mostCommon(evidence, func(e model.Event) string { return e.Category })
		return fmt.Sprintf("Burst of blocked requests (%s)", category)
	case api.DetectorTypeNewSourceIp:
		return "Requests from an unusual address for " + lead.User
	case api.DetectorTypeRareDestination:
		return "Requests to a host only one user reached: " + lead.Host
	default:
		return "Large transfer in quiet hours by " + lead.User
	}
}

// marker is the short callout on the triage timeline, taken from the
// incident's heaviest signal.
func marker(s api.Signal) (label string, detail *string) {
	switch s.Name {
	case detect.SignalBytesOut, detect.SignalOffHours:
		label = s.Observed + " out"
	default:
		label = s.Observed
	}
	if s.Name == detect.SignalBytesOut && s.Value != nil && s.Baseline != nil && *s.Baseline > 0 {
		ratio := *s.Value / *s.Baseline
		if ratio >= 1000 {
			return label, ptr("over 1,000× median")
		}
		return label, ptr(fmt.Sprintf("%.0f× median", ratio))
	}
	return label, s.ObservedDetail
}

// mitre lists the ATT&CK techniques the evidence could fit. The UI shows
// each with the word "Possible": a mapping is a reading, not a finding.
func mitre(g detect.Group, evidence []model.Event) []api.MitreTechnique {
	techniques := []api.MitreTechnique{}
	add := func(id, name, tactic string) {
		for _, t := range techniques {
			if t.Id == id {
				return
			}
		}
		techniques = append(techniques, api.MitreTechnique{Id: id, Name: name, Tactic: tactic})
	}
	for _, a := range g.Anomalies {
		switch api.DetectorType(a.Detector) {
		case api.DetectorTypeExfilVolume:
			add("T1567", "Exfiltration Over Web Service", "Exfiltration")
		case api.DetectorTypeBeaconing:
			add("T1071.001", "Application Layer Protocol: Web Protocols", "Command and Control")
		case api.DetectorTypeNewSourceIp:
			add("T1078", "Valid Accounts", "Initial Access")
		case api.DetectorTypeThreatHit:
			for _, e := range evidence {
				if strings.Contains(strings.ToLower(e.ThreatName), "phish") {
					add("T1566.002", "Phishing: Spearphishing Link", "Initial Access")
				}
			}
		case api.DetectorTypeBlockedBurst:
			if mostCommon(evidence, func(e model.Event) string { return e.Category }) == "Anonymizer" {
				add("T1090", "Proxy", "Command and Control")
			}
		}
	}
	return techniques
}

// timeline picks the moments worth reading in order: where each anomaly
// starts, and where the big ones end.
func timeline(g detect.Group, file *fileView) []api.TimelineItem {
	items := []api.TimelineItem{}
	seen := map[string]bool{}
	add := func(e model.Event, label, kind string) {
		key := model.EventID(e.LineNo) + kind
		if seen[key] {
			return
		}
		seen[key] = true
		items = append(items, api.TimelineItem{Ts: e.TS, EventId: model.EventID(e.LineNo), Label: label, Kind: kind})
	}
	for _, a := range g.Anomalies {
		first, ok := file.byLine[a.LineNos[0]]
		if !ok {
			continue
		}
		last := file.byLine[a.LineNos[len(a.LineNos)-1]]
		for _, n := range a.LineNos { // line order is not always time order
			if e := file.byLine[n]; e.TS.Before(first.TS) {
				first = e
			} else if e.TS.After(last.TS) {
				last = e
			}
		}
		observed := a.Signals[0].Observed
		switch api.DetectorType(a.Detector) {
		case api.DetectorTypeNewSourceIp:
			add(first, "First request from "+first.SrcIP, "new_source")
		case api.DetectorTypeRareDestination:
			add(first, "First request to "+a.Host, "first_seen")
		case api.DetectorTypeExfilVolume:
			add(first, fmt.Sprintf("Transfer to %s begins (%s)", a.Host, detect.FormatBytes(float64(first.BytesOut))), "transfer")
			add(last, fmt.Sprintf("Transfer ends; %s sent in all", observed), "transfer")
		case api.DetectorTypeBeaconing:
			add(first, fmt.Sprintf("Requests to %s begin, %s", a.Host, observed), "beacon")
			add(last, "Last request in the rhythm", "beacon")
		case api.DetectorTypeThreatHit:
			add(first, fmt.Sprintf("Proxy tagged a request to %s as %s (%s)", a.Host, first.ThreatName, first.Action), "threat")
		case api.DetectorTypeBlockedBurst:
			add(first, "Burst of blocked requests begins", "blocked")
			add(last, fmt.Sprintf("Burst ends; %s", observed), "blocked")
		case api.DetectorTypeOffHours:
			add(first, "Large transfers begin in quiet hours", "off_hours")
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Ts.Before(items[j].Ts) })
	return items
}

// graph draws who talked to what: user, source address, destinations and,
// for a big transfer, the bytes that left. Positions run left to right in
// four columns, so the UI needs no layout engine.
func graph(g detect.Group, evidence []model.Event, file *fileView) api.IncidentGraph {
	const colUser, colIP, colDomain, colData = 0, 260, 520, 780
	const rowHeight = 90

	nodes := map[string]*api.GraphNode{}
	edges := map[string]*api.GraphEdge{}
	var nodeOrder, edgeOrder []string

	node := func(id string, kind api.GraphNodeType, label string, neighbour bool) *api.GraphNode {
		if nodes[id] == nil {
			nodes[id] = &api.GraphNode{Id: id, Type: kind, Label: label, Neighbour: neighbour, EventIds: []string{}}
			nodeOrder = append(nodeOrder, id)
		}
		return nodes[id]
	}
	edge := func(source, target string, e model.Event, withBytes bool) {
		id := source + ">" + target
		if edges[id] == nil {
			edges[id] = &api.GraphEdge{Id: id, Source: source, Target: target, EventIds: []string{}}
			edgeOrder = append(edgeOrder, id)
		}
		ed := edges[id]
		ed.EventCount++
		ed.EventIds = append(ed.EventIds, model.EventID(e.LineNo))
		if withBytes {
			ed.Bytes = ptr(orZero(ed.Bytes) + e.BytesOut)
		}
	}
	touch := func(n *api.GraphNode, e model.Event) {
		n.EventIds = append(n.EventIds, model.EventID(e.LineNo))
	}

	// Which entities do the signals name?
	flaggedHosts, flaggedIPs := map[string]bool{}, map[string]bool{}
	for _, a := range g.Anomalies {
		switch api.DetectorType(a.Detector) {
		case api.DetectorTypeNewSourceIp:
			if e, ok := file.byLine[a.LineNos[0]]; ok {
				flaggedIPs[e.SrcIP] = true
			}
		case api.DetectorTypeOffHours:
			// names a time, not an entity
		default:
			for _, h := range a.Hosts {
				flaggedHosts[h] = true
			}
		}
	}

	// Keep the busiest destinations when there are too many to draw.
	perHost := map[string]int{}
	for _, e := range evidence {
		perHost[e.Host]++
	}
	keep := topKeys(perHost, maxGraphDomains)

	var totalOut int64
	for _, e := range evidence {
		if !keep[e.Host] {
			continue
		}
		userID, ipID, domainID := "user:"+e.User, "ip:"+e.SrcIP, "domain:"+e.Host
		touch(node(userID, api.GraphNodeTypeUser, e.User, false), e)
		ip := node(ipID, api.GraphNodeTypeIp, e.SrcIP, false)
		ip.Flagged = flaggedIPs[e.SrcIP]
		if e.Device != "" {
			ip.Sublabel = ptr(e.Device)
		}
		touch(ip, e)
		domain := node(domainID, api.GraphNodeTypeDomain, e.Host, false)
		domain.Flagged = flaggedHosts[e.Host]
		domain.Sublabel = ptr(domainSublabel(e, file))
		touch(domain, e)

		edge(userID, ipID, e, false)
		edge(ipID, domainID, e, true)
		totalOut += e.BytesOut
	}

	if totalOut >= dataNodeMinBytes {
		data := node("data:bytes_out", api.GraphNodeTypeData, detect.FormatBytes(float64(totalOut))+" out", false)
		data.Sublabel = ptr("total sent")
		data.Flagged = true
		for _, e := range evidence {
			if keep[e.Host] && e.BytesOut > 0 {
				touch(data, e)
				edge("domain:"+e.Host, "data:bytes_out", e, true)
			}
		}
	}

	// Neighbours: where else this user went during the incident. Context
	// only; the UI hides them until the analyst asks.
	inEvidence := map[int]bool{}
	for _, e := range evidence {
		inEvidence[e.LineNo] = true
	}
	lead := leadAnomaly(g)
	var around []model.Event
	perNeighbour := map[string]int{}
	for _, e := range file.byUser[lead.User] {
		if e.TS.Before(g.Start) || e.TS.After(g.End) || inEvidence[e.LineNo] || perHost[e.Host] > 0 {
			continue
		}
		around = append(around, e)
		perNeighbour[e.Host]++
	}
	keepNeighbour := topKeys(perNeighbour, maxNeighbours)
	drawn := map[string]int{}
	for _, e := range around {
		if !keepNeighbour[e.Host] || drawn[e.Host] >= maxNeighbourEvents {
			continue
		}
		drawn[e.Host]++
		ipID, domainID := "ip:"+e.SrcIP, "domain:"+e.Host
		if nodes[ipID] == nil {
			ip := node(ipID, api.GraphNodeTypeIp, e.SrcIP, true)
			if e.Device != "" {
				ip.Sublabel = ptr(e.Device)
			}
			edge("user:"+e.User, ipID, e, false)
		}
		domain := node(domainID, api.GraphNodeTypeDomain, e.Host, true)
		domain.Sublabel = ptr(domainSublabel(e, file))
		touch(domain, e)
		edge(ipID, domainID, e, true)
	}

	// Flag the connections between flagged entities, then lay out.
	out := api.IncidentGraph{Nodes: []api.GraphNode{}, Edges: []api.GraphEdge{}}
	rows := map[api.GraphNodeType]int{}
	columns := map[api.GraphNodeType]float64{
		api.GraphNodeTypeUser: colUser, api.GraphNodeTypeIp: colIP,
		api.GraphNodeTypeDomain: colDomain, api.GraphNodeTypeData: colData,
	}
	for _, id := range nodeOrder {
		n := nodes[id]
		n.X = ptr(columns[n.Type])
		n.Y = ptr(float64(rows[n.Type] * rowHeight))
		rows[n.Type]++
		out.Nodes = append(out.Nodes, *n)
	}
	for _, id := range edgeOrder {
		ed := edges[id]
		source, target := nodes[ed.Source], nodes[ed.Target]
		ed.Flagged = (source.Flagged || source.Type == api.GraphNodeTypeUser) && target.Flagged
		if first, err := model.ParseEventID(ed.EventIds[0]); err == nil {
			ed.Label = ptr(file.byLine[first].TS.Format("15:04") + " UTC")
		}
		out.Edges = append(out.Edges, *ed)
	}
	return out
}

// domainSublabel says what kind of place a destination is and how widely
// it is used: "Miscellaneous or Unknown · 21 requests in this file".
func domainSublabel(e model.Event, file *fileView) string {
	return fmt.Sprintf("%s · %s in this file", e.Category, pluralize(file.counts["domain:"+e.Host], "request"))
}

// indicators lists the entities worth copying or pivoting on, with how
// often each appears in the whole upload.
func indicators(evidence []model.Event, file *fileView) []api.Indicator {
	out := []api.Indicator{}
	seen := map[string]bool{}
	add := func(kind api.GraphNodeType, key, value string) {
		if value == "" || seen[key+value] {
			return
		}
		seen[key+value] = true
		out = append(out, api.Indicator{Type: kind, Value: value, EventCount: file.counts[key+value]})
	}
	for _, e := range evidence {
		add(api.GraphNodeTypeUser, "user:", e.User)
	}
	for _, e := range evidence {
		add(api.GraphNodeTypeHost, "host:", e.Device)
	}
	for _, e := range evidence {
		add(api.GraphNodeTypeIp, "ip:", e.SrcIP)
	}
	for _, e := range evidence {
		add(api.GraphNodeTypeDomain, "domain:", e.Host)
	}
	return out
}

// chart frames the bytes-out chart: a window around the incident, and the
// size of this user's typical request outside it.
func chart(g detect.Group, user string, file *fileView) api.IncidentChart {
	c := api.IncidentChart{Window: api.TimeRange{Start: g.Start.Add(-chartPadding), End: g.End.Add(chartPadding)}}

	inEvidence := map[int]bool{}
	for _, n := range g.LineNos {
		inEvidence[n] = true
	}
	var sizes []int64
	for _, e := range file.byUser[user] {
		if !inEvidence[e.LineNo] {
			sizes = append(sizes, e.BytesOut)
		}
	}
	if len(sizes) == 0 {
		return c // no baseline; the UI says so
	}
	sort.Slice(sizes, func(i, j int) bool { return sizes[i] < sizes[j] })
	typical := sizes[len(sizes)/2]
	c.BaselineBytesOut = &typical
	if typical < 100_000 {
		c.BaselineLabel = ptr("median: under 0.1 MB")
	} else {
		c.BaselineLabel = ptr("median: " + detect.FormatBytes(float64(typical)))
	}
	return c
}

// lessonSuggestions offers ready-made scopes for the "teach it once"
// dialog, narrowest first.
func lessonSuggestions(user string, evidence []model.Event) []api.LessonSuggestion {
	out := []api.LessonSuggestion{}
	perHost := map[string]int{}
	for _, e := range evidence {
		perHost[e.Host]++
	}
	host := topKey(perHost)
	if host != "" {
		out = append(out, api.LessonSuggestion{
			Scope: api.LessonScope{Type: api.LessonScopeTypeDomain, Value: host},
			Label: "requests to " + host,
		})
		// "s3.us-west.backblazeb2.com" suggests "*.backblazeb2.com".
		if labels := strings.Split(host, "."); len(labels) > 2 {
			parent := strings.Join(labels[len(labels)-2:], ".")
			out = append(out, api.LessonSuggestion{
				Scope: api.LessonScope{Type: api.LessonScopeTypePattern, Value: "*." + parent},
				Label: "requests to any host under " + parent,
			})
		}
	}
	if user != "" {
		out = append(out, api.LessonSuggestion{
			Scope: api.LessonScope{Type: api.LessonScopeTypeUser, Value: user},
			Label: "everything from " + user,
		})
	}
	return out
}

func unavailableNarrative() api.Narrative {
	return api.Narrative{
		Status:    api.NarrativeStatusUnavailable,
		Sentences: []api.NarrativeSentence{},
		NextSteps: []string{},
	}
}

// ---------- Small helpers ----------

func mostCommon(events []model.Event, field func(model.Event) string) string {
	counts := map[string]int{}
	for _, e := range events {
		if v := field(e); v != "" {
			counts[v]++
		}
	}
	return topKey(counts)
}

// topKey returns the key with the highest count; ties go to the key that
// sorts first, so the answer never depends on map order.
func topKey(counts map[string]int) string {
	best := ""
	for key, n := range counts {
		if best == "" || n > counts[best] || (n == counts[best] && key < best) {
			best = key
		}
	}
	return best
}

// topKeys returns the n keys with the highest counts.
func topKeys(counts map[string]int, n int) map[string]bool {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	keep := map[string]bool{}
	for _, k := range keys[:min(n, len(keys))] {
		keep[k] = true
	}
	return keep
}

// pluralize writes a count with its unit: "1 incident", "41 anomalies".
func pluralize(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	if stem, ok := strings.CutSuffix(unit, "y"); ok {
		return detect.FormatCount(n) + " " + stem + "ies"
	}
	return detect.FormatCount(n) + " " + unit + "s"
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// orZero reads through a pointer, giving the zero value for nil.
func orZero[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

func ptr[T any](v T) *T { return &v }
