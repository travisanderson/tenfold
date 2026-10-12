package blob

import (
	"bufio"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"tenfold/backend/internal/config"
	"tenfold/backend/internal/samples"
)

// TestSamplesBucketServesTheDemoLogs reads the real samples bucket in .env
// the way the API does. It changes nothing. It only runs when asked:
//
//	SUPABASE_STORAGE_TEST=1 go test ./internal/blob/
func TestSamplesBucketServesTheDemoLogs(t *testing.T) {
	if os.Getenv("SUPABASE_STORAGE_TEST") == "" {
		t.Skip("SUPABASE_STORAGE_TEST is not set")
	}
	if err := config.LoadDotEnv("../../.env"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	catalog := samples.NewCatalog(NewSupabase(cfg.SupabaseURL, cfg.SupabaseServiceKey, cfg.SamplesBucket))
	all, err := catalog.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("the samples bucket is empty; run `go run ./cmd/uploadsamples`")
	}
	for _, s := range all {
		t.Logf("%-12s %6d lines %9d bytes answerKey=%v", s.Name, s.LineCount, s.SizeBytes, s.HasAnswerKey)
		if !s.HasAnswerKey || s.LineCount == 0 || s.SizeBytes == 0 {
			t.Errorf("sample %s is missing its answer key, line count or size", s.Name)
		}
	}

	// The log itself streams, and its first line is a proxy log line.
	file, err := catalog.Open(ctx, all[0].Name)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer file.Close()
	first, err := bufio.NewReader(file).ReadString('\n')
	if err != nil || !strings.HasPrefix(first, "time=") {
		t.Fatalf("first line of %s: %q, err %v", all[0].Name, first[:min(len(first), 40)], err)
	}

	if _, err := catalog.Get(ctx, "no-such-sample"); err != samples.ErrNotFound {
		t.Fatalf("Get of a missing sample: err = %v, want ErrNotFound", err)
	}
	if _, err := catalog.Get(ctx, "../logs/secret"); err != samples.ErrNotFound {
		t.Fatalf("Get with a path in the name: err = %v, want ErrNotFound", err)
	}
}
