package server

import (
	"context"
	"encoding/json"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/auth"
	"tenfold/backend/internal/model"
)

// maxEvidence is the contract's cap on the events returned with one
// incident. The rest are one listEvents call away.
const maxEvidence = 500

// incident returns one of the caller's incidents, or a 404.
func (s *Server) incident(ctx context.Context, id uuid.UUID) (model.Incident, error) {
	return s.store.GetIncident(ctx, auth.UserFrom(ctx).ID, id)
}

func (s *Server) ListIncidents(ctx context.Context, req api.ListIncidentsRequestObject) (api.ListIncidentsResponseObject, error) {
	u, err := s.upload(ctx, req.UploadId)
	if err != nil {
		return nil, err
	}
	incidents, err := s.store.ListIncidents(ctx, u.UserID, u.ID)
	if err != nil {
		return nil, err
	}
	out := make(api.ListIncidents200JSONResponse, 0, len(incidents))
	for _, i := range incidents {
		out = append(out, toIncidentSummary(i))
	}
	return out, nil
}

// GetIncident reads stored results only. Nothing here calls a model: the
// narrative was written, checked and saved by the background job.
func (s *Server) GetIncident(ctx context.Context, req api.GetIncidentRequestObject) (api.GetIncidentResponseObject, error) {
	i, err := s.incident(ctx, req.IncidentId)
	if err != nil {
		return nil, err
	}
	events, err := s.store.EventsByLineNos(ctx, i.UploadID, i.LineNos, maxEvidence)
	if err != nil {
		return nil, err
	}
	evidence, err := s.withFlags(ctx, i.UploadID, events)
	if err != nil {
		return nil, err
	}
	notes, err := s.store.ListNotes(ctx, i.ID)
	if err != nil {
		return nil, err
	}

	sum := toIncidentSummary(i)
	d := i.Detail
	out := api.IncidentDetail{
		Id: sum.Id, UploadId: sum.UploadId, Title: sum.Title, Severity: sum.Severity,
		RiskScore: sum.RiskScore, AiVerdict: sum.AiVerdict, Summary: sum.Summary,
		MarkerLabel: sum.MarkerLabel, MarkerDetail: sum.MarkerDetail, Assignee: sum.Assignee,
		Principal: sum.Principal, TimeRange: sum.TimeRange, EventCount: sum.EventCount,
		AnomalyCount: sum.AnomalyCount, Mitre: sum.Mitre, AnalystVerdict: sum.AnalystVerdict,

		Narrative:         d.Narrative,
		Signals:           orEmpty(d.Signals),
		Timeline:          orEmpty(d.Timeline),
		Graph:             api.IncidentGraph{Nodes: orEmpty(d.Graph.Nodes), Edges: orEmpty(d.Graph.Edges)},
		Indicators:        orEmpty(d.Indicators),
		Chart:             d.Chart,
		LessonSuggestions: orEmpty(d.LessonSuggestions),
		EvidenceEventIds:  model.EventIDs(i.LineNos),
		Evidence:          evidence,
		Notes:             make([]api.IncidentNote, 0, len(notes)),
	}
	out.Narrative.Sentences = orEmpty(out.Narrative.Sentences)
	out.Narrative.NextSteps = orEmpty(out.Narrative.NextSteps)
	for _, n := range notes {
		out.Notes = append(out.Notes, toNote(n))
	}
	return api.GetIncident200JSONResponse(out), nil
}

func (s *Server) UpdateIncident(ctx context.Context, req api.UpdateIncidentRequestObject) (api.UpdateIncidentResponseObject, error) {
	user := auth.UserFrom(ctx)
	var id *uuid.UUID
	var name *string
	if req.Body.AssignedToMe {
		id, name = &user.ID, ptr(user.Name())
	}
	if err := s.store.SetAssignee(ctx, user.ID, req.IncidentId, id, name); err != nil {
		return nil, err
	}
	i, err := s.incident(ctx, req.IncidentId)
	if err != nil {
		return nil, err
	}
	return api.UpdateIncident200JSONResponse(toIncidentSummary(i)), nil
}

func (s *Server) AddIncidentNote(ctx context.Context, req api.AddIncidentNoteRequestObject) (api.AddIncidentNoteResponseObject, error) {
	user := auth.UserFrom(ctx)
	text := strings.TrimSpace(req.Body.Text)
	if text == "" || len(req.Body.Text) > 2000 {
		return nil, invalid(field("text", "Must be 1 to 2000 characters."))
	}
	note := model.Note{ID: uuid.New(), Author: user.Name(), Text: text, CreatedAt: time.Now().UTC()}
	if err := s.store.AddNote(ctx, user.ID, req.IncidentId, note); err != nil {
		return nil, err
	}
	return api.AddIncidentNote201JSONResponse(toNote(note)), nil
}

func (s *Server) SetIncidentVerdict(ctx context.Context, req api.SetIncidentVerdictRequestObject) (api.SetIncidentVerdictResponseObject, error) {
	user := auth.UserFrom(ctx)
	body := req.Body

	var bad []api.FieldError
	switch body.Status {
	case api.VerdictStatusConfirmed, api.VerdictStatusDismissed, api.VerdictStatusEscalated:
	default:
		bad = append(bad, field("status", "Must be confirmed, dismissed or escalated."))
	}
	var note *model.Note
	if body.Note != nil && strings.TrimSpace(*body.Note) != "" {
		if len(*body.Note) > 2000 {
			bad = append(bad, field("note", "Must be at most 2000 characters."))
		}
		note = &model.Note{ID: uuid.New(), Author: user.Name(), Text: strings.TrimSpace(*body.Note), CreatedAt: time.Now().UTC()}
	}
	var lesson *model.Lesson
	if body.Lesson != nil {
		built, lessonErrors := newLesson(user.ID, *body.Lesson, "lesson.")
		bad = append(bad, lessonErrors...)
		lesson = &built
	}
	if len(bad) > 0 {
		return nil, invalid(bad...)
	}

	// The verdict, its note and its lesson are saved together or not at all.
	if err := s.store.SetVerdict(ctx, user.ID, req.IncidentId, string(body.Status), note, lesson); err != nil {
		return nil, err
	}
	i, err := s.incident(ctx, req.IncidentId)
	if err != nil {
		return nil, err
	}
	return api.SetIncidentVerdict200JSONResponse(toIncidentSummary(i)), nil
}

// newLesson checks a lesson and builds the row. prefix names the field in
// error messages when the lesson is nested in another body.
func newLesson(userID uuid.UUID, in api.LessonCreate, prefix string) (model.Lesson, []api.FieldError) {
	var bad []api.FieldError
	text, value := strings.TrimSpace(in.Text), strings.TrimSpace(in.Scope.Value)
	if text == "" || len(in.Text) > 1000 {
		bad = append(bad, field(prefix+"text", "Must be 1 to 1000 characters."))
	}
	if value == "" || len(in.Scope.Value) > 255 {
		bad = append(bad, field(prefix+"scope.value", "Must be 1 to 255 characters."))
	}
	switch in.Scope.Type {
	case api.LessonScopeTypeUser, api.LessonScopeTypeDomain:
	case api.LessonScopeTypePattern:
		if _, err := path.Match(value, ""); err != nil {
			bad = append(bad, field(prefix+"scope.value", "Is not a valid pattern. Use * for any run of characters, as in *.example.com."))
		}
		// A pattern of only wildcards would hide everything, forever.
		if strings.Trim(value, "*?.") == "" && value != "" {
			bad = append(bad, field(prefix+"scope.value", "Must name at least part of a host."))
		}
	default:
		bad = append(bad, field(prefix+"scope.type", "Must be user, domain or pattern."))
	}
	if !in.ExpiresAt.After(time.Now()) {
		bad = append(bad, field(prefix+"expiresAt", "Must be in the future."))
	}
	return model.Lesson{
		ID: uuid.New(), UserID: userID, Text: text, ScopeType: string(in.Scope.Type),
		ScopeValue: strings.ToLower(value), CreatedAt: time.Now().UTC(), ExpiresAt: in.ExpiresAt.UTC(),
	}, bad
}

// GetIncidentContext returns the incident in a compact form for AI agents:
// the same facts as the incident screen, trimmed to cut tokens.
func (s *Server) GetIncidentContext(ctx context.Context, req api.GetIncidentContextRequestObject) (api.GetIncidentContextResponseObject, error) {
	i, err := s.incident(ctx, req.IncidentId)
	if err != nil {
		return nil, err
	}
	events, err := s.store.EventsByLineNos(ctx, i.UploadID, i.LineNos, maxEvidence)
	if err != nil {
		return nil, err
	}
	d := i.Detail

	out := api.IncidentContext{
		SchemaVersion:      api.ContextSchemaVersionN1,
		IncidentId:         i.ID,
		RiskScore:          i.RiskScore,
		Severity:           api.Severity(i.Severity),
		AnalystStatus:      api.AnalystStatus(i.AnalystStatus),
		Entities:           api.ContextEntities{Users: []string{}, Hosts: []string{}, Domains: []string{}, Ips: []string{}},
		Timeline:           make([]api.ContextTimelineItem, 0, len(d.Timeline)),
		Evidence:           make([]api.ContextEvidence, 0, len(events)),
		Mitre:              orEmpty(i.Mitre),
		RecommendedActions: orEmpty(d.Narrative.NextSteps),
		Provenance: api.ContextProvenance{
			Detectors:         orEmpty(d.Detectors),
			DecisionProvider:  api.DecisionProviderRules, // no model, unless one ran
			NarrativeProvider: d.Narrative.Provider,
			GeneratedAt:       i.CreatedAt.UTC(),
		},
	}
	if i.AIVerdict != nil {
		out.AiVerdict = ptr(api.AiVerdict(*i.AIVerdict))
	}
	if d.DecisionProvider != nil {
		out.Provenance.DecisionProvider = *d.DecisionProvider
	}
	if d.Narrative.GeneratedAt != nil {
		out.Provenance.GeneratedAt = d.Narrative.GeneratedAt.UTC()
	}

	// The summary prefers the checked narrative. Without one, it falls
	// back to the measured facts, which need no model.
	switch {
	case i.Summary != nil:
		out.Summary = *i.Summary
	case len(d.Signals) > 0:
		out.Summary = i.Title + ". " + d.Signals[0].Description + "."
	default:
		out.Summary = i.Title + "."
	}

	for _, ind := range d.Indicators {
		switch ind.Type {
		case api.GraphNodeTypeUser:
			out.Entities.Users = append(out.Entities.Users, ind.Value)
		case api.GraphNodeTypeHost:
			out.Entities.Hosts = append(out.Entities.Hosts, ind.Value)
		case api.GraphNodeTypeDomain:
			out.Entities.Domains = append(out.Entities.Domains, ind.Value)
		case api.GraphNodeTypeIp:
			out.Entities.Ips = append(out.Entities.Ips, ind.Value)
		}
	}
	for _, t := range d.Timeline {
		out.Timeline = append(out.Timeline, api.ContextTimelineItem{Ts: t.Ts, EventId: t.EventId, Label: t.Label})
	}
	for _, e := range events {
		out.Evidence = append(out.Evidence, api.ContextEvidence{
			Id: model.EventID(e.LineNo), Ts: e.TS, User: e.User, Host: e.Host,
			Action: api.EventAction(e.Action), BytesOut: e.BytesOut, ThreatName: optional(e.ThreatName),
		})
	}

	// About four characters to a token is the usual rough guide.
	if encoded, err := json.Marshal(out); err == nil {
		out.TokenEstimate = len(encoded) / 4
	}
	return api.GetIncidentContext200JSONResponse(out), nil
}

// orEmpty returns an empty slice for nil, so JSON says [] and never null.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
