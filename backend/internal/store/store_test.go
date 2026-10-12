package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/model"
	"tenfold/backend/internal/store"
	"tenfold/backend/internal/store/storetest"
)

var ctx = context.Background()

func newUpload(userID uuid.UUID, createdAt time.Time) model.Upload {
	return model.Upload{
		ID: uuid.New(), UserID: userID, Filename: "a.log", Source: "upload",
		SizeBytes: 10, Status: "queued", Stage: "uploaded", RiskThreshold: 30,
		Pipeline: []api.StageProgress{}, CreatedAt: createdAt,
	}
}

func event(lineNo int, ts time.Time, user, host, action string) model.Event {
	return model.Event{
		LineNo: lineNo, TS: ts, User: user, SrcIP: "10.20.1.1", Host: host,
		URL: "https://" + host + "/", Category: "Unknown", Method: "GET", Status: 200,
		BytesOut: 100, BytesIn: 200, Action: action, Raw: "raw line",
	}
}

func TestUploadsBelongToTheirUser(t *testing.T) {
	s := storetest.New(t)
	owner, stranger := uuid.New(), uuid.New()
	u := newUpload(owner, time.Now().UTC())
	if err := s.CreateUpload(ctx, u); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetUpload(ctx, owner, u.ID)
	if err != nil {
		t.Fatalf("owner cannot read their upload: %v", err)
	}
	if got.Filename != "a.log" || got.VerdictCounts != nil || got.Stop != nil {
		t.Fatalf("unexpected upload: %+v", got)
	}
	if _, err := s.GetUpload(ctx, stranger, u.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("a stranger read the upload: err = %v", err)
	}
	if _, err := s.DeleteUpload(ctx, stranger, u.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("a stranger deleted the upload: err = %v", err)
	}
	if claimed, _ := s.ClaimUpload(ctx, stranger, u.ID); claimed {
		t.Fatal("a stranger claimed the upload")
	}
}

func TestClaimUploadIsExclusive(t *testing.T) {
	s := storetest.New(t)
	user := uuid.New()
	u := newUpload(user, time.Now().UTC())
	if err := s.CreateUpload(ctx, u); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ClaimUpload(ctx, user, u.ID); err != nil || !ok {
		t.Fatalf("first claim: ok = %v, err = %v", ok, err)
	}
	if ok, _ := s.ClaimUpload(ctx, user, u.ID); ok {
		t.Fatal("second claim succeeded while a job holds the upload")
	}
	stranded, err := s.ProcessingUploads(ctx)
	if err != nil || len(stranded) != 1 {
		t.Fatalf("ProcessingUploads = %d, %v", len(stranded), err)
	}
}

func TestListUploadsPages(t *testing.T) {
	s := storetest.New(t)
	user := uuid.New()
	start := time.Date(2026, 10, 9, 11, 0, 0, 0, time.UTC)
	for i := range 5 {
		if err := s.CreateUpload(ctx, newUpload(user, start.Add(time.Duration(i)*time.Minute))); err != nil {
			t.Fatal(err)
		}
	}
	page, total, err := s.ListUploads(ctx, user, nil, 2)
	if err != nil || total != 5 || len(page) != 2 {
		t.Fatalf("page 1: len = %d, total = %d, err = %v", len(page), total, err)
	}
	if !page[0].CreatedAt.After(page[1].CreatedAt) {
		t.Fatal("uploads are not newest first")
	}
	last := page[1]
	next, _, err := s.ListUploads(ctx, user, &store.UploadCursor{CreatedAt: last.CreatedAt, ID: last.ID}, 10)
	if err != nil || len(next) != 3 {
		t.Fatalf("page 2: len = %d, err = %v", len(next), err)
	}
}

func TestEventsFilterSearchAndPage(t *testing.T) {
	s := storetest.New(t)
	user := uuid.New()
	u := newUpload(user, time.Now().UTC())
	if err := s.CreateUpload(ctx, u); err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 10, 8, 6, 0, 0, 0, time.UTC)
	events := []model.Event{
		event(1, t0, "dana.k", "files-sync-cdn.net", "allowed"),
		event(2, t0.Add(time.Minute), "dana.k", "app.slack.com", "allowed"),
		event(3, t0.Add(2*time.Minute), "ben.t", "anon-surf.example", "blocked"),
		event(4, t0.Add(3*time.Minute), "ben.t", "100%_real.example", "blocked"),
	}
	if err := s.InsertEvents(ctx, u.ID, events); err != nil {
		t.Fatal(err)
	}

	count := func(name string, f store.EventFilter, want int) {
		t.Helper()
		f.Limit = 100
		got, total, err := s.ListEvents(ctx, u.ID, f)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(got) != want || total != want {
			t.Fatalf("%s: got %d events, total %d, want %d", name, len(got), total, want)
		}
	}
	count("no filter", store.EventFilter{}, 4)
	count("user", store.EventFilter{User: "dana.k"}, 2)
	count("action", store.EventFilter{Action: "blocked"}, 2)
	count("from", store.EventFilter{From: ptr(t0.Add(2 * time.Minute))}, 2)
	count("to is exclusive", store.EventFilter{To: ptr(t0.Add(2 * time.Minute))}, 2)
	count("fielded OR", store.EventFilter{Search: []store.SearchTerm{{"user", "dana.k"}, {"domain", "anon-surf.example"}}}, 3)
	count("domain matches subdomains", store.EventFilter{Search: []store.SearchTerm{{"domain", "slack.com"}}}, 1)
	count("id", store.EventFilter{Search: []store.SearchTerm{{"id", "e3"}}}, 1)
	count("bad id matches nothing", store.EventFilter{Search: []store.SearchTerm{{"id", "nope"}}}, 0)
	count("bare term", store.EventFilter{Search: []store.SearchTerm{{"", "SLACK"}}}, 1)
	count("percent is literal", store.EventFilter{Search: []store.SearchTerm{{"", "100%_real"}}}, 1)
	count("underscore is literal", store.EventFilter{Search: []store.SearchTerm{{"", "a_p"}}}, 0)

	// Paging, newest first.
	page, total, err := s.ListEvents(ctx, u.ID, store.EventFilter{Descending: true, Limit: 3})
	if err != nil || total != 4 || len(page) != 3 || page[0].LineNo != 4 {
		t.Fatalf("page 1: %+v total %d err %v", page, total, err)
	}
	last := page[2]
	page, total, err = s.ListEvents(ctx, u.ID, store.EventFilter{
		Descending: true, Limit: 3, After: &store.EventCursor{TS: last.TS, LineNo: last.LineNo}})
	if err != nil || total != 4 || len(page) != 1 || page[0].LineNo != 1 {
		t.Fatalf("page 2: %+v total %d err %v", page, total, err)
	}
}

func TestResultsRoundTrip(t *testing.T) {
	s := storetest.New(t)
	user, stranger := uuid.New(), uuid.New()
	u := newUpload(user, time.Now().UTC())
	if err := s.CreateUpload(ctx, u); err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 10, 8, 6, 0, 0, 0, time.UTC)
	if err := s.InsertEvents(ctx, u.ID, []model.Event{
		event(1, t0, "dana.k", "files-sync-cdn.net", "allowed"),
		event(2, t0.Add(time.Minute), "dana.k", "files-sync-cdn.net", "allowed"),
		event(3, t0.Add(20*time.Minute), "ben.t", "app.slack.com", "blocked"),
	}); err != nil {
		t.Fatal(err)
	}

	a := model.Anomaly{
		ID: uuid.New(), UploadID: u.ID, Detector: "exfil_volume", Reason: "r", Label: "bytes out 30× baseline",
		Signals:   []api.Signal{{Name: "bytes_out_vs_baseline", Weight: 34, EventIds: []string{"e1", "e2"}}},
		RiskScore: 34, Disposition: model.DispositionPending, User: "dana.k",
		Host: "files-sync-cdn.net", Hosts: []string{"files-sync-cdn.net"},
		LineNos: []int{1, 2}, FirstSeen: t0, LastSeen: t0.Add(time.Minute),
	}
	if err := s.ReplaceAnomalies(ctx, u.ID, []model.Anomaly{a}); err != nil {
		t.Fatal(err)
	}

	// Pending anomalies are hidden from the API's readers.
	if _, total, _ := s.ListAnomalies(ctx, u.ID, store.AnomalyFilter{Limit: 10}); total != 0 {
		t.Fatalf("a pending anomaly was listed: total = %d", total)
	}

	inc := model.Incident{
		ID: uuid.New(), UploadID: u.ID, UserID: user, Title: "Large outbound transfer",
		Severity: "high", RiskScore: 91, MarkerLabel: "40 MB out", TimeStart: t0,
		TimeEnd: t0.Add(time.Minute), EventCount: 2, AnomalyCount: 1,
		Mitre:   []api.MitreTechnique{{Id: "T1567", Name: "Exfiltration Over Web Service", Tactic: "Exfiltration"}},
		LineNos: []int{1, 2},
		Detail: model.IncidentDetail{Narrative: api.Narrative{Status: api.NarrativeStatusUnavailable,
			Sentences: []api.NarrativeSentence{}, NextSteps: []string{}}},
	}
	a.Disposition, a.IncidentID = model.DispositionInIncident, &inc.ID
	if err := s.ReplaceIncidents(ctx, u.ID, []model.Incident{inc}, []model.Anomaly{a}); err != nil {
		t.Fatal(err)
	}

	listed, total, err := s.ListAnomalies(ctx, u.ID, store.AnomalyFilter{Limit: 10})
	if err != nil || total != 1 || len(listed[0].LineNos) != 2 || listed[0].Signals[0].Weight != 34 {
		t.Fatalf("ListAnomalies: %+v total %d err %v", listed, total, err)
	}

	flags, err := s.EventFlags(ctx, u.ID, []int{1, 2, 3})
	if err != nil || len(flags[1]) != 1 || len(flags[3]) != 0 || *flags[1][0].IncidentID != inc.ID {
		t.Fatalf("EventFlags: %+v err %v", flags, err)
	}
	yes := true
	if _, n, _ := s.ListEvents(ctx, u.ID, store.EventFilter{HasAnomaly: &yes, Limit: 10}); n != 2 {
		t.Fatalf("hasAnomaly=true matched %d events, want 2", n)
	}

	buckets, err := s.EventBuckets(ctx, u.ID, 10*time.Minute, t0)
	if err != nil || len(buckets) != 2 || buckets[0].Total != 2 || buckets[0].Anomalous != 2 || buckets[1].Blocked != 1 {
		t.Fatalf("EventBuckets: %+v err %v", buckets, err)
	}
	spans, err := s.AnomalySpans(ctx, u.ID)
	if err != nil || len(spans) != 1 || spans[0].RiskScore != 91 {
		t.Fatalf("AnomalySpans: %+v err %v", spans, err)
	}

	// Incidents: owner only.
	if _, err := s.GetIncident(ctx, stranger, inc.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("a stranger read the incident: %v", err)
	}
	got, err := s.GetIncident(ctx, user, inc.ID)
	if err != nil || got.Mitre[0].Id != "T1567" || got.AnalystStatus != "open" || got.Detail.Narrative.Status != api.NarrativeStatusUnavailable {
		t.Fatalf("GetIncident: %+v err %v", got, err)
	}

	// Verdict with a note and a lesson, in one transaction.
	note := model.Note{ID: uuid.New(), Author: "sam.r", Text: "Approved backup.", CreatedAt: time.Now().UTC()}
	lesson := model.Lesson{ID: uuid.New(), UserID: user, Text: "Nightly backup.", ScopeType: "domain",
		ScopeValue: "files-sync-cdn.net", CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().Add(time.Hour).UTC()}
	if err := s.SetVerdict(ctx, stranger, inc.ID, "dismissed", &note, &lesson); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("a stranger set a verdict: %v", err)
	}
	if lessons, _ := s.ListLessons(ctx, user, false); len(lessons) != 0 {
		t.Fatal("a failed verdict still created its lesson")
	}
	if err := s.SetVerdict(ctx, user, inc.ID, "dismissed", &note, &lesson); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetIncident(ctx, user, inc.ID)
	notes, _ := s.ListNotes(ctx, inc.ID)
	lessons, _ := s.ListLessons(ctx, user, true)
	if got.AnalystStatus != "dismissed" || got.DecidedAt == nil || len(notes) != 1 || len(lessons) != 1 {
		t.Fatalf("after verdict: status %s notes %d lessons %d", got.AnalystStatus, len(notes), len(lessons))
	}
	withCounts, _ := s.GetUpload(ctx, user, u.ID)
	if withCounts.VerdictCounts == nil || withCounts.VerdictCounts.Dismissed != 1 {
		t.Fatalf("verdict counts: %+v", withCounts.VerdictCounts)
	}

	if err := s.AddNote(ctx, stranger, inc.ID, model.Note{ID: uuid.New(), Author: "x", Text: "x", CreatedAt: time.Now()}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("a stranger added a note: %v", err)
	}
	if err := s.AddLessonMatches(ctx, map[uuid.UUID]int{lesson.ID: 3}); err != nil {
		t.Fatal(err)
	}
	if lessons, _ = s.ListLessons(ctx, user, true); lessons[0].MatchCount != 3 {
		t.Fatalf("match count = %d, want 3", lessons[0].MatchCount)
	}
	if err := s.DeleteLesson(ctx, stranger, lesson.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("a stranger deleted a lesson: %v", err)
	}

	// Deleting the upload takes everything with it.
	if _, err := s.DeleteUpload(ctx, user, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetIncident(ctx, user, inc.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("the incident outlived its upload")
	}
	if left, _ := s.LoadEvents(ctx, u.ID); len(left) != 0 {
		t.Fatal("events outlived their upload")
	}
}

func ptr[T any](v T) *T { return &v }
