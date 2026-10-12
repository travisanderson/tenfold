package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"time"
)

// script is every name and number the scenarios use. The demo profiles play
// demoScript, which never changes. The holdout profile draws its own.
type script struct {
	backup  backupPlan
	push    pushPlan
	updater updaterPlan
	exfil   exfilPlan
	beacon  beaconPlan
	phish   phishPlan
	burst   burstPlan
}

// Every start is an Eastern time of day, measured from midnight. Every
// jitter is in seconds, either way. A note describes what a holdout drew.

type backupPlan struct {
	host, app, ua string
	start         time.Duration
	chunks        int
	chunkBytes    float64
	gap           time.Duration
	jitter        int
	note          string
}

type pushPlan struct {
	user, repo string
	start      time.Duration
	pushes     int
	chunkBytes float64
	gap        time.Duration
	jitter     int
	note       string
}

type updaterPlan struct {
	host, ua, path string
	every          time.Duration
	note           string
	drawn          map[string]float64
}

type exfilPlan struct {
	user, ip, host, ua    string
	putPath, statusPath   string
	start                 time.Duration
	uploads               int
	uploadBytes           float64
	gap                   time.Duration
	jitter                int
	probeRisk, uploadRisk int
	note                  string
}

type beaconPlan struct {
	user, host, ua, path string
	start                time.Duration
	every                time.Duration
	jitter               int
	calls                int
	risk                 int
	note                 string
	drawn                map[string]float64
}

type click struct {
	sec  int
	path string
}

type phishPlan struct {
	user, host, threat string
	start              time.Duration
	clicks             []click
	note               string
}

type burstPlan struct {
	user   string
	hosts  []string
	start  time.Duration
	tries  int
	gap    time.Duration
	jitter int
	note   string
}

func clock(hour, min, sec int) time.Duration {
	return time.Duration(hour)*time.Hour + time.Duration(min)*time.Minute + time.Duration(sec)*time.Second
}

// demoScript is the fixed story the demo samples tell. The mockups, the
// README and the backend's tests all name its users, hosts and times.
func demoScript() script {
	return script{
		backup: backupPlan{
			host: "s3.us-west.backblazeb2.com", app: "Backblaze B2", ua: "rclone/v1.65.0",
			start: clock(1, 0, 0), chunks: 250, chunkBytes: 8 << 20, gap: 24 * time.Second, jitter: 3,
		},
		push: pushPlan{
			user: "ravi.p", repo: "platform",
			start: clock(14, 31, 0), pushes: 40, chunkBytes: 7.5 * (1 << 20), gap: 9 * time.Second, jitter: 2,
		},
		updater: updaterPlan{
			host: "update.vendor-agent.example", ua: "VendorAgent/4.2.1", path: "/v2/check?ver=4.2.1",
			every: 15 * time.Minute,
		},
		exfil: exfilPlan{
			user: "dana.k", ip: "10.20.99.47", host: "files-sync-cdn.net", ua: "curl/8.4.0",
			putPath: "/api/v2/put", statusPath: "/api/v2/status",
			start: clock(2, 13, 52), uploads: 19, uploadBytes: 2218500, gap: 77 * time.Second, jitter: 6,
			probeRisk: 61, uploadRisk: 78,
		},
		beacon: beaconPlan{
			user: "marcus.l", host: "update-chk-svc.io", path: "/c?id=7f3a91c2",
			ua:    "Mozilla/4.0 (compatible; MSIE 8.0; Windows NT 6.1; Trident/4.0)",
			start: clock(9, 12, 0), every: time.Minute, jitter: 2, calls: 181, risk: 45,
		},
		phish: phishPlan{
			user: "priya.s", host: "secure-login-verify.net", threat: "Phish.Generic",
			start: clock(14, 37, 0),
			clicks: []click{
				{2, "/o365/signin?id=4c1e9a"},
				{5, "/o365/signin?id=4c1e9a"},
				{9, "/favicon.ico"},
				{21, "/o365/signin?id=4c1e9a"},
			},
		},
		burst: burstPlan{
			user:  "ben.t",
			hosts: []string{"freeproxy-tunnel.example", "hide-route.example", "anon-surf.example", "vpn-quickgate.example"},
			start: clock(16, 5, 0), tries: 80, gap: 3 * time.Second, jitter: 1,
		},
	}
}

// ---------- The holdout ----------

const (
	mb = 1 << 20
	gb = 1 << 30
)

// Words for invented hosts. They are plain on purpose: a name should not
// tell a reader whether its traffic is an attack.
var (
	hostWords = []string{
		"cdn", "sync", "files", "static", "assets", "metrics", "update", "edge", "cloud",
		"data", "media", "relay", "hub", "node", "cache", "store", "link", "core", "stack", "pulse",
	}
	lureWords  = []string{"secure", "login", "verify", "account", "auth", "portal", "signin", "sso", "docs", "share"}
	proxyWords = []string{"proxy", "vpn", "tunnel", "hide", "anon", "surf", "unblock", "free", "route", "gate", "mask", "quick"}
)

var backupStores = []struct{ host, app string }{
	{"s3.us-west.backblazeb2.com", "Backblaze B2"},
	{"s3.us-east-1.amazonaws.com", "Amazon S3"},
	{"s3.wasabisys.com", "Wasabi"},
	{"northwindlabs.blob.core.windows.net", "Azure Blob Storage"},
}

var (
	backupTools = []string{"rclone/v1.65.0", "restic/0.16.4", "Veeam/12.1", "aws-sdk-go/1.55.5 (go1.22; linux; amd64)"}
	repos       = []string{"platform", "web", "mobile", "infra", "data-pipeline"}
	vendors     = []string{"FleetDesk", "PatchWorks", "AssetWatch", "EndpointShield", "DeskMonitor"}
	exfilTools  = []string{"curl/8.4.0", "python-requests/2.32.3", "Go-http-client/2.0", "rclone/v1.67.0", browserUAs[0]}
	exfilPaths  = []string{"/api/v2/put", "/upload", "/api/v1/objects", "/sync/chunk", "/dav/files"}
	beaconTools = []string{
		"Mozilla/4.0 (compatible; MSIE 8.0; Windows NT 6.1; Trident/4.0)",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0.0.0 Safari/537.36",
		"python-requests/2.31.0",
		"WinHTTP/1.1",
	}
	beaconPaths = []string{"/c?id=%08x", "/api/ping?h=%08x", "/status/%08x", "/v1/poll?s=%08x"}
	threats     = []string{"Phish.Generic", "HTML.Phish.Microsoft", "JS.Phish.Credential", "Phish.O365.Login"}
	lurePaths   = []string{"/o365/signin", "/okta/login", "/account/verify", "/sharepoint/view", "/secure/document"}
	updateTimes = []time.Duration{5 * time.Minute, 10 * time.Minute, 15 * time.Minute, 20 * time.Minute, 30 * time.Minute, time.Hour}
)

// holdoutScript draws a script: the same seven scenarios, with their users,
// times, sizes, hosts and intervals chosen by the seed.
//
// The ranges were chosen for what such traffic looks like in practice, and
// set down before any detector read a holdout file. They are wide on
// purpose. Do not narrow one because a detector misses inside it: that
// would turn the holdout into one more sample the detectors were built for.
func holdoutScript(rng *rand.Rand, users []user) script {
	var engineers, people []string
	for _, u := range users {
		if u.service {
			continue
		}
		people = append(people, u.name)
		if u.dept == "Engineering" {
			engineers = append(engineers, u.name)
		}
	}
	// Each scenario gets its own person, so the answer key stays clean.
	pusher := oneOf(rng, engineers)
	rng.Shuffle(len(people), func(i, j int) { people[i], people[j] = people[j], people[i] })
	var cast []string
	for _, name := range people {
		if name != pusher {
			cast = append(cast, name)
		}
	}

	var s script

	// The backup: 0.5 to 8 GB, starting between 00:10 and 04:00, over 40
	// minutes to 3 hours.
	{
		store := oneOf(rng, backupStores)
		total := spread(rng, 0.5*gb, 8*gb)
		chunk := float64(oneOf(rng, []int{4, 8, 16, 32}) * mb)
		chunks := max(int(total/chunk), 8)
		lasts := span(rng, 40*time.Minute, 3*time.Hour)
		gap := max(lasts/time.Duration(chunks), 2*time.Second).Truncate(time.Second)
		s.backup = backupPlan{
			host: store.host, app: store.app, ua: oneOf(rng, backupTools),
			start: span(rng, clock(0, 10, 0), clock(4, 0, 0)), chunks: chunks, chunkBytes: chunk, gap: gap,
			jitter: min(3, seconds(gap)/4),
		}
		s.backup.note = fmt.Sprintf("%s sent in %d chunks, one every %s, from %s ET",
			size(chunk*float64(chunks)), chunks, gap, wall(s.backup.start))
	}

	// The code push: 50 MB to 1 GB during the working day.
	{
		const chunk = 7.5 * mb
		pushes := max(int(spread(rng, 50*mb, 1*gb)/chunk), 3)
		gap := span(rng, 3*time.Second, 15*time.Second)
		s.push = pushPlan{
			user: pusher, repo: oneOf(rng, repos),
			start: span(rng, clock(9, 0, 0), clock(17, 0, 0)), pushes: pushes, chunkBytes: chunk, gap: gap,
			jitter: min(2, seconds(gap)/3),
		}
		s.push.note = fmt.Sprintf("%s pushed in %d requests, one every %s, from %s ET",
			size(chunk*float64(pushes)), pushes, gap, wall(s.push.start))
	}

	// The update agent: every workstation polls every 5 to 60 minutes.
	{
		vendor := oneOf(rng, vendors)
		version := fmt.Sprintf("%d.%d.%d", 1+rng.IntN(9), rng.IntN(10), rng.IntN(10))
		s.updater = updaterPlan{
			host: "update." + strings.ToLower(vendor) + ".example", ua: vendor + "Agent/" + version,
			path: "/v2/check?ver=" + version, every: oneOf(rng, updateTimes),
		}
		s.updater.note = fmt.Sprintf("every workstation polls every %s", s.updater.every)
		s.updater.drawn = map[string]float64{"everySeconds": s.updater.every.Seconds()}
	}

	// The exfiltration: 5 to 500 MB in 5 to 200 uploads, 5 seconds to 2
	// minutes apart, at any hour.
	{
		total := spread(rng, 5*mb, 500*mb)
		uploads := int(spread(rng, 5, 200))
		gap := span(rng, 5*time.Second, 2*time.Minute)
		lasts := exfilLead + time.Duration(uploads)*gap + time.Minute
		s.exfil = exfilPlan{
			user: cast[0], host: inventHost(rng, hostWords), ua: oneOf(rng, exfilTools),
			// Staff addresses are all 10.20.x.x, so this one is new to the account.
			ip:      fmt.Sprintf("10.%d.%d.%d", 21+rng.IntN(200), rng.IntN(255), 2+rng.IntN(250)),
			putPath: oneOf(rng, exfilPaths), statusPath: "/status",
			start: span(rng, 0, 24*time.Hour-lasts-time.Minute), uploads: uploads,
			uploadBytes: total / float64(uploads), gap: gap, jitter: seconds(gap) / 12,
			probeRisk: 30 + rng.IntN(40), uploadRisk: 40 + rng.IntN(46),
		}
		s.exfil.note = fmt.Sprintf("%s sent in %d uploads, one every %s, from %s ET, with %s",
			size(total), uploads, gap, wall(s.exfil.start), s.exfil.ua)
	}

	// The beacon: one call every 30 seconds to 30 minutes, up to a quarter
	// of that off the beat, for 2 to 10 hours, at any hour.
	{
		every := time.Duration(spread(rng, 30, 1800)) * time.Second
		jitter := int(float64(seconds(every)) * 0.25 * rng.Float64())
		lasts := span(rng, 2*time.Hour, 10*time.Hour)
		slack := time.Duration(jitter)*time.Second + time.Minute
		s.beacon = beaconPlan{
			user: cast[1], host: inventHost(rng, hostWords), ua: oneOf(rng, beaconTools),
			path:  fmt.Sprintf(oneOf(rng, beaconPaths), rng.Uint32()),
			start: span(rng, slack, 24*time.Hour-lasts-slack), every: every, jitter: jitter,
			calls: int(lasts/every) + 1, risk: 25 + rng.IntN(36),
		}
		s.beacon.note = fmt.Sprintf("%d calls, one every %s give or take %ds, for %s from %s ET",
			s.beacon.calls, every, jitter, lasts.Round(time.Minute), wall(s.beacon.start))
		s.beacon.drawn = map[string]float64{
			"calls": float64(s.beacon.calls), "everySeconds": every.Seconds(), "jitterSeconds": float64(jitter),
		}
	}

	// The phishing click: 1 to 6 blocked requests between 07:30 and 18:30.
	{
		lure := oneOf(rng, lurePaths) + fmt.Sprintf("?id=%06x", rng.IntN(1<<24))
		s.phish = phishPlan{
			user: cast[2], host: inventHost(rng, lureWords), threat: oneOf(rng, threats),
			start: span(rng, clock(7, 30, 0), clock(18, 30, 0)),
		}
		sec := 0
		for i := range 1 + rng.IntN(6) {
			sec += 1 + rng.IntN(12)
			path := lure
			if i > 0 && rng.IntN(4) == 0 {
				path = "/favicon.ico"
			}
			s.phish.clicks = append(s.phish.clicks, click{sec, path})
		}
		s.phish.note = fmt.Sprintf("%d blocked requests at %s ET, named %s",
			len(s.phish.clicks), wall(s.phish.start), s.phish.threat)
	}

	// The burst: 20 to 200 blocked tries at 2 to 8 anonymizer sites, 1 to 15
	// seconds apart, between 08:00 and 18:00.
	{
		gap := span(rng, time.Second, 15*time.Second)
		s.burst = burstPlan{
			user: cast[3], start: span(rng, clock(8, 0, 0), clock(18, 0, 0)),
			tries: int(spread(rng, 20, 200)), gap: gap, jitter: seconds(gap) / 3,
		}
		for sites := 2 + rng.IntN(7); len(s.burst.hosts) < sites; {
			s.burst.hosts = appendNew(s.burst.hosts, inventHost(rng, proxyWords))
		}
		s.burst.note = fmt.Sprintf("%d blocked tries at %d sites, one every %s, from %s ET",
			s.burst.tries, len(s.burst.hosts), gap, wall(s.burst.start))
	}
	return s
}

// spread returns a value between lo and hi, as likely to fall between 1
// and 10 as between 10 and 100.
func spread(rng *rand.Rand, lo, hi float64) float64 {
	return lo * math.Pow(hi/lo, rng.Float64())
}

// span returns a whole number of seconds between lo and hi.
func span(rng *rand.Rand, lo, hi time.Duration) time.Duration {
	return (lo + time.Duration(rng.Int64N(int64(hi-lo)+1))).Truncate(time.Second)
}

func oneOf[T any](rng *rand.Rand, list []T) T { return list[rng.IntN(len(list))] }

func seconds(d time.Duration) int { return int(d / time.Second) }

// inventHost joins two or three of the words into a host under .example,
// which can never belong to a real business.
func inventHost(rng *rand.Rand, words []string) string {
	var parts []string
	for count := 2 + rng.IntN(2); len(parts) < count; {
		parts = appendNew(parts, oneOf(rng, words))
	}
	return strings.Join(parts, "-") + ".example"
}

func appendNew(list []string, s string) []string {
	for _, have := range list {
		if have == s {
			return list
		}
	}
	return append(list, s)
}

func size(bytes float64) string {
	if bytes >= gb {
		return fmt.Sprintf("%.1f GB", bytes/gb)
	}
	return fmt.Sprintf("%.0f MB", bytes/mb)
}

// wall writes a time of day as 14:05.
func wall(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d", int(d/time.Hour), int(d/time.Minute)%60)
}
