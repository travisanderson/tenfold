// Command uploadsamples copies the generated demo logs and their answer
// keys into the private samples bucket, where the API reads them.
//
//	make samples                      # from the repo root: writes ../samples
//	go run ./cmd/uploadsamples        # from backend/: uploads them
//
// Run it again whenever the samples are regenerated. It replaces the files
// in place.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tenfold/backend/internal/blob"
	"tenfold/backend/internal/config"
	"tenfold/backend/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "uploadsamples:", err)
		os.Exit(1)
	}
}

func run() error {
	dir := "../samples"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := config.LoadDotEnv(".env"); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	bucket := blob.NewSupabase(cfg.SupabaseURL, cfg.SupabaseServiceKey, cfg.SamplesBucket)
	if err := bucket.EnsureBucket(ctx, server.MaxUploadBytes); err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("%w (run `make samples` from the repo root first)", err)
	}
	uploaded := 0
	for _, entry := range entries {
		name := entry.Name()
		contentType := ""
		switch {
		case strings.HasSuffix(name, ".answers.json"):
			contentType = "application/json"
		case strings.HasSuffix(name, ".log"):
			contentType = "text/plain"
		default:
			continue
		}
		if err := upload(ctx, bucket, filepath.Join(dir, name), name, contentType); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		uploaded++
	}
	if uploaded == 0 {
		return fmt.Errorf("no .log or .answers.json files in %s", dir)
	}
	fmt.Printf("uploaded %d files to the %q bucket\n", uploaded, cfg.SamplesBucket)
	return nil
}

func upload(ctx context.Context, bucket *blob.Supabase, path, name, contentType string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() > server.MaxUploadBytes {
		return fmt.Errorf("%d bytes is over the %d byte cap", info.Size(), server.MaxUploadBytes)
	}
	started := time.Now()
	if err := bucket.Put(ctx, name, contentType, f, info.Size()); err != nil {
		return err
	}
	fmt.Printf("  %-28s %6.1f MB  %s\n", name, float64(info.Size())/(1<<20), time.Since(started).Round(100*time.Millisecond))
	return nil
}
