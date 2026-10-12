// Package server implements the HTTP API described in api/openapi.yaml.
// The routing, the request types and the response types are generated from
// that contract; this package fills in what each operation does.
package server

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/auth"
	"tenfold/backend/internal/blob"
	"tenfold/backend/internal/pipeline"
	"tenfold/backend/internal/samples"
	"tenfold/backend/internal/store"
)

// MaxUploadBytes is the size cap on one log file: 50 MB.
const MaxUploadBytes = 52_428_800

// maxBodyBytes caps a request body. Every body in this API is a small JSON
// document; files go straight to storage and never pass through here.
const maxBodyBytes = 1 << 20

// Server holds what the handlers need. It implements
// api.StrictServerInterface, one method per operation in the contract.
type Server struct {
	store         *store.Store
	blobs         blob.Store
	samples       samples.Catalog
	runner        *pipeline.Runner
	hub           *pipeline.Hub
	verifier      *auth.Verifier
	riskThreshold int
	log           *slog.Logger
}

// Options are a Server's dependencies.
type Options struct {
	Store         *store.Store
	Blobs         blob.Store
	Samples       samples.Catalog
	Runner        *pipeline.Runner
	Hub           *pipeline.Hub
	Verifier      *auth.Verifier
	RiskThreshold int
	Log           *slog.Logger
}

// New returns a server.
func New(o Options) *Server {
	return &Server{
		store: o.Store, blobs: o.Blobs, samples: o.Samples, runner: o.Runner, hub: o.Hub,
		verifier: o.Verifier, riskThreshold: o.RiskThreshold, log: o.Log,
	}
}

// Compile-time proof that every operation in the contract is implemented.
var _ api.StrictServerInterface = (*Server)(nil)

// Handler returns the API as one http.Handler: the generated router, with
// logging, panic recovery and authentication wrapped around it.
func (s *Server) Handler() http.Handler {
	strict := api.NewStrictHandlerWithOptions(s, nil, api.StrictHTTPServerOptions{
		// The body was not valid JSON.
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			s.writeError(w, r, badRequest("The request body is not valid JSON for this operation."))
		},
		// A handler returned an error.
		ResponseErrorHandlerFunc: s.writeError,
	})
	router := api.HandlerWithOptions(strict, api.StdHTTPServerOptions{
		// A path or query parameter did not parse.
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			s.writeError(w, r, badRequest(err.Error()))
		},
	})
	return s.logRequests(s.recoverPanics(s.authenticate(router)))
}

// publicPaths need no token. Everything else does, including paths that
// do not exist, so the API is closed by default.
var publicPaths = map[string]bool{
	"/healthz":      true,
	"/openapi.yaml": true,
	"/docs":         true,
}

// authenticate verifies the bearer token and puts the caller in the
// request context. It runs before routing, so an anonymous caller learns
// nothing about which resources exist.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		unauthorized := &problem{status: http.StatusUnauthorized, code: "unauthorized"}
		token, ok := auth.FromHeader(r.Header.Get("Authorization"))
		if !ok {
			w.Header().Set("WWW-Authenticate", "Bearer")
			s.writeError(w, r, unauthorized)
			return
		}
		user, err := s.verifier.Verify(token)
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			s.writeError(w, r, unauthorized)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), user)))
	})
}

// recoverPanics turns a panic in a handler into a 500, so one bad request
// cannot take the server down.
func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler {
					panic(p) // the client went away mid-stream; let net/http handle it
				}
				s.log.Error("handler panicked", "method", r.Method, "path", r.URL.Path, "panic", fmt.Sprint(p))
				s.writeError(w, r, &problem{status: http.StatusInternalServerError, code: "internal_error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// logRequests writes one line per request: method, path, status, time.
// It never logs a body, a query string or a token; queries can hold
// usernames and search text.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		if r.URL.Path == "/healthz" {
			return // the platform probes this every few seconds
		}
		s.log.Info("request", "method", r.Method, "path", r.URL.Path,
			"status", recorder.status, "ms", time.Since(started).Milliseconds())
	})
}

// statusRecorder remembers the status code a handler wrote.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Flush passes streaming flushes through, which SSE depends on.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// ---------- System ----------

func (s *Server) GetHealth(ctx context.Context, _ api.GetHealthRequestObject) (api.GetHealthResponseObject, error) {
	return api.GetHealth200JSONResponse{Status: "ok", Version: api.Version}, nil
}

func (s *Server) GetOpenApiSpec(ctx context.Context, _ api.GetOpenApiSpecRequestObject) (api.GetOpenApiSpecResponseObject, error) {
	spec := api.Spec()
	return api.GetOpenApiSpec200ApplicationyamlResponse{Body: bytes.NewReader(spec), ContentLength: int64(len(spec))}, nil
}

func (s *Server) GetDocs(ctx context.Context, _ api.GetDocsRequestObject) (api.GetDocsResponseObject, error) {
	page := api.DocsPage()
	return api.GetDocs200TexthtmlResponse{Body: bytes.NewReader(page), ContentLength: int64(len(page))}, nil
}
