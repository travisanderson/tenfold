package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/model"
	"tenfold/backend/internal/store"
)

// maxTimeseriesPoints stops a narrow bucket over a long file from
// returning more points than any chart can draw.
const maxTimeseriesPoints = 5000

func (s *Server) ListEvents(ctx context.Context, req api.ListEventsRequestObject) (api.ListEventsResponseObject, error) {
	u, err := s.upload(ctx, req.UploadId)
	if err != nil {
		return nil, err
	}
	p := req.Params
	limit, err := pageLimit(p.Limit, 200, 1000)
	if err != nil {
		return nil, err
	}
	if p.Q != nil && len(*p.Q) > 200 {
		return nil, badRequest("Query parameter 'q' may be at most 200 characters.")
	}
	if p.Action != nil && *p.Action != api.EventActionAllowed && *p.Action != api.EventActionBlocked {
		return nil, badRequest("Query parameter 'action' must be allowed or blocked.")
	}
	if p.Sort != nil && *p.Sort != api.EventSortTs && *p.Sort != api.EventSortMinusTs {
		return nil, badRequest("Query parameter 'sort' must be ts or -ts.")
	}

	filter := store.EventFilter{
		User:       orZero(p.User),
		SrcIP:      orZero(p.SrcIp),
		Host:       orZero(p.Host),
		Category:   orZero(p.Category),
		Action:     string(orZero(p.Action)),
		HasAnomaly: p.HasAnomaly,
		From:       p.From,
		To:         p.To,
		Search:     parseSearch(orZero(p.Q)),
		Descending: p.Sort != nil && *p.Sort == api.EventSortMinusTs,
		Limit:      limit + 1, // one extra row tells us whether another page follows
	}
	var position store.EventCursor
	if ok, err := decodeCursor(p.Cursor, &position); err != nil {
		return nil, err
	} else if ok {
		filter.After = &position
	}

	events, total, err := s.store.ListEvents(ctx, u.ID, filter)
	if err != nil {
		return nil, err
	}
	page := api.EventPage{Total: total}
	if len(events) > limit {
		events = events[:limit]
		last := events[limit-1]
		page.NextCursor = encodeCursor(store.EventCursor{TS: last.TS, LineNo: last.LineNo})
	}
	page.Items, err = s.withFlags(ctx, u.ID, events)
	if err != nil {
		return nil, err
	}
	return api.ListEvents200JSONResponse(page), nil
}

// withFlags attaches to each event the anomalies it is evidence for.
func (s *Server) withFlags(ctx context.Context, uploadID uuid.UUID, events []model.Event) ([]api.Event, error) {
	lineNos := make([]int, len(events))
	for i, e := range events {
		lineNos[i] = e.LineNo
	}
	flags, err := s.store.EventFlags(ctx, uploadID, lineNos)
	if err != nil {
		return nil, err
	}
	out := make([]api.Event, len(events))
	for i, e := range events {
		out[i] = toEvent(e, flags[e.LineNo])
	}
	return out, nil
}

// parseSearch reads the q parameter: terms joined by " OR ", each either
// fielded (user:, domain:, ip:, id:) or bare. That is the whole grammar.
// Anything else, such as an unknown field, is searched for as plain text.
func parseSearch(q string) []store.SearchTerm {
	var terms []store.SearchTerm
	for _, part := range strings.Split(q, " OR ") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		term := store.SearchTerm{Value: part}
		for _, name := range []string{"user", "domain", "ip", "id"} {
			if value, ok := strings.CutPrefix(part, name+":"); ok && strings.TrimSpace(value) != "" {
				term = store.SearchTerm{Field: name, Value: strings.TrimSpace(value)}
				break
			}
		}
		terms = append(terms, term)
	}
	return terms
}

// ExportEvents streams every event as one JSON object per line. Rows go
// from the database to the response one at a time, so memory use does not
// grow with the size of the file.
func (s *Server) ExportEvents(ctx context.Context, req api.ExportEventsRequestObject) (api.ExportEventsResponseObject, error) {
	u, err := s.upload(ctx, req.UploadId)
	if err != nil {
		return nil, err
	}
	flags, err := s.store.EventFlags(ctx, u.ID, nil) // nil: every flagged line
	if err != nil {
		return nil, err
	}

	reader, writer := io.Pipe()
	go func() {
		buffered := bufio.NewWriterSize(writer, 64<<10)
		encoder := json.NewEncoder(buffered) // writes a newline after each value
		err := s.store.StreamEvents(ctx, u.ID, func(e model.Event) error {
			return encoder.Encode(toEvent(e, flags[e.LineNo]))
		})
		if err == nil {
			err = buffered.Flush()
		}
		if err != nil && ctx.Err() == nil {
			s.log.Error("event export stopped early", "upload", u.ID, "error", store.SafeError(err))
		}
		// Closing with the error makes the response end abruptly, so the
		// client can tell a cut-off export from a complete one.
		writer.CloseWithError(err)
	}()
	return api.ExportEvents200ApplicationxNdjsonResponse{Body: reader}, nil
}

var bucketWidths = map[api.TimeseriesBucket]time.Duration{
	api.TimeseriesBucketN1m:  time.Minute,
	api.TimeseriesBucketN5m:  5 * time.Minute,
	api.TimeseriesBucketN10m: 10 * time.Minute,
	api.TimeseriesBucketN15m: 15 * time.Minute,
	api.TimeseriesBucketN1h:  time.Hour,
}

func (s *Server) GetTimeseries(ctx context.Context, req api.GetTimeseriesRequestObject) (api.GetTimeseriesResponseObject, error) {
	u, err := s.upload(ctx, req.UploadId)
	if err != nil {
		return nil, err
	}
	bucket := api.TimeseriesBucketN5m
	if req.Params.Bucket != nil {
		bucket = *req.Params.Bucket
	}
	width, ok := bucketWidths[bucket]
	if !ok {
		return nil, badRequest("Query parameter 'bucket' must be one of 1m, 5m, 10m, 15m, 1h.")
	}
	out := api.TimeseriesResponse{BucketSeconds: int(width.Seconds()), Points: []api.TimeseriesPoint{}}
	if u.TimeStart == nil || u.TimeEnd == nil {
		return api.GetTimeseries200JSONResponse(out), nil // not parsed yet
	}

	// Buckets align to the clock (10:00, 10:05, ...), not to the first event.
	first, last := u.TimeStart.UTC().Truncate(width), u.TimeEnd.UTC().Truncate(width)
	count := int(last.Sub(first)/width) + 1
	if count > maxTimeseriesPoints {
		return nil, badRequest("This bucket is too narrow for a file that spans this much time. Choose a wider one.")
	}

	buckets, err := s.store.EventBuckets(ctx, u.ID, width, time.Unix(0, 0).UTC())
	if err != nil {
		return nil, err
	}
	spans, err := s.store.AnomalySpans(ctx, u.ID)
	if err != nil {
		return nil, err
	}

	// Every bucket in the file's range gets a point, so the chart has no gaps.
	out.Points = make([]api.TimeseriesPoint, count)
	for i := range out.Points {
		out.Points[i].Ts = first.Add(time.Duration(i) * width)
	}
	index := func(t time.Time) int {
		return min(max(int(t.UTC().Truncate(width).Sub(first)/width), 0), count-1)
	}
	for _, b := range buckets {
		p := &out.Points[index(b.Start)]
		p.Total, p.Blocked, p.Anomalous = b.Total, b.Blocked, b.Anomalous
	}
	// An anomaly lifts the risk line across every bucket it was active in.
	for _, span := range spans {
		for i := index(span.FirstSeen); i <= index(span.LastSeen); i++ {
			out.Points[i].PeakRiskScore = max(out.Points[i].PeakRiskScore, span.RiskScore)
		}
	}
	return api.GetTimeseries200JSONResponse(out), nil
}

func (s *Server) ListAnomalies(ctx context.Context, req api.ListAnomaliesRequestObject) (api.ListAnomaliesResponseObject, error) {
	u, err := s.upload(ctx, req.UploadId)
	if err != nil {
		return nil, err
	}
	p := req.Params
	limit, err := pageLimit(p.Limit, 50, 200)
	if err != nil {
		return nil, err
	}
	filter := store.AnomalyFilter{
		Detector:       string(orZero(p.Detector)),
		Disposition:    string(orZero(p.Disposition)),
		JudgesDisagree: p.JudgesDisagree,
		Limit:          limit + 1,
	}
	var position store.AnomalyCursor
	if ok, err := decodeCursor(p.Cursor, &position); err != nil {
		return nil, err
	} else if ok {
		filter.After = &position
	}

	anomalies, total, err := s.store.ListAnomalies(ctx, u.ID, filter)
	if err != nil {
		return nil, err
	}
	page := api.AnomalyPage{Items: make([]api.Anomaly, 0, len(anomalies)), Total: total}
	if len(anomalies) > limit {
		anomalies = anomalies[:limit]
		last := anomalies[limit-1]
		page.NextCursor = encodeCursor(store.AnomalyCursor{RiskScore: last.RiskScore, ID: last.ID})
	}
	for _, a := range anomalies {
		page.Items = append(page.Items, toAnomaly(a))
	}
	return api.ListAnomalies200JSONResponse(page), nil
}
