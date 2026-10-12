package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tenfold/backend/internal/model"
)

// ---------- Anomalies ----------

const anomalyColumns = `
	a.id, a.upload_id, a.detector, a.reason, a.label, a.signals, a.risk_score,
	a.decision, a.shadow_decision, a.disposition, a.incident_id,
	coalesce(a.usr, ''), coalesce(a.device, ''), coalesce(a.host, ''), a.hosts,
	a.line_nos, a.first_seen, a.last_seen, a.suppressed_by_lesson_id`

func scanAnomaly(row pgx.CollectableRow) (model.Anomaly, error) {
	var a model.Anomaly
	err := row.Scan(&a.ID, &a.UploadID, &a.Detector, &a.Reason, &a.Label, &a.Signals,
		&a.RiskScore, &a.Decision, &a.ShadowDecision, &a.Disposition, &a.IncidentID,
		&a.User, &a.Device, &a.Host, &a.Hosts, &a.LineNos, &a.FirstSeen, &a.LastSeen, &a.LessonID)
	a.FirstSeen, a.LastSeen = a.FirstSeen.UTC(), a.LastSeen.UTC()
	return a, err
}

// ReplaceAnomalies swaps an upload's anomalies, and the incidents built on
// them, for a fresh set. It runs in one transaction, so a reader never sees
// half of the old results and half of the new.
func (s *Store) ReplaceAnomalies(ctx context.Context, uploadID uuid.UUID, anomalies []model.Anomaly) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `delete from incidents where upload_id = $1`, uploadID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `delete from anomalies where upload_id = $1`, uploadID); err != nil {
			return err
		}

		_, err := tx.CopyFrom(ctx, pgx.Identifier{"anomalies"},
			[]string{"id", "upload_id", "detector", "reason", "label", "signals", "risk_score",
				"disposition", "usr", "device", "host", "hosts", "line_nos", "first_seen",
				"last_seen", "suppressed_by_lesson_id"},
			pgx.CopyFromSlice(len(anomalies), func(i int) ([]any, error) {
				a := anomalies[i]
				return []any{a.ID, uploadID, a.Detector, a.Reason, a.Label, jsonb(a.Signals),
					a.RiskScore, a.Disposition, nullable(a.User), nullable(a.Device),
					nullable(a.Host), a.Hosts, a.LineNos, a.FirstSeen, a.LastSeen, a.LessonID}, nil
			}))
		if err != nil {
			return fmt.Errorf("insert anomalies: %w", err)
		}

		var links [][]any
		for _, a := range anomalies {
			for _, lineNo := range a.LineNos {
				links = append(links, []any{uploadID, lineNo, a.ID})
			}
		}
		_, err = tx.CopyFrom(ctx, pgx.Identifier{"anomaly_events"},
			[]string{"upload_id", "line_no", "anomaly_id"}, pgx.CopyFromRows(links))
		if err != nil {
			return fmt.Errorf("link anomalies to events: %w", err)
		}
		return nil
	})
}

// LoadAnomalies returns every anomaly of an upload, including pending ones.
// Only the job calls it.
func (s *Store) LoadAnomalies(ctx context.Context, uploadID uuid.UUID) ([]model.Anomaly, error) {
	rows, err := s.pool.Query(ctx,
		`select `+anomalyColumns+` from anomalies a where a.upload_id = $1 order by a.first_seen, a.id`, uploadID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanAnomaly)
}

// SaveDecisions writes the AI decisions and dispositions the judging stage
// reached.
func (s *Store) SaveDecisions(ctx context.Context, anomalies []model.Anomaly) error {
	batch := &pgx.Batch{}
	for _, a := range anomalies {
		batch.Queue(`update anomalies set decision = $2, shadow_decision = $3, disposition = $4 where id = $1`,
			a.ID, jsonb(a.Decision), jsonb(a.ShadowDecision), a.Disposition)
	}
	return s.pool.SendBatch(ctx, batch).Close()
}

// AnomalyCursor marks where the previous page of anomalies ended.
type AnomalyCursor struct {
	RiskScore int
	ID        uuid.UUID
}

// AnomalyFilter narrows ListAnomalies. Zero values mean "no filter".
type AnomalyFilter struct {
	Detector       string
	Disposition    string
	JudgesDisagree *bool
	After          *AnomalyCursor
	Limit          int
}

// ListAnomalies returns one page of an upload's anomalies, highest risk
// score first, and the total that match.
func (s *Store) ListAnomalies(ctx context.Context, uploadID uuid.UUID, f AnomalyFilter) ([]model.Anomaly, int, error) {
	where := `a.upload_id = $1 and a.disposition <> 'pending'`
	args := []any{uploadID}
	if f.Detector != "" {
		args = append(args, f.Detector)
		where += fmt.Sprintf(" and a.detector = $%d", len(args))
	}
	if f.Disposition != "" {
		args = append(args, f.Disposition)
		where += fmt.Sprintf(" and a.disposition = $%d", len(args))
	}
	if f.JudgesDisagree != nil {
		// Both judges ruled and their labels differ. Without a shadow
		// decision there is nothing to disagree with.
		const disagree = `(a.decision is not null and a.shadow_decision is not null
			and a.decision->>'label' <> a.shadow_decision->>'label')`
		if *f.JudgesDisagree {
			where += " and " + disagree
		} else {
			where += " and not " + disagree
		}
	}

	var total int
	if err := s.pool.QueryRow(ctx, `select count(*) from anomalies a where `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	if f.After != nil {
		args = append(args, f.After.RiskScore, f.After.ID)
		where += fmt.Sprintf(" and (a.risk_score < $%d or (a.risk_score = $%d and a.id > $%d))",
			len(args)-1, len(args)-1, len(args))
	}
	args = append(args, f.Limit)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(
		`select %s from anomalies a where %s order by a.risk_score desc, a.id limit $%d`,
		anomalyColumns, where, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	anomalies, err := pgx.CollectRows(rows, scanAnomaly)
	return anomalies, total, err
}

// AnomalySpan is the little the timeseries needs from an anomaly.
type AnomalySpan struct {
	FirstSeen time.Time
	LastSeen  time.Time
	RiskScore int // the incident's score when the anomaly is in one
}

// AnomalySpans returns when each decided anomaly was active and how high
// it scored, for the risk line on the triage screen.
func (s *Store) AnomalySpans(ctx context.Context, uploadID uuid.UUID) ([]AnomalySpan, error) {
	rows, err := s.pool.Query(ctx, `
		select a.first_seen, a.last_seen, greatest(a.risk_score, coalesce(i.risk_score, 0))
		from anomalies a
		left join incidents i on i.id = a.incident_id
		where a.upload_id = $1 and a.disposition <> 'pending'`, uploadID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (AnomalySpan, error) {
		var sp AnomalySpan
		err := row.Scan(&sp.FirstSeen, &sp.LastSeen, &sp.RiskScore)
		return sp, err
	})
}

// ---------- Incidents ----------

const incidentColumns = `
	i.id, i.upload_id, i.user_id, i.title, i.severity, i.risk_score, i.ai_verdict,
	i.summary, i.marker_label, i.marker_detail, i.principal_user, i.principal_host,
	i.time_start, i.time_end, i.event_count, i.anomaly_count, i.mitre, i.line_nos,
	i.detail, i.analyst_status, i.analyst_note, i.decided_at, i.assignee_id,
	i.assignee_name, i.created_at`

func scanIncident(row pgx.CollectableRow) (model.Incident, error) {
	var i model.Incident
	err := row.Scan(&i.ID, &i.UploadID, &i.UserID, &i.Title, &i.Severity, &i.RiskScore,
		&i.AIVerdict, &i.Summary, &i.MarkerLabel, &i.MarkerDetail, &i.PrincipalUser,
		&i.PrincipalHost, &i.TimeStart, &i.TimeEnd, &i.EventCount, &i.AnomalyCount,
		&i.Mitre, &i.LineNos, &i.Detail, &i.AnalystStatus, &i.AnalystNote, &i.DecidedAt,
		&i.AssigneeID, &i.AssigneeName, &i.CreatedAt)
	i.TimeStart, i.TimeEnd = i.TimeStart.UTC(), i.TimeEnd.UTC()
	return i, err
}

// ReplaceIncidents swaps an upload's incidents for a fresh set and records
// what became of every anomaly, in one transaction.
func (s *Store) ReplaceIncidents(ctx context.Context, uploadID uuid.UUID, incidents []model.Incident, anomalies []model.Anomaly) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `delete from incidents where upload_id = $1`, uploadID); err != nil {
			return err
		}
		batch := &pgx.Batch{}
		for _, i := range incidents {
			batch.Queue(`
				insert into incidents (id, upload_id, user_id, title, severity, severity_rank,
					risk_score, ai_verdict, summary, marker_label, marker_detail, principal_user,
					principal_host, time_start, time_end, event_count, anomaly_count, mitre,
					line_nos, detail)
				values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)`,
				i.ID, uploadID, i.UserID, i.Title, i.Severity, model.SeverityRank(i.Severity),
				i.RiskScore, i.AIVerdict, i.Summary, i.MarkerLabel, i.MarkerDetail, i.PrincipalUser,
				i.PrincipalHost, i.TimeStart, i.TimeEnd, i.EventCount, i.AnomalyCount, jsonb(i.Mitre),
				i.LineNos, jsonb(i.Detail))
		}
		for _, a := range anomalies {
			batch.Queue(`update anomalies set disposition = $2, incident_id = $3 where id = $1`,
				a.ID, a.Disposition, a.IncidentID)
		}
		return tx.SendBatch(ctx, batch).Close()
	})
}

// SaveNarrative writes what the narrating stage produced for one incident.
func (s *Store) SaveNarrative(ctx context.Context, i model.Incident) error {
	_, err := s.pool.Exec(ctx,
		`update incidents set detail = $2, summary = $3 where id = $1`, i.ID, jsonb(i.Detail), i.Summary)
	return err
}

// LoadIncidents returns every incident of an upload. Only the job calls it.
func (s *Store) LoadIncidents(ctx context.Context, uploadID uuid.UUID) ([]model.Incident, error) {
	return s.queryIncidents(ctx, `i.upload_id = $1`, uploadID)
}

// ListIncidents returns the user's incidents for one upload, most severe
// first, then by risk score.
func (s *Store) ListIncidents(ctx context.Context, userID, uploadID uuid.UUID) ([]model.Incident, error) {
	return s.queryIncidents(ctx, `i.upload_id = $1 and i.user_id = $2`, uploadID, userID)
}

func (s *Store) queryIncidents(ctx context.Context, where string, args ...any) ([]model.Incident, error) {
	rows, err := s.pool.Query(ctx, `select `+incidentColumns+` from incidents i where `+where+
		` order by i.severity_rank, i.risk_score desc, i.time_start, i.id`, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanIncident)
}

// GetIncident returns one of the user's incidents.
func (s *Store) GetIncident(ctx context.Context, userID, id uuid.UUID) (model.Incident, error) {
	rows, err := s.pool.Query(ctx,
		`select `+incidentColumns+` from incidents i where i.id = $1 and i.user_id = $2`, id, userID)
	if err != nil {
		return model.Incident{}, err
	}
	i, err := pgx.CollectExactlyOneRow(rows, scanIncident)
	return i, notFound(err)
}

// SetAssignee assigns an incident, or clears the assignee when id is nil.
func (s *Store) SetAssignee(ctx context.Context, userID, incidentID uuid.UUID, id *uuid.UUID, name *string) error {
	tag, err := s.pool.Exec(ctx,
		`update incidents set assignee_id = $3, assignee_name = $4 where id = $1 and user_id = $2`,
		incidentID, userID, id, name)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// SetVerdict records the analyst's decision. A note, if given, is also
// appended to the incident's notes. A lesson, if given, is created in the
// same transaction.
func (s *Store) SetVerdict(ctx context.Context, userID, incidentID uuid.UUID, status string, note *model.Note, lesson *model.Lesson) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var noteText *string
		if note != nil {
			noteText = &note.Text
		}
		tag, err := tx.Exec(ctx, `
			update incidents set analyst_status = $3, analyst_note = $4, decided_at = now()
			where id = $1 and user_id = $2`, incidentID, userID, status, noteText)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if note != nil {
			if err := insertNote(ctx, tx, incidentID, *note); err != nil {
				return err
			}
		}
		if lesson != nil {
			return insertLesson(ctx, tx, *lesson)
		}
		return nil
	})
}

// ---------- Notes ----------

// execer is the part of a pool or a transaction that runs one statement.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func insertNote(ctx context.Context, db execer, incidentID uuid.UUID, n model.Note) error {
	_, err := db.Exec(ctx,
		`insert into incident_notes (id, incident_id, author, text, created_at) values ($1, $2, $3, $4, $5)`,
		n.ID, incidentID, n.Author, n.Text, n.CreatedAt)
	return err
}

// AddNote appends a note to one of the user's incidents.
func (s *Store) AddNote(ctx context.Context, userID, incidentID uuid.UUID, n model.Note) error {
	tag, err := s.pool.Exec(ctx, `
		insert into incident_notes (id, incident_id, author, text, created_at)
		select $1, i.id, $4, $5, $6 from incidents i where i.id = $2 and i.user_id = $3`,
		n.ID, incidentID, userID, n.Author, n.Text, n.CreatedAt)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ListNotes returns an incident's notes, oldest first.
func (s *Store) ListNotes(ctx context.Context, incidentID uuid.UUID) ([]model.Note, error) {
	rows, err := s.pool.Query(ctx, `
		select id, author, text, created_at from incident_notes
		where incident_id = $1 order by created_at, id`, incidentID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.Note, error) {
		var n model.Note
		err := row.Scan(&n.ID, &n.Author, &n.Text, &n.CreatedAt)
		n.CreatedAt = n.CreatedAt.UTC()
		return n, err
	})
}

// ---------- Lessons ----------

func insertLesson(ctx context.Context, db execer, l model.Lesson) error {
	_, err := db.Exec(ctx, `
		insert into lessons (id, user_id, text, scope_type, scope_value, created_at, expires_at)
		values ($1, $2, $3, $4, $5, $6, $7)`,
		l.ID, l.UserID, l.Text, l.ScopeType, l.ScopeValue, l.CreatedAt, l.ExpiresAt)
	return err
}

// CreateLesson saves a lesson.
func (s *Store) CreateLesson(ctx context.Context, l model.Lesson) error {
	return insertLesson(ctx, s.pool, l)
}

// ListLessons returns the user's lessons, newest first. With activeOnly, it
// leaves out the ones that have expired.
func (s *Store) ListLessons(ctx context.Context, userID uuid.UUID, activeOnly bool) ([]model.Lesson, error) {
	query := `select id, user_id, text, scope_type, scope_value, created_at, expires_at, match_count
		from lessons where user_id = $1`
	if activeOnly {
		query += ` and expires_at > now()`
	}
	rows, err := s.pool.Query(ctx, query+` order by created_at desc, id`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.Lesson, error) {
		var l model.Lesson
		err := row.Scan(&l.ID, &l.UserID, &l.Text, &l.ScopeType, &l.ScopeValue,
			&l.CreatedAt, &l.ExpiresAt, &l.MatchCount)
		l.CreatedAt, l.ExpiresAt = l.CreatedAt.UTC(), l.ExpiresAt.UTC()
		return l, err
	})
}

// DeleteLesson removes one of the user's lessons.
func (s *Store) DeleteLesson(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `delete from lessons where id = $1 and user_id = $2`, id, userID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// AddLessonMatches adds to each lesson's count of suppressed anomalies.
func (s *Store) AddLessonMatches(ctx context.Context, counts map[uuid.UUID]int) error {
	batch := &pgx.Batch{}
	for id, n := range counts {
		batch.Queue(`update lessons set match_count = match_count + $2 where id = $1`, id, n)
	}
	return s.pool.SendBatch(ctx, batch).Close()
}
