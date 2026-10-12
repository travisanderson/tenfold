// Package store is the only code that talks to Postgres. Every query that
// reads or writes a user's data takes the user's ID and filters by it, so a
// request for someone else's row finds nothing.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound means the row does not exist or belongs to another user.
var ErrNotFound = errors.New("not found")

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Store wraps the connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// Open connects to Postgres and checks the connection. maxConns caps the
// pool; zero keeps pgx's default.
func Open(ctx context.Context, databaseURL string, maxConns int) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		// pgx's message can quote the URL, password included.
		return nil, errors.New("connect: DATABASE_URL is not a valid Postgres connection string")
	}
	if maxConns > 0 {
		cfg.MaxConns = int32(maxConns)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases the pool.
func (s *Store) Close() { s.pool.Close() }

// Ping reports whether the database answers.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Migrate applies every migration file that has not run yet, in name order,
// each in its own transaction.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `
		create table if not exists schema_migrations (
			name text primary key,
			applied_at timestamptz not null default now()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	// Supabase exposes the public schema over REST; keep this table closed too.
	if _, err := s.pool.Exec(ctx, `alter table schema_migrations enable row level security`); err != nil {
		return fmt.Errorf("secure schema_migrations: %w", err)
	}

	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		var done bool
		if err := s.pool.QueryRow(ctx,
			`select exists (select 1 from schema_migrations where name = $1)`, name).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		sql, err := migrationFiles.ReadFile(name)
		if err != nil {
			return err
		}
		err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `insert into schema_migrations (name) values ($1)`, name)
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
	}
	return nil
}

// SafeError describes an error for a log without quoting customer data. A
// Postgres error can carry the offending row in its detail and context
// fields, so only its code and message are kept.
func SafeError(err error) string {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return fmt.Sprintf("postgres %s: %s", pg.Code, pg.Message)
	}
	return err.Error()
}

// notFound turns pgx's "no rows" into ErrNotFound.
func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
