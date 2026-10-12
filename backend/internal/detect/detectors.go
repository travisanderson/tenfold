package detect

import (
	"fmt"
	"math"
	"sort"
	"time"

	"tenfold/backend/internal/api"
	"tenfold/backend/internal/model"
)

// Thresholds. Each is a fixed rule; none is learned.
const (
	// threat_hit has no threshold: any line where the proxy names a threat.

	burstWindow    = 5 * time.Minute // blocked_burst looks at windows this long
	burstMinBlocks = 20              // and needs this many blocked requests in one

	exfilSessionGap = 10 * time.Minute // a gap this long ends a transfer
	exfilMinBytes   = 10 << 20         // a transfer under 10 MB is never flagged
	exfilMinRatio   = 10.0             // and it must be 10× the baseline hour
	exfilMinHours   = 3                // hours of a user's own history a baseline needs

	// A destination half the company uses is a common one, and a transfer to
	// it must be larger to count. Everyone uploads to the shared drive; one
	// 15 MB file there is a slide deck. The cost is named: under 50 MB to a
	// common host is never flagged for its size alone.
	exfilCommonShare    = 0.5
	exfilCommonMinBytes = 50 << 20

	beaconMinRequests = 30              // too few requests cannot show a rhythm
	beaconTolerance   = 0.10            // an interval within 10% of the median is "on beat"
	beaconMinOnBeat   = 0.80            // share of intervals that must be on beat
	beaconFast        = 5 * time.Minute // at or under this, the rhythm scores as fast
	beaconSustained   = 2 * time.Hour   // a fast rhythm kept up this long adds a signal

	rareMinUsers = 5 // a file with fewer users cannot say what is rare

	newIPHomeShare = 0.90 // a user's home address carries this share of requests

	offHoursMinBytes     = 5 << 20  // outbound bytes in one quiet hour worth flagging
	offHoursMinEventSize = 64 << 10 // smaller requests are background noise
)

// threatHits flags requests the proxy itself tagged with a threat name, one
// anomaly per user and destination.
func threatHits(p *profile) []model.Anomaly {
	var out []model.Anomaly
	for _, user := range p.users {
		byHost := map[string][]model.Event{}
		var hosts []string
		for _, e := range p.byUser[user] {
			if e.ThreatName == "" {
				continue
			}
			if byHost[e.Host] == nil {
				hosts = append(hosts, e.Host)
			}
			byHost[e.Host] = append(byHost[e.Host], e)
		}
		for _, host := range hosts {
			hits := byHost[host]
			allowed := 0
			for _, e := range hits {
				if e.Action == "allowed" {
					allowed++
				}
			}
			weight, outcome := WeightThreatBlocked, "all blocked"
			if allowed > 0 {
				weight, outcome = WeightThreatAllowed, fmt.Sprintf("%d allowed", allowed)
			}
			threat := hits[0].ThreatName
			signal := api.Signal{
				Name:           SignalThreat,
				Description:    fmt.Sprintf("The proxy tagged requests to %s as %s", host, threat),
				Weight:         weight,
				Layer:          api.SignalLayerRules,
				Observed:       threat,
				ObservedDetail: ptr(fmt.Sprintf("%s, %s", plural(len(hits), "request"), outcome)),
				BaselineSource: api.BaselineSourceNone,
				EventIds:       eventIDs(hits),
			}
			reason := fmt.Sprintf("%s made %s to %s, which the proxy tagged as %s (%s).",
				user, plural(len(hits), "request"), host, threat, outcome)
			out = append(out, anomaly(api.DetectorTypeThreatHit, hits, reason, "proxy threat: "+threat, signal))
		}
	}
	return out
}

// blockedBursts flags a user who is blocked many times in a short window.
// Routine blocks (ads, social media) trickle in; a burst does not.
func blockedBursts(p *profile) []model.Anomaly {
	var out []model.Anomaly
	for _, user := range p.users {
		var blocked []model.Event
		for _, e := range p.byUser[user] {
			if e.Action == "blocked" {
				blocked = append(blocked, e)
			}
		}
		// Slide a window over the blocked requests. Windows that qualify
		// and overlap merge into one burst.
		inBurst := make([]bool, len(blocked))
		start := 0
		for end := range blocked {
			for blocked[end].TS.Sub(blocked[start].TS) > burstWindow {
				start++
			}
			if end-start+1 >= burstMinBlocks {
				for i := start; i <= end; i++ {
					inBurst[i] = true
				}
			}
		}
		for i := 0; i < len(blocked); {
			if !inBurst[i] {
				i++
				continue
			}
			j := i
			for j < len(blocked) && inBurst[j] {
				j++
			}
			burst := blocked[i:j]
			i = j

			hosts := map[string]bool{}
			categories := map[string]int{}
			for _, e := range burst {
				hosts[e.Host] = true
				categories[e.Category]++
			}
			category := topKey(categories)
			took := formatSpan(span(burst))
			signal := api.Signal{
				Name:           SignalBlockedBurst,
				Description:    fmt.Sprintf("%d blocked requests in %s, mostly %s", len(burst), took, category),
				Weight:         WeightBlockedBurst,
				Layer:          api.SignalLayerRules,
				Observed:       plural(len(burst), "blocked request"),
				ObservedDetail: ptr(fmt.Sprintf("in %s, to %s", took, plural(len(hosts), "host"))),
				BaselineText:   ptr(fmt.Sprintf("%d", burstMinBlocks)),
				BaselineDetail: ptr(fmt.Sprintf("blocked requests in %s, the fixed limit", formatSpan(burstWindow))),
				BaselineSource: api.BaselineSourceNone,
				Value:          ptr(float64(len(burst))),
				Baseline:       ptr(float64(burstMinBlocks)),
				Unit:           ptr("requests"),
				EventIds:       eventIDs(burst),
			}
			reason := fmt.Sprintf("%s was blocked %d times in %s across %s, mostly in the %s category.",
				user, len(burst), took, plural(len(hosts), "host"), category)
			out = append(out, anomaly(api.DetectorTypeBlockedBurst, burst, reason,
				fmt.Sprintf("%d blocked in %s", len(burst), took), signal))
		}
	}
	return out
}

// exfilVolume flags one user sending far more to one host, in one sitting,
// than that user sends in a typical hour.
func exfilVolume(p *profile) []model.Anomaly {
	// The fallback baseline: a typical user-hour across the whole file.
	var allHours []float64
	for _, user := range p.users {
		allHours = append(allHours, hourlyBytesOut(p.byUser[user], nil)...)
	}
	companyHour := median(allHours)

	var out []model.Anomaly
	for _, user := range p.users {
		byHost := map[string][]model.Event{}
		var hosts []string
		for _, e := range p.byUser[user] {
			if byHost[e.Host] == nil {
				hosts = append(hosts, e.Host)
			}
			byHost[e.Host] = append(byHost[e.Host], e)
		}
		for _, host := range hosts {
			for _, transfer := range sessions(byHost[host], exfilSessionGap) {
				var sent float64
				for _, e := range transfer {
					sent += float64(e.BytesOut)
				}
				if sent < exfilMinBytes || (p.common(host) && sent < exfilCommonMinBytes) {
					continue
				}

				// The user's own median hour, leaving out the hours of this
				// transfer so it cannot hide inside its own baseline.
				first, last := transfer[0].TS.Truncate(time.Hour), transfer[len(transfer)-1].TS.Truncate(time.Hour)
				ownHours := hourlyBytesOut(p.byUser[user], func(hour time.Time) bool {
					return !hour.Before(first) && !hour.After(last)
				})
				baseline, whose, detail := median(ownHours), "this user's", "per hour, this user's median in this file"
				if len(ownHours) < exfilMinHours {
					baseline, whose, detail = companyHour, "a typical user's", "per hour, median across all users in this file"
				}
				if baseline <= 0 || sent/baseline < exfilMinRatio {
					continue
				}
				ratio := formatRatio(sent / baseline)
				took := formatSpan(span(transfer))

				signal := api.Signal{
					Name:           SignalBytesOut,
					Description:    fmt.Sprintf("Outbound bytes %s %s median hour in this file", ratio, whose),
					Weight:         WeightBytesOut,
					Layer:          api.SignalLayerBaselines,
					Observed:       FormatBytes(sent),
					ObservedDetail: ptr("in " + took),
					BaselineText:   ptr(FormatBytes(baseline)),
					BaselineDetail: ptr(detail),
					BaselineSource: api.BaselineSourceInFile,
					Value:          ptr(sent),
					Baseline:       ptr(math.Round(baseline)),
					Unit:           ptr("bytes"),
					EventIds:       eventIDs(transfer),
				}
				reason := fmt.Sprintf("%s sent %s to %s in %s, %s %s median hour.",
					user, FormatBytes(sent), host, took, ratio, whose)
				out = append(out, anomaly(api.DetectorTypeExfilVolume, transfer, reason,
					"bytes out "+ratio+" baseline", signal))
			}
		}
	}
	return out
}

// beaconing flags requests from one user to one host that keep a steady
// beat. A fast beat scores high. A slow one scores low: plenty of honest
// software polls every few minutes.
func beaconing(p *profile) []model.Anomaly {
	var out []model.Anomaly
	for _, user := range p.users {
		byHost := map[string][]model.Event{}
		var hosts []string
		for _, e := range p.byUser[user] {
			if byHost[e.Host] == nil {
				hosts = append(hosts, e.Host)
			}
			byHost[e.Host] = append(byHost[e.Host], e)
		}
		for _, host := range hosts {
			calls := byHost[host]
			if len(calls) < beaconMinRequests {
				continue
			}
			intervals := make([]float64, len(calls)-1)
			for i := 1; i < len(calls); i++ {
				intervals[i-1] = calls[i].TS.Sub(calls[i-1].TS).Seconds()
			}
			beat := median(intervals)
			if beat < 1 {
				continue // bursts of same-second requests have no rhythm
			}
			onBeat := 0
			for _, gap := range intervals {
				if math.Abs(gap-beat) <= beaconTolerance*beat {
					onBeat++
				}
			}
			share := float64(onBeat) / float64(len(intervals))
			if share < beaconMinOnBeat {
				continue
			}

			every := formatSpan(time.Duration(beat * float64(time.Second)))
			lasted := span(calls)
			fast := time.Duration(beat*float64(time.Second)) <= beaconFast
			weight := WeightRegularSlow
			if fast {
				weight = WeightRegularFast
			}
			signals := []api.Signal{{
				Name:           SignalRegularInterval,
				Description:    fmt.Sprintf("Requests to %s every %s, %.0f%% of them on the beat", host, every, share*100),
				Weight:         weight,
				Layer:          api.SignalLayerBaselines,
				Observed:       "every " + every,
				ObservedDetail: ptr(fmt.Sprintf("%s over %s", plural(len(calls), "request"), formatSpan(lasted))),
				BaselineText:   ptr(fmt.Sprintf("%.0f%% on beat", beaconMinOnBeat*100)),
				BaselineDetail: ptr("the fixed limit; people do not browse on a timer"),
				BaselineSource: api.BaselineSourceInFile,
				Value:          ptr(math.Round(beat)),
				Unit:           ptr("seconds"),
				EventIds:       eventIDs(calls),
			}}
			if fast && lasted >= beaconSustained {
				signals = append(signals, api.Signal{
					Name:           SignalSustained,
					Description:    fmt.Sprintf("The rhythm held for %s", formatSpan(lasted)),
					Weight:         WeightSustained,
					Layer:          api.SignalLayerRules,
					Observed:       formatSpan(lasted),
					ObservedDetail: ptr(plural(len(calls), "request")),
					BaselineText:   ptr(formatSpan(beaconSustained)),
					BaselineDetail: ptr("the fixed limit"),
					BaselineSource: api.BaselineSourceNone,
					Value:          ptr(math.Round(lasted.Seconds())),
					Baseline:       ptr(beaconSustained.Seconds()),
					Unit:           ptr("seconds"),
					EventIds:       eventIDs(calls),
				})
			}
			reason := fmt.Sprintf("%s made %s to %s over %s, one about every %s.",
				user, plural(len(calls), "request"), host, formatSpan(lasted), every)
			out = append(out, anomaly(api.DetectorTypeBeaconing, calls, reason, "request every "+every, signals...))
		}
	}
	return out
}

// rareDestinations flags a host that exactly one user reached. On its own
// it is weak; it matters when it backs up something else.
func rareDestinations(p *profile) []model.Anomaly {
	if len(p.users) < rareMinUsers {
		return nil
	}
	var out []model.Anomaly
	for _, user := range p.users {
		byHost := map[string][]model.Event{}
		var hosts []string
		for _, e := range p.byUser[user] {
			if p.hostUsers[e.Host] != 1 {
				continue
			}
			if byHost[e.Host] == nil {
				hosts = append(hosts, e.Host)
			}
			byHost[e.Host] = append(byHost[e.Host], e)
		}
		for _, host := range hosts {
			visits := byHost[host]
			signal := api.Signal{
				Name:           SignalRareDestination,
				Description:    fmt.Sprintf("Only %s reached %s in this file", user, host),
				Weight:         WeightRareDestination,
				Layer:          api.SignalLayerBaselines,
				Observed:       "1 user",
				ObservedDetail: ptr(plural(len(visits), "request")),
				BaselineText:   ptr(plural(len(p.users), "user")),
				BaselineDetail: ptr("in this file"),
				BaselineSource: api.BaselineSourceInFile,
				Value:          ptr(1.0),
				Baseline:       ptr(float64(len(p.users))),
				Unit:           ptr("users"),
				EventIds:       eventIDs(visits),
			}
			reason := fmt.Sprintf("%s is the only one of %d users who reached %s (%s).",
				user, len(p.users), host, plural(len(visits), "request"))
			out = append(out, anomaly(api.DetectorTypeRareDestination, visits, reason, "host only this user reached", signal))
		}
	}
	return out
}

// newSourceIPs flags requests that carry a user's name but come from an
// address that user almost never uses.
func newSourceIPs(p *profile) []model.Anomaly {
	var out []model.Anomaly
	for _, user := range p.users {
		events := p.byUser[user]
		counts := map[string]int{}
		for _, e := range events {
			counts[e.SrcIP]++
		}
		if len(counts) < 2 {
			continue
		}
		home := topKey(counts)
		if float64(counts[home]) < newIPHomeShare*float64(len(events)) {
			continue // no clear home address, so nothing to compare against
		}
		var addresses []string
		for ip := range counts {
			if ip != home {
				addresses = append(addresses, ip)
			}
		}
		sort.Strings(addresses)
		for _, ip := range addresses {
			var from []model.Event
			for _, e := range events {
				if e.SrcIP == ip {
					from = append(from, e)
				}
			}
			signal := api.Signal{
				Name:           SignalNewSourceIP,
				Description:    fmt.Sprintf("Requests for %s came from %s, an address this user rarely uses", user, ip),
				Weight:         WeightNewSourceIP,
				Layer:          api.SignalLayerBaselines,
				Observed:       ip,
				ObservedDetail: ptr(fmt.Sprintf("%s of %s requests", FormatCount(len(from)), FormatCount(len(events)))),
				BaselineText:   ptr(home),
				BaselineDetail: ptr(fmt.Sprintf("this user's usual address, %s requests in this file", FormatCount(counts[home]))),
				BaselineSource: api.BaselineSourceInFile,
				Value:          ptr(float64(len(from))),
				Baseline:       ptr(float64(counts[home])),
				Unit:           ptr("requests"),
				EventIds:       eventIDs(from),
			}
			reason := fmt.Sprintf("%s of %s's %s requests came from %s; the rest came from %s.",
				FormatCount(len(from)), user, FormatCount(len(events)), ip, home)
			out = append(out, anomaly(api.DetectorTypeNewSourceIp, from, reason, "unusual source address", signal))
		}
	}
	return out
}

// offHours flags a user who sends a lot of data during the hours when the
// whole company is quiet. Small overnight requests are background noise
// and are left out.
func offHours(p *profile) []model.Anomaly {
	quietHours := describeQuietHours(p.quiet)
	if quietHours == "" {
		return nil // the file has no quiet hours, so there is no baseline
	}
	var out []model.Anomaly
	for _, user := range p.users {
		// Outbound bytes per quiet clock hour.
		perHour := map[time.Time]float64{}
		for _, e := range p.byUser[user] {
			if p.quiet[e.TS.Hour()] {
				perHour[e.TS.Truncate(time.Hour)] += float64(e.BytesOut)
			}
		}
		var transfers []model.Event
		var sent float64
		for _, e := range p.byUser[user] {
			if perHour[e.TS.Truncate(time.Hour)] >= offHoursMinBytes && e.BytesOut >= offHoursMinEventSize {
				transfers = append(transfers, e)
				sent += float64(e.BytesOut)
			}
		}
		if len(transfers) == 0 {
			continue
		}
		from, to := transfers[0].TS.Format("15:04"), transfers[len(transfers)-1].TS.Format("15:04")
		signal := api.Signal{
			Name:           SignalOffHours,
			Description:    fmt.Sprintf("%s sent during the company's quiet hours", FormatBytes(sent)),
			Weight:         WeightOffHours,
			Layer:          api.SignalLayerBaselines,
			Observed:       FormatBytes(sent),
			ObservedDetail: ptr(fmt.Sprintf("%s–%s UTC", from, to)),
			BaselineText:   ptr(quietHours),
			BaselineDetail: ptr(fmt.Sprintf("quiet hours: the typical user is under %.0f%% of their peak", quietShare*100)),
			BaselineSource: api.BaselineSourceInFile,
			Value:          ptr(sent),
			Baseline:       ptr(float64(offHoursMinBytes)),
			Unit:           ptr("bytes"),
			EventIds:       eventIDs(transfers),
		}
		reason := fmt.Sprintf("%s sent %s between %s and %s UTC, when the company is quiet.", user, FormatBytes(sent), from, to)
		out = append(out, anomaly(api.DetectorTypeOffHours, transfers, reason, "large transfer in quiet hours", signal))
	}
	return out
}

// describeQuietHours writes the quiet hours as ranges: "03:00–11:00 UTC".
func describeQuietHours(quiet [24]bool) string {
	var ranges []string
	for hour := 0; hour < 24; {
		if !quiet[hour] {
			hour++
			continue
		}
		end := hour
		for end < 24 && quiet[end] {
			end++
		}
		ranges = append(ranges, fmt.Sprintf("%02d:00–%02d:00", hour, end%24))
		hour = end
	}
	if len(ranges) == 0 {
		return ""
	}
	text := ranges[0]
	for _, r := range ranges[1:] {
		text += ", " + r
	}
	return text + " UTC"
}

// topKey returns the key with the highest count. Ties go to the key that
// sorts first, so the answer never depends on map order.
func topKey(counts map[string]int) string {
	best := ""
	for key, n := range counts {
		if best == "" || n > counts[best] || (n == counts[best] && key < best) {
			best = key
		}
	}
	return best
}
