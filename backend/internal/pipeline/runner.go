// Package pipeline runs the background job that analyzes one upload:
// parse, detect, judge, group, narrate. Each stage reads its input from the
// database and writes its output back, so a job can stop after any stage
// and a later job can pick up where it left off.
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/blob"
	"tenfold/backend/internal/detect"
	"tenfold/backend/internal/model"
	"tenfold/backend/internal/samples"
	"tenfold/backend/internal/store"
)

var (
	// ErrRunning means a job already holds the upload.
	ErrRunning = errors.New("this upload is already being analyzed")
	// ErrNeedsAcceptance means the upload stopped on malformed lines and
	// the caller has not said to go on without them.
	ErrNeedsAcceptance = errors.New("some lines could not be read; accept them to continue")
)

// stageOrder is the pipeline, in the order the stages run.
var stageOrder = []api.PipelineStage{
	api.PipelineStageParsing,
	api.PipelineStageDetecting,
	api.PipelineStageJudging,
	api.PipelineStageGrouping,
	api.PipelineStageNarrating,
}

// maxRunningJobs caps how many jobs work at once. A job holds its upload's
// events in memory while it detects and groups, and keeps a few database
// connections and model requests busy. A job past the cap waits its turn.
const maxRunningJobs = 2

// progressRange is the slice of the progress bar each stage fills.
var progressRange = map[api.PipelineStage][2]int{
	api.PipelineStageParsing:   {0, 45},
	api.PipelineStageDetecting: {45, 65},
	api.PipelineStageJudging:   {65, 80},
	api.PipelineStageGrouping:  {80, 90},
	api.PipelineStageNarrating: {90, 100},
}

// Runner starts, tracks and cancels analysis jobs. Jobs run as goroutines
// inside this process: simple, and enough for one server. The database
// claim in Analyze, not this struct, is what guarantees one job per upload.
type Runner struct {
	store   *store.Store
	blobs   blob.Store
	samples samples.Catalog
	hub     *Hub
	ai      AI
	log     *slog.Logger

	mu    sync.Mutex
	jobs  map[uuid.UUID]context.CancelFunc
	wg    sync.WaitGroup
	slots chan struct{} // one token per job at work
}

// NewRunner wires a runner to its dependencies.
func NewRunner(st *store.Store, blobs blob.Store, catalog samples.Catalog, hub *Hub, ai AI, log *slog.Logger) *Runner {
	return &Runner{
		store: st, blobs: blobs, samples: catalog, hub: hub, ai: ai, log: log,
		jobs:  map[uuid.UUID]context.CancelFunc{},
		slots: make(chan struct{}, maxRunningJobs),
	}
}

// Options are the choices a caller makes when starting analysis.
type Options struct {
	AcceptParseErrors bool // analyze the good lines although some were malformed
	Restart           bool // run every stage again instead of resuming
}

// Analyze starts or resumes the job for one of the user's uploads and
// returns at once with the upload's new state.
func (r *Runner) Analyze(ctx context.Context, userID, uploadID uuid.UUID, opts Options) (model.Upload, error) {
	u, err := r.store.GetUpload(ctx, userID, uploadID)
	if err != nil {
		return model.Upload{}, err
	}
	if u.Status == string(api.UploadStatusProcessing) {
		return model.Upload{}, ErrRunning
	}
	from, err := startStage(u, opts)
	if err != nil {
		return model.Upload{}, err
	}

	// The claim is one UPDATE that only succeeds if no job holds the
	// upload. Two requests racing here cannot both win.
	claimed, err := r.store.ClaimUpload(ctx, userID, uploadID)
	if err != nil {
		return model.Upload{}, err
	}
	if !claimed {
		return model.Upload{}, ErrRunning
	}

	j := &job{r: r, u: u, accept: opts.AcceptParseErrors}
	j.reset(from)
	if err := r.store.SaveJobState(ctx, j.u); err != nil {
		return model.Upload{}, err
	}

	// Copy the state to return before the job starts changing it. The job's
	// goroutine owns j from here on.
	queued := j.u
	queued.Pipeline = slices.Clone(j.u.Pipeline)

	// The job outlives the request that started it.
	jobCtx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.jobs[uploadID] = cancel
	r.mu.Unlock()
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer func() {
			r.mu.Lock()
			delete(r.jobs, uploadID)
			r.mu.Unlock()
			cancel()
		}()
		if !r.takeSlot(jobCtx, j) {
			return // deleted while it waited
		}
		defer func() { <-r.slots }()
		j.run(jobCtx, from)
	}()
	return queued, nil
}

// takeSlot waits until fewer than maxRunningJobs jobs are at work. It
// reports false if the job was cancelled first.
func (r *Runner) takeSlot(ctx context.Context, j *job) bool {
	select {
	case r.slots <- struct{}{}:
		return true
	default:
	}
	j.u.Message = "Waiting for another analysis to finish"
	j.save(ctx)
	select {
	case r.slots <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

// startStage decides where a new job begins.
func startStage(u model.Upload, opts Options) (api.PipelineStage, error) {
	first := stageOrder[0]
	if opts.Restart || u.Status != string(api.UploadStatusPartial) || u.Stop == nil {
		return first, nil
	}
	if u.Stop.Reason == api.StopReasonParseErrors {
		if !opts.AcceptParseErrors {
			return "", ErrNeedsAcceptance
		}
		// The good lines are already saved; carry on from the next stage.
		return api.PipelineStageDetecting, nil
	}
	for _, s := range stageOrder {
		if s == u.Stop.Stage {
			return s, nil
		}
	}
	return first, nil
}

// Cancel stops the job for an upload, if one is running. The caller is
// deleting the upload, so the job saves nothing more.
func (r *Runner) Cancel(uploadID uuid.UUID) {
	r.mu.Lock()
	cancel := r.jobs[uploadID]
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Wait blocks until every running job has ended.
func (r *Runner) Wait() { r.wg.Wait() }

// RecoverStranded marks uploads left "processing" by a previous process as
// stopped, so the analyst can resume them. Call it once at startup, before
// serving: a job cannot survive a restart.
func (r *Runner) RecoverStranded(ctx context.Context) error {
	stranded, err := r.store.ProcessingUploads(ctx)
	if err != nil {
		return err
	}
	for _, u := range stranded {
		j := &job{r: r, u: u}
		stage := api.PipelineStage(u.Stage)
		if stage == api.PipelineStageUploaded || stage == api.PipelineStageDone {
			stage = stageOrder[0]
		}
		j.stop(ctx, stage, api.StopReasonStageFailed, "The server restarted during analysis. Resume to continue.")
		r.log.Warn("marked a stranded upload as stopped", "upload", u.ID, "stage", stage)
	}
	return nil
}

// ---------- One job ----------

// job is the state of one run. Only the job's goroutine touches it.
type job struct {
	r      *Runner
	u      model.Upload
	accept bool

	// aiDown is the first AI stage that could not run. The job carries on
	// without it and reports the gap at the end.
	aiDown *api.PipelineStage

	stageStarted time.Time
	lastSaved    time.Time
}

// errAIUnavailable is returned by a stage when its model cannot be reached
// or is not configured.
var errAIUnavailable = errors.New("the AI provider is unavailable")

// failure ends a job with nothing usable saved.
type failure struct{ code, message string }

func (f failure) Error() string { return f.message }

// parseStop ends a job after parsing because some lines were malformed.
type parseStop struct{ message string }

func (p parseStop) Error() string { return p.message }

// reset prepares the upload for a run that begins at the given stage:
// earlier stages keep what they recorded, later ones start clean.
func (j *job) reset(from api.PipelineStage) {
	previous := map[api.PipelineStage]api.StageProgress{}
	for _, s := range j.u.Pipeline {
		previous[s.Stage] = s
	}
	pipeline := make([]api.StageProgress, 0, len(stageOrder))
	before := true
	for _, s := range stageOrder {
		if s == from {
			before = false
		}
		entry := api.StageProgress{Stage: s, State: api.StageStatePending}
		if before {
			entry = previous[s]
			entry.Stage, entry.State = s, api.StageStateDone
		}
		pipeline = append(pipeline, entry)
	}

	now := time.Now().UTC()
	j.u.Pipeline = pipeline
	j.u.Status = string(api.UploadStatusProcessing)
	j.u.Stage = string(from)
	j.u.Progress = progressRange[from][0]
	j.u.Message = "Queued"
	j.u.Stop, j.u.Error, j.u.CompletedAt = nil, nil, nil
	j.u.StartedAt = &now
	if from == stageOrder[0] {
		j.u.FirstParseError = nil
		j.u.LineCount, j.u.EventCount, j.u.ParseErrorCount = nil, nil, nil
		j.u.AnomalyCount, j.u.IncidentCount = nil, nil
		j.u.UserCount, j.u.HostCount, j.u.TimeStart, j.u.TimeEnd = nil, nil, nil, nil
	}
}

func (j *job) run(ctx context.Context, from api.PipelineStage) {
	log := j.r.log.With("upload", j.u.ID)
	current := from
	defer func() {
		if p := recover(); p != nil {
			log.Error("the job panicked", "stage", current, "panic", fmt.Sprint(p))
			j.stop(context.Background(), current, api.StopReasonStageFailed, stageFailedMessage(current))
		}
	}()

	stages := map[api.PipelineStage]func(context.Context) error{
		api.PipelineStageParsing:   j.parse,
		api.PipelineStageDetecting: j.detect,
		api.PipelineStageJudging:   j.judge,
		api.PipelineStageGrouping:  j.group,
		api.PipelineStageNarrating: j.narrate,
	}
	started := false
	for _, stage := range stageOrder {
		if stage == from {
			started = true
		}
		if !started {
			continue
		}
		current = stage
		j.begin(ctx, stage)
		err := stages[stage](ctx)

		var fail failure
		var malformed parseStop
		switch {
		case ctx.Err() != nil:
			log.Info("the job was cancelled", "stage", stage)
			return
		case errors.As(err, &fail):
			log.Warn("the job failed", "stage", stage, "code", fail.code)
			j.fail(ctx, stage, fail)
			return
		case errors.As(err, &malformed):
			j.stop(ctx, stage, api.StopReasonParseErrors, malformed.message)
			return
		case errors.Is(err, errAIUnavailable):
			log.Warn("an AI stage could not run; continuing without it", "stage", stage, "error", err)
			if j.aiDown == nil {
				j.aiDown = &stage
			}
			j.stage(stage).State = api.StageStateStopped
		case err != nil:
			// The analyst gets a plain line; the detail goes to the log.
			log.Error("a stage stopped", "stage", stage, "error", store.SafeError(err))
			j.stop(ctx, stage, api.StopReasonStageFailed, stageFailedMessage(stage))
			return
		}
	}

	if j.aiDown != nil {
		j.stop(ctx, *j.aiDown, api.StopReasonAiUnavailable,
			"The AI model could not be reached. Risk scores and evidence are complete.")
		return
	}
	now := time.Now().UTC()
	j.u.Status = string(api.UploadStatusCompleted)
	j.u.Stage = string(api.PipelineStageDone)
	j.u.Progress = 100
	j.u.CompletedAt = &now
	j.u.Message = fmt.Sprintf("Done: %s", pluralize(orZero(j.u.IncidentCount), "incident"))
	j.save(ctx)
	log.Info("the job completed", "events", orZero(j.u.EventCount),
		"anomalies", orZero(j.u.AnomalyCount), "incidents", orZero(j.u.IncidentCount))
}

func stageFailedMessage(stage api.PipelineStage) string {
	return fmt.Sprintf("The %s stage stopped before it finished. Earlier stages are saved. Resume to try again.", stage)
}

// stage returns the pipeline entry for a stage, to update in place.
func (j *job) stage(s api.PipelineStage) *api.StageProgress {
	for i := range j.u.Pipeline {
		if j.u.Pipeline[i].Stage == s {
			return &j.u.Pipeline[i]
		}
	}
	// A pipeline saved by an older version may lack the entry; add it.
	j.u.Pipeline = append(j.u.Pipeline, api.StageProgress{Stage: s})
	return &j.u.Pipeline[len(j.u.Pipeline)-1]
}

// begin marks a stage as running.
func (j *job) begin(ctx context.Context, s api.PipelineStage) {
	j.stageStarted = time.Now()
	j.u.Stage = string(s)
	j.u.Progress = progressRange[s][0]
	*j.stage(s) = api.StageProgress{Stage: s, State: api.StageStateRunning}
	j.save(ctx)
}

// advance records progress inside the running stage: how many items it
// has handled, and how far along it is as done out of total (which may be
// in other units, such as bytes read). Saves are spaced out so a fast loop
// does not flood the database or the stream.
func (j *job) advance(ctx context.Context, s api.PipelineStage, processed, done, total int, message string) {
	st := j.stage(s)
	st.Processed = &processed
	st.ElapsedMs = ptr(int(time.Since(j.stageStarted).Milliseconds()))
	lo, hi := progressRange[s][0], progressRange[s][1]
	if total > 0 {
		j.u.Progress = lo + (hi-lo)*min(done, total)/total
	}
	j.u.Message = message
	if time.Since(j.lastSaved) >= 300*time.Millisecond {
		j.save(ctx)
	}
}

// finish marks a stage as done, with what went in and what came out.
func (j *job) finish(ctx context.Context, s api.PipelineStage, in, out int, message string) {
	*j.stage(s) = api.StageProgress{
		Stage: s, State: api.StageStateDone,
		CountIn: &in, Processed: &in, CountOut: &out,
		ElapsedMs: ptr(int(time.Since(j.stageStarted).Milliseconds())),
	}
	j.u.Progress = progressRange[s][1]
	j.u.Message = message
	j.save(ctx)
}

// stop ends the job early in a state the analyst can resume from.
func (j *job) stop(ctx context.Context, s api.PipelineStage, reason api.StopReason, message string) {
	now := time.Now().UTC()
	st := j.stage(s)
	st.State = api.StageStateStopped
	j.u.Status = string(api.UploadStatusPartial)
	j.u.Stage = string(s)
	j.u.Stop = &api.UploadStop{Stage: s, Reason: reason, Message: message}
	j.u.Message = message
	j.u.CompletedAt = &now
	j.save(ctx)
}

// fail ends the job with nothing usable saved.
func (j *job) fail(ctx context.Context, s api.PipelineStage, f failure) {
	now := time.Now().UTC()
	j.stage(s).State = api.StageStateStopped
	j.u.Status = string(api.UploadStatusFailed)
	j.u.Error = &api.UploadError{Code: f.code, Message: f.message}
	j.u.Message = f.message
	j.u.CompletedAt = &now
	j.save(ctx)
}

// save writes the job's state and tells anyone watching.
func (j *job) save(ctx context.Context) {
	if ctx.Err() != nil {
		return // cancelled: the upload is being deleted
	}
	j.lastSaved = time.Now()
	if err := j.r.store.SaveJobState(ctx, j.u); err != nil {
		j.r.log.Error("could not save job state", "upload", j.u.ID, "error", store.SafeError(err))
	}
	j.r.hub.Publish(ProgressOf(j.u))
}

// ProgressOf builds the progress event for an upload's current state. The
// pipeline is copied: the event is read on another goroutine while the job
// keeps updating its own.
func ProgressOf(u model.Upload) api.ProgressEvent {
	return api.ProgressEvent{
		UploadId: u.ID,
		Status:   api.UploadStatus(u.Status),
		Stage:    api.PipelineStage(u.Stage),
		Progress: u.Progress,
		Message:  u.Message,
		Counts: api.ProgressCounts{
			Lines:     orZero(u.LineCount),
			Events:    orZero(u.EventCount),
			Anomalies: orZero(u.AnomalyCount),
			Incidents: orZero(u.IncidentCount),
		},
		Pipeline: slices.Clone(u.Pipeline),
		At:       time.Now().UTC(),
	}
}

// qualifies reports whether a group scores high enough to be judged and,
// if kept, raised as an incident.
func (j *job) qualifies(g detect.Group) bool {
	return g.RiskScore >= j.u.RiskThreshold
}
