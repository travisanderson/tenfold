// Package storetest gives tests a real, empty database. Tests that need one
// are skipped unless TEST_DATABASE_URL is set, so `go test ./...` passes on
// a machine without Postgres.
//
//	docker compose up -d
//	TEST_DATABASE_URL=postgres://tenfold:tenfold@localhost:54329/tenfold go test ./...
package storetest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tenfold/backend/internal/store"
)

// New returns a store on a fresh schema with every migration applied. The
// schema is dropped when the test ends, so tests never see each other's rows.
func New(t *testing.T) *store.Store {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	schema := "test_" + uuid.NewString()[:8]

	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := admin.Exec(ctx, "create schema "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		admin.Exec(ctx, "drop schema "+schema+" cascade")
		admin.Close(ctx)
	})

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	s, err := store.Open(ctx, u.String(), 4)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(s.Close)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", fmt.Errorf("%w", err))
	}
	return s
}
