package store

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tenfold/backend/internal/model"
)

// jsonb encodes a value for a jsonb column. A nil pointer becomes SQL NULL.
func jsonb(v any) any {
	if v == nil {
		return nil
	}
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return nil
	}
	encoded, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("store: cannot encode %T as JSON: %v", v, err))
	}
	return string(encoded)
}

// nullable stores an empty string as SQL NULL.
func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

var eventCopyColumns = []string{
	"upload_id", "line_no", "ts", "usr", "dept", "device", "src_ip", "host",
	"url", "category", "dst_ip", "method", "status", "bytes_out", "bytes_in",
	"user_agent", "action", "reason", "threat_name", "risk_score", "raw",
}

// DeleteEvents removes every event of an upload, before a fresh parse.
func (s *Store) DeleteEvents(ctx context.Context, uploadID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `delete from events where upload_id = $1`, uploadID)
	return err
}

// InsertEvents bulk-loads a batch of events with COPY, the fastest way
// into Postgres.
func (s *Store) InsertEvents(ctx context.Context, uploadID uuid.UUID, events []model.Event) error {
	_, err := s.pool.CopyFrom(ctx, pgx.Identifier{"events"}, eventCopyColumns,
		pgx.CopyFromSlice(len(events), func(i int) ([]any, error) {
			e := events[i]
			return []any{
				uploadID, e.LineNo, e.TS, e.User, nullable(e.Dept), nullable(e.Device),
				e.SrcIP, e.Host, e.URL, e.Category, nullable(e.DstIP), e.Method,
				e.Status, e.BytesOut, e.BytesIn, nullable(e.UserAgent), e.Action,
				nullable(e.Reason), nullable(e.ThreatName), e.RiskScore, e.Raw,
			}, nil
		}))
	return err
}

const eventColumns = `
	e.line_no, e.ts, e.usr, coalesce(e.dept, ''), coalesce(e.device, ''), e.src_ip,
	e.host, e.url, e.category, coalesce(e.dst_ip, ''), e.method, e.status,
	e.bytes_out, e.bytes_in, coalesce(e.user_agent, ''), e.action,
	coalesce(e.reason, ''), coalesce(e.threat_name, ''), coalesce(e.risk_score, 0)`

func scanEvent(row pgx.Row, withRaw bool) (model.Event, error) {
	var e model.Event
	dest := []any{
		&e.LineNo, &e.TS, &e.User, &e.Dept, &e.Device, &e.SrcIP, &e.Host, &e.URL,
		&e.Category, &e.DstIP, &e.Method, &e.Status, &e.BytesOut, &e.BytesIn,
		&e.UserAgent, &e.Action, &e.Reason, &e.ThreatName, &e.RiskScore,
	}
	if withRaw {
		dest = append(dest, &e.Raw)
	}
	err := row.Scan(dest...)
	e.TS = e.TS.UTC()
	return e, err
}

// LoadEvents returns every event of an upload in time order, without the
// raw line. The detectors work on this.
func (s *Store) LoadEvents(ctx context.Context, uploadID uuid.UUID) ([]model.Event, error) {
	rows, err := s.pool.Query(ctx,
		`select `+eventColumns+` from events e where e.upload_id = $1 order by e.ts, e.line_no`, uploadID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.Event, error) {
		return scanEvent(row, false)
	})
}

// StreamEvents calls fn for every event of an upload, in time order, with
// the raw line. It holds one row in memory at a time.
func (s *Store) StreamEvents(ctx context.Context, uploadID uuid.UUID, fn func(model.Event) error) error {
	rows, err := s.pool.Query(ctx,
		`select `+eventColumns+`, e.raw from events e where e.upload_id = $1 order by e.ts, e.line_no`, uploadID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		e, err := scanEvent(rows, true)
		if err != nil {
			return err
		}
		if err := fn(e); err != nil {
			return err
		}
	}
	return rows.Err()
}

// EventsByLineNos returns the events on the given lines, in time order.
func (s *Store) EventsByLineNos(ctx context.Context, uploadID uuid.UUID, lineNos []int, limit int) ([]model.Event, error) {
	rows, err := s.pool.Query(ctx, `
		select `+eventColumns+`, e.raw from events e
		where e.upload_id = $1 and e.line_no = any($2)
		order by e.ts, e.line_no limit $3`, uploadID, lineNos, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.Event, error) {
		return scanEvent(row, true)
	})
}

// SearchTerm is one term of the q parameter. Field is "user", "domain",
// "ip", "id", or empty for a bare term.
type SearchTerm struct {
	Field string
	Value string
}

// EventCursor marks where the previous page of events ended.
type EventCursor struct {
	TS     time.Time
	LineNo int
}

// EventFilter narrows ListEvents. Zero values mean "no filter".
type EventFilter struct {
	User, SrcIP, Host, Category, Action string
	HasAnomaly                          *bool
	From, To                            *time.Time
	Search                              []SearchTerm // joined by OR
	Descending                          bool
	After                               *EventCursor
	Limit                               int
}

// where builds the SQL conditions and their arguments. $1 is the upload.
func (f EventFilter) where(uploadID uuid.UUID) (string, []any) {
	args := []any{uploadID}
	conds := []string{"e.upload_id = $1"}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	exact := func(column, value string) {
		if value != "" {
			conds = append(conds, column+" = "+arg(value))
		}
	}
	exact("e.usr", f.User)
	exact("e.src_ip", f.SrcIP)
	exact("e.host", strings.ToLower(f.Host))
	exact("e.category", f.Category)
	exact("e.action", f.Action)

	const flagged = `exists (select 1 from anomaly_events ae
		where ae.upload_id = e.upload_id and ae.line_no = e.line_no)`
	if f.HasAnomaly != nil {
		if *f.HasAnomaly {
			conds = append(conds, flagged)
		} else {
			conds = append(conds, "not "+flagged)
		}
	}
	if f.From != nil {
		conds = append(conds, "e.ts >= "+arg(*f.From))
	}
	if f.To != nil {
		conds = append(conds, "e.ts < "+arg(*f.To))
	}

	var anyOf []string
	for _, term := range f.Search {
		switch term.Field {
		case "user":
			anyOf = append(anyOf, "e.usr = "+arg(term.Value))
		case "domain":
			host := strings.ToLower(term.Value)
			anyOf = append(anyOf, fmt.Sprintf("(e.host = %s or e.host like %s)",
				arg(host), arg("%."+escapeLike(host))))
		case "ip":
			p := arg(term.Value)
			anyOf = append(anyOf, fmt.Sprintf("(e.src_ip = %s or e.dst_ip = %s)", p, p))
		case "id":
			// An ID that is not e{n} matches nothing.
			lineNo, err := model.ParseEventID(term.Value)
			if err != nil {
				lineNo = -1
			}
			anyOf = append(anyOf, "e.line_no = "+arg(lineNo))
		default:
			p := arg("%" + escapeLike(term.Value) + "%")
			anyOf = append(anyOf, fmt.Sprintf(
				"(e.usr ilike %s or e.host ilike %s or e.url ilike %s or e.threat_name ilike %s)", p, p, p, p))
		}
	}
	if len(anyOf) > 0 {
		conds = append(conds, "("+strings.Join(anyOf, " or ")+")")
	}
	return strings.Join(conds, " and "), args
}

// escapeLike makes a user's text literal inside a LIKE pattern.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// ListEvents returns one page of events and the total that match the
// filter across all pages.
func (s *Store) ListEvents(ctx context.Context, uploadID uuid.UUID, f EventFilter) ([]model.Event, int, error) {
	where, args := f.where(uploadID)

	var total int
	if err := s.pool.QueryRow(ctx, `select count(*) from events e where `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	order, compare := "asc", ">"
	if f.Descending {
		order, compare = "desc", "<"
	}
	if f.After != nil {
		args = append(args, f.After.TS, f.After.LineNo)
		where += fmt.Sprintf(" and (e.ts, e.line_no) %s ($%d, $%d)", compare, len(args)-1, len(args))
	}
	args = append(args, f.Limit)
	query := fmt.Sprintf(`select %s, e.raw from events e where %s order by e.ts %s, e.line_no %s limit $%d`,
		eventColumns, where, order, order, len(args))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	events, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.Event, error) {
		return scanEvent(row, true)
	})
	return events, total, err
}

// EventFlag ties one event to one anomaly it is evidence for.
type EventFlag struct {
	AnomalyID   uuid.UUID
	Label       string
	Disposition string
	IncidentID  *uuid.UUID
}

// EventFlags returns, for each of the given lines, the anomalies it is
// evidence for. Anomalies the job has not yet decided on are left out.
func (s *Store) EventFlags(ctx context.Context, uploadID uuid.UUID, lineNos []int) (map[int][]EventFlag, error) {
	flags := map[int][]EventFlag{}
	if len(lineNos) == 0 {
		return flags, nil
	}
	rows, err := s.pool.Query(ctx, `
		select ae.line_no, a.id, a.label, a.disposition, a.incident_id
		from anomaly_events ae
		join anomalies a on a.id = ae.anomaly_id
		where ae.upload_id = $1 and ae.line_no = any($2) and a.disposition <> 'pending'
		order by a.risk_score desc, a.id`, uploadID, lineNos)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var lineNo int
		var f EventFlag
		if err := rows.Scan(&lineNo, &f.AnomalyID, &f.Label, &f.Disposition, &f.IncidentID); err != nil {
			return nil, err
		}
		flags[lineNo] = append(flags[lineNo], f)
	}
	return flags, rows.Err()
}

// Bucket is the event counts for one slice of time.
type Bucket struct {
	Start     time.Time
	Total     int
	Blocked   int
	Anomalous int
}

// EventBuckets counts an upload's events per bucket of the given width,
// aligned to origin. Buckets with no events are not returned.
func (s *Store) EventBuckets(ctx context.Context, uploadID uuid.UUID, width time.Duration, origin time.Time) ([]Bucket, error) {
	rows, err := s.pool.Query(ctx, `
		select date_bin($2::interval, e.ts, $3) as bucket,
			count(*),
			count(*) filter (where e.action = 'blocked'),
			count(*) filter (where exists (
				select 1 from anomaly_events ae
				where ae.upload_id = e.upload_id and ae.line_no = e.line_no))
		from events e
		where e.upload_id = $1
		group by bucket order by bucket`, uploadID, width, origin)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Bucket, error) {
		var b Bucket
		err := row.Scan(&b.Start, &b.Total, &b.Blocked, &b.Anomalous)
		b.Start = b.Start.UTC()
		return b, err
	})
}
