package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/pipeline"
	"tenfold/backend/internal/samples"
	"tenfold/backend/internal/store"
)

// problem is an error that knows its HTTP answer. Handlers return one and
// a single function, writeError, turns it into RFC 9457 Problem Details.
// That keeps every error response in the API the same shape.
type problem struct {
	status int
	code   string
	detail string
	fields []api.FieldError
}

func (p *problem) Error() string { return fmt.Sprintf("%d %s: %s", p.status, p.code, p.detail) }

func badRequest(detail string) error {
	return &problem{status: http.StatusBadRequest, code: "bad_request", detail: detail}
}

func notFound() error {
	return &problem{status: http.StatusNotFound, code: "not_found"}
}

func conflict(code, detail string) error {
	return &problem{status: http.StatusConflict, code: code, detail: detail}
}

func tooLarge(detail string) error {
	return &problem{status: http.StatusRequestEntityTooLarge, code: "upload_too_large", detail: detail}
}

// invalid is a 422: the JSON was well formed but a value is not allowed.
func invalid(fields ...api.FieldError) error {
	return &problem{status: http.StatusUnprocessableEntity, code: "validation_failed", fields: fields}
}

func field(name, message string) api.FieldError {
	return api.FieldError{Field: name, Message: message}
}

// asProblem maps any error to the problem the caller should see. Errors it
// does not recognize become a 500 with no detail: internals stay internal.
func asProblem(err error) *problem {
	var p *problem
	var connect *pgconn.ConnectError
	var network net.Error
	switch {
	case errors.As(err, &p):
		return p
	case errors.Is(err, store.ErrNotFound), errors.Is(err, samples.ErrNotFound):
		// "Not yours" and "does not exist" are the same answer on purpose.
		return notFound().(*problem)
	case errors.Is(err, pipeline.ErrRunning):
		return conflict("analysis_in_progress", "This upload is already being analyzed.").(*problem)
	case errors.Is(err, pipeline.ErrNeedsAcceptance):
		return invalid(field("acceptParseErrors",
			"Some lines could not be read. Set this to true to analyze the good lines, or delete the upload.")).(*problem)
	case errors.As(err, &connect), errors.As(err, &network), errors.Is(err, context.DeadlineExceeded):
		return &problem{status: http.StatusServiceUnavailable, code: "dependency_unavailable"}
	default:
		return &problem{status: http.StatusInternalServerError, code: "internal_error"}
	}
}

// body builds the Problem Details document for a request.
func (p *problem) body(r *http.Request) api.Problem {
	doc := api.Problem{
		Type:     "about:blank",
		Title:    http.StatusText(p.status),
		Status:   p.status,
		Code:     p.code,
		Instance: &r.URL.Path,
	}
	if p.detail != "" {
		doc.Detail = &p.detail
	}
	if len(p.fields) > 0 {
		doc.Errors = &p.fields
	}
	return doc
}

// writeError answers a request with Problem Details. Server faults are
// logged; client mistakes are not worth the noise.
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	p := asProblem(err)
	if p.status >= 500 {
		s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "status", p.status, "error", store.SafeError(err))
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.status)
	_ = json.NewEncoder(w).Encode(p.body(r))
}
