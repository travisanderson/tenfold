package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/blob"
	"tenfold/backend/internal/model"
	"tenfold/backend/internal/samples"
	"tenfold/backend/internal/store"
	"tenfold/backend/internal/store/storetest"
	"tenfold/backend/internal/zscaler"
)

const samplesDir = "../../../samples"

var ctx = context.Background()

// memoryBlobs is file storage held in a map, for tests.
type memoryBlobs map[string]string

func (m memoryBlobs) SignedUploadURL(context.Context, string) (string, time.Time, error) {
	return "http://storage.test/put", time.Now().Add(time.Hour), nil
}
func (m memoryBlobs) Open(_ context.Context, key string) (io.ReadCloser, error) {
	content, ok := m[key]
	if !ok {
		return nil, blob.ErrNotFound
	}
	return io.NopCloser(strings.NewReader(content)), nil
}
func (m memoryBlobs) Delete(_ context.Context, key string) error { delete(m, key); return nil }

func newRunner(t *testing.T, st *store.Store, blobs blob.Store, ai AI) *Runner {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRunner(st, blobs, samples.NewCatalog(samples.Dir(samplesDir)), NewHub(), ai, log)
}

func sampleUpload(t *testing.T, st *store.Store, user uuid.UUID, name string) model.Upload {
	t.Helper()
	if _, err := os.Stat(samplesDir + "/" + name + ".log"); err != nil {
		t.Skipf("sample %s is not generated", name)
	}
	u := model.Upload{
		ID: uuid.New(), UserID: user, Filename: name + ".log", Source: "sample", SampleName: &name,
		SizeBytes: 1, Status: "queued", Stage: "uploaded", RiskThreshold: 30,
		Pipeline: []api.StageProgress{}, CreatedAt: time.Now().UTC(),
	}
	if err := st.CreateUpload(ctx, u); err != nil {
		t.Fatal(err)
	}
	return u
}

func fileUpload(t *testing.T, st *store.Store, blobs memoryBlobs, user uuid.UUID, content string) model.Upload {
	t.Helper()
	key := uuid.NewString()
	blobs[key] = content
	u := model.Upload{
		ID: uuid.New(), UserID: user, Filename: "a.log", Source: "upload", StorageKey: &key,
		SizeBytes: int64(len(content)), Status: "queued", Stage: "uploaded", RiskThreshold: 30,
		Pipeline: []api.StageProgress{}, CreatedAt: time.Now().UTC(),
	}
	if err := st.CreateUpload(ctx, u); err != nil {
		t.Fatal(err)
	}
	return u
}

// analyze runs a job to its end and returns the upload as stored.
func analyze(t *testing.T, r *Runner, st *store.Store, u model.Upload, opts Options) model.Upload {
	t.Helper()
	if _, err := r.Analyze(ctx, u.UserID, u.ID, opts); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	r.Wait()
	got, err := st.GetUpload(ctx, u.UserID, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestAttackDayEndToEnd(t *testing.T) {
	st := storetest.New(t)
	user := uuid.New()
	r := newRunner(t, st, memoryBlobs{}, NewAI("mock"))
	u := sampleUpload(t, st, user, "attack-day")

	// The sample has malformed lines on purpose: the job stops and asks.
	got := analyze(t, r, st, u, Options{})
	if got.Status != "partial" || got.Stop == nil || got.Stop.Reason != api.StopReasonParseErrors {
		t.Fatalf("first run: status %s, stop %+v", got.Status, got.Stop)
	}
	if *got.LineCount != 48002 || *got.ParseErrorCount != 48 || got.FirstParseError == nil {
		t.Fatalf("first run: lines %d, parse errors %d", *got.LineCount, *got.ParseErrorCount)
	}
	if _, err := r.Analyze(ctx, user, u.ID, Options{}); !errors.Is(err, ErrNeedsAcceptance) {
		t.Fatalf("resume without accepting: err = %v", err)
	}

	// Accepting resumes after parsing; the events are not read again.
	got = analyze(t, r, st, u, Options{AcceptParseErrors: true})
	if got.Status != "completed" || got.Stage != "done" || got.Progress != 100 || got.Stop != nil {
		t.Fatalf("second run: status %s stage %s progress %d stop %+v", got.Status, got.Stage, got.Progress, got.Stop)
	}
	for _, s := range got.Pipeline {
		if s.State != api.StageStateDone {
			t.Errorf("stage %s is %s, want done", s.Stage, s.State)
		}
	}

	incidents, err := st.ListIncidents(ctx, user, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d lines -> %d events -> %d anomalies -> %d incidents",
		*got.LineCount, *got.EventCount, *got.AnomalyCount, *got.IncidentCount)
	key, _ := samples.NewCatalog(samples.Dir(samplesDir)).AnswerKey(ctx, "attack-day")
	for _, scenario := range key.Scenarios {
		if !scenario.Malicious() {
			continue
		}
		caught := false
		for _, inc := range incidents {
			if overlap(inc.LineNos, scenario.LineNos) >= 0.5 {
				caught = true
			}
		}
		if !caught {
			t.Errorf("no incident covers planted attack %s", scenario.ID)
		}
	}

	for i, inc := range incidents {
		t.Logf("  %-8s %3d  %-16s %s", inc.Severity, inc.RiskScore, orZero(inc.AIVerdict), inc.Title)
		if i > 0 && model.SeverityRank(incidents[i-1].Severity) > model.SeverityRank(inc.Severity) {
			t.Error("incidents are not sorted by severity")
		}
		n := inc.Detail.Narrative
		if n.Status != api.NarrativeStatusGenerated || len(n.Sentences) == 0 || inc.Summary == nil || inc.AIVerdict == nil {
			t.Errorf("incident %q: narrative %s, %d sentences, summary %v", inc.Title, n.Status, len(n.Sentences), inc.Summary)
		}
		if err := checkCitations(Draft{Sentences: n.Sentences}, inc.LineNos); err != nil {
			t.Errorf("incident %q: %v", inc.Title, err)
		}
		weights := 0
		for _, s := range inc.Detail.Signals {
			weights += s.Weight
		}
		if min(weights, 100) != inc.RiskScore {
			t.Errorf("incident %q: signal weights sum to %d, risk score is %d", inc.Title, weights, inc.RiskScore)
		}
		if len(inc.Detail.Graph.Nodes) < 3 || len(inc.Detail.Timeline) == 0 || len(inc.Detail.Indicators) == 0 {
			t.Errorf("incident %q is missing its graph, timeline or indicators", inc.Title)
		}
	}

	// Every anomaly has a final disposition, and one group was set aside
	// by the judge: the engineer's large push to GitHub.
	all, total, _ := st.ListAnomalies(ctx, u.ID, store.AnomalyFilter{Limit: 200})
	if total != *got.AnomalyCount {
		t.Errorf("%d anomalies listed, %d counted; some are still pending", total, *got.AnomalyCount)
	}
	benign := 0
	for _, a := range all {
		if a.Disposition == model.DispositionJudgedBenign {
			benign++
		}
	}
	if benign == 0 {
		t.Error("the judge set nothing aside")
	}
}

func TestLessonSuppressesOnTheNextRun(t *testing.T) {
	st := storetest.New(t)
	user := uuid.New()
	r := newRunner(t, st, memoryBlobs{}, NewAI("mock"))
	u := sampleUpload(t, st, user, "attack-day")

	before := analyze(t, r, st, u, Options{AcceptParseErrors: true})
	lesson := model.Lesson{ID: uuid.New(), UserID: user, Text: "Nightly backup.", ScopeType: "pattern",
		ScopeValue: "*.backblazeb2.com", CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().Add(time.Hour).UTC()}
	if err := st.CreateLesson(ctx, lesson); err != nil {
		t.Fatal(err)
	}
	after := analyze(t, r, st, u, Options{AcceptParseErrors: true, Restart: true})

	if *after.IncidentCount != *before.IncidentCount-1 {
		t.Fatalf("incidents: %d before the lesson, %d after; want one fewer", *before.IncidentCount, *after.IncidentCount)
	}
	suppressed, n, _ := st.ListAnomalies(ctx, u.ID, store.AnomalyFilter{Disposition: model.DispositionLesson, Limit: 50})
	if n != 3 || suppressed[0].LessonID == nil || *suppressed[0].LessonID != lesson.ID {
		t.Fatalf("%d anomalies suppressed by the lesson, want 3", n)
	}
	lessons, _ := st.ListLessons(ctx, user, true)
	if lessons[0].MatchCount != 3 {
		t.Fatalf("lesson match count = %d, want 3", lessons[0].MatchCount)
	}
}

func TestWithoutAITheJobStillScoresAndGroups(t *testing.T) {
	st := storetest.New(t)
	user := uuid.New()
	u := sampleUpload(t, st, user, "attack-day")

	got := analyze(t, newRunner(t, st, memoryBlobs{}, NewAI("none")), st, u, Options{AcceptParseErrors: true})
	if got.Status != "partial" || got.Stop == nil || got.Stop.Reason != api.StopReasonAiUnavailable || got.Stop.Stage != api.PipelineStageJudging {
		t.Fatalf("status %s, stop %+v", got.Status, got.Stop)
	}
	incidents, _ := st.ListIncidents(ctx, user, u.ID)
	if len(incidents) == 0 {
		t.Fatal("no incidents without AI; risk scores and evidence should be complete")
	}
	for _, inc := range incidents {
		if inc.AIVerdict != nil || inc.Summary != nil || inc.Detail.Narrative.Status != api.NarrativeStatusUnavailable {
			t.Errorf("incident %q carries AI output though the AI did not run", inc.Title)
		}
	}
	withoutAI := len(incidents)

	// "Try the AI stages again": resume picks up at judging.
	got = analyze(t, newRunner(t, st, memoryBlobs{}, NewAI("mock")), st, u, Options{})
	if got.Status != "completed" {
		t.Fatalf("after resume: status %s, stop %+v", got.Status, got.Stop)
	}
	incidents, _ = st.ListIncidents(ctx, user, u.ID)
	if len(incidents) != withoutAI-1 {
		t.Errorf("%d incidents with AI, %d without; the judge should set one aside", len(incidents), withoutAI)
	}
}

func TestBadFilesFailCleanly(t *testing.T) {
	st := storetest.New(t)
	user := uuid.New()
	blobs := memoryBlobs{}
	r := newRunner(t, st, blobs, NewAI("mock"))

	notALog := fileUpload(t, st, blobs, user, "hello\nthis is not a proxy log\n")
	got := analyze(t, r, st, notALog, Options{})
	if got.Status != "failed" || got.Error == nil || got.Error.Code != "parse_failed" {
		t.Fatalf("not a log: status %s, error %+v", got.Status, got.Error)
	}

	neverArrived := fileUpload(t, st, blobs, user, "x")
	delete(blobs, *neverArrived.StorageKey)
	got = analyze(t, r, st, neverArrived, Options{})
	if got.Status != "failed" || got.Error == nil || got.Error.Code != "file_missing" {
		t.Fatalf("missing file: status %s, error %+v", got.Status, got.Error)
	}

	// A failed upload can be analyzed again once the file is there.
	blobs[*neverArrived.StorageKey] = "still not a log\n"
	if got = analyze(t, r, st, neverArrived, Options{}); got.Error.Code != "parse_failed" {
		t.Fatalf("retry: error %+v", got.Error)
	}
}

func TestSecondAnalyzeIsRefusedWhileOneRuns(t *testing.T) {
	st := storetest.New(t)
	user := uuid.New()
	r := newRunner(t, st, memoryBlobs{}, NewAI("mock"))
	u := sampleUpload(t, st, user, "attack-day")

	if _, err := r.Analyze(ctx, user, u.ID, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Analyze(ctx, user, u.ID, Options{}); !errors.Is(err, ErrRunning) {
		t.Fatalf("second Analyze: err = %v, want ErrRunning", err)
	}
	if _, err := r.Analyze(ctx, uuid.New(), u.ID, Options{}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("a stranger's Analyze: err = %v, want ErrNotFound", err)
	}
	r.Cancel(u.ID)
	r.Wait()
}

// gatedBlobs is storage whose files open only when the test says so. It
// holds a job at the start of parsing, which is the first thing a job does
// once it has a slot.
type gatedBlobs struct {
	memoryBlobs
	opened  chan struct{} // one value per Open call
	release chan struct{} // closed to let every Open return
}

func (g gatedBlobs) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	g.opened <- struct{}{}
	<-g.release
	return g.memoryBlobs.Open(ctx, key)
}

func TestJobsPastTheCapWaitTheirTurn(t *testing.T) {
	st := storetest.New(t)
	user := uuid.New()
	blobs := gatedBlobs{memoryBlobs{}, make(chan struct{}, maxRunningJobs+1), make(chan struct{})}
	r := newRunner(t, st, blobs, NewAI("mock"))

	uploads := make([]model.Upload, maxRunningJobs+1)
	for i := range uploads {
		uploads[i] = fileUpload(t, st, blobs.memoryBlobs, user, "not a proxy log\n")
		if _, err := r.Analyze(ctx, user, uploads[i].ID, Options{}); err != nil {
			t.Fatal(err)
		}
	}

	// The cap's worth of jobs reach storage. The one past it does not.
	for range maxRunningJobs {
		select {
		case <-blobs.opened:
		case <-time.After(30 * time.Second):
			t.Fatal("a job within the cap never started")
		}
	}
	select {
	case <-blobs.opened:
		t.Fatalf("more than %d jobs are at work", maxRunningJobs)
	case <-time.After(2 * time.Second):
	}
	last, err := st.GetUpload(ctx, user, uploads[maxRunningJobs].ID)
	if err != nil {
		t.Fatal(err)
	}
	if last.Status != "processing" || !strings.HasPrefix(last.Message, "Waiting") {
		t.Fatalf("the waiting job: status %s, message %q", last.Status, last.Message)
	}

	// Once the others finish, it takes its turn and runs to its end.
	close(blobs.release)
	r.Wait()
	for _, u := range uploads {
		got, err := st.GetUpload(ctx, user, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != "failed" || got.Error == nil || got.Error.Code != "parse_failed" {
			t.Errorf("upload %s: status %s, error %+v", u.ID, got.Status, got.Error)
		}
	}
}

func overlap(evidence, scenario []int) float64 {
	in := map[int]bool{}
	for _, n := range evidence {
		in[n] = true
	}
	hit := 0
	for _, n := range scenario {
		if in[n] {
			hit++
		}
	}
	return float64(hit) / float64(len(scenario))
}

// threatLog is the smallest file that raises an incident: a few ordinary
// requests and one the proxy tagged as phishing and let through.
func threatLog() string {
	at := time.Date(2026, 10, 6, 14, 0, 0, 0, time.UTC)
	var lines []string
	for i := range 6 {
		r := zscaler.Record{
			Time: at.Add(time.Duration(i) * time.Minute), RecordID: int64(i + 1),
			Login: "priya.s@northwindlabs.example", Dept: "Marketing", ClientIP: "10.20.4.9", ServerIP: "203.0.113.7",
			Host: "www.wikipedia.org", URL: "https://www.wikipedia.org/", URLCategory: "Reference",
			Action: "Allowed", Method: "GET", Status: 200, ReqSize: 400, RespSize: 9000, DeviceHostname: "WS-109",
		}
		if i == 3 {
			r.Host, r.URL = "secure-login-verify.example", "https://secure-login-verify.example/signin"
			r.URLCategory, r.ThreatName, r.RiskScore = "Phishing", "Phish.Generic", 90
		}
		lines = append(lines, r.Format())
	}
	return strings.Join(lines, "\n") + "\n"
}

// stubNarrator returns a fixed draft or a fixed error.
type stubNarrator struct {
	draft Draft
	err   error
}

func (stubNarrator) Provider() api.DecisionProvider { return api.DecisionProviderClaude }
func (n stubNarrator) Narrate(context.Context, NarrateInput) (Draft, error) {
	return n.draft, n.err
}

// stubJudge returns a fixed error.
type stubJudge struct{ err error }

func (j stubJudge) Decide(context.Context, JudgeInput) (api.Decision, error) {
	return api.Decision{}, j.err
}

func TestANarrativeThatCannotBeUsedBecomesTheTemplate(t *testing.T) {
	st := storetest.New(t)
	invented := Draft{Summary: "Invented.", Sentences: []api.NarrativeSentence{
		{Text: "Something happened on a line that is not in the evidence.", Citations: []string{"e999999"}},
	}}
	for name, narrator := range map[string]Narrator{
		"cites outside the evidence": stubNarrator{draft: invented},
		"gives no answer":            stubNarrator{err: fmt.Errorf("%w: it declined the request", errNoAnswer)},
	} {
		t.Run(name, func(t *testing.T) {
			blobs, user := memoryBlobs{}, uuid.New()
			r := newRunner(t, st, blobs, AI{Judge: MockJudge{}, Narrator: narrator, Shadow: MockJudge{}})
			got := analyze(t, r, st, fileUpload(t, st, blobs, user, threatLog()), Options{})
			if got.Status != "completed" {
				t.Fatalf("status %s, stop %+v: one bad draft must not stop the job", got.Status, got.Stop)
			}
			incidents, err := st.ListIncidents(ctx, user, got.ID)
			if err != nil || len(incidents) != 1 {
				t.Fatalf("%d incidents, err %v", len(incidents), err)
			}
			n := incidents[0].Detail.Narrative
			if n.Status != api.NarrativeStatusTemplate || n.Provider == nil || *n.Provider != api.DecisionProviderRules {
				t.Errorf("narrative status %s, provider %v", n.Status, n.Provider)
			}
			if err := checkCitations(Draft{Sentences: n.Sentences}, incidents[0].LineNos); err != nil {
				t.Errorf("the template narrative: %v", err)
			}

			// The shadow judge's rulings are saved beside the judge's.
			anomalies, _ := st.LoadAnomalies(ctx, got.ID)
			for _, a := range anomalies {
				if a.Disposition == model.DispositionInIncident && (a.Decision == nil || a.ShadowDecision == nil) {
					t.Errorf("%s: decision %v, shadow %v", a.Detector, a.Decision, a.ShadowDecision)
				}
			}
		})
	}
}

func TestWhenTheModelCannotBeReachedTheScoresStillArrive(t *testing.T) {
	st := storetest.New(t)
	down := errors.New("connection refused")
	for name, ai := range map[string]AI{
		"judge down":    {Judge: stubJudge{err: down}, Narrator: MockNarrator{}},
		"narrator down": {Judge: MockJudge{}, Narrator: stubNarrator{err: down}},
	} {
		t.Run(name, func(t *testing.T) {
			blobs, user := memoryBlobs{}, uuid.New()
			got := analyze(t, newRunner(t, st, blobs, ai), st, fileUpload(t, st, blobs, user, threatLog()), Options{})
			if got.Status != "partial" || got.Stop == nil || got.Stop.Reason != api.StopReasonAiUnavailable {
				t.Fatalf("status %s, stop %+v", got.Status, got.Stop)
			}
			incidents, _ := st.ListIncidents(ctx, user, got.ID)
			if len(incidents) != 1 || incidents[0].RiskScore != 60 {
				t.Fatalf("incidents = %+v, want the one threat hit at risk 60", incidents)
			}
		})
	}
}
