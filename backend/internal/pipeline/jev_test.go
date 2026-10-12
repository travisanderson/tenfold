package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tenfold/backend/internal/api"
)

// fakeJev stands in for the TypeSafe API. Each request gets the next reply;
// the last reply repeats.
func fakeJev(t *testing.T, replies ...func(w http.ResponseWriter)) (JevJudge, *[]jevSeen) {
	t.Helper()
	seen := &[]jevSeen{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got := jevSeen{Auth: r.Header.Get("Authorization"), Raw: string(raw)}
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Errorf("request body: %v", err)
		}
		*seen = append(*seen, got)
		replies[min(len(*seen), len(replies))-1](w)
	}))
	t.Cleanup(server.Close)
	judge := NewJevJudge("test-key", DefaultJevModel)
	judge.url, judge.pause = server.URL, time.Millisecond
	return judge, seen
}

type jevSeen struct {
	Auth  string
	Raw   string
	Model string `json:"model"`
	State struct {
		Anomaly struct {
			Detector string `json:"detector"`
			Finding  string `json:"finding"`
		} `json:"anomaly"`
		Related []map[string]string `json:"related_findings"`
		Events  []map[string]any    `json:"events"`
	} `json:"state"`
	Questions map[string]struct {
		Type     string            `json:"type"`
		Criteria map[string]string `json:"criteria"`
	} `json:"questions"`
}

// verdict is a Jev reply that chose an option with some confidence.
func verdict(choice string, confidence float64) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		fmt.Fprintf(w, `{"model":"jev-1.13.0","answers":{"verdict":{"type":"choice","choice":%q,
			"probabilities":{"malicious":0.5,"benign":0.5},"confidence":%v}},"usage":{"input_tokens":900,"output_tokens":30}}`,
			choice, confidence)
	}
}

func status(code int, body string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.WriteHeader(code)
		io.WriteString(w, body)
	}
}

func TestJevJudgeAsksOneTypedQuestion(t *testing.T) {
	judge, seen := fakeJev(t, verdict("malicious", 0.96))
	in, _ := exfilCase()
	d, err := judge.Decide(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if d.Label != api.AiVerdictLikelyMalicious || d.Confidence != 0.96 || d.Provider != api.DecisionProviderJev ||
		d.Model == nil || *d.Model != "jev-1.13.0" {
		t.Errorf("decision = %+v", d)
	}

	req := (*seen)[0]
	if req.Auth != "Bearer test-key" || req.Model != DefaultJevModel {
		t.Errorf("auth %q, model %q", req.Auth, req.Model)
	}
	q := req.Questions["verdict"]
	if q.Type != "choice" || len(q.Criteria) != 2 || q.Criteria[jevMalicious] == "" || q.Criteria[jevBenign] == "" {
		t.Errorf("question = %+v", q)
	}
	if req.State.Anomaly.Detector != "exfil_volume" || !strings.Contains(req.State.Anomaly.Finding, "41.0 MB") {
		t.Errorf("anomaly = %+v", req.State.Anomaly)
	}
	if len(req.State.Related) != 1 || req.State.Related[0]["detector"] != "rare_destination" {
		t.Errorf("related findings = %v", req.State.Related)
	}
	if len(req.State.Events) != 2 || req.State.Events[0]["id"] != "e31877" || req.State.Events[0]["sent"] != "4.0 MB" {
		t.Errorf("events = %v", req.State.Events)
	}
	// Like Claude, Jev is a second opinion: it must not see the first one's score.
	for _, hidden := range []string{`"91"`, ":91", "weight", "risk"} {
		if strings.Contains(strings.ToLower(req.Raw), hidden) {
			t.Errorf("Jev was shown %q", hidden)
		}
	}
}

func TestJevJudgeLeavesTheUnsureCasesToAPerson(t *testing.T) {
	cases := map[string]struct {
		reply      func(http.ResponseWriter)
		label      api.AiVerdict
		confidence float64
	}{
		"sure it is benign":    {verdict("benign", 0.84), api.AiVerdictLikelyBenign, 0.84},
		"at the line":          {verdict("benign", 0.5), api.AiVerdictLikelyBenign, 0.5},
		"leaning benign":       {verdict("benign", 0.31), api.AiVerdictInconclusive, 0.31},
		"leaning malicious":    {verdict("malicious", 0.2), api.AiVerdictInconclusive, 0.2},
		"an option we did not": {verdict("harmless", 0.9), api.AiVerdictInconclusive, 0},
		"no answer at all":     {status(200, `{"model":"jev-1.13.0","answers":{}}`), api.AiVerdictInconclusive, 0},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			judge, _ := fakeJev(t, c.reply)
			in, _ := exfilCase()
			d, err := judge.Decide(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}
			if d.Label != c.label || d.Confidence != c.confidence {
				t.Errorf("decision = %s %v, want %s %v", d.Label, d.Confidence, c.label, c.confidence)
			}
		})
	}
}

func TestJevJudgeWaitsWhenTheAPIIsBusy(t *testing.T) {
	in, _ := exfilCase()

	judge, seen := fakeJev(t, status(529, `{"error":"overloaded"}`), status(429, `{"error":"slow down"}`), verdict("benign", 0.9))
	d, err := judge.Decide(context.Background(), in)
	if err != nil || d.Label != api.AiVerdictLikelyBenign || len(*seen) != 3 {
		t.Errorf("after two busy replies: decision %+v, err %v, %d requests", d, err, len(*seen))
	}

	judge, seen = fakeJev(t, status(529, `{"error":"overloaded"}`))
	if _, err := judge.Decide(context.Background(), in); err == nil || len(*seen) != jevAttempts {
		t.Errorf("always busy: err %v after %d requests, want an error after %d", err, len(*seen), jevAttempts)
	}

	// A bad key is not "busy": fail at once, and keep the key out of the error.
	judge, seen = fakeJev(t, status(401, `{"error":"invalid API key"}`))
	_, err = judge.Decide(context.Background(), in)
	if err == nil || len(*seen) != 1 || strings.Contains(err.Error(), "test-key") {
		t.Errorf("bad key: err %v after %d requests", err, len(*seen))
	}
}
