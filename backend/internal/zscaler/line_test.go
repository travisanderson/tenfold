package zscaler

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestFormatParseRoundTrip(t *testing.T) {
	want := Record{
		Time:           time.Date(2026, 10, 8, 6, 17, 41, 0, time.UTC),
		RecordID:       1048,
		Login:          "dana.k@northwindlabs.example",
		Dept:           "Finance",
		ClientIP:       "10.20.99.47",
		ServerIP:       "203.0.113.45",
		Host:           "files-sync-cdn.net",
		URL:            "https://files-sync-cdn.net/api/v2/put",
		URLCategory:    "Miscellaneous or Unknown",
		Action:         "Allowed",
		Reason:         "Allowed",
		Method:         "POST",
		Status:         200,
		ReqSize:        2306867,
		RespSize:       412,
		UserAgent:      "rclone/v1.65.0",
		AppName:        "General Browsing",
		AppClass:       "General Browsing",
		DeviceHostname: "LT-022",
		RiskScore:      78,
	}

	line := want.Format()
	if !strings.HasPrefix(line, "time=2026-10-08T06:17:41Z\trecordid=1048\tlogin=") {
		t.Fatalf("unexpected line start: %q", line)
	}
	if !strings.Contains(line, "\tthreatname=None\t") {
		t.Fatalf("empty threat name should be written as None: %q", line)
	}

	got, err := Parse(line)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got != want {
		t.Fatalf("round trip changed the record\n got: %+v\nwant: %+v", got, want)
	}
}

func TestParseRejectsBrokenLines(t *testing.T) {
	good := Record{
		Time: time.Date(2026, 10, 8, 6, 0, 0, 0, time.UTC), RecordID: 1,
		Login: "a@b.example", ClientIP: "10.20.1.1", Host: "h.example",
		URL: "https://h.example/", Action: "Allowed", Method: "GET", Status: 200,
	}.Format()

	cases := map[string]string{
		"empty":        "",
		"truncated":    good[:len(good)/2],
		"missing time": strings.SplitN(good, "\t", 2)[1],
		"garbage":      "!!! nss feed reconnect 9f3a",
		"bad action":   strings.Replace(good, "action=Allowed", "action=Maybe", 1),
		"bad number":   strings.Replace(good, "respcode=200", "respcode=two", 1),
	}
	for name, line := range cases {
		if _, err := Parse(line); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}

// The sample logs live outside version control, in the samples folder at the
// repo root. When they are present, every line must parse except those the
// answer key lists as malformed.
func TestParsesGeneratedSamples(t *testing.T) {
	keys, _ := filepath.Glob("../../../samples/*.answers.json")
	if len(keys) == 0 {
		t.Skip("no generated samples; run 'make samples'")
	}
	for _, keyPath := range keys {
		raw, err := os.ReadFile(keyPath)
		if err != nil {
			t.Fatal(err)
		}
		var key struct {
			LineCount      int   `json:"lineCount"`
			MalformedLines []int `json:"malformedLines"`
		}
		if err := json.Unmarshal(raw, &key); err != nil {
			t.Fatalf("%s: %v", keyPath, err)
		}

		logPath := strings.TrimSuffix(keyPath, ".answers.json") + ".log"
		f, err := os.Open(logPath)
		if err != nil {
			t.Fatal(err)
		}
		lineNo := 0
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			lineNo++
			_, err := Parse(scanner.Text())
			malformed := slices.Contains(key.MalformedLines, lineNo)
			if malformed && err == nil {
				t.Errorf("%s line %d: listed as malformed but parses", logPath, lineNo)
			}
			if !malformed && err != nil {
				t.Fatalf("%s line %d: %v", logPath, lineNo, err)
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		if lineNo != key.LineCount {
			t.Errorf("%s: %d lines, answer key says %d", logPath, lineNo, key.LineCount)
		}
	}
}
