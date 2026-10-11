// Package zscaler reads and writes the web-proxy log line used by this
// project. The format is modeled on Zscaler NSS web-log feeds: key=value
// pairs separated by a single tab, always in the same order. It carries a
// subset of Zscaler's fields and is used only with synthetic data.
package zscaler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// None is what the feed writes for a missing value.
const None = "None"

// Fields lists the keys of one log line, in the order they appear.
var Fields = [...]string{
	"time", "recordid", "login", "dept", "cip", "sip", "host", "url",
	"urlcat", "action", "reason", "reqmethod", "respcode", "reqsize",
	"respsize", "ua", "appname", "appclass", "devicehostname", "threatname",
	"riskscore",
}

// Record is one parsed log line. Optional string fields are empty when the
// line says None.
type Record struct {
	Time           time.Time
	RecordID       int64
	Login          string
	Dept           string
	ClientIP       string
	ServerIP       string
	Host           string
	URL            string
	URLCategory    string
	Action         string
	Reason         string
	Method         string
	Status         int
	ReqSize        int64 // bytes sent by the client
	RespSize       int64 // bytes received by the client
	UserAgent      string
	AppName        string
	AppClass       string
	DeviceHostname string
	ThreatName     string
	RiskScore      int
}

// Format writes the record as one log line, without a trailing newline.
func (r Record) Format() string {
	values := [len(Fields)]string{
		r.Time.UTC().Format(time.RFC3339),
		strconv.FormatInt(r.RecordID, 10),
		r.Login,
		orNone(r.Dept),
		r.ClientIP,
		orNone(r.ServerIP),
		r.Host,
		r.URL,
		orNone(r.URLCategory),
		r.Action,
		orNone(r.Reason),
		r.Method,
		strconv.Itoa(r.Status),
		strconv.FormatInt(r.ReqSize, 10),
		strconv.FormatInt(r.RespSize, 10),
		orNone(r.UserAgent),
		orNone(r.AppName),
		orNone(r.AppClass),
		orNone(r.DeviceHostname),
		orNone(r.ThreatName),
		strconv.Itoa(r.RiskScore),
	}
	var b strings.Builder
	for i, key := range Fields {
		if i > 0 {
			b.WriteByte('\t')
		}
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(values[i])
	}
	return b.String()
}

// Parse reads one log line. It returns an error for any line that does not
// carry every field, in order, with valid values. Callers count and skip
// such lines.
func Parse(line string) (Record, error) {
	parts := strings.Split(strings.TrimRight(line, "\r\n"), "\t")
	if len(parts) != len(Fields) {
		return Record{}, fmt.Errorf("expected %d fields, found %d", len(Fields), len(parts))
	}
	var v [len(Fields)]string
	for i, key := range Fields {
		value, ok := strings.CutPrefix(parts[i], key+"=")
		if !ok {
			return Record{}, fmt.Errorf("field %d: expected key %q", i+1, key)
		}
		v[i] = value
	}

	var r Record
	var err error
	if r.Time, err = time.Parse(time.RFC3339, v[0]); err != nil {
		return Record{}, fmt.Errorf("time: %w", err)
	}
	if r.RecordID, err = strconv.ParseInt(v[1], 10, 64); err != nil {
		return Record{}, fmt.Errorf("recordid: %w", err)
	}
	if r.Status, err = strconv.Atoi(v[12]); err != nil {
		return Record{}, fmt.Errorf("respcode: %w", err)
	}
	if r.ReqSize, err = strconv.ParseInt(v[13], 10, 64); err != nil {
		return Record{}, fmt.Errorf("reqsize: %w", err)
	}
	if r.RespSize, err = strconv.ParseInt(v[14], 10, 64); err != nil {
		return Record{}, fmt.Errorf("respsize: %w", err)
	}
	if r.RiskScore, err = strconv.Atoi(v[20]); err != nil {
		return Record{}, fmt.Errorf("riskscore: %w", err)
	}
	if v[2] == "" || v[4] == "" || v[6] == "" {
		return Record{}, fmt.Errorf("login, cip and host are required")
	}
	if v[9] != "Allowed" && v[9] != "Blocked" {
		return Record{}, fmt.Errorf("action: %q is not Allowed or Blocked", v[9])
	}

	r.Time = r.Time.UTC()
	r.Login = v[2]
	r.Dept = noneToEmpty(v[3])
	r.ClientIP = v[4]
	r.ServerIP = noneToEmpty(v[5])
	r.Host = v[6]
	r.URL = v[7]
	r.URLCategory = noneToEmpty(v[8])
	r.Action = v[9]
	r.Reason = noneToEmpty(v[10])
	r.Method = v[11]
	r.UserAgent = noneToEmpty(v[15])
	r.AppName = noneToEmpty(v[16])
	r.AppClass = noneToEmpty(v[17])
	r.DeviceHostname = noneToEmpty(v[18])
	r.ThreatName = noneToEmpty(v[19])
	return r, nil
}

func orNone(s string) string {
	if s == "" {
		return None
	}
	return s
}

func noneToEmpty(s string) string {
	if s == None {
		return ""
	}
	return s
}
