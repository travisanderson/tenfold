package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/model"
	"tenfold/backend/internal/store"
)

// This file maps stored rows to the shapes in the contract. The mapping is
// one-way and has no logic beyond leaving out what is empty.

func toUpload(u model.Upload) api.Upload {
	out := api.Upload{
		Id:              u.ID,
		Filename:        u.Filename,
		Source:          api.UploadSource(u.Source),
		SampleName:      u.SampleName,
		Status:          api.UploadStatus(u.Status),
		Stage:           api.PipelineStage(u.Stage),
		Progress:        u.Progress,
		SizeBytes:       u.SizeBytes,
		LineCount:       u.LineCount,
		EventCount:      u.EventCount,
		ParseErrorCount: u.ParseErrorCount,
		AnomalyCount:    u.AnomalyCount,
		IncidentCount:   u.IncidentCount,
		Error:           u.Error,
		Pipeline:        u.Pipeline,
		Stop:            u.Stop,
		FirstParseError: u.FirstParseError,
		RiskThreshold:   u.RiskThreshold,
		UserCount:       u.UserCount,
		HostCount:       u.HostCount,
		VerdictCounts:   u.VerdictCounts,
		CreatedAt:       u.CreatedAt.UTC(),
	}
	if out.Pipeline == nil {
		out.Pipeline = []api.StageProgress{}
	}
	if u.TimeStart != nil && u.TimeEnd != nil {
		out.TimeRange = &api.TimeRange{Start: u.TimeStart.UTC(), End: u.TimeEnd.UTC()}
	}
	if u.StartedAt != nil {
		out.StartedAt = ptr(u.StartedAt.UTC())
	}
	if u.CompletedAt != nil {
		out.CompletedAt = ptr(u.CompletedAt.UTC())
	}
	return out
}

func toEvent(e model.Event, flags []store.EventFlag) api.Event {
	out := api.Event{
		Id:     model.EventID(e.LineNo),
		LineNo: e.LineNo,
		Ts:     e.TS,
		Principal: api.EventPrincipal{
			User: e.User, Dept: optional(e.Dept), Device: optional(e.Device), SrcIp: e.SrcIP,
		},
		Target: api.EventTarget{
			Host: e.Host, Url: e.URL, Category: e.Category, Ip: optional(e.DstIP),
		},
		Network: api.EventNetwork{
			Method: e.Method, Status: e.Status, BytesOut: e.BytesOut, BytesIn: e.BytesIn,
			UserAgent: optional(e.UserAgent),
		},
		SecurityResult: api.EventSecurityResult{
			Action: api.EventAction(e.Action), Reason: optional(e.Reason),
			ThreatName: optional(e.ThreatName), RiskScore: &e.RiskScore,
		},
		Flags: make([]api.EventFlag, 0, len(flags)),
		Raw:   e.Raw,
	}
	for _, f := range flags {
		out.Flags = append(out.Flags, api.EventFlag{
			AnomalyId: f.AnomalyID, Label: f.Label, Disposition: api.AnomalyDisposition(f.Disposition),
		})
		if f.IncidentID != nil && out.IncidentId == nil {
			out.IncidentId = f.IncidentID
		}
	}
	return out
}

func toAnomaly(a model.Anomaly) api.Anomaly {
	return api.Anomaly{
		Id:                   a.ID,
		UploadId:             a.UploadID,
		Detector:             api.DetectorType(a.Detector),
		Reason:               a.Reason,
		Signals:              a.Signals,
		RiskScore:            a.RiskScore,
		Decision:             a.Decision,
		ShadowDecision:       a.ShadowDecision,
		Disposition:          api.AnomalyDisposition(a.Disposition),
		IncidentId:           a.IncidentID,
		EventIds:             model.EventIDs(a.LineNos),
		User:                 optional(a.User),
		Host:                 optional(a.Host),
		FirstSeen:            a.FirstSeen,
		LastSeen:             a.LastSeen,
		SuppressedByLessonId: a.LessonID,
	}
}

func toIncidentSummary(i model.Incident) api.IncidentSummary {
	out := api.IncidentSummary{
		Id:           i.ID,
		UploadId:     i.UploadID,
		Title:        i.Title,
		Severity:     api.Severity(i.Severity),
		RiskScore:    i.RiskScore,
		Summary:      i.Summary,
		MarkerLabel:  i.MarkerLabel,
		MarkerDetail: i.MarkerDetail,
		Principal:    api.IncidentPrincipal{User: i.PrincipalUser, Host: i.PrincipalHost},
		TimeRange:    api.TimeRange{Start: i.TimeStart, End: i.TimeEnd},
		EventCount:   i.EventCount,
		AnomalyCount: i.AnomalyCount,
		Mitre:        i.Mitre,
		AnalystVerdict: api.AnalystVerdict{
			Status: api.AnalystStatus(i.AnalystStatus), Note: i.AnalystNote,
		},
	}
	if out.Mitre == nil {
		out.Mitre = []api.MitreTechnique{}
	}
	if i.AIVerdict != nil {
		out.AiVerdict = ptr(api.AiVerdict(*i.AIVerdict))
	}
	if i.DecidedAt != nil {
		out.AnalystVerdict.DecidedAt = ptr(i.DecidedAt.UTC())
	}
	if i.AssigneeID != nil && i.AssigneeName != nil {
		out.Assignee = &api.Assignee{Id: *i.AssigneeID, Name: *i.AssigneeName}
	}
	return out
}

func toLesson(l model.Lesson) api.Lesson {
	return api.Lesson{
		Id:         l.ID,
		Text:       l.Text,
		Scope:      api.LessonScope{Type: api.LessonScopeType(l.ScopeType), Value: l.ScopeValue},
		CreatedAt:  l.CreatedAt,
		ExpiresAt:  l.ExpiresAt,
		MatchCount: l.MatchCount,
	}
}

func toNote(n model.Note) api.IncidentNote {
	return api.IncidentNote{Id: n.ID, Author: n.Author, Text: n.Text, CreatedAt: n.CreatedAt}
}

// ---------- Cursors and limits ----------

// encodeCursor packs a page position into an opaque string. Clients pass
// it back unchanged; its contents are not part of the contract.
func encodeCursor(position any) *string {
	raw, err := json.Marshal(position)
	if err != nil {
		panic(fmt.Sprintf("server: cannot encode cursor %T: %v", position, err))
	}
	return ptr(base64.RawURLEncoding.EncodeToString(raw))
}

// decodeCursor unpacks a cursor into position. A nil cursor leaves
// position alone and reports false.
func decodeCursor(cursor *string, position any) (bool, error) {
	if cursor == nil || *cursor == "" {
		return false, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(*cursor)
	if err == nil {
		err = json.Unmarshal(raw, position)
	}
	if err != nil {
		return false, badRequest("Query parameter 'cursor' is not a cursor this API issued.")
	}
	return true, nil
}

// pageLimit applies a default and checks the bounds the contract sets.
func pageLimit(limit *int, fallback, maximum int) (int, error) {
	if limit == nil {
		return fallback, nil
	}
	if *limit < 1 || *limit > maximum {
		return 0, badRequest(fmt.Sprintf("Query parameter 'limit' must be between 1 and %d.", maximum))
	}
	return *limit, nil
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ptr[T any](v T) *T { return &v }

func orZero[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
