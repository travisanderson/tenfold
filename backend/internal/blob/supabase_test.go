package blob

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"tenfold/backend/internal/config"
)

// TestSupabaseRoundTrip talks to the real Supabase project in .env. It
// writes one small file to the bucket and deletes it again. It only runs
// when asked:
//
//	SUPABASE_STORAGE_TEST=1 go test ./internal/blob/
func TestSupabaseRoundTrip(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store := NewSupabase(cfg.SupabaseURL, cfg.SupabaseServiceKey, cfg.StorageBucket)
	if err := store.EnsureBucket(ctx, 52_428_800); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}
	key := "test/" + uuid.NewString() + ".log"
	const content = "time=2026-10-08T04:00:03Z\trecordid=1\n"

	// A file that was never uploaded is "not found", not a server error.
	if _, err := store.Open(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open before upload: err = %v, want ErrNotFound", err)
	}

	// The browser's half: PUT straight to the signed URL, with no key.
	uploadURL, expiresAt, err := store.SignedUploadURL(ctx, key)
	if err != nil {
		t.Fatalf("SignedUploadURL: %v", err)
	}
	if !expiresAt.After(time.Now()) || strings.Contains(uploadURL, cfg.SupabaseServiceKey) {
		t.Fatal("the signed URL is expired or carries the secret key")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, strings.NewReader(content))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT to the signed URL: status %d: %s", resp.StatusCode, body)
	}
	t.Cleanup(func() { store.Delete(context.Background(), key) })

	file, err := store.Open(ctx, key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, _ := io.ReadAll(file)
	file.Close()
	if string(got) != content {
		t.Fatalf("read back %q, want %q", got, content)
	}

	// The bucket is private: without a key, the file cannot be read.
	public, err := http.Get(cfg.SupabaseURL + "/storage/v1/object/public/" + cfg.StorageBucket + "/" + key)
	if err != nil {
		t.Fatal(err)
	}
	public.Body.Close()
	if public.StatusCode == http.StatusOK {
		t.Fatal("the file is readable without a key; the bucket must be private")
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Open(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open after delete: err = %v, want ErrNotFound", err)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("deleting a missing file should not fail: %v", err)
	}
}
