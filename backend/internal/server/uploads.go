package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"strings"
	"time"

	"github.com/google/uuid"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/auth"
	"tenfold/backend/internal/model"
	"tenfold/backend/internal/pipeline"
	"tenfold/backend/internal/store"
)

// acceptedTypes are the MIME types a log file may have.
var acceptedTypes = map[string]bool{"text/plain": true, "text/csv": true}

// heartbeat is how often an idle progress stream sends a comment line, so
// proxies between the browser and this server keep the connection open.
const heartbeat = 15 * time.Second

// upload returns one of the caller's uploads, or a 404.
func (s *Server) upload(ctx context.Context, id uuid.UUID) (model.Upload, error) {
	return s.store.GetUpload(ctx, auth.UserFrom(ctx).ID, id)
}

func (s *Server) CreateUpload(ctx context.Context, req api.CreateUploadRequestObject) (api.CreateUploadResponseObject, error) {
	user := auth.UserFrom(ctx)
	body := req.Body

	if body.SizeBytes > MaxUploadBytes {
		return nil, tooLarge(fmt.Sprintf("Files may be at most %d bytes.", MaxUploadBytes))
	}
	var bad []api.FieldError
	if name := strings.TrimSpace(body.Filename); name == "" || len(body.Filename) > 255 {
		bad = append(bad, field("filename", "Must be 1 to 255 characters."))
	}
	if body.SizeBytes < 1 {
		bad = append(bad, field("sizeBytes", "Must be at least 1."))
	}
	// "text/plain; charset=utf-8" is still text/plain.
	if mediaType, _, err := mime.ParseMediaType(body.ContentType); err != nil || !acceptedTypes[mediaType] {
		bad = append(bad, field("contentType", "Must be text/plain or text/csv."))
	}
	if len(bad) > 0 {
		return nil, invalid(bad...)
	}

	// The storage path holds only IDs. The filename is kept in the
	// database, so nothing a user types ends up in a URL.
	id := uuid.New()
	key := user.ID.String() + "/" + id.String() + ".log"
	uploadURL, expiresAt, err := s.blobs.SignedUploadURL(ctx, key)
	if err != nil {
		return nil, err
	}
	u := model.Upload{
		ID: id, UserID: user.ID, Filename: body.Filename, Source: string(api.UploadSourceUpload),
		StorageKey: &key, SizeBytes: body.SizeBytes, Status: string(api.UploadStatusQueued),
		Stage: string(api.PipelineStageUploaded), Pipeline: []api.StageProgress{},
		RiskThreshold: s.riskThreshold, CreatedAt: time.Now().UTC(),
	}
	if err := s.store.CreateUpload(ctx, u); err != nil {
		return nil, err
	}
	return api.CreateUpload201JSONResponse{Upload: toUpload(u), UploadUrl: uploadURL, ExpiresAt: expiresAt}, nil
}

func (s *Server) ListUploads(ctx context.Context, req api.ListUploadsRequestObject) (api.ListUploadsResponseObject, error) {
	limit, err := pageLimit(req.Params.Limit, 50, 200)
	if err != nil {
		return nil, err
	}
	var after *store.UploadCursor
	var position store.UploadCursor
	if ok, err := decodeCursor(req.Params.Cursor, &position); err != nil {
		return nil, err
	} else if ok {
		after = &position
	}

	// One extra row tells us whether another page follows.
	uploads, total, err := s.store.ListUploads(ctx, auth.UserFrom(ctx).ID, after, limit+1)
	if err != nil {
		return nil, err
	}
	page := api.UploadPage{Items: make([]api.Upload, 0, len(uploads)), Total: total}
	if len(uploads) > limit {
		uploads = uploads[:limit]
		last := uploads[limit-1]
		page.NextCursor = encodeCursor(store.UploadCursor{CreatedAt: last.CreatedAt, ID: last.ID})
	}
	for _, u := range uploads {
		page.Items = append(page.Items, toUpload(u))
	}
	return api.ListUploads200JSONResponse(page), nil
}

func (s *Server) GetUpload(ctx context.Context, req api.GetUploadRequestObject) (api.GetUploadResponseObject, error) {
	u, err := s.upload(ctx, req.UploadId)
	if err != nil {
		return nil, err
	}
	return api.GetUpload200JSONResponse(toUpload(u)), nil
}

func (s *Server) DeleteUpload(ctx context.Context, req api.DeleteUploadRequestObject) (api.DeleteUploadResponseObject, error) {
	// Check ownership before touching the job: a stranger must not be
	// able to cancel someone else's analysis.
	if _, err := s.upload(ctx, req.UploadId); err != nil {
		return nil, err
	}
	s.runner.Cancel(req.UploadId)
	key, err := s.store.DeleteUpload(ctx, auth.UserFrom(ctx).ID, req.UploadId)
	if err != nil {
		return nil, err
	}
	if key != nil {
		// The rows are gone, so the user's view is correct either way. A
		// file left behind is a cleanup problem, not a failed request.
		if err := s.blobs.Delete(ctx, *key); err != nil {
			s.log.Error("could not delete a stored file", "upload", req.UploadId, "error", store.SafeError(err))
		}
	}
	return api.DeleteUpload204Response{}, nil
}

func (s *Server) AnalyzeUpload(ctx context.Context, req api.AnalyzeUploadRequestObject) (api.AnalyzeUploadResponseObject, error) {
	var opts pipeline.Options
	if req.Body != nil {
		opts.AcceptParseErrors = orZero(req.Body.AcceptParseErrors)
		opts.Restart = orZero(req.Body.Restart)
	}
	u, err := s.runner.Analyze(ctx, auth.UserFrom(ctx).ID, req.UploadId, opts)
	if err != nil {
		return nil, err
	}
	return api.AnalyzeUpload202JSONResponse(toUpload(u)), nil
}

// StreamUploadProgress serves the job's progress as server-sent events.
//
// The stream opens with the upload's current state, then relays each
// change. It ends when the job does: "done" on success, "error" on
// failure, and a last "progress" carrying status "partial" when the job
// stopped early and can resume.
func (s *Server) StreamUploadProgress(ctx context.Context, req api.StreamUploadProgressRequestObject) (api.StreamUploadProgressResponseObject, error) {
	u, err := s.upload(ctx, req.UploadId)
	if err != nil {
		return nil, err
	}
	// Subscribe before reading the current state, so no change can slip
	// between the two.
	changes, unsubscribe := s.hub.Subscribe(u.ID)
	u, err = s.upload(ctx, req.UploadId)
	if err != nil {
		unsubscribe()
		return nil, err
	}

	reader, writer := io.Pipe()
	go func() {
		defer unsubscribe()
		defer writer.Close()

		// send writes one event and reports whether the stream is over.
		send := func(e api.ProgressEvent) (over bool) {
			name, data := "progress", any(e)
			switch e.Status {
			case api.UploadStatusCompleted:
				name = "done"
			case api.UploadStatusFailed:
				name = "error"
				data = api.Problem{
					Type: "about:blank", Title: "Analysis failed", Status: 500,
					Code: "analysis_failed", Detail: &e.Message,
				}
			}
			encoded, _ := json.Marshal(data)
			if _, err := fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", name, encoded); err != nil {
				return true // the client went away
			}
			return e.Status != api.UploadStatusProcessing && e.Status != api.UploadStatusQueued
		}

		if send(pipeline.ProgressOf(u)) {
			return
		}
		tick := time.NewTicker(heartbeat)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case e := <-changes:
				if send(e) {
					return
				}
			case <-tick.C:
				if _, err := io.WriteString(writer, ": keep-alive\n\n"); err != nil {
					return
				}
			}
		}
	}()
	return api.StreamUploadProgress200TexteventStreamResponse{Body: reader}, nil
}

// ---------- Samples ----------

func (s *Server) ListSamples(ctx context.Context, _ api.ListSamplesRequestObject) (api.ListSamplesResponseObject, error) {
	all, err := s.samples.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make(api.ListSamples200JSONResponse, 0, len(all))
	for _, sample := range all {
		out = append(out, api.Sample{
			Name: sample.Name, Title: sample.Title, Description: sample.Description,
			LineCount: sample.LineCount, SizeBytes: sample.SizeBytes, HasAnswerKey: sample.HasAnswerKey,
		})
	}
	return out, nil
}

func (s *Server) LoadSample(ctx context.Context, req api.LoadSampleRequestObject) (api.LoadSampleResponseObject, error) {
	user := auth.UserFrom(ctx)
	sample, err := s.samples.Get(ctx, req.SampleName)
	if err != nil {
		return nil, err
	}
	u := model.Upload{
		ID: uuid.New(), UserID: user.ID, Filename: sample.Name + ".log",
		Source: string(api.UploadSourceSample), SampleName: &sample.Name, SizeBytes: sample.SizeBytes,
		Status: string(api.UploadStatusQueued), Stage: string(api.PipelineStageUploaded),
		Pipeline: []api.StageProgress{}, RiskThreshold: s.riskThreshold, CreatedAt: time.Now().UTC(),
	}
	if err := s.store.CreateUpload(ctx, u); err != nil {
		return nil, err
	}
	started, err := s.runner.Analyze(ctx, user.ID, u.ID, pipeline.Options{})
	if err != nil {
		return nil, err
	}
	return api.LoadSample202JSONResponse(toUpload(started)), nil
}
