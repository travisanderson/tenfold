package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tenfold/backend/internal/model"
)

// uploadColumns lists what every upload read selects. verdict_counts is
// counted from the upload's incidents and is null until some exist.
const uploadColumns = `
	u.id, u.user_id, u.filename, u.source, u.sample_name, u.storage_key,
	u.size_bytes, u.status, u.stage, u.progress, u.message, u.pipeline,
	u.stop, u.first_parse_error, u.error, u.risk_threshold, u.line_count,
	u.event_count, u.parse_error_count, u.anomaly_count, u.incident_count,
	u.user_count, u.host_count, u.time_start, u.time_end, u.created_at,
	u.started_at, u.completed_at,
	(select jsonb_build_object(
		'open',      count(*) filter (where i.analyst_status = 'open'),
		'confirmed', count(*) filter (where i.analyst_status = 'confirmed'),
		'dismissed', count(*) filter (where i.analyst_status = 'dismissed'),
		'escalated', count(*) filter (where i.analyst_status = 'escalated'))
	 from incidents i where i.upload_id = u.id having count(*) > 0) as verdict_counts`

func scanUploads(rows pgx.Rows) ([]model.Upload, error) {
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Upload])
}

// CreateUpload saves a new upload.
func (s *Store) CreateUpload(ctx context.Context, u model.Upload) error {
	_, err := s.pool.Exec(ctx, `
		insert into uploads (id, user_id, filename, source, sample_name, storage_key,
			size_bytes, status, stage, pipeline, risk_threshold, created_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		u.ID, u.UserID, u.Filename, u.Source, u.SampleName, u.StorageKey,
		u.SizeBytes, u.Status, u.Stage, jsonb(u.Pipeline), u.RiskThreshold, u.CreatedAt)
	return err
}

// GetUpload returns one of the user's uploads.
func (s *Store) GetUpload(ctx context.Context, userID, id uuid.UUID) (model.Upload, error) {
	rows, err := s.pool.Query(ctx,
		`select `+uploadColumns+` from uploads u where u.id = $1 and u.user_id = $2`, id, userID)
	if err != nil {
		return model.Upload{}, err
	}
	u, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[model.Upload])
	return u, notFound(err)
}

// UploadCursor marks where the previous page of uploads ended.
type UploadCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// ListUploads returns a page of the user's uploads, newest first, and the
// total across all pages.
func (s *Store) ListUploads(ctx context.Context, userID uuid.UUID, after *UploadCursor, limit int) ([]model.Upload, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx,
		`select count(*) from uploads where user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `select ` + uploadColumns + ` from uploads u where u.user_id = $1`
	args := []any{userID}
	if after != nil {
		query += ` and (u.created_at, u.id) < ($3, $4)`
		args = append(args, limit, after.CreatedAt, after.ID)
	} else {
		args = append(args, limit)
	}
	query += ` order by u.created_at desc, u.id desc limit $2`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	uploads, err := scanUploads(rows)
	return uploads, total, err
}

// DeleteUpload removes an upload and, by cascade, its events and results.
// It returns the deleted row so the caller can remove the stored file.
func (s *Store) DeleteUpload(ctx context.Context, userID, id uuid.UUID) (storageKey *string, err error) {
	err = s.pool.QueryRow(ctx,
		`delete from uploads where id = $1 and user_id = $2 returning storage_key`, id, userID).Scan(&storageKey)
	return storageKey, notFound(err)
}

// ClaimUpload marks an upload as processing, unless a job already holds it.
// It reports whether the claim succeeded. This one statement is what stops
// two jobs from running on the same upload.
func (s *Store) ClaimUpload(ctx context.Context, userID, id uuid.UUID) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		update uploads set status = 'processing', started_at = now(), completed_at = null
		where id = $1 and user_id = $2 and status <> 'processing'`, id, userID)
	return tag.RowsAffected() == 1, err
}

// SaveJobState writes everything a running job changes on its upload.
func (s *Store) SaveJobState(ctx context.Context, u model.Upload) error {
	_, err := s.pool.Exec(ctx, `
		update uploads set
			status = $2, stage = $3, progress = $4, message = $5, pipeline = $6,
			stop = $7, first_parse_error = $8, error = $9, line_count = $10,
			event_count = $11, parse_error_count = $12, anomaly_count = $13,
			incident_count = $14, user_count = $15, host_count = $16,
			time_start = $17, time_end = $18, completed_at = $19
		where id = $1`,
		u.ID, u.Status, u.Stage, u.Progress, u.Message, jsonb(u.Pipeline),
		jsonb(u.Stop), jsonb(u.FirstParseError), jsonb(u.Error), u.LineCount,
		u.EventCount, u.ParseErrorCount, u.AnomalyCount, u.IncidentCount,
		u.UserCount, u.HostCount, u.TimeStart, u.TimeEnd, u.CompletedAt)
	return err
}

// ProcessingUploads returns every upload marked as processing, for any
// user. The server calls it once at startup: a job cannot survive a
// restart, so these uploads are stranded and must be marked as stopped.
func (s *Store) ProcessingUploads(ctx context.Context) ([]model.Upload, error) {
	rows, err := s.pool.Query(ctx,
		`select `+uploadColumns+` from uploads u where u.status = 'processing'`)
	if err != nil {
		return nil, err
	}
	return scanUploads(rows)
}
