package main

import (
	"bytes"
	"crypto/sha256"
	"slices"
	"strings"
	"testing"
	"time"
)

var testDay = time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)

func profileNamed(t *testing.T, name string) Profile {
	t.Helper()
	for _, p := range profiles {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("no profile named %s", name)
	return Profile{}
}

func lines(log []byte) []string {
	return strings.Split(strings.TrimSuffix(string(log), "\n"), "\n")
}

func TestSameSeedGivesIdenticalFiles(t *testing.T) {
	for _, p := range profiles {
		logA, keyA := Generate(p, 42, testDay)
		logB, keyB := Generate(p, 42, testDay)
		if sha256.Sum256(logA) != sha256.Sum256(logB) {
			t.Errorf("%s: same seed gave different logs", p.Name)
		}
		if !bytes.Equal(MarshalKey(keyA), MarshalKey(keyB)) {
			t.Errorf("%s: same seed gave different answer keys", p.Name)
		}
	}

	p := profileNamed(t, "clean-day")
	logA, _ := Generate(p, 42, testDay)
	logC, _ := Generate(p, 43, testDay)
	if bytes.Equal(logA, logC) {
		t.Error("different seeds gave the same log")
	}
}

func TestEveryLineParsesExceptTheMalformed(t *testing.T) {
	for _, p := range profiles {
		log, key := Generate(p, 42, testDay)
		all := lines(log)
		if len(all) != key.LineCount {
			t.Fatalf("%s: answer key says %d lines, file has %d", p.Name, key.LineCount, len(all))
		}

		var last time.Time
		for i, line := range all {
			lineNo := i + 1
			rec, err := Parse(line)
			if slices.Contains(key.MalformedLines, lineNo) {
				if err == nil {
					t.Errorf("%s line %d: listed as malformed but parses", p.Name, lineNo)
				}
				continue
			}
			if err != nil {
				t.Fatalf("%s line %d: %v", p.Name, lineNo, err)
			}
			if rec.RecordID != int64(lineNo) {
				t.Fatalf("%s line %d: recordid is %d", p.Name, lineNo, rec.RecordID)
			}
			if rec.Time.Before(last) {
				t.Fatalf("%s line %d: out of time order", p.Name, lineNo)
			}
			if rec.Time.Before(key.TimeRange.Start) || rec.Time.After(key.TimeRange.End) {
				t.Fatalf("%s line %d: %s is outside the day", p.Name, lineNo, rec.Time)
			}
			if rec.Format() != line {
				t.Fatalf("%s line %d: field order is not stable", p.Name, lineNo)
			}
			last = rec.Time
		}

		share := float64(len(key.MalformedLines)) / float64(key.LineCount)
		if share < 0.0005 || share > 0.002 {
			t.Errorf("%s: %.4f of lines are malformed, want about 0.001", p.Name, share)
		}
		if diff := key.LineCount - p.Lines; diff < -p.Lines/50 || diff > p.Lines/50 {
			t.Errorf("%s: %d lines, want about %d", p.Name, key.LineCount, p.Lines)
		}
	}
}

func TestAnswerKeyPointsAtTheRightLines(t *testing.T) {
	log, key := Generate(profileNamed(t, "attack-day"), 42, testDay)
	all := lines(log)

	byID := map[string]Scenario{}
	for _, s := range key.Scenarios {
		byID[s.ID] = s
		if len(s.LineNos) == 0 {
			t.Fatalf("%s has no lines", s.ID)
		}
		for _, n := range s.LineNos {
			rec, err := Parse(all[n-1])
			if err != nil {
				t.Fatalf("%s line %d does not parse: %v", s.ID, n, err)
			}
			name, _, _ := strings.Cut(rec.Login, "@")
			if !slices.Contains(s.Users, name) {
				t.Fatalf("%s line %d: user %s is not in the scenario", s.ID, n, name)
			}
			if !slices.Contains(s.Hosts, rec.DeviceHostname) {
				t.Fatalf("%s line %d: device %s is not in the scenario", s.ID, n, rec.DeviceHostname)
			}
			if rec.Time.Before(s.TimeRange.Start) || rec.Time.After(s.TimeRange.End) {
				t.Fatalf("%s line %d: outside the scenario's time range", s.ID, n)
			}
		}
	}

	for _, id := range []string{"atk-exfil", "atk-beacon", "atk-phish", "atk-burst"} {
		if byID[id].Label != "malicious" {
			t.Errorf("%s: want a malicious scenario, got %q", id, byID[id].Label)
		}
	}
	for _, id := range []string{"decoy-backup", "decoy-dev-push", "decoy-updater"} {
		if byID[id].Label != "benign" {
			t.Errorf("%s: want a benign scenario, got %q", id, byID[id].Label)
		}
	}

	// The exfiltration: 23 requests, about 40 MB to one first-seen domain.
	exfil := byID["atk-exfil"]
	if len(exfil.LineNos) != 23 {
		t.Errorf("atk-exfil: %d lines, want 23", len(exfil.LineNos))
	}
	var sent int64
	for _, n := range exfil.LineNos {
		rec, _ := Parse(all[n-1])
		if rec.Host == "files-sync-cdn.net" {
			sent += rec.ReqSize
		}
	}
	if mb := float64(sent) / (1 << 20); mb < 37 || mb > 43 {
		t.Errorf("atk-exfil: %.1f MB sent, want about 40", mb)
	}
	for i, line := range all {
		if strings.Contains(line, "host=files-sync-cdn.net\t") && !slices.Contains(exfil.LineNos, i+1) {
			t.Fatalf("line %d reaches the exfil domain but is not in the answer key", i+1)
		}
	}

	// The beacon: 181 requests, each close to 60 seconds after the last.
	beacon := byID["atk-beacon"]
	if len(beacon.LineNos) != 181 {
		t.Fatalf("atk-beacon: %d lines, want 181", len(beacon.LineNos))
	}
	var prev time.Time
	for i, n := range beacon.LineNos {
		rec, _ := Parse(all[n-1])
		if i > 0 {
			if gap := rec.Time.Sub(prev); gap < 56*time.Second || gap > 64*time.Second {
				t.Fatalf("atk-beacon: gap of %s at line %d", gap, n)
			}
		}
		prev = rec.Time
	}

	if n := len(byID["atk-burst"].LineNos); n != 80 {
		t.Errorf("atk-burst: %d lines, want 80", n)
	}
}

func TestCleanDayHasNoAttacks(t *testing.T) {
	log, key := Generate(profileNamed(t, "clean-day"), 42, testDay)
	for _, s := range key.Scenarios {
		if s.Label != "benign" {
			t.Errorf("clean-day contains %s (%s)", s.ID, s.Label)
		}
	}
	for _, marker := range []string{"files-sync-cdn.net", "update-chk-svc.io", "Phish.Generic", "urlcat=Anonymizer"} {
		if bytes.Contains(log, []byte(marker)) {
			t.Errorf("clean-day contains %q", marker)
		}
	}
}

// The holdout must hold together for any seed: every scenario line names the
// scenario's own user, device and domain, and falls inside the day.
func TestHoldoutHoldsTogetherForAnySeed(t *testing.T) {
	p := profileNamed(t, "holdout")
	scripts := map[string]bool{}
	for seed := uint64(1); seed <= 12; seed++ {
		log, key := Generate(p, seed, testDay)
		all := lines(log)
		if len(key.Scenarios) != 7 {
			t.Fatalf("seed %d: %d scenarios, want 7", seed, len(key.Scenarios))
		}
		script := ""
		cast := map[string]string{}
		for _, s := range key.Scenarios {
			script += s.Note + "|" + strings.Join(s.Domains, ",") + "|"
			if s.Note == "" || len(s.LineNos) == 0 {
				t.Fatalf("seed %d: %s has no note or no lines", seed, s.ID)
			}
			if len(s.ExpectedDetectors) != 0 {
				t.Errorf("seed %d: %s promises detectors %v", seed, s.ID, s.ExpectedDetectors)
			}
			if len(s.Users) == 1 && s.Users[0] != "svc-backup" {
				if other, taken := cast[s.Users[0]]; taken {
					t.Errorf("seed %d: %s and %s share %s", seed, s.ID, other, s.Users[0])
				}
				cast[s.Users[0]] = s.ID
			}
			for _, n := range s.LineNos {
				rec, err := Parse(all[n-1])
				if err != nil {
					t.Fatalf("seed %d: %s line %d does not parse: %v", seed, s.ID, n, err)
				}
				name, _, _ := strings.Cut(rec.Login, "@")
				if !slices.Contains(s.Users, name) || !slices.Contains(s.Hosts, rec.DeviceHostname) {
					t.Fatalf("seed %d: %s line %d belongs to %s on %s", seed, s.ID, n, name, rec.DeviceHostname)
				}
				if rec.Host != "login.microsoftonline.com" && !slices.Contains(s.Domains, rec.Host) {
					t.Fatalf("seed %d: %s line %d reaches %s", seed, s.ID, n, rec.Host)
				}
				if rec.Time.Before(key.TimeRange.Start) || rec.Time.After(key.TimeRange.End) {
					t.Fatalf("seed %d: %s line %d: %s is outside the day", seed, s.ID, n, rec.Time)
				}
			}
		}
		scripts[script] = true
	}
	if len(scripts) != 12 {
		t.Errorf("12 seeds drew %d different scripts", len(scripts))
	}
}
