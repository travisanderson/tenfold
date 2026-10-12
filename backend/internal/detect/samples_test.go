package detect

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
// SAMPLES_DIR points them at another set, such as one from a different seed.
var samplesDir = cmp.Or(os.Getenv("SAMPLES_DIR"), "../../../samples")

func loadSample(t *testing.T, name string) ([]model.Event, samples.AnswerKey) {
	t.Helper()
	return loadSampleFrom(t, samplesDir, name)
}

func loadSampleFrom(t *testing.T, dir, name string) ([]model.Event, samples.AnswerKey) {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, name+".log"))
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

	key, err := samples.NewCatalog(samples.Dir(dir)).AnswerKey(context.Background(), name)
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

// TestHoldout scores the detectors on the holdout sample: the demo's seven
// scenarios with their users, times, sizes, hosts and intervals drawn at
// random (`make holdout`). The detectors were not built against that file.
//
// It prints a scorecard and asserts nothing. A miss here is a finding to
// report. Changing a detector until this passes would make the holdout one
// more sample the detectors were built for.
func TestHoldout(t *testing.T) {
	events, key := loadSampleFrom(t, filepath.Join(samplesDir, "holdout"), "holdout")
	anomalies := Run(uuid.New(), events)
	groups := GroupAnomalies(anomalies)
	score := scoreHoldout(groups, key, t.Logf)

	attacks, caught := 0, 0
	for _, s := range key.Scenarios {
		if !s.Malicious() {
			continue
		}
		attacks++
		if score.caught[s.ID] {
			caught++
			continue
		}
		t.Logf("MISSED  %s", s.ID)
		// Say how close the detectors came.
		touched := false
		for _, g := range groups {
			if share := coverage(g, s); share > 0 {
				touched = true
				t.Logf("        a group scoring %d holds %.0f%% of its lines: %v", g.RiskScore, 100*share, signalNames(g))
			}
		}
		if !touched {
			t.Logf("        no detector flagged any of its %d lines", len(s.LineNos))
		}
	}
	decoys := 0
	for _, n := range score.decoyGroups {
		decoys += n
	}
	t.Logf("%d events -> %d anomalies -> %d groups -> %d at or above %d",
		len(events), len(anomalies), len(groups), score.raised, threshold)
	t.Logf("attacks caught: %d of %d. false positives: %d (%d decoys, %d matching nothing planted)",
		caught, attacks, decoys+score.unplanted, decoys, score.unplanted)
}

// holdoutScore is how the detectors did on one holdout day.
type holdoutScore struct {
	raised      int             // groups at or above the threshold
	caught      map[string]bool // by attack ID
	decoyGroups map[string]int  // raised groups made of a decoy, by decoy ID
	unplanted   int             // raised groups that match nothing planted
}

// scoreHoldout applies the scoring rule in samples/README.md: an attack is
// caught when a raised group holds at least half its lines. A raised group
// that catches no attack is a false positive, decoy or not. logf, if not
// nil, is told about each raised group.
func scoreHoldout(groups []Group, key samples.AnswerKey, logf func(string, ...any)) holdoutScore {
	score := holdoutScore{caught: map[string]bool{}, decoyGroups: map[string]int{}}
	for _, g := range groups {
		if g.RiskScore < threshold {
			continue
		}
		score.raised++
		match := ""
		for _, s := range key.Scenarios {
			if s.Malicious() && coverage(g, s) >= 0.5 {
				score.caught[s.ID] = true
				match = "catches " + s.ID
			}
		}
		if match == "" {
			decoy := ""
			for _, s := range key.Scenarios {
				if !s.Malicious() && (coverage(g, s) >= 0.5 || madeOf(g, s) >= 0.5) {
					decoy = s.ID
				}
			}
			if decoy == "" {
				score.unplanted++
				match = "false positive, matches nothing planted"
			} else {
				score.decoyGroups[decoy]++
				match = "false positive, the decoy " + decoy
			}
		}
		if logf != nil {
			logf("raised  %3d %-8s %-12s %d anomalies, %d events, %v: %s",
				g.RiskScore, Severity(g.RiskScore), g.Anomalies[0].User, len(g.Anomalies), len(g.LineNos), signalNames(g), match)
		}
	}
	return score
}

// tally counts hits out of tries.
type tally struct{ hit, of int }

func (c *tally) add(hit bool) {
	c.of++
	if hit {
		c.hit++
	}
}

func (c tally) String() string { return fmt.Sprintf("%d of %d", c.hit, c.of) }

// TestHoldoutSweep scores the detectors on many holdout days and prints the
// tables in the README's Evaluation section. HOLDOUT_SWEEP_DIR holds one
// folder per seed, each with a holdout.log and its answer key;
// scripts/holdout-sweep.sh writes them and runs this. HOLDOUT_SWEEP_OUT, if
// set, also receives the report.
//
// Like TestHoldout it asserts nothing, and for the same reason.
func TestHoldoutSweep(t *testing.T) {
	root := os.Getenv("HOLDOUT_SWEEP_DIR")
	if root == "" {
		t.Skip("set HOLDOUT_SWEEP_DIR, or run scripts/holdout-sweep.sh")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var seeds []int
	for _, e := range entries {
		if seed, err := strconv.Atoi(e.Name()); err == nil && e.IsDir() {
			seeds = append(seeds, seed)
		}
	}
	sort.Ints(seeds)
	if len(seeds) == 0 {
		t.Fatalf("no seed folders in %s", root)
	}

	attackIDs := []string{"atk-exfil", "atk-phish", "atk-burst", "atk-beacon"}
	decoyIDs := []string{"decoy-backup", "decoy-dev-push", "decoy-updater"}
	intervals := []struct {
		name  string
		under float64 // seconds
	}{{"under 1 minute", 60}, {"1 to 5 minutes", 300}, {"5 to 15 minutes", 900}, {"15 to 30 minutes", 1801}}

	var (
		all, fewCalls, steady, jittery tally
		caught                         = map[string]*tally{}
		byInterval                     = make([]tally, len(intervals))
		decoyDays                      = map[string]*tally{}
		decoyGroups, decoyMost         = map[string]int{}, map[string]int{}
		pollDays                       = map[float64]*tally{} // update agent, by polling interval
		raisedPerDay                   []int
		unplanted                      int
		perSeed                        strings.Builder
	)
	for _, id := range attackIDs {
		caught[id] = &tally{}
	}
	for _, id := range decoyIDs {
		decoyDays[id] = &tally{}
	}

	for _, seed := range seeds {
		dir := filepath.Join(root, strconv.Itoa(seed))
		events, key := loadSampleFrom(t, dir, "holdout")
		drawn := readDrawn(t, filepath.Join(dir, "holdout.answers.json"))
		score := scoreHoldout(GroupAnomalies(Run(uuid.New(), events)), key, nil)

		raisedPerDay = append(raisedPerDay, score.raised)
		unplanted += score.unplanted
		fmt.Fprintf(&perSeed, "seed %d: raised %2d;", seed, score.raised)
		for _, id := range attackIDs {
			caught[id].add(score.caught[id])
			all.add(score.caught[id])
			fmt.Fprintf(&perSeed, " %s %s;", id, map[bool]string{true: "caught", false: "MISSED"}[score.caught[id]])
		}
		for _, id := range decoyIDs {
			n := score.decoyGroups[id]
			decoyDays[id].add(n > 0)
			decoyGroups[id] += n
			decoyMost[id] = max(decoyMost[id], n)
			fmt.Fprintf(&perSeed, " %s %d;", id, n)
		}

		beacon, hit := drawn["atk-beacon"], score.caught["atk-beacon"]
		for i, bucket := range intervals {
			if beacon["everySeconds"] < bucket.under {
				byInterval[i].add(hit)
				break
			}
		}
		offBeat := beacon["jitterSeconds"] / beacon["everySeconds"]
		switch {
		case beacon["calls"] < 30:
			fewCalls.add(hit)
		case offBeat < 0.10:
			steady.add(hit)
		default:
			jittery.add(hit)
		}
		every := drawn["decoy-updater"]["everySeconds"]
		if pollDays[every] == nil {
			pollDays[every] = &tally{}
		}
		pollDays[every].add(score.decoyGroups["decoy-updater"] > 0)
		fmt.Fprintf(&perSeed, " beacon %.0f calls, every %.0fs, give or take %.0fs; agent polls every %.0fs\n",
			beacon["calls"], beacon["everySeconds"], beacon["jitterSeconds"], every)
	}

	var out strings.Builder
	fmt.Fprintf(&out, "Holdout sweep: seeds %d to %d, %d days, detectors only, threshold %d\n\n",
		seeds[0], seeds[len(seeds)-1], len(seeds), threshold)
	fmt.Fprintf(&out, "Attacks caught: %s\n", all)
	for _, id := range attackIDs {
		fmt.Fprintf(&out, "  %-15s caught on %s days\n", id, caught[id])
	}
	fmt.Fprintf(&out, "\nDecoys raised:\n")
	for _, id := range decoyIDs {
		fmt.Fprintf(&out, "  %-15s raised on %s days; %d groups in all, at most %d on one day\n",
			id, decoyDays[id], decoyGroups[id], decoyMost[id])
	}
	sort.Ints(raisedPerDay)
	fmt.Fprintf(&out, "\nGroups raised that match nothing planted: %d\n", unplanted)
	fmt.Fprintf(&out, "Groups raised per day: median %d, least %d, most %d\n",
		raisedPerDay[len(raisedPerDay)/2], raisedPerDay[0], raisedPerDay[len(raisedPerDay)-1])

	fmt.Fprintf(&out, "\nBeacon, by the interval drawn:\n")
	for i, bucket := range intervals {
		fmt.Fprintf(&out, "  %-17s caught %s\n", bucket.name, byInterval[i])
	}
	fmt.Fprintf(&out, "\nBeacon, by what the rule needs:\n")
	fmt.Fprintf(&out, "  fewer than %d calls in the day                       caught %s\n", beaconMinRequests, fewCalls)
	fmt.Fprintf(&out, "  %d or more, timing varies by under 10%% of the beat   caught %s\n", beaconMinRequests, steady)
	fmt.Fprintf(&out, "  %d or more, timing varies by 10%% or more             caught %s\n", beaconMinRequests, jittery)

	fmt.Fprintf(&out, "\nUpdate agent, by how often it polls:\n")
	var polls []float64
	for every := range pollDays {
		polls = append(polls, every)
	}
	sort.Float64s(polls)
	for _, every := range polls {
		fmt.Fprintf(&out, "  every %2.0f minutes   raised on %s days\n", every/60, pollDays[every])
	}
	fmt.Fprintf(&out, "\nEach day:\n%s", perSeed.String())

	t.Log("\n" + out.String())
	if path := os.Getenv("HOLDOUT_SWEEP_OUT"); path != "" {
		if err := os.WriteFile(path, []byte(out.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// readDrawn returns the numbers a holdout drew for each scenario, by
// scenario ID. The server's answer key type has no use for them.
func readDrawn(t *testing.T, path string) map[string]map[string]float64 {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var key struct {
		Scenarios []struct {
			ID    string             `json:"id"`
			Drawn map[string]float64 `json:"drawn"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(raw, &key); err != nil {
		t.Fatal(err)
	}
	drawn := map[string]map[string]float64{}
	for _, s := range key.Scenarios {
		drawn[s.ID] = s.Drawn
	}
	return drawn
}

// madeOf is the share of a group's evidence that belongs to a scenario.
func madeOf(g Group, s samples.Scenario) float64 {
	in := map[int]bool{}
	for _, n := range s.LineNos {
		in[n] = true
	}
	hit := 0
	for _, n := range g.LineNos {
		if in[n] {
			hit++
		}
	}
	return float64(hit) / float64(len(g.LineNos))
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
