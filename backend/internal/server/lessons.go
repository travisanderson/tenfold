package server

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/auth"
	"tenfold/backend/internal/detect"
	"tenfold/backend/internal/model"
)

func (s *Server) ListLessons(ctx context.Context, _ api.ListLessonsRequestObject) (api.ListLessonsResponseObject, error) {
	lessons, err := s.store.ListLessons(ctx, auth.UserFrom(ctx).ID, false)
	if err != nil {
		return nil, err
	}
	out := make(api.ListLessons200JSONResponse, 0, len(lessons))
	for _, l := range lessons {
		out = append(out, toLesson(l))
	}
	return out, nil
}

func (s *Server) CreateLesson(ctx context.Context, req api.CreateLessonRequestObject) (api.CreateLessonResponseObject, error) {
	lesson, bad := newLesson(auth.UserFrom(ctx).ID, *req.Body, "")
	if len(bad) > 0 {
		return nil, invalid(bad...)
	}
	if err := s.store.CreateLesson(ctx, lesson); err != nil {
		return nil, err
	}
	return api.CreateLesson201JSONResponse(toLesson(lesson)), nil
}

func (s *Server) DeleteLesson(ctx context.Context, req api.DeleteLessonRequestObject) (api.DeleteLessonResponseObject, error) {
	if err := s.store.DeleteLesson(ctx, auth.UserFrom(ctx).ID, req.LessonId); err != nil {
		return nil, err
	}
	return api.DeleteLesson204Response{}, nil
}

// PreviewLesson counts what a scope would set aside in one upload. It uses
// the same matching rule as the pipeline and saves nothing.
func (s *Server) PreviewLesson(ctx context.Context, req api.PreviewLessonRequestObject) (api.PreviewLessonResponseObject, error) {
	scope := req.Body.Scope
	var bad []api.FieldError
	switch scope.Type {
	case api.LessonScopeTypeUser, api.LessonScopeTypeDomain, api.LessonScopeTypePattern:
	default:
		bad = append(bad, field("scope.type", "Must be user, domain or pattern."))
	}
	if strings.TrimSpace(scope.Value) == "" || len(scope.Value) > 255 {
		bad = append(bad, field("scope.value", "Must be 1 to 255 characters."))
	}
	if len(bad) > 0 {
		return nil, invalid(bad...)
	}

	u, err := s.upload(ctx, req.Body.UploadId)
	if err != nil {
		return nil, err
	}
	anomalies, err := s.store.LoadAnomalies(ctx, u.ID)
	if err != nil {
		return nil, err
	}

	var out api.LessonPreviewResponse
	// For each incident: does any of its anomalies survive the lesson?
	survives := map[uuid.UUID]bool{}
	for _, a := range anomalies {
		if a.Disposition == model.DispositionLesson || a.Disposition == model.DispositionPending {
			continue // already set aside, or not decided yet
		}
		matched := detect.MatchesLesson(a, string(scope.Type), scope.Value)
		if matched {
			out.WouldSuppressAnomalies++
		}
		if a.IncidentID != nil {
			survives[*a.IncidentID] = survives[*a.IncidentID] || !matched
		}
	}
	for _, alive := range survives {
		if !alive {
			out.WouldResolveIncidents++
		}
	}
	return api.PreviewLesson200JSONResponse(out), nil
}
