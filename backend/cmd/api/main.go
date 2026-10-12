// Command api serves the Tenfold API.
//
// Startup order matters: load settings, connect to Postgres, apply the
// schema, mark any jobs a previous process left running as stopped, and
// only then start answering requests.
package main

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tenfold/backend/internal/auth"
	"tenfold/backend/internal/blob"
	"tenfold/backend/internal/config"
	"tenfold/backend/internal/pipeline"
	"tenfold/backend/internal/samples"
	"tenfold/backend/internal/server"
	"tenfold/backend/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("the server stopped", "error", err.Error())
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	// ctx ends on Ctrl-C or the platform's stop signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := config.LoadDotEnv(".env"); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	st, err := store.Open(startup, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Migrate(startup); err != nil {
		return err
	}

	blobs := blob.NewSupabase(cfg.SupabaseURL, cfg.SupabaseServiceKey, cfg.StorageBucket).
		ReachedFromBrowserAt(cfg.SupabasePublicURL)
	if err := blobs.EnsureBucket(startup, server.MaxUploadBytes); err != nil {
		return err
	}

	var verifier *auth.Verifier
	if cfg.JWTSecret != "" {
		verifier = auth.NewSecretVerifier(cfg.JWTSecret)
	} else if verifier, err = auth.NewJWKSVerifier(ctx, cfg.JWKSURL); err != nil {
		return err
	}

	// Demo logs come from their own private bucket, or from a folder when
	// SAMPLES_DIR is set.
	var sampleFiles samples.Files = blob.NewSupabase(cfg.SupabaseURL, cfg.SupabaseServiceKey, cfg.SamplesBucket)
	if cfg.SamplesDir != "" {
		sampleFiles = samples.Dir(cfg.SamplesDir)
	}
	catalog := samples.NewCatalog(sampleFiles)
	if found, err := catalog.List(startup); err != nil || len(found) == 0 {
		log.Warn("no demo samples found; 'Load demo data' will be empty. Run `go run ./cmd/uploadsamples`.")
	}

	hub := pipeline.NewHub()
	runner := pipeline.NewRunner(st, blobs, catalog, hub, aiFor(cfg), log)
	if err := runner.RecoverStranded(startup); err != nil {
		return err
	}

	api := server.New(server.Options{
		Store: st, Blobs: blobs, Samples: catalog, Runner: runner, Hub: hub,
		Verifier: verifier, RiskThreshold: cfg.RiskThreshold, Log: log,
	})
	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// No WriteTimeout: the progress stream and the event export are
		// long-lived responses by design.
		IdleTimeout: 2 * time.Minute,
	}

	// Take the port before saying so, so a port already in use is an
	// error and not a false "listening" line.
	listener, err := net.Listen("tcp", httpServer.Addr)
	if err != nil {
		return err
	}
	failed := make(chan error, 1)
	go func() { failed <- httpServer.Serve(listener) }()
	log.Info("listening", "port", cfg.Port, "judge", cfg.JudgeProvider, "shadowJudge", cfg.ShadowJudgeProvider,
		"narrator", cfg.NarrativeProvider, "riskThreshold", cfg.RiskThreshold)

	select {
	case err := <-failed:
		return err
	case <-ctx.Done():
	}

	// Stop taking requests, give open ones a moment, then leave. A job
	// still running is marked as stopped at the next startup.
	log.Info("shutting down")
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := httpServer.Shutdown(shutdown); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

// aiFor seats a provider in each AI role, as the settings name them. A role
// set to "none" stays empty: no entry in the map means nobody fills it.
func aiFor(cfg config.Config) pipeline.AI {
	claudeJudge, claudeNarrator := pipeline.NewClaude(cfg.AnthropicAPIKey,
		cmp.Or(cfg.ClaudeJudgeModel, pipeline.DefaultJudgeModel),
		cmp.Or(cfg.ClaudeNarrativeModel, pipeline.DefaultNarrativeModel))
	judges := map[string]pipeline.Judge{
		"jev":    pipeline.NewJevJudge(cfg.JevAPIKey, cmp.Or(cfg.JevModel, pipeline.DefaultJevModel)),
		"claude": claudeJudge,
		"mock":   pipeline.MockJudge{},
	}
	narrators := map[string]pipeline.Narrator{
		"claude": claudeNarrator,
		"mock":   pipeline.MockNarrator{},
	}
	return pipeline.AI{
		Judge:    judges[cfg.JudgeProvider],
		Shadow:   judges[cfg.ShadowJudgeProvider],
		Narrator: narrators[cfg.NarrativeProvider],
	}
}
