package server_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/auth"
	"tenfold/backend/internal/blob"
	"tenfold/backend/internal/pipeline"
	"tenfold/backend/internal/samples"
	"tenfold/backend/internal/server"
	"tenfold/backend/internal/store/storetest"
)

const (
	samplesDir = "../../../samples"
	secret     = "test-secret"
)

// fakeBlobs stands in for Supabase Storage.
type fakeBlobs struct{}

func (fakeBlobs) SignedUploadURL(_ context.Context, key string) (string, time.Time, error) {
	return "https://storage.test/upload/" + key + "?token=t", time.Now().Add(2 * time.Hour).UTC(), nil
}
func (fakeBlobs) Open(context.Context, string) (io.ReadCloser, error) { return nil, blob.ErrNotFound }
func (fakeBlobs) Delete(context.Context, string) error                { return nil }

// harness is a running API with a real database behind it.
type harness struct {
	t      *testing.T
	url    string
	runner *pipeline.Runner
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	st := storetest.New(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	catalog := samples.NewCatalog(samples.Dir(samplesDir))
	hub := pipeline.NewHub()
	runner := pipeline.NewRunner(st, fakeBlobs{}, catalog, hub, pipeline.NewAI("mock"), log)
	srv := httptest.NewServer(server.New(server.Options{
		Store: st, Blobs: fakeBlobs{}, Samples: catalog, Runner: runner, Hub: hub,
		Verifier: auth.NewSecretVerifier(secret), RiskThreshold: 30, Log: log,
	}).Handler())
	t.Cleanup(func() { runner.Wait(); srv.Close() })
	return &harness{t: t, url: srv.URL, runner: runner}
}

// token mints an access token the way Supabase does: the user's ID in
// "sub", the audience "authenticated".
func token(t *testing.T, user uuid.UUID, email string, mutate ...func(jwt.MapClaims)) string {
	t.Helper()
	claims := jwt.MapClaims{"sub": user.String(), "aud": "authenticated", "email": email,
		"exp": time.Now().Add(time.Hour).Unix()}
	for _, m := range mutate {
		m(claims)
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

// call makes one request and decodes a JSON answer into out, if given.
func (h *harness) call(method, path, bearer string, body, out any) (int, http.Header, []byte) {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		switch b := body.(type) {
		case string:
			reader = strings.NewReader(b)
		default:
			encoded, _ := json.Marshal(b)
			reader = bytes.NewReader(encoded)
		}
	}
	req, _ := http.NewRequest(method, h.url+path, reader)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			h.t.Fatalf("%s %s: status %d, cannot decode %q: %v", method, path, resp.StatusCode, truncate(raw), err)
		}
	}
	return resp.StatusCode, resp.Header, raw
}

// want fails the test unless the call answers with the status.
func (h *harness) want(status int, method, path, bearer string, body, out any) []byte {
	h.t.Helper()
	got, _, raw := h.call(method, path, bearer, body, out)
	if got != status {
		h.t.Fatalf("%s %s: status %d, want %d: %s", method, path, got, status, truncate(raw))
	}
	return raw
}

// wantProblem checks for a Problem Details answer with the status and code.
func (h *harness) wantProblem(status int, code, method, path, bearer string, body any) api.Problem {
	h.t.Helper()
	var p api.Problem
	got, header, raw := h.call(method, path, bearer, body, &p)
	if got != status || p.Code != code || p.Status != status {
		h.t.Fatalf("%s %s: status %d code %q, want %d %q: %s", method, path, got, p.Code, status, code, truncate(raw))
	}
	if ct := header.Get("Content-Type"); ct != "application/problem+json" {
		h.t.Fatalf("%s %s: Content-Type %q, want application/problem+json", method, path, ct)
	}
	return p
}

func truncate(raw []byte) string {
	if len(raw) > 300 {
		return string(raw[:300]) + "..."
	}
	return string(raw)
}

func TestPublicPathsNeedNoTokenAndEverythingElseDoes(t *testing.T) {
	h := newHarness(t)

	var health api.Health
	h.want(200, "GET", "/healthz", "", nil, &health)
	if health.Status != "ok" {
		t.Fatalf("health: %+v", health)
	}
	if spec := h.want(200, "GET", "/openapi.yaml", "", nil, nil); !bytes.HasPrefix(spec, []byte("openapi: 3.0.3")) {
		t.Fatal("/openapi.yaml did not return the contract")
	}
	h.want(200, "GET", "/docs", "", nil, nil)

	user := uuid.New()
	h.wantProblem(401, "unauthorized", "GET", "/v1/uploads", "", nil)
	h.wantProblem(401, "unauthorized", "GET", "/v1/uploads", "not-a-token", nil)
	h.wantProblem(401, "unauthorized", "GET", "/v1/no-such-path", "", nil)

	expired := token(t, user, "a@b.example", func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Minute).Unix() })
	h.wantProblem(401, "unauthorized", "GET", "/v1/uploads", expired, nil)
	anon := token(t, user, "a@b.example", func(c jwt.MapClaims) { c["aud"] = "anon" })
	h.wantProblem(401, "unauthorized", "GET", "/v1/uploads", anon, nil)
	forged, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": user.String(), "aud": "authenticated", "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte("someone-else's-secret"))
	h.wantProblem(401, "unauthorized", "GET", "/v1/uploads", forged, nil)
	unsigned, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub": user.String(), "aud": "authenticated", "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	h.wantProblem(401, "unauthorized", "GET", "/v1/uploads", unsigned, nil)

	h.want(200, "GET", "/v1/uploads", token(t, user, "a@b.example"), nil, nil)
}

func TestCreateUploadValidatesAndSigns(t *testing.T) {
	h := newHarness(t)
	me := token(t, uuid.New(), "sam.r@northwind.example")

	h.wantProblem(413, "upload_too_large", "POST", "/v1/uploads", me,
		api.CreateUploadRequest{Filename: "big.log", SizeBytes: 52_428_801, ContentType: "text/plain"})
	p := h.wantProblem(422, "validation_failed", "POST", "/v1/uploads", me,
		api.CreateUploadRequest{Filename: "", SizeBytes: 0, ContentType: "application/zip"})
	if p.Errors == nil || len(*p.Errors) != 3 {
		t.Fatalf("want three field errors, got %+v", p.Errors)
	}
	h.wantProblem(400, "bad_request", "POST", "/v1/uploads", me, "{not json")
	h.wantProblem(400, "bad_request", "GET", "/v1/uploads?limit=0", me, nil)
	h.wantProblem(400, "bad_request", "GET", "/v1/uploads?limit=abc", me, nil)
	h.wantProblem(400, "bad_request", "GET", "/v1/uploads?cursor=garbage", me, nil)
	h.wantProblem(400, "bad_request", "GET", "/v1/uploads/not-a-uuid", me, nil)

	var created api.CreateUploadResponse
	h.want(201, "POST", "/v1/uploads", me,
		api.CreateUploadRequest{Filename: "northwind.log", SizeBytes: 1234, ContentType: "text/plain; charset=utf-8"}, &created)
	u := created.Upload
	if u.Status != api.UploadStatusQueued || u.Stage != api.PipelineStageUploaded || u.RiskThreshold != 30 ||
		!strings.HasPrefix(created.UploadUrl, "https://storage.test/") || !created.ExpiresAt.After(time.Now()) {
		t.Fatalf("created: %+v", created)
	}
	if strings.Contains(created.UploadUrl, "northwind") {
		t.Fatal("the filename leaked into the storage URL")
	}

	// Paging: three uploads, two per page.
	for range 2 {
		h.want(201, "POST", "/v1/uploads", me,
			api.CreateUploadRequest{Filename: "more.log", SizeBytes: 1, ContentType: "text/csv"}, nil)
	}
	var page1, page2 api.UploadPage
	h.want(200, "GET", "/v1/uploads?limit=2", me, nil, &page1)
	if len(page1.Items) != 2 || page1.Total != 3 || page1.NextCursor == nil {
		t.Fatalf("page 1: %d items, total %d, cursor %v", len(page1.Items), page1.Total, page1.NextCursor)
	}
	h.want(200, "GET", "/v1/uploads?limit=2&cursor="+*page1.NextCursor, me, nil, &page2)
	if len(page2.Items) != 1 || page2.NextCursor != nil || page2.Items[0].Id != u.Id {
		t.Fatalf("page 2: %d items, cursor %v", len(page2.Items), page2.NextCursor)
	}

	// The file never arrived, so analysis fails cleanly.
	h.want(202, "POST", "/v1/uploads/"+u.Id.String()+"/analyze", me, nil, nil)
	h.runner.Wait()
	var after api.Upload
	h.want(200, "GET", "/v1/uploads/"+u.Id.String(), me, nil, &after)
	if after.Status != api.UploadStatusFailed || after.Error == nil || after.Error.Code != "file_missing" {
		t.Fatalf("after analyze: status %s, error %+v", after.Status, after.Error)
	}
	h.want(204, "DELETE", "/v1/uploads/"+u.Id.String(), me, nil, nil)
	h.wantProblem(404, "not_found", "GET", "/v1/uploads/"+u.Id.String(), me, nil)
}

func TestAttackDayThroughTheAPI(t *testing.T) {
	if _, err := os.Stat(samplesDir + "/attack-day.log"); err != nil {
		t.Skip("sample attack-day is not generated")
	}
	h := newHarness(t)
	myID := uuid.New()
	me := token(t, myID, "sam.r@northwind.example")
	stranger := token(t, uuid.New(), "eve@elsewhere.example")

	var available []api.Sample
	h.want(200, "GET", "/v1/samples", me, nil, &available)
	if len(available) != 3 || !available[0].HasAnswerKey || available[0].Name != "attack-day" {
		t.Fatalf("samples: %+v", available)
	}
	h.wantProblem(404, "not_found", "POST", "/v1/samples/no-such-sample/load", me, nil)

	// Load the demo. It stops on the malformed lines planted in it.
	var u api.Upload
	h.want(202, "POST", "/v1/samples/attack-day/load", me, nil, &u)
	if u.Status != api.UploadStatusProcessing || u.Source != api.UploadSourceSample {
		t.Fatalf("loaded: %+v", u)
	}
	base := "/v1/uploads/" + u.Id.String()
	h.runner.Wait()
	h.want(200, "GET", base, me, nil, &u)
	if u.Status != api.UploadStatusPartial || u.Stop == nil || u.Stop.Reason != api.StopReasonParseErrors ||
		u.FirstParseError == nil || *u.ParseErrorCount != 48 {
		t.Fatalf("after load: status %s stop %+v", u.Status, u.Stop)
	}
	p := h.wantProblem(422, "validation_failed", "POST", base+"/analyze", me, nil)
	if p.Errors == nil || (*p.Errors)[0].Field != "acceptParseErrors" {
		t.Fatalf("resume without accepting: %+v", p)
	}
	h.want(202, "POST", base+"/analyze", me, api.AnalyzeRequest{AcceptParseErrors: ptr(true)}, nil)
	h.runner.Wait()
	h.want(200, "GET", base, me, nil, &u)
	if u.Status != api.UploadStatusCompleted || *u.IncidentCount != 5 || u.TimeRange == nil ||
		*u.UserCount != 25 || u.VerdictCounts == nil || u.VerdictCounts.Open != 5 {
		t.Fatalf("completed: status %s incidents %v users %v verdicts %+v", u.Status, u.IncidentCount, u.UserCount, u.VerdictCounts)
	}

	// A stranger sees none of it: every route answers 404, never 403.
	for _, probe := range [][2]string{
		{"GET", base}, {"DELETE", base}, {"POST", base + "/analyze"}, {"GET", base + "/progress"},
		{"GET", base + "/events"}, {"GET", base + "/events/export"}, {"GET", base + "/timeseries"},
		{"GET", base + "/anomalies"}, {"GET", base + "/incidents"}, {"GET", base + "/eval"},
	} {
		h.wantProblem(404, "not_found", probe[0], probe[1], stranger, nil)
	}

	// The progress stream of a finished job says so at once and closes.
	if stream := string(h.want(200, "GET", base+"/progress", me, nil, nil)); !strings.HasPrefix(stream, "event: done\ndata: {") {
		t.Fatalf("progress stream: %q", stream[:min(len(stream), 120)])
	}

	// Incidents, most severe first.
	var incidents []api.IncidentSummary
	h.want(200, "GET", base+"/incidents", me, nil, &incidents)
	if len(incidents) != 5 || incidents[0].RiskScore != 91 || incidents[0].Severity != api.SeverityHigh ||
		*incidents[0].Principal.User != "dana.k" || *incidents[0].AiVerdict != api.AiVerdictLikelyMalicious ||
		incidents[0].AnalystVerdict.Status != api.AnalystStatusOpen {
		t.Fatalf("incidents: %+v", incidents)
	}
	exfil := "/v1/incidents/" + incidents[0].Id.String()
	for _, probe := range [][2]string{{"GET", exfil}, {"GET", exfil + "/context"}} {
		h.wantProblem(404, "not_found", probe[0], probe[1], stranger, nil)
	}
	h.wantProblem(404, "not_found", "PATCH", exfil, stranger, api.UpdateIncidentRequest{AssignedToMe: true})
	h.wantProblem(404, "not_found", "POST", exfil+"/notes", stranger, api.AddNoteRequest{Text: "hi"})
	h.wantProblem(404, "not_found", "POST", exfil+"/verdict", stranger, api.VerdictRequest{Status: api.VerdictStatusDismissed})

	var detail api.IncidentDetail
	h.want(200, "GET", exfil, me, nil, &detail)
	if len(detail.Signals) != 4 || len(detail.Evidence) != 23 || len(detail.EvidenceEventIds) != 23 ||
		detail.Narrative.Status != api.NarrativeStatusGenerated || len(detail.Graph.Nodes) < 4 ||
		len(detail.Timeline) == 0 || len(detail.LessonSuggestions) == 0 || detail.Chart.BaselineBytesOut == nil ||
		detail.Notes == nil || len(detail.Mitre) == 0 {
		t.Fatalf("detail: signals %d evidence %d narrative %s nodes %d", len(detail.Signals), len(detail.Evidence),
			detail.Narrative.Status, len(detail.Graph.Nodes))
	}
	if e := detail.Evidence[0]; len(e.Flags) == 0 || e.IncidentId == nil || !strings.Contains(e.Raw, "\t") {
		t.Fatalf("evidence event lacks its flags, incident or raw line: %+v", e)
	}

	// Events: paging, filters and search.
	var events api.EventPage
	h.want(200, "GET", base+"/events?limit=1000", me, nil, &events)
	if events.Total != *u.EventCount || len(events.Items) != 1000 || events.NextCursor == nil {
		t.Fatalf("events: total %d, items %d", events.Total, len(events.Items))
	}
	h.want(200, "GET", base+"/events?user=dana.k&host=files-sync-cdn.net&sort=-ts", me, nil, &events)
	if events.Total != 21 || events.Items[0].Ts.Before(events.Items[20].Ts) {
		t.Fatalf("filtered events: total %d", events.Total)
	}
	h.want(200, "GET", base+"/events?q="+strings.ReplaceAll("user:priya.s OR domain:update-chk-svc.io", " ", "%20")+"&hasAnomaly=true", me, nil, &events)
	// priya.s: 4 phishing requests and 96 updater polls (a slow beacon,
	// below the threshold but still flagged). marcus.l: 181 beacon requests.
	if events.Total != 4+96+181 {
		t.Fatalf("search: total %d, want 281", events.Total)
	}
	h.want(200, "GET", base+"/events?action=blocked&from=2026-10-08T20:00:00Z&to=2026-10-08T20:10:00Z&hasAnomaly=true", me, nil, &events)
	if events.Total != 80 {
		t.Fatalf("blocked burst window: total %d, want 80", events.Total)
	}
	h.wantProblem(400, "bad_request", "GET", base+"/events?limit=1001", me, nil)
	h.wantProblem(400, "bad_request", "GET", base+"/events?sort=sideways", me, nil)
	h.wantProblem(400, "bad_request", "GET", base+"/events?from=yesterday", me, nil)

	// Export: one JSON event per line, every event.
	lines := 0
	scanner := bufio.NewScanner(bytes.NewReader(h.want(200, "GET", base+"/events/export", me, nil, nil)))
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		var e api.Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil || e.Id == "" {
			t.Fatalf("export line %d is not an event: %v", lines+1, err)
		}
		lines++
	}
	if lines != *u.EventCount {
		t.Fatalf("export: %d lines, want %d", lines, *u.EventCount)
	}

	// Timeseries: a full day in ten-minute buckets, peaking at the top incident.
	var series api.TimeseriesResponse
	h.want(200, "GET", base+"/timeseries?bucket=10m", me, nil, &series)
	peak, total := 0, 0
	for _, point := range series.Points {
		peak, total = max(peak, point.PeakRiskScore), total+point.Total
	}
	if series.BucketSeconds != 600 || len(series.Points) != 144 || peak != 91 || total != *u.EventCount {
		t.Fatalf("timeseries: %d points, peak %d, total %d", len(series.Points), peak, total)
	}
	h.wantProblem(400, "bad_request", "GET", base+"/timeseries?bucket=7m", me, nil)

	// Anomalies, including the ones set aside.
	var anomalies api.AnomalyPage
	h.want(200, "GET", base+"/anomalies?limit=200", me, nil, &anomalies)
	if anomalies.Total != *u.AnomalyCount || anomalies.Items[0].RiskScore < anomalies.Items[len(anomalies.Items)-1].RiskScore {
		t.Fatalf("anomalies: total %d", anomalies.Total)
	}
	h.want(200, "GET", base+"/anomalies?disposition=judged_benign", me, nil, &anomalies)
	if anomalies.Total != 1 || anomalies.Items[0].Decision == nil || *anomalies.Items[0].User != "ravi.p" {
		t.Fatalf("judged benign: %+v", anomalies)
	}
	h.want(200, "GET", base+"/anomalies?detector=beaconing&disposition=below_threshold", me, nil, &anomalies)
	if anomalies.Total != 24 {
		t.Fatalf("slow updater beacons below the threshold: %d, want 24", anomalies.Total)
	}

	// The scorecard: four planted attacks, four caught.
	var report api.EvalReport
	h.want(200, "GET", base+"/eval", me, nil, &report)
	if report.TruePositives != 4 || report.FalseNegatives != 0 || report.FalsePositives != 1 ||
		report.Recall != 1 || report.Precision != 0.8 || len(report.Scoresheet) != 5 || len(report.Providers) != 1 {
		t.Fatalf("eval: %+v", report)
	}

	// Agent context: compact and cited.
	var agent api.IncidentContext
	h.want(200, "GET", exfil+"/context", me, nil, &agent)
	if agent.SchemaVersion != "1" || len(agent.Evidence) != 23 || agent.TokenEstimate == 0 ||
		agent.Entities.Users[0] != "dana.k" || agent.Provenance.DecisionProvider != api.DecisionProviderMock {
		t.Fatalf("context: %+v", agent)
	}

	// Triage: assign, note, then dismiss the backup with a lesson.
	var summary api.IncidentSummary
	h.want(200, "PATCH", exfil, me, api.UpdateIncidentRequest{AssignedToMe: true}, &summary)
	if summary.Assignee == nil || summary.Assignee.Name != "sam.r" || summary.Assignee.Id != myID {
		t.Fatalf("assignee: %+v", summary.Assignee)
	}
	var note api.IncidentNote
	h.want(201, "POST", exfil+"/notes", me, api.AddNoteRequest{Text: "Called Dana; she was asleep at 02:14."}, &note)
	h.wantProblem(422, "validation_failed", "POST", exfil+"/notes", me, api.AddNoteRequest{Text: "  "})
	h.want(200, "POST", exfil+"/verdict", me, api.VerdictRequest{Status: api.VerdictStatusConfirmed}, &summary)
	if summary.AnalystVerdict.Status != api.AnalystStatusConfirmed || summary.AnalystVerdict.DecidedAt == nil {
		t.Fatalf("verdict: %+v", summary.AnalystVerdict)
	}

	var backup api.IncidentSummary
	for _, inc := range incidents {
		if *inc.Principal.User == "svc-backup" {
			backup = inc
		}
	}
	scope := api.LessonScope{Type: api.LessonScopeTypeDomain, Value: "s3.us-west.backblazeb2.com"}
	var preview api.LessonPreviewResponse
	h.want(200, "POST", "/v1/lessons/preview", me, api.LessonPreviewRequest{UploadId: u.Id, Scope: scope}, &preview)
	if preview.WouldSuppressAnomalies != 3 || preview.WouldResolveIncidents != 1 {
		t.Fatalf("preview: %+v", preview)
	}
	h.wantProblem(404, "not_found", "POST", "/v1/lessons/preview", stranger, api.LessonPreviewRequest{UploadId: u.Id, Scope: scope})

	verdict := "/v1/incidents/" + backup.Id.String() + "/verdict"
	p = h.wantProblem(422, "validation_failed", "POST", verdict, me, api.VerdictRequest{
		Status: api.VerdictStatusDismissed,
		Lesson: &api.LessonCreate{Text: "Backup.", Scope: scope, ExpiresAt: time.Now().Add(-time.Hour)},
	})
	if (*p.Errors)[0].Field != "lesson.expiresAt" {
		t.Fatalf("expired lesson: %+v", p.Errors)
	}
	h.want(200, "POST", verdict, me, api.VerdictRequest{
		Status: api.VerdictStatusDismissed, Note: ptr("Approved vendor backup."),
		Lesson: &api.LessonCreate{Text: "This is our nightly backup server.", Scope: scope, ExpiresAt: time.Now().Add(90 * 24 * time.Hour)},
	}, &summary)
	if summary.AnalystVerdict.Status != api.AnalystStatusDismissed || *summary.AnalystVerdict.Note != "Approved vendor backup." {
		t.Fatalf("dismissal: %+v", summary.AnalystVerdict)
	}
	h.want(200, "GET", "/v1/incidents/"+backup.Id.String(), me, nil, &detail)
	if len(detail.Notes) != 1 || detail.Notes[0].Author != "sam.r" {
		t.Fatalf("the verdict's note was not saved as a note: %+v", detail.Notes)
	}

	var lessons []api.Lesson
	h.want(200, "GET", "/v1/lessons", me, nil, &lessons)
	if len(lessons) != 1 || lessons[0].MatchCount != 0 {
		t.Fatalf("lessons: %+v", lessons)
	}
	h.want(200, "GET", "/v1/lessons", stranger, nil, &lessons)
	if len(lessons) != 0 {
		t.Fatal("a stranger can see my lessons")
	}

	// The lesson takes effect on the next run: one incident fewer.
	h.want(202, "POST", base+"/analyze", me, api.AnalyzeRequest{Restart: ptr(true), AcceptParseErrors: ptr(true)}, nil)
	h.wantProblem(409, "analysis_in_progress", "POST", base+"/analyze", me, nil)
	h.runner.Wait()
	h.want(200, "GET", base, me, nil, &u)
	h.want(200, "GET", "/v1/lessons", me, nil, &lessons)
	if *u.IncidentCount != 4 || lessons[0].MatchCount != 3 {
		t.Fatalf("after the lesson: %d incidents, lesson matched %d", *u.IncidentCount, lessons[0].MatchCount)
	}
	h.wantProblem(404, "not_found", "DELETE", "/v1/lessons/"+lessons[0].Id.String(), stranger, nil)
	h.want(204, "DELETE", "/v1/lessons/"+lessons[0].Id.String(), me, nil, nil)

	h.want(204, "DELETE", base, me, nil, nil)
	h.wantProblem(404, "not_found", "GET", exfil, me, nil)
}

func TestProgressStreamFollowsARunningJob(t *testing.T) {
	if _, err := os.Stat(samplesDir + "/large-100k.log"); err != nil {
		t.Skip("sample large-100k is not generated")
	}
	h := newHarness(t)
	me := token(t, uuid.New(), "sam.r@northwind.example")

	var u api.Upload
	h.want(202, "POST", "/v1/samples/large-100k/load", me, nil, &u)
	stream := string(h.want(200, "GET", fmt.Sprintf("/v1/uploads/%s/progress", u.Id), me, nil, nil))

	// The job stops on malformed lines, so the stream ends on a progress
	// event carrying status "partial".
	frames := strings.Split(strings.TrimSpace(stream), "\n\n")
	last := frames[len(frames)-1]
	if !strings.HasPrefix(frames[0], "event: progress\ndata: {") || !strings.Contains(last, `"status":"partial"`) {
		t.Fatalf("stream of %d frames; first %q; last %q", len(frames), frames[0][:60], last[:min(len(last), 200)])
	}
	t.Logf("stream carried %d frames", len(frames))
}

func ptr[T any](v T) *T { return &v }
