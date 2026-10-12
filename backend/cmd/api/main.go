// Command api serves the Tenfold API.
//
// Startup order matters: load settings, connect to Postgres, apply the
// schema, mark any jobs a previous process left running as stopped, and
// only then start answering requests.
package main

import (
	"context"
	"errors"
	"log/slog"
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

	st, err := store.Open(startup, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Migrate(startup); err != nil {
		return err
	}

	blobs := blob.NewSupabase(cfg.SupabaseURL, cfg.SupabaseServiceKey, cfg.StorageBucket)
	if err := blobs.EnsureBucket(startup, server.MaxUploadBytes); err != nil {
		return err
	}

	var verifier *auth.Verifier
	if cfg.JWTSecret != "" {
		verifier = auth.NewSecretVerifier(cfg.JWTSecret)
	} else if verifier, err = auth.NewJWKSVerifier(ctx, cfg.JWKSURL); err != nil {
		return err
	}

	catalog := samples.NewCatalog(cfg.SamplesDir)
	if found, _ := catalog.List(); len(found) == 0 {
		log.Warn("no demo samples found; 'Load demo data' will be empty", "dir", cfg.SamplesDir)
	}

	hub := pipeline.NewHub()
	runner := pipeline.NewRunner(st, blobs, catalog, hub, pipeline.NewAI(cfg.AIProvider), log)
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

	failed := make(chan error, 1)
	go func() { failed <- httpServer.ListenAndServe() }()
	log.Info("listening", "port", cfg.Port, "ai", cfg.AIProvider, "riskThreshold", cfg.RiskThreshold)

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
