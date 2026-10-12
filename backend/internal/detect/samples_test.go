package detect

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/uuid"

	"tenfold/backend/internal/model"
	"tenfold/backend/internal/samples"
	"tenfold/backend/internal/zscaler"
)

// threshold mirrors the server's default RISK_THRESHOLD.
const threshold = 30

// The samples folder is not in version control. These tests run when it
// has been generated (`make samples`) and are skipped when it has not.
const samplesDir = "../../../samples"

func loadSample(t *testing.T, name string) ([]model.Event, samples.AnswerKey) {
	t.Helper()
	f, err := os.Open(filepath.Join(samplesDir, name+".log"))
	if err != nil {
		t.Skipf("sample %s is not generated: %v", name, err)
	}
	defer f.Close()

	var events []model.Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		if record, err := zscaler.Parse(scanner.Text()); err == nil {
			events = append(events, model.EventFromRecord(lineNo, record, ""))
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].TS.Before(events[j].TS) })

	key, err := samples.NewCatalog(samples.Dir(samplesDir)).AnswerKey(context.Background(), name)
	if err != nil {
		t.Fatalf("answer key: %v", err)
	}
	return events, key
}

// coverage is the share of a scenario's lines that a group's evidence holds.
func coverage(g Group, s samples.Scenario) float64 {
	in := map[int]bool{}
	for _, n := range g.LineNos {
		in[n] = true
	}
	hit := 0
	for _, n := range s.LineNos {
		if in[n] {
			hit++
		}
	}
	return float64(hit) / float64(len(s.LineNos))
}

func TestAttackDayCatchesEveryPlantedAttack(t *testing.T) {
	for _, name := range []string{"attack-day", "large-100k"} {
		t.Run(name, func(t *testing.T) {
			events, key := loadSample(t, name)
			anomalies := Run(uuid.New(), events)
			groups := GroupAnomalies(anomalies)

			raised := 0
			for _, g := range groups {
				if g.RiskScore >= threshold {
					raised++
					t.Logf("raised  %3d %-8s %s  (%d anomalies, %d events) %s",
						g.RiskScore, Severity(g.RiskScore), g.Anomalies[0].User, len(g.Anomalies), len(g.LineNos), signalNames(g))
				}
			}
			t.Logf("%d events -> %d anomalies -> %d groups -> %d at or above %d",
				len(events), len(anomalies), len(groups), raised, threshold)

			for _, scenario := range key.Scenarios {
				if !scenario.Malicious() {
					continue
				}
				caught := false
				for _, g := range groups {
					if g.RiskScore >= threshold && coverage(g, scenario) >= 0.5 {
						caught = true
					}
				}
				if !caught {
					t.Errorf("missed planted attack %s", scenario.ID)
				}
			}
			// The firehose must come out as a handful, not a flood.
			if raised > 10 {
				t.Errorf("%d groups reached the threshold; want a handful", raised)
			}
		})
	}
}

// A day with no attacks should raise the decoys, which are built to look
// suspicious, and nothing else at all.
func TestCleanDayRaisesNothingButTheDecoys(t *testing.T) {
	events, key := loadSample(t, "clean-day")
	groups := GroupAnomalies(Run(uuid.New(), events))
	for _, g := range groups {
		if g.RiskScore < threshold {
			continue
		}
		explained := false
		for _, scenario := range key.Scenarios {
			if coverage(g, scenario) >= 0.5 {
				explained = true
			}
		}
		t.Logf("raised  %3d %-8s %s %s decoy=%v", g.RiskScore, Severity(g.RiskScore), g.Anomalies[0].User, signalNames(g), explained)
		if !explained {
			t.Errorf("a group for %s scored %d on a day with no attacks and no matching decoy",
				g.Anomalies[0].User, g.RiskScore)
		}
	}
}

func TestRunIsDeterministic(t *testing.T) {
	events, _ := loadSample(t, "attack-day")
	id := uuid.New()
	first, second := Run(id, events), Run(id, events)
	if len(first) != len(second) {
		t.Fatalf("two runs found %d and %d anomalies", len(first), len(second))
	}
	for i := range first {
		a, b := first[i], second[i]
		if a.Detector != b.Detector || a.Reason != b.Reason || a.RiskScore != b.RiskScore || len(a.LineNos) != len(b.LineNos) {
			t.Fatalf("anomaly %d differs between runs:\n%+v\n%+v", i, a, b)
		}
	}
}

func signalNames(g Group) []string {
	var names []string
	for _, s := range g.Signals {
		names = append(names, s.Name)
	}
	return names
}
