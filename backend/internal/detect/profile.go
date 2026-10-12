package detect

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"tenfold/backend/internal/model"
)

// profile is what the detectors know about the file as a whole: who is in
// it, where they go, and when the company is quiet. Baselines come from
// here, measured in this file alone.
type profile struct {
	events    []model.Event            // every event, in time order
	users     []string                 // sorted
	byUser    map[string][]model.Event // each user's events, in time order
	hostUsers map[string]int           // how many distinct users reached each host

	// quiet marks the hours of the day (UTC) in which the typical user
	// made under quietShare of the requests they make in the busiest hour.
	// "Typical" is the median across users, so one machine working through
	// the night (a backup job, say) cannot make the night look busy.
	quiet [24]bool
}

// common reports whether a host is one much of the company uses. A file
// with too few users cannot say.
func (p *profile) common(host string) bool {
	return len(p.users) >= rareMinUsers &&
		float64(p.hostUsers[host]) >= exfilCommonShare*float64(len(p.users))
}

// quietShare defines the file's quiet hours: under a tenth of the peak hour.
const quietShare = 0.10

func newProfile(events []model.Event) *profile {
	p := &profile{
		events:    events,
		byUser:    map[string][]model.Event{},
		hostUsers: map[string]int{},
	}
	seen := map[string]map[string]bool{}
	var perHour [24]map[string]float64 // requests per user, per hour of day
	for hour := range perHour {
		perHour[hour] = map[string]float64{}
	}
	for _, e := range events {
		p.byUser[e.User] = append(p.byUser[e.User], e)
		if seen[e.Host] == nil {
			seen[e.Host] = map[string]bool{}
		}
		seen[e.Host][e.User] = true
		perHour[e.TS.Hour()][e.User]++
	}
	for user := range p.byUser {
		p.users = append(p.users, user)
	}
	sort.Strings(p.users)
	for host, users := range seen {
		p.hostUsers[host] = len(users)
	}

	var typical [24]float64
	peak := 0.0
	for hour, counts := range perHour {
		perUser := make([]float64, 0, len(p.users))
		for _, user := range p.users {
			perUser = append(perUser, counts[user]) // 0 for a user idle that hour
		}
		typical[hour] = median(perUser)
		peak = max(peak, typical[hour])
	}
	for hour := range typical {
		p.quiet[hour] = typical[hour] < quietShare*peak
	}
	return p
}

// hourlyBytesOut totals a user's outbound bytes per clock hour, skipping
// any hour for which skip returns true.
func hourlyBytesOut(events []model.Event, skip func(hour time.Time) bool) []float64 {
	totals := map[time.Time]float64{}
	for _, e := range events {
		hour := e.TS.Truncate(time.Hour)
		if skip != nil && skip(hour) {
			continue
		}
		totals[hour] += float64(e.BytesOut)
	}
	values := make([]float64, 0, len(totals))
	for _, v := range totals {
		values = append(values, v)
	}
	return values
}

// median returns the middle value, or 0 for no values.
func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

// sessions splits ordered events into runs with no gap longer than maxGap.
func sessions(events []model.Event, maxGap time.Duration) [][]model.Event {
	var out [][]model.Event
	start := 0
	for i := 1; i <= len(events); i++ {
		if i == len(events) || events[i].TS.Sub(events[i-1].TS) > maxGap {
			out = append(out, events[start:i])
			start = i
		}
	}
	return out
}

// ---------- Formatting for signal text ----------

// FormatBytes writes a byte count the way the UI shows it: "40.2 MB".
func FormatBytes(n float64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", n/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", n/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", n/(1<<10))
	default:
		return fmt.Sprintf("%.0f B", n)
	}
}

// FormatCount writes an integer with thousands separators: "48,212".
func FormatCount(n int) string {
	digits := strconv.Itoa(n)
	if n < 0 {
		return "-" + FormatCount(-n)
	}
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	return b.String()
}

// formatSpan writes a duration in the largest unit that fits: "23 minutes".
func formatSpan(d time.Duration) string {
	switch {
	case d >= 90*time.Minute:
		return fmt.Sprintf("%.1f hours", d.Hours())
	case d >= 90*time.Second:
		return plural(int(math.Round(d.Minutes())), "minute")
	default:
		return plural(int(math.Round(d.Seconds())), "second")
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return FormatCount(n) + " " + unit + "s"
}

// formatRatio writes "30×", "1.5×" for small ratios, and stops counting
// past a thousand, where the exact figure no longer means anything.
func formatRatio(r float64) string {
	if r >= 1000 {
		return "over 1,000×"
	}
	if r >= 10 {
		return fmt.Sprintf("%.0f×", r)
	}
	return fmt.Sprintf("%.1f×", r)
}
