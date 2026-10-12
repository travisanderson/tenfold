package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"sort"
	"time"
)

const generatorVersion = "1"

// malformedRate is the share of lines deliberately broken.
const malformedRate = 0.001

// Profile is one sample file the generator can write.
type Profile struct {
	Name    string
	Lines   int // approximate; the answer key records the exact count
	Users   int
	Attacks bool
	// Holdout draws each scenario's users, times, sizes, hosts and intervals
	// from the seed, where the demo profiles replay one fixed script. It is
	// for scoring the detectors on a day they were not built against. --all
	// leaves it out, so it never joins the demo samples.
	Holdout bool
}

var profiles = []Profile{
	{Name: "clean-day", Lines: 30000, Users: 25, Attacks: false},
	{Name: "attack-day", Lines: 48000, Users: 25, Attacks: true},
	{Name: "large-100k", Lines: 100000, Users: 60, Attacks: true},
	{Name: "holdout", Lines: 48000, Users: 25, Attacks: true, Holdout: true},
}

// AnswerKey says exactly which lines belong to which scenario.
type AnswerKey struct {
	Sample           string     `json:"sample"`
	Seed             uint64     `json:"seed"`
	GeneratorVersion string     `json:"generatorVersion"`
	LineCount        int        `json:"lineCount"`
	TimeRange        TimeRange  `json:"timeRange"`
	MalformedLines   []int      `json:"malformedLines"`
	Scenarios        []Scenario `json:"scenarios"`
}

type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Scenario is one planted attack or benign decoy.
type Scenario struct {
	ID                string   `json:"id"`
	Label             string   `json:"label"` // malicious or benign
	Type              string   `json:"type"`
	ExpectedDetectors []string `json:"expectedDetectors"`
	Mitre             []string `json:"mitre"`
	Users             []string `json:"users"`
	Hosts             []string `json:"hosts"`
	Domains           []string `json:"domains"`
	Note              string   `json:"note,omitempty"` // what a holdout drew
	// Drawn holds a holdout's numbers for the scenario, for a scorecard to
	// group by. Times are in seconds.
	Drawn     map[string]float64 `json:"drawn,omitempty"`
	TimeRange TimeRange          `json:"timeRange"`
	LineNos   []int              `json:"lineNos"`
}

type event struct {
	rec      Record
	scenario int // index into gen.scenarios, or -1
	seq      int // insertion order; breaks ties when sorting by time
}

type gen struct {
	rng       *rand.Rand
	users     []user
	dayStart  time.Time // 00:00 Eastern, as UTC
	plan      script
	events    []event
	scenarios []Scenario
}

// Generate builds one sample. The same profile, seed and day always give the
// same bytes.
func Generate(p Profile, seed uint64, day time.Time) ([]byte, AnswerKey) {
	name := fnv.New64a()
	name.Write([]byte(p.Name))
	rng := rand.New(rand.NewPCG(seed, name.Sum64()))

	g := &gen{
		rng:   rng,
		users: buildUsers(rng, p.Users, !p.Holdout),
		plan:  demoScript(),
		// The data covers one Eastern business day. October is daylight
		// time, a fixed four hours behind UTC.
		dayStart: time.Date(day.Year(), day.Month(), day.Day(), 4, 0, 0, 0, time.UTC),
	}

	if p.Holdout {
		// The script has a random stream of its own, so it does not shift
		// with the traffic drawn around it.
		g.plan = holdoutScript(rand.New(rand.NewPCG(seed, name.Sum64()+1)), g.users)
	}

	g.decoyBackup()
	g.decoyDevPush()
	g.decoyUpdater()
	if p.Attacks {
		g.attackExfil()
		g.attackBeacon()
		g.attackPhish()
		g.attackBurst()
	}
	g.backgroundNoise()
	g.humanTraffic(p.Lines - len(g.events))

	sort.SliceStable(g.events, func(i, j int) bool {
		a, b := g.events[i], g.events[j]
		if !a.rec.Time.Equal(b.rec.Time) {
			return a.rec.Time.Before(b.rec.Time)
		}
		return a.seq < b.seq
	})

	malformed := g.pickMalformed()

	var out bytes.Buffer
	lineNos := make([][]int, len(g.scenarios))
	for i, e := range g.events {
		lineNo := i + 1
		e.rec.RecordID = int64(lineNo)
		line := e.rec.Format()
		if malformed[i] {
			line = g.corrupt(line)
		}
		out.WriteString(line)
		out.WriteByte('\n')
		if e.scenario >= 0 {
			lineNos[e.scenario] = append(lineNos[e.scenario], lineNo)
		}
	}

	key := AnswerKey{
		Sample:           p.Name,
		Seed:             seed,
		GeneratorVersion: generatorVersion,
		LineCount:        len(g.events),
		TimeRange:        TimeRange{Start: g.dayStart, End: g.dayStart.Add(24*time.Hour - time.Second)},
		MalformedLines:   []int{},
		Scenarios:        g.scenarios,
	}
	for i := range g.events {
		if malformed[i] {
			key.MalformedLines = append(key.MalformedLines, i+1)
		}
	}
	for i := range key.Scenarios {
		key.Scenarios[i].LineNos = lineNos[i]
		if p.Holdout {
			// The list describes the fixed script. A holdout promises no
			// detector anything.
			key.Scenarios[i].ExpectedDetectors = []string{}
		}
	}
	return out.Bytes(), key
}

// MarshalKey writes the answer key as indented JSON with a trailing newline.
func MarshalKey(key AnswerKey) []byte {
	b, err := json.MarshalIndent(key, "", "  ")
	if err != nil {
		panic(err) // the key holds only plain values
	}
	return append(b, '\n')
}

func (g *gen) user(name string) user {
	for _, u := range g.users {
		if u.name == name {
			return u
		}
	}
	panic("loggen: no user named " + name)
}

func (g *gen) add(rec Record, scenario int) {
	g.events = append(g.events, event{rec: rec, scenario: scenario, seq: len(g.events)})
}

// startScenario registers a scenario and returns its index. Its time range
// and line numbers are filled in from the events tagged with that index.
func (g *gen) startScenario(s Scenario) int {
	if s.Mitre == nil {
		s.Mitre = []string{}
	}
	g.scenarios = append(g.scenarios, s)
	return len(g.scenarios) - 1
}

// closeScenario sets the scenario's time range from its events.
func (g *gen) closeScenario(idx int) {
	var first, last time.Time
	for _, e := range g.events {
		if e.scenario != idx {
			continue
		}
		if first.IsZero() || e.rec.Time.Before(first) {
			first = e.rec.Time
		}
		if e.rec.Time.After(last) {
			last = e.rec.Time
		}
	}
	g.scenarios[idx].TimeRange = TimeRange{Start: first, End: last}
}

// base fills the fields every request from a user shares.
func (g *gen) base(u user, t time.Time, host, path string) Record {
	return Record{
		Time:           t,
		Login:          u.login(),
		Dept:           u.dept,
		ClientIP:       u.ip,
		ServerIP:       serverIP(host),
		Host:           host,
		URL:            "https://" + host + path,
		Action:         "Allowed",
		Reason:         "Allowed",
		Method:         "GET",
		Status:         200,
		UserAgent:      u.ua,
		AppName:        "General Browsing",
		AppClass:       "General Browsing",
		DeviceHostname: u.device,
	}
}

func (g *gen) jitter(seconds int) time.Duration {
	return time.Duration(g.rng.IntN(2*seconds+1)-seconds) * time.Second
}

// ---------- Normal traffic ----------

// dayShape is how busy each minute of the Eastern day is: a working day with
// a lunch dip, a light evening and a nearly silent night.
func dayShape(minute int) float64 {
	h := float64(minute) / 60
	switch {
	case h < 6:
		return 0.003
	case h < 7:
		return 0.05
	case h < 8.5:
		return 0.05 + 0.95*(h-7)/1.5
	case h < 12:
		return 1
	case h < 13:
		return 0.6
	case h < 17.5:
		return 1
	case h < 19:
		return 1 - 0.8*(h-17.5)/1.5
	case h < 23:
		return 0.1
	default:
		return 0.03
	}
}

func (g *gen) humanTraffic(total int) {
	shape := make([]float64, 24*60)
	sum := 0.0
	for m := range shape {
		sum += dayShape(m)
		shape[m] = sum
	}

	volume := 0.0
	for _, u := range g.users {
		if !u.service {
			volume += u.volume
		}
	}

	for _, u := range g.users {
		if u.service {
			continue
		}
		n := int(math.Round(float64(total) * u.volume / volume))
		for range n {
			minute := sort.SearchFloat64s(shape, g.rng.Float64()*sum) + u.shiftMin
			minute = min(max(minute, 0), 24*60-1)
			t := g.dayStart.Add(time.Duration(minute)*time.Minute + time.Duration(g.rng.IntN(60))*time.Second)

			i := sort.SearchFloat64s(u.destCum, g.rng.Float64()*u.destCum[len(u.destCum)-1])
			g.add(g.visit(u, t, catalog[i], u.ua), -1)
		}
	}
}

// visit builds one ordinary request to a catalog destination.
func (g *gen) visit(u user, t time.Time, d dest, ua string) Record {
	path := d.paths[g.rng.IntN(len(d.paths))]
	if len(path) > 1 && path[len(path)-1] == '/' {
		path += fmt.Sprintf("%x", g.rng.Uint32())
	}
	rec := g.base(u, t, d.host, path)
	rec.UserAgent = ua
	rec.URLCategory = d.cat
	rec.AppName = d.app
	rec.AppClass = d.class
	rec.RiskScore = d.risk + g.rng.IntN(5)

	switch {
	case d.blocked:
		rec.Action = "Blocked"
		rec.Reason = d.reason
		rec.Status = 403
		rec.ReqSize = around(g.rng, 650, 0.35)
		rec.RespSize = 0
	case g.rng.Float64() < d.upload:
		rec.Method = d.uploadMethod
		sigma := 0.5
		if d.uploadMedian >= 100000 {
			sigma = 1.1 // file uploads vary far more than form posts
		}
		rec.ReqSize = around(g.rng, d.uploadMedian*u.uploadMult, sigma)
		rec.RespSize = around(g.rng, 500, 0.5)
	default:
		rec.ReqSize = around(g.rng, 650, 0.35)
		rec.RespSize = around(g.rng, d.respMedian, 1.0)
		switch r := g.rng.Float64(); {
		case r < 0.04:
			rec.Status = 304
			rec.RespSize = around(g.rng, 250, 0.2)
		case r < 0.05:
			rec.Status = 404
			rec.RespSize = around(g.rng, 900, 0.3)
		}
	}
	return rec
}

// backgroundNoise adds the machine chatter every device makes around the
// clock: update checks, telemetry, chat keepalives.
func (g *gen) backgroundNoise() {
	for _, u := range g.users {
		if u.service {
			continue
		}
		n := 70 + g.rng.IntN(30)
		for range n {
			t := g.dayStart.Add(time.Duration(g.rng.IntN(24*60*60)) * time.Second)
			b := background[g.rng.IntN(len(background))]
			g.add(g.visit(u, t, b.dest, b.ua), -1)
		}
	}
}

// ---------- Benign decoys ----------

// decoyBackup: the nightly backup. In the demo, about 2 GB leaves in 8 MB
// chunks between 01:00 and 02:40. It looks like exfiltration by volume and hour.
func (g *gen) decoyBackup() {
	p := g.plan.backup
	u := g.user("svc-backup")
	idx := g.startScenario(Scenario{
		ID: "decoy-backup", Label: "benign", Type: "scheduled_backup",
		ExpectedDetectors: []string{"exfil_volume", "off_hours"},
		Users:             []string{u.name}, Hosts: []string{u.device}, Domains: []string{p.host}, Note: p.note,
	})
	date := g.dayStart.Format("2006-01-02")
	for i := range p.chunks {
		t := g.dayStart.Add(p.start + time.Duration(i)*p.gap + g.jitter(p.jitter))
		rec := g.base(u, t, p.host, fmt.Sprintf("/nwl-nightly/%s/part-%04d", date, i+1))
		rec.Method = "PUT"
		rec.URLCategory = "Online Storage"
		rec.AppName = p.app
		rec.AppClass = "File Share"
		rec.UserAgent = p.ua
		rec.ReqSize = around(g.rng, p.chunkBytes, 0.02)
		rec.RespSize = around(g.rng, 420, 0.1)
		rec.RiskScore = 22
		g.add(rec, idx)
	}
	g.closeScenario(idx)
}

// decoyDevPush: an engineer pushes a large repository. In the demo, about
// 300 MB mid-afternoon. It looks like a volume spike.
func (g *gen) decoyDevPush() {
	p := g.plan.push
	u := g.user(p.user)
	const host = "github.com"
	idx := g.startScenario(Scenario{
		ID: "decoy-dev-push", Label: "benign", Type: "large_code_push",
		ExpectedDetectors: []string{"exfil_volume"},
		Users:             []string{u.name}, Hosts: []string{u.device}, Domains: []string{host}, Note: p.note,
	})
	for i := range p.pushes {
		t := g.dayStart.Add(p.start + time.Duration(i)*p.gap + g.jitter(p.jitter))
		rec := g.base(u, t, host, "/northwindlabs-example/"+p.repo+".git/git-receive-pack")
		rec.Method = "POST"
		rec.URLCategory = "Professional Services"
		rec.AppName = "GitHub"
		rec.AppClass = "Business"
		rec.UserAgent = "git/2.46.0"
		rec.ReqSize = around(g.rng, p.chunkBytes, 0.1)
		rec.RespSize = around(g.rng, 380, 0.2)
		rec.RiskScore = 8
		g.add(rec, idx)
	}
	g.closeScenario(idx)
}

// decoyUpdater: a legitimate agent polls its vendor from every workstation,
// every 15 minutes in the demo. It looks like beaconing, but it is slow and
// company-wide.
func (g *gen) decoyUpdater() {
	p := g.plan.updater
	s := Scenario{
		ID: "decoy-updater", Label: "benign", Type: "software_updater",
		ExpectedDetectors: []string{"beaconing"},
		Users:             []string{}, Hosts: []string{}, Domains: []string{p.host}, Note: p.note, Drawn: p.drawn,
	}
	for _, u := range g.users {
		if !u.service {
			s.Users = append(s.Users, u.name)
			s.Hosts = append(s.Hosts, u.device)
		}
	}
	idx := g.startScenario(s)
	for _, u := range g.users {
		if u.service {
			continue
		}
		// Each workstation keeps its own beat, starting somewhere in the
		// first interval of the day.
		offset := time.Duration(30+g.rng.IntN(seconds(p.every)-60)) * time.Second
		for i := range int(24 * time.Hour / p.every) {
			t := g.dayStart.Add(offset + time.Duration(i)*p.every + g.jitter(20))
			rec := g.base(u, t, p.host, p.path)
			rec.URLCategory = "Information Technology"
			rec.UserAgent = p.ua
			rec.ReqSize = around(g.rng, 310, 0.05)
			rec.RespSize = around(g.rng, 180, 0.05)
			rec.RiskScore = 10
			g.add(rec, idx)
		}
	}
	g.closeScenario(idx)
}

// ---------- Planted attacks ----------

// exfilLead is how long after the sign-in the first upload leaves.
const exfilLead = 229 * time.Second

// attackExfil: someone signs in as a user from an address that user has
// nowhere else, then sends data to a first-seen, uncategorized domain. In
// the demo it is dana.k at 02:14, and about 40 MB.
func (g *gen) attackExfil() {
	p := g.plan.exfil
	u := g.user(p.user)
	u.ip = p.ip // an address this account uses nowhere else in the file
	const login = "login.microsoftonline.com"
	idx := g.startScenario(Scenario{
		ID: "atk-exfil", Label: "malicious", Type: "exfiltration",
		ExpectedDetectors: []string{"exfil_volume", "rare_destination", "new_source_ip", "off_hours"},
		Mitre:             []string{"T1567"},
		Users:             []string{u.name}, Hosts: []string{u.device}, Domains: []string{p.host}, Note: p.note,
	})
	start := g.dayStart.Add(p.start)

	signIn := func(t time.Time, method, path string, out, in int64, risk int) {
		rec := g.base(u, t, login, path)
		rec.Method = method
		rec.URLCategory = catOffice
		rec.AppName = "Microsoft Login"
		rec.AppClass = "Business"
		rec.ReqSize, rec.RespSize, rec.RiskScore = out, in, risk
		g.add(rec, idx)
	}
	signIn(start, "GET", "/common/oauth2/v2.0/authorize", 1126, 14541, 8)
	signIn(start.Add(15*time.Second), "POST", "/common/oauth2/v2.0/token", 2150, 6963, 12)

	drop := func(t time.Time, method, path string, out, in int64, risk int) {
		rec := g.base(u, t, p.host, path)
		rec.Method = method
		rec.URLCategory = catUnknown
		rec.UserAgent = p.ua
		rec.ReqSize, rec.RespSize, rec.RiskScore = out, in, risk
		g.add(rec, idx)
	}
	first := start.Add(exfilLead)
	drop(first.Add(-19*time.Second), "GET", "/", 410, 1228, p.probeRisk)
	// In the demo: 19 uploads between 02:17:41 and 02:40:48, about 2.1 MB each.
	for i := range p.uploads {
		t := first.Add(time.Duration(i)*p.gap + g.jitter(p.jitter))
		if i == 0 {
			t = first
		}
		drop(t, "POST", p.putPath, around(g.rng, p.uploadBytes, 0.05), around(g.rng, 412, 0.05), p.uploadRisk)
	}
	last := first.Add(time.Duration(p.uploads-1) * p.gap)
	drop(last.Add(25*time.Second), "GET", p.statusPath, 315, 640, p.probeRisk)
	g.closeScenario(idx)
}

// attackBeacon: one workstation calls one host on a steady beat. Small
// requests, one user agent. In the demo it is WS-114, every 60 seconds give
// or take two, for three hours.
func (g *gen) attackBeacon() {
	p := g.plan.beacon
	u := g.user(p.user)
	idx := g.startScenario(Scenario{
		ID: "atk-beacon", Label: "malicious", Type: "beaconing",
		ExpectedDetectors: []string{"beaconing", "rare_destination"},
		Mitre:             []string{"T1071.001"},
		Users:             []string{u.name}, Hosts: []string{u.device}, Domains: []string{p.host}, Note: p.note, Drawn: p.drawn,
	})
	for i := range p.calls {
		t := g.dayStart.Add(p.start + time.Duration(i)*p.every + g.jitter(p.jitter))
		rec := g.base(u, t, p.host, p.path)
		rec.URLCategory = catUnknown
		rec.UserAgent = p.ua
		rec.ReqSize = int64(200 + g.rng.IntN(201))
		rec.RespSize = int64(120 + g.rng.IntN(61))
		rec.RiskScore = p.risk
		g.add(rec, idx)
	}
	g.closeScenario(idx)
}

// attackPhish: a user follows a link. The proxy blocks it and names the
// threat. In the demo it is priya.s at 14:37.
func (g *gen) attackPhish() {
	p := g.plan.phish
	u := g.user(p.user)
	idx := g.startScenario(Scenario{
		ID: "atk-phish", Label: "malicious", Type: "phishing_click",
		ExpectedDetectors: []string{"threat_hit"},
		Mitre:             []string{"T1566.002"},
		Users:             []string{u.name}, Hosts: []string{u.device}, Domains: []string{p.host}, Note: p.note,
	})
	for _, c := range p.clicks {
		rec := g.base(u, g.dayStart.Add(p.start+time.Duration(c.sec)*time.Second), p.host, c.path)
		rec.URLCategory = "Phishing"
		rec.Action = "Blocked"
		rec.Reason = "Phishing site blocked"
		rec.Status = 403
		rec.ThreatName = p.threat
		rec.ReqSize = around(g.rng, 720, 0.1)
		rec.RespSize = 0
		rec.RiskScore = 95
		g.add(rec, idx)
	}
	g.closeScenario(idx)
}

// attackBurst: a user tries one anonymizer site after another. Every request
// is blocked. In the demo it is ben.t, about 80 tries in four minutes.
func (g *gen) attackBurst() {
	p := g.plan.burst
	u := g.user(p.user)
	idx := g.startScenario(Scenario{
		ID: "atk-burst", Label: "malicious", Type: "policy_evasion_burst",
		ExpectedDetectors: []string{"blocked_burst"},
		Mitre:             []string{"T1090"},
		Users:             []string{u.name}, Hosts: []string{u.device}, Domains: p.hosts, Note: p.note,
	})
	for i := range p.tries {
		t := g.dayStart.Add(p.start + time.Duration(i)*p.gap + g.jitter(p.jitter))
		rec := g.base(u, t, p.hosts[g.rng.IntN(len(p.hosts))], "/")
		rec.URLCategory = "Anonymizer"
		rec.Action = "Blocked"
		rec.Reason = reasonBlock
		rec.Status = 403
		rec.ReqSize = around(g.rng, 640, 0.15)
		rec.RespSize = 0
		rec.RiskScore = 75
		g.add(rec, idx)
	}
	g.closeScenario(idx)
}

// ---------- Malformed lines ----------

// pickMalformed chooses which lines to break. Scenario lines are never
// broken, so the answer key stays exact.
func (g *gen) pickMalformed() []bool {
	picked := make([]bool, len(g.events))
	want := int(math.Round(float64(len(g.events)) * malformedRate))
	for n := 0; n < want; {
		i := g.rng.IntN(len(g.events))
		if picked[i] || g.events[i].scenario >= 0 {
			continue
		}
		picked[i] = true
		n++
	}
	return picked
}

// corrupt breaks a line in one of three ways a real feed does.
func (g *gen) corrupt(line string) string {
	switch g.rng.IntN(3) {
	case 0: // truncated mid-write
		return line[:len(line)/5+g.rng.IntN(len(line)/2)]
	case 1: // the time field is missing
		for i := 0; i < len(line); i++ {
			if line[i] == '\t' {
				return line[i+1:]
			}
		}
		return line
	default: // not a log line at all
		return fmt.Sprintf("!!! nss feed reconnect %08x", g.rng.Uint32())
	}
}
