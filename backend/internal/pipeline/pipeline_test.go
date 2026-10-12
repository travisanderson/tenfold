package pipeline

import (
	"context"
	"errors"
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
