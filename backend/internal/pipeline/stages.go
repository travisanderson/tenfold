package pipeline

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/blob"
	"tenfold/backend/internal/detect"
	"tenfold/backend/internal/model"
	"tenfold/backend/internal/samples"
	"tenfold/backend/internal/zscaler"
)

const (
	insertBatch     = 5000    // events per COPY
	maxLineBytes    = 1 << 20 // no proxy log line is a megabyte long
	parseSampleSize = 120     // characters of a bad line kept for the analyst

	// aiSetAsideBelow guards the AI's one power: setting a group aside as
	// benign. It may do so only below this risk score. At or above it, the
	// group reaches the analyst whatever the model says.
	aiSetAsideBelow = 65
)

// ---------- Stage 1: parsing ----------

// parse reads the file line by line, normalizes each good line into an
// event and bulk-loads the events. It never holds the whole file in memory.
func (j *job) parse(ctx context.Context) error {
	const stage = api.PipelineStageParsing

	file, err := j.open(ctx)
	if err != nil {
		return err
	}
	defer file.Close()
	read := &countingReader{r: file}

	if err := j.r.store.DeleteEvents(ctx, j.u.ID); err != nil {
		return err
	}

	var (
		batch       = make([]model.Event, 0, insertBatch)
		events      int
		parseErrors int
		firstError  *api.ParseError
		users       = map[string]bool{}
		devices     = map[string]bool{}
		start, end  time.Time
	)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := j.r.store.InsertEvents(ctx, j.u.ID, batch); err != nil {
			return err
		}
		batch = batch[:0]
		// Progress follows bytes read, since the line count is not known yet.
		j.u.LineCount, j.u.EventCount, j.u.ParseErrorCount = ptr(events+parseErrors), &events, &parseErrors
		j.advance(ctx, stage, events+parseErrors, int(read.n>>10), int(j.u.SizeBytes>>10),
			fmt.Sprintf("Reading lines: %s so far", detect.FormatCount(events+parseErrors)))
		return nil
	}

	scanner := bufio.NewScanner(read)
	scanner.Buffer(make([]byte, 0, 64<<10), maxLineBytes)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		record, err := parseLine(line)
		if err != nil {
			parseErrors++
			if firstError == nil {
				firstError = &api.ParseError{LineNo: lineNo, Reason: err.Error(), Sample: truncate(line, parseSampleSize)}
			}
			continue
		}
		e := model.EventFromRecord(lineNo, record, line)
		events++
		users[e.User] = true
		if e.Device != "" {
			devices[e.Device] = true
		}
		if start.IsZero() || e.TS.Before(start) {
			start = e.TS
		}
		if e.TS.After(end) {
			end = e.TS
		}
		batch = append(batch, e)
		if len(batch) == insertBatch {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if errors.Is(scanner.Err(), bufio.ErrTooLong) {
		return failure{"parse_failed", "A line is longer than 1 MB. This does not look like a proxy log."}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
	}

	lines := events + parseErrors
	if events == 0 {
		return failure{"parse_failed", "No line matched the Zscaler web-proxy format."}
	}
	j.u.LineCount, j.u.EventCount, j.u.ParseErrorCount = &lines, &events, &parseErrors
	j.u.FirstParseError = firstError
	j.u.UserCount, j.u.HostCount = ptr(len(users)), ptr(len(devices))
	j.u.TimeStart, j.u.TimeEnd = &start, &end
	j.finish(ctx, stage, lines, events, fmt.Sprintf("Read %s", pluralize(lines, "line")))

	if parseErrors > 0 && !j.accept {
		return parseStop{fmt.Sprintf("%s of %s lines could not be read. Analyze the good lines, or cancel this upload.",
			detect.FormatCount(parseErrors), detect.FormatCount(lines))}
	}
	return nil
}

// parseLine wraps the Zscaler parser with one more check: Postgres text
// cannot hold invalid UTF-8 or a NUL byte, so such a line is malformed.
func parseLine(line string) (zscaler.Record, error) {
	if !utf8.ValidString(line) || strings.ContainsRune(line, 0) {
		return zscaler.Record{}, errors.New("line is not valid UTF-8 text")
	}
	return zscaler.Parse(line)
}

// open returns the upload's bytes: a bundled sample, or the stored file.
func (j *job) open(ctx context.Context) (io.ReadCloser, error) {
	if j.u.Source == string(api.UploadSourceSample) && j.u.SampleName != nil {
		f, err := j.r.samples.Open(*j.u.SampleName)
		if errors.Is(err, samples.ErrNotFound) {
			return nil, failure{"sample_missing", "This demo sample is no longer available on the server."}
		}
		return f, err
	}
	if j.u.StorageKey == nil {
		return nil, failure{"file_missing", "This upload has no file. Upload it again."}
	}
	f, err := j.r.blobs.Open(ctx, *j.u.StorageKey)
	if errors.Is(err, blob.ErrNotFound) {
		return nil, failure{"file_missing", "The file never reached storage. Upload it again."}
	}
	return f, err
}

// ---------- Stage 2: detecting ----------

// detect runs the deterministic detectors, applies the analyst's lessons,
// and sets aside every group that scores under the threshold.
func (j *job) detect(ctx context.Context) error {
	const stage = api.PipelineStageDetecting

	events, err := j.r.store.LoadEvents(ctx, j.u.ID)
	if err != nil {
		return err
	}
	j.advance(ctx, stage, 0, 0, 1, fmt.Sprintf("Scoring %s", pluralize(len(events), "event")))
	anomalies := detect.Run(j.u.ID, events)

	// Lessons first: what an analyst has already explained never reaches
	// the threshold, the judge or an incident.
	lessons, err := j.r.store.ListLessons(ctx, j.u.UserID, true)
	if err != nil {
		return err
	}
	matches := map[uuid.UUID]int{}
	for i := range anomalies {
		for _, lesson := range lessons {
			if detect.MatchesLesson(anomalies[i], lesson.ScopeType, lesson.ScopeValue) {
				anomalies[i].Disposition = model.DispositionLesson
				anomalies[i].LessonID = ptr(lesson.ID)
				matches[lesson.ID]++
				break
			}
		}
	}

	for _, g := range detect.GroupAnomalies(anomalies) {
		if !j.qualifies(g) {
			for _, a := range g.Anomalies {
				a.Disposition = model.DispositionBelowThreshold
			}
		}
	}

	if err := j.r.store.ReplaceAnomalies(ctx, j.u.ID, anomalies); err != nil {
		return err
	}
	if err := j.r.store.AddLessonMatches(ctx, matches); err != nil {
		return err
	}
	j.u.AnomalyCount, j.u.IncidentCount = ptr(len(anomalies)), nil
	j.finish(ctx, stage, len(events), len(anomalies), fmt.Sprintf("Found %s", pluralize(len(anomalies), "anomaly")))
	return nil
}

// ---------- Stage 3: judging ----------

// judge asks the AI for a label on every anomaly in a group that reached
// the threshold. A group the AI calls benign is set aside, unless its risk
// score is too high for the AI to overrule.
func (j *job) judge(ctx context.Context) error {
	const stage = api.PipelineStageJudging

	anomalies, err := j.r.store.LoadAnomalies(ctx, j.u.ID)
	if err != nil {
		return err
	}
	var groups []detect.Group
	candidates := 0
	for _, g := range detect.GroupAnomalies(anomalies) {
		if j.qualifies(g) {
			groups = append(groups, g)
			candidates += len(g.Anomalies)
			for _, a := range g.Anomalies { // a clean slate when the stage reruns
				a.Disposition, a.Decision, a.ShadowDecision = model.DispositionPending, nil, nil
			}
		}
	}
	if j.r.ai.Judge == nil {
		return errAIUnavailable
	}

	done, kept := 0, 0
	for _, g := range groups {
		for _, a := range g.Anomalies {
			decision, err := j.r.ai.Judge.Decide(ctx, JudgeInput{Anomaly: *a, Group: g})
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return fmt.Errorf("%w: %v", errAIUnavailable, err)
			}
			a.Decision = &decision
			done++
			j.advance(ctx, stage, done, done, candidates, fmt.Sprintf("Judging anomaly %d of %d", done, candidates))
		}
		verdict, _ := groupVerdict(g)
		if verdict == api.AiVerdictLikelyBenign && g.RiskScore < aiSetAsideBelow {
			for _, a := range g.Anomalies {
				a.Disposition = model.DispositionJudgedBenign
			}
		} else {
			kept += len(g.Anomalies)
		}
	}

	if err := j.r.store.SaveDecisions(ctx, anomalies); err != nil {
		return err
	}
	j.finish(ctx, stage, candidates, kept,
		fmt.Sprintf("Kept %d of %s", kept, pluralize(candidates, "anomaly")))
	return nil
}

// ---------- Stage 4: grouping ----------

// group turns each surviving group into an incident, with everything the
// incident screen draws except the narrative.
func (j *job) group(ctx context.Context) error {
	const stage = api.PipelineStageGrouping

	anomalies, err := j.r.store.LoadAnomalies(ctx, j.u.ID)
	if err != nil {
		return err
	}
	events, err := j.r.store.LoadEvents(ctx, j.u.ID)
	if err != nil {
		return err
	}
	file := newFileView(events)
	j.advance(ctx, stage, 0, 0, 1, "Grouping related anomalies")

	// When this stage reruns, the old incidents are about to be replaced,
	// so no anomaly may keep pointing at one.
	for i := range anomalies {
		anomalies[i].IncidentID = nil
	}

	var incidents []model.Incident
	received := 0
	for _, g := range detect.GroupAnomalies(anomalies) {
		if !j.qualifies(g) || g.Anomalies[0].Disposition == model.DispositionJudgedBenign {
			continue
		}
		received += len(g.Anomalies)
		incident := buildIncident(j.u.ID, j.u.UserID, g, file)
		for _, a := range g.Anomalies {
			a.Disposition, a.IncidentID = model.DispositionInIncident, ptr(incident.ID)
			if a.Decision != nil {
				incident.Detail.DecisionProvider = ptr(a.Decision.Provider)
			}
		}
		incidents = append(incidents, incident)
	}
	// Nothing may stay pending once grouping is done.
	for i := range anomalies {
		if anomalies[i].Disposition == model.DispositionPending {
			anomalies[i].Disposition = model.DispositionBelowThreshold
		}
	}

	if err := j.r.store.ReplaceIncidents(ctx, j.u.ID, incidents, anomalies); err != nil {
		return err
	}
	j.u.IncidentCount = ptr(len(incidents))
	j.finish(ctx, stage, received, len(incidents), fmt.Sprintf("Raised %s", pluralize(len(incidents), "incident")))
	return nil
}

// ---------- Stage 5: narrating ----------

// narrate asks the AI for an account of each incident and checks every
// citation against the incident's evidence. A draft that fails the check
// is replaced by the deterministic template.
func (j *job) narrate(ctx context.Context) error {
	const stage = api.PipelineStageNarrating

	incidents, err := j.r.store.LoadIncidents(ctx, j.u.ID)
	if err != nil {
		return err
	}
	anomalies, err := j.r.store.LoadAnomalies(ctx, j.u.ID)
	if err != nil {
		return err
	}
	byIncident := map[uuid.UUID][]model.Anomaly{}
	for _, a := range anomalies {
		if a.IncidentID != nil {
			byIncident[*a.IncidentID] = append(byIncident[*a.IncidentID], a)
		}
	}

	// If judging could not run, the narrator is not asked either: the AI
	// panel says "not available" as a whole, never half an opinion.
	narrator := j.r.ai.Narrator
	if narrator == nil || j.aiDown != nil {
		for _, inc := range incidents {
			inc.Detail.Narrative, inc.Summary = unavailableNarrative(), nil
			if err := j.r.store.SaveNarrative(ctx, inc); err != nil {
				return err
			}
		}
		return errAIUnavailable
	}

	generated := 0
	for i, inc := range incidents {
		in := NarrateInput{Incident: inc, Anomalies: byIncident[inc.ID]}
		draft, err := narrator.Narrate(ctx, in)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("%w: %v", errAIUnavailable, err)
		}
		now := time.Now().UTC()
		if err := checkCitations(draft, inc.LineNos); err != nil {
			j.r.log.Warn("a narrative failed its citation check; using the template",
				"upload", j.u.ID, "incident", inc.ID, "reason", err.Error())
			draft = templateDraft(in)
			inc.Detail.Narrative = narrative(draft, api.NarrativeStatusTemplate, api.DecisionProviderRules, now)
		} else {
			inc.Detail.Narrative = narrative(draft, api.NarrativeStatusGenerated, narrator.Provider(), now)
			generated++
		}
		inc.Summary = optional(draft.Summary)
		if err := j.r.store.SaveNarrative(ctx, inc); err != nil {
			return err
		}
		j.advance(ctx, stage, i+1, i+1, len(incidents), fmt.Sprintf("Writing report %d of %d", i+1, len(incidents)))
	}
	j.finish(ctx, stage, len(incidents), generated, "Report written")
	return nil
}

// ---------- Helpers ----------

// countingReader counts the bytes read through it, for the progress bar.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// truncate cuts a string to at most n characters, on a character boundary.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
