// Package model holds the types the store, the pipeline and the handlers
// share. Shapes that are saved as JSON and returned unchanged reuse the
// generated API types, so there is one definition of each.
package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/zscaler"
)

// Upload is one log file and the state of its analysis job.
type Upload struct {
	ID              uuid.UUID           `db:"id"`
	UserID          uuid.UUID           `db:"user_id"`
	Filename        string              `db:"filename"`
	Source          string              `db:"source"`
	SampleName      *string             `db:"sample_name"`
	StorageKey      *string             `db:"storage_key"`
	SizeBytes       int64               `db:"size_bytes"`
	Status          string              `db:"status"`
	Stage           string              `db:"stage"`
	Progress        int                 `db:"progress"`
	Message         string              `db:"message"`
	Pipeline        []api.StageProgress `db:"pipeline"`
	Stop            *api.UploadStop     `db:"stop"`
	FirstParseError *api.ParseError     `db:"first_parse_error"`
	Error           *api.UploadError    `db:"error"`
	RiskThreshold   int                 `db:"risk_threshold"`
	LineCount       *int                `db:"line_count"`
	EventCount      *int                `db:"event_count"`
	ParseErrorCount *int                `db:"parse_error_count"`
	AnomalyCount    *int                `db:"anomaly_count"`
	IncidentCount   *int                `db:"incident_count"`
	UserCount       *int                `db:"user_count"`
	HostCount       *int                `db:"host_count"`
	TimeStart       *time.Time          `db:"time_start"`
	TimeEnd         *time.Time          `db:"time_end"`
	CreatedAt       time.Time           `db:"created_at"`
	StartedAt       *time.Time          `db:"started_at"`
	CompletedAt     *time.Time          `db:"completed_at"`
	// VerdictCounts is read with the upload; it is not a column.
	VerdictCounts *api.VerdictCounts `db:"verdict_counts"`
}

// Event is one log line in the vendor-neutral shape the API returns.
type Event struct {
	LineNo     int
	TS         time.Time
	User       string
	Dept       string
	Device     string
	SrcIP      string
	Host       string
	URL        string
	Category   string
	DstIP      string
	Method     string
	Status     int
	BytesOut   int64
	BytesIn    int64
	UserAgent  string
	Action     string // "allowed" or "blocked"
	Reason     string
	ThreatName string
	RiskScore  int // Zscaler's own page score, as logged
	Raw        string
}

// UnknownCategory stands in when the proxy logged no URL category.
const UnknownCategory = "Unknown"

// EventFromRecord normalizes one parsed Zscaler line. The user is the login
// without its mail domain, so "dana.k@northwindlabs.example" becomes "dana.k".
func EventFromRecord(lineNo int, r zscaler.Record, raw string) Event {
	user, _, _ := strings.Cut(r.Login, "@")
	category := r.URLCategory
	if category == "" {
		category = UnknownCategory
	}
	return Event{
		LineNo:     lineNo,
		TS:         r.Time,
		User:       user,
		Dept:       r.Dept,
		Device:     r.DeviceHostname,
		SrcIP:      r.ClientIP,
		Host:       strings.ToLower(r.Host),
		URL:        r.URL,
		Category:   category,
		DstIP:      r.ServerIP,
		Method:     r.Method,
		Status:     r.Status,
		BytesOut:   r.ReqSize,
		BytesIn:    r.RespSize,
		UserAgent:  r.UserAgent,
		Action:     strings.ToLower(r.Action),
		Reason:     r.Reason,
		ThreatName: r.ThreatName,
		RiskScore:  r.RiskScore,
		Raw:        raw,
	}
}

// EventID is the API's ID for the event on a line: "e" plus the line number.
func EventID(lineNo int) string { return "e" + strconv.Itoa(lineNo) }

// EventIDs maps line numbers to event IDs.
func EventIDs(lineNos []int) []string {
	ids := make([]string, len(lineNos))
	for i, n := range lineNos {
		ids[i] = EventID(n)
	}
	return ids
}

// ParseEventID reads an event ID back into its line number.
func ParseEventID(id string) (int, error) {
	digits, ok := strings.CutPrefix(id, "e")
	if !ok {
		return 0, fmt.Errorf("event ID %q does not start with e", id)
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("event ID %q has no line number", id)
	}
	return n, nil
}

// Dispositions. Pending is internal: the job has not decided yet, and the
// API never returns an anomaly in that state.
const (
	DispositionPending        = "pending"
	DispositionInIncident     = string(api.AnomalyDispositionInIncident)
	DispositionBelowThreshold = string(api.AnomalyDispositionBelowThreshold)
	DispositionJudgedBenign   = string(api.AnomalyDispositionJudgedBenign)
	DispositionLesson         = string(api.AnomalyDispositionLesson)
)

// Anomaly is one detector's finding.
type Anomaly struct {
	ID             uuid.UUID
	UploadID       uuid.UUID
	Detector       string
	Reason         string // one plain sentence
	Label          string // a few words, for a table cell
	Signals        []api.Signal
	RiskScore      int
	Decision       *api.Decision
	ShadowDecision *api.Decision
	Disposition    string
	IncidentID     *uuid.UUID
	User           string
	Device         string
	Host           string   // the main destination
	Hosts          []string // every destination, for lesson matching
	LineNos        []int
	FirstSeen      time.Time
	LastSeen       time.Time
	LessonID       *uuid.UUID
}

// IncidentDetail is everything the incident screen draws beyond the summary.
// It is built once by the job and saved as JSON.
type IncidentDetail struct {
	Narrative         api.Narrative          `json:"narrative"`
	Signals           []api.Signal           `json:"signals"`
	Timeline          []api.TimelineItem     `json:"timeline"`
	Graph             api.IncidentGraph      `json:"graph"`
	Indicators        []api.Indicator        `json:"indicators"`
	Chart             api.IncidentChart      `json:"chart"`
	LessonSuggestions []api.LessonSuggestion `json:"lessonSuggestions"`
	// Provenance, for the agent context endpoint.
	Detectors        []api.DetectorType    `json:"detectors"`
	DecisionProvider *api.DecisionProvider `json:"decisionProvider,omitempty"`
}

// Incident is a group of anomalies raised to the analyst.
type Incident struct {
	ID            uuid.UUID
	UploadID      uuid.UUID
	UserID        uuid.UUID
	Title         string
	Severity      string
	RiskScore     int
	AIVerdict     *string
	Summary       *string
	MarkerLabel   string
	MarkerDetail  *string
	PrincipalUser *string
	PrincipalHost *string
	TimeStart     time.Time
	TimeEnd       time.Time
	EventCount    int
	AnomalyCount  int
	Mitre         []api.MitreTechnique
	LineNos       []int
	Detail        IncidentDetail
	AnalystStatus string
	AnalystNote   *string
	DecidedAt     *time.Time
	AssigneeID    *uuid.UUID
	AssigneeName  *string
	CreatedAt     time.Time
}

// SeverityRank orders severities for sorting: critical first.
func SeverityRank(severity string) int {
	switch api.Severity(severity) {
	case api.SeverityCritical:
		return 0
	case api.SeverityHigh:
		return 1
	case api.SeverityMedium:
		return 2
	default:
		return 3
	}
}

// Lesson is something an analyst taught the system once.
type Lesson struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Text       string
	ScopeType  string
	ScopeValue string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	MatchCount int
}

// Note is one analyst note on an incident.
type Note struct {
	ID        uuid.UUID
	Author    string
	Text      string
	CreatedAt time.Time
}
