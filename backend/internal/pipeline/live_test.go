package pipeline

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/detect"
	"tenfold/backend/internal/model"
	"tenfold/backend/internal/samples"
	"tenfold/backend/internal/store/storetest"
	"tenfold/backend/internal/zscaler"
)

// TestLiveJudges asks the real models about every anomaly that reaches the
// threshold in a demo sample and prints their rulings beside the answer
// key. It calls paid APIs, so it runs only when asked:
//
//	LIVE_AI=1 JEV_API_KEY=... ANTHROPIC_API_KEY=... go test ./internal/pipeline/ -run TestLiveJudges -v
//
// A judge whose key is missing is left out. Set LIVE_SAMPLE to pick a
// sample other than attack-day. Nothing here is asserted: it is a tool for
// reading how the judges do, not a gate.
func TestLiveJudges(t *testing.T) {
	if os.Getenv("LIVE_AI") == "" {
		t.Skip("set LIVE_AI=1 to call the real models")
	}
	judges := map[string]Judge{}
	jevUsage := &countTokens{next: http.DefaultTransport}
	if key := os.Getenv("JEV_API_KEY"); key != "" {
		jev := NewJevJudge(key, DefaultJevModel)
		jev.http.Transport = jevUsage
		judges["jev"] = jev
	}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		judges["claude"] = NewClaudeAI(key, DefaultJudgeModel, DefaultNarrativeModel).Judge
	}
	if len(judges) == 0 {
		t.Skip("no API key is set")
	}
	events, byLine, key := liveSample(t)

	names := make([]string, 0, len(judges))
	for n := range judges {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, g := range detect.GroupAnomalies(detect.Run(uuid.New(), events)) {
		if g.RiskScore < 30 {
			continue
		}
		t.Logf("group: %s, risk %d, %d anomalies", g.Anomalies[0].User, g.RiskScore, len(g.Anomalies))
		for _, a := range g.Anomalies {
			var evidence []model.Event
			for _, n := range sampleLines(a.LineNos, judgeEvidence) {
				evidence = append(evidence, byLine[n])
			}
			truth := "noise"
			for _, s := range key.Scenarios {
				if overlap(s.LineNos, a.LineNos) >= 0.5 {
					truth = s.Label + " (" + s.ID + ")"
				}
			}
			t.Logf("  %-16s answer key: %s", a.Detector, truth)
			for _, n := range names {
				d, err := judges[n].Decide(ctx, JudgeInput{Anomaly: *a, Group: g, Evidence: evidence})
				if err != nil {
					t.Errorf("    %-6s %v", n, err)
					continue
				}
				t.Logf("    %-6s %-16s confidence %.2f  %5d ms  %s", n, d.Label, d.Confidence, d.LatencyMs, orZero(d.Model))
			}
		}
	}
	if jevUsage.requests > 0 {
		t.Logf("jev: %d requests, %d input tokens, %d on average",
			jevUsage.requests, jevUsage.inputTokens, jevUsage.inputTokens/jevUsage.requests)
	}
}

// TestLiveNarrator asks the real narrator for the account of every incident
// in a demo sample, checks the citations, and prints what it wrote. Run it
// the same way as TestLiveJudges; it needs ANTHROPIC_API_KEY.
func TestLiveNarrator(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if os.Getenv("LIVE_AI") == "" || apiKey == "" {
		t.Skip("set LIVE_AI=1 and ANTHROPIC_API_KEY to call the real model")
	}
	narrator := NewClaudeAI(apiKey, DefaultJudgeModel, DefaultNarrativeModel).Narrator
	events, byLine, _ := liveSample(t)
	file := newFileView(events)

	for _, g := range detect.GroupAnomalies(detect.Run(uuid.New(), events)) {
		if g.RiskScore < 30 {
			continue
		}
		in := NarrateInput{Incident: buildIncident(uuid.New(), uuid.New(), g, file)}
		for _, a := range g.Anomalies {
			in.Anomalies = append(in.Anomalies, *a)
		}
		for _, n := range narrativeSample(in.Anomalies) {
			in.Evidence = append(in.Evidence, byLine[n])
		}

		started := time.Now()
		draft, err := narrator.Narrate(ctx, in)
		if err != nil {
			t.Errorf("%s: %v", in.Incident.Title, err)
			continue
		}
		t.Logf("%s (%s, risk %d), %d ms, shown %d of %d events",
			in.Incident.Title, g.Anomalies[0].User, g.RiskScore, time.Since(started).Milliseconds(), len(in.Evidence), len(g.LineNos))
		if err := checkCitations(draft, in.Incident.LineNos); err != nil {
			t.Errorf("  citation check failed: %v", err)
		}
		t.Logf("  summary: %s", draft.Summary)
		for _, s := range draft.Sentences {
			t.Logf("  - %s %v", s.Text, s.Citations)
		}
		for _, step := range draft.NextSteps {
			t.Logf("  next: %s", step)
		}
	}
}

// TestLivePipeline runs a demo sample through the whole job the way
// AI_PROVIDER=jev does: Jev judges, Claude judges in shadow, Claude
// narrates. It needs both keys and TEST_DATABASE_URL.
func TestLivePipeline(t *testing.T) {
	jevKey, claudeKey := os.Getenv("JEV_API_KEY"), os.Getenv("ANTHROPIC_API_KEY")
	if os.Getenv("LIVE_AI") == "" || jevKey == "" || claudeKey == "" {
		t.Skip("set LIVE_AI=1, JEV_API_KEY and ANTHROPIC_API_KEY to call the real models")
	}
	name := os.Getenv("LIVE_SAMPLE")
	if name == "" {
		name = "attack-day"
	}
	st := storetest.New(t)
	ai := NewClaudeAI(claudeKey, DefaultJudgeModel, DefaultNarrativeModel)
	ai.Shadow, ai.Judge = ai.Judge, NewJevJudge(jevKey, DefaultJevModel)

	user := uuid.New()
	u := sampleUpload(t, st, user, name)
	got := analyze(t, newRunner(t, st, memoryBlobs{}, ai), st, u, Options{AcceptParseErrors: true})
	t.Logf("status %s: %s", got.Status, got.Message)
	for _, s := range got.Pipeline {
		t.Logf("  stage %-10s %-8s in %5d  out %5d  %6d ms", s.Stage, s.State, orZero(s.CountIn), orZero(s.CountOut), orZero(s.ElapsedMs))
	}
	if got.Status != "completed" {
		t.Fatalf("stop: %+v", got.Stop)
	}

	anomalies, _ := st.LoadAnomalies(ctx, u.ID)
	both, agreed := 0, 0
	for _, a := range anomalies {
		if a.Decision != nil && a.ShadowDecision != nil {
			both++
			if a.Decision.Label == a.ShadowDecision.Label {
				agreed++
			}
		}
	}
	t.Logf("judges agreed on %d of %d anomalies", agreed, both)

	incidents, _ := st.ListIncidents(ctx, user, u.ID)
	for _, inc := range incidents {
		n := inc.Detail.Narrative
		t.Logf("  %-8s %3d  %-16s narrative %-9s by %-6s %s", inc.Severity, inc.RiskScore, orZero(inc.AIVerdict),
			n.Status, orZero(n.Provider), inc.Title)
		if n.Status != api.NarrativeStatusGenerated {
			t.Errorf("incident %q fell back to the %s narrative", inc.Title, n.Status)
		}
	}
}

// liveSample reads the demo sample named by LIVE_SAMPLE, or attack-day.
func liveSample(t *testing.T) ([]model.Event, map[int]model.Event, samples.AnswerKey) {
	t.Helper()
	name := os.Getenv("LIVE_SAMPLE")
	if name == "" {
		name = "attack-day"
	}
	f, err := os.Open(samplesDir + "/" + name + ".log")
	if err != nil {
		t.Skipf("sample %s is not generated: %v", name, err)
	}
	defer f.Close()
	var events []model.Event
	byLine := map[int]model.Event{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64<<10), maxLineBytes)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		if record, err := zscaler.Parse(scanner.Text()); err == nil {
			e := model.EventFromRecord(lineNo, record, "")
			events, byLine[lineNo] = append(events, e), e
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].TS.Before(events[j].TS) })
	key, err := samples.NewCatalog(samples.Dir(samplesDir)).AnswerKey(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	return events, byLine, key
}

// countTokens adds up the input tokens Jev reports, to price a decision.
type countTokens struct {
	next                  http.RoundTripper
	requests, inputTokens int
}

func (c *countTokens) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := c.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	var body jevResponse
	if json.Unmarshal(raw, &body) == nil && body.Usage.InputTokens > 0 {
		c.requests++
		c.inputTokens += body.Usage.InputTokens
	}
	return resp, err
}
