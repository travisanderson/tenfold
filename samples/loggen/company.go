package main

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
)

// The fictional company. Everything here is invented; the .example domain
// can never belong to a real business.
const (
	companyDomain = "northwindlabs.example"
	backupServer  = "SRV-BACKUP-01"
)

var departments = []string{"Engineering", "Finance", "Sales", "Marketing", "HR", "IT"}

// user is one employee with a stable device, address and browsing profile.
type user struct {
	name   string
	dept   string
	device string
	ip     string
	ua     string

	volume     float64   // share of the day's traffic
	shiftMin   int       // personal offset of the working day, in minutes
	uploadMult float64   // scales this user's upload sizes
	destCum    []float64 // cumulative weights over catalog
	service    bool      // a service account: no browsing of its own
}

func (u user) login() string { return u.name + "@" + companyDomain }

// The 25 named staff. The scenarios rely on dana.k, marcus.l, priya.s,
// ben.t, ravi.p and svc-backup.
var roster = []struct{ name, dept string }{
	{"marcus.l", "Engineering"}, {"ravi.p", "Engineering"}, {"elena.v", "Engineering"},
	{"tom.h", "Engineering"}, {"aisha.n", "Engineering"}, {"lucas.m", "Engineering"},
	{"wei.z", "Engineering"},
	{"dana.k", "Finance"}, {"omar.f", "Finance"}, {"grace.y", "Finance"}, {"henrik.s", "Finance"},
	{"ben.t", "Sales"}, {"carla.d", "Sales"}, {"james.o", "Sales"}, {"nina.w", "Sales"},
	{"victor.a", "Sales"},
	{"priya.s", "Marketing"}, {"sofia.r", "Marketing"}, {"daniel.c", "Marketing"},
	{"mei.t", "Marketing"},
	{"laura.b", "HR"}, {"kofi.a", "HR"},
	{"ian.g", "IT"}, {"zoe.p", "IT"},
	{"svc-backup", "IT"},
}

// Devices the mockups and scenarios name. Everyone else gets the next free
// WS number.
var pinnedDevices = map[string]string{
	"marcus.l":   "WS-114",
	"priya.s":    "WS-107",
	"dana.k":     "LT-022",
	"svc-backup": backupServer,
}

// dana.k's upload sizes are fixed, not drawn, so that her median working
// hour sends about 1.3 MB and the planted 40 MB stands roughly 30 times
// above it.
var pinnedUploadMult = map[string]float64{"dana.k": 2.6}

var extraFirstNames = []string{
	"alex", "bella", "chris", "diana", "ethan", "fiona", "george", "hana",
	"ivan", "julia", "kevin", "lena", "mateo", "nora", "oscar", "paula",
	"quinn", "rosa", "stefan", "tara", "umar", "vera", "will", "xena",
	"yusuf", "zara", "adam", "beth", "cole", "dora", "emil", "faye",
	"gus", "hope", "igor", "jade",
}

var browserUAs = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36 Edg/129.0.0.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:131.0) Gecko/20100101 Firefox/131.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36",
}

// buildUsers returns n users: the named roster first, then generated staff.
// The demo script pins one user's upload sizes; a holdout pins nothing.
func buildUsers(rng *rand.Rand, n int, demo bool) []user {
	users := make([]user, 0, n)
	nextWS := 101
	perDept := map[string]int{}

	add := func(name, dept string) {
		u := user{name: name, dept: dept}
		if d, ok := pinnedDevices[name]; ok {
			u.device = d
		} else {
			for nextWS == 107 || nextWS == 114 {
				nextWS++
			}
			u.device = fmt.Sprintf("WS-%d", nextWS)
			nextWS++
		}
		perDept[dept]++
		u.ip = fmt.Sprintf("10.20.%d.%d", deptIndex(dept)+1, 10+perDept[dept])
		u.ua = browserUAs[rng.IntN(len(browserUAs))]
		u.volume = logNormal(rng, 0, 0.35)
		u.shiftMin = rng.IntN(91) - 45
		u.uploadMult = logNormal(rng, 0, 0.4)
		if m, ok := pinnedUploadMult[name]; ok && demo {
			u.uploadMult = m
		}
		u.service = name == "svc-backup"
		u.destCum = personalWeights(rng, dept)
		users = append(users, u)
	}

	for _, r := range roster {
		if len(users) == n {
			break
		}
		add(r.name, r.dept)
	}
	for i := 0; len(users) < n; i++ {
		first := extraFirstNames[i%len(extraFirstNames)]
		initial := string(rune('a' + (i*7+3)%26))
		add(first+"."+initial, departments[i%len(departments)])
	}
	return users
}

func deptIndex(dept string) int {
	for i, d := range departments {
		if d == dept {
			return i
		}
	}
	return 0
}

// dest is one place traffic goes, with the shape of a typical request.
type dest struct {
	host, cat, app, class string
	paths                 []string
	weight                float64 // popularity across the company
	favors                string  // a department that uses it far more, if any
	upload                float64 // share of requests that send data
	uploadMethod          string
	uploadMedian          float64 // median bytes sent by an upload
	respMedian            float64 // median bytes received by a GET
	risk                  int     // Zscaler's page risk, roughly
	blocked               bool
	reason                string
}

const (
	catOffice   = "Office 365"
	catUnknown  = "Miscellaneous or Unknown"
	reasonBlock = "Not allowed to browse this category"
)

// catalog is ordinary traffic. Real, well-known services are used only as
// benign destinations, under their true categories.
var catalog = []dest{
	{host: "login.microsoftonline.com", cat: catOffice, app: "Microsoft Login", class: "Business", paths: []string{"/common/oauth2/v2.0/authorize", "/common/oauth2/v2.0/token"}, weight: 3, upload: 0.4, uploadMethod: "POST", uploadMedian: 2100, respMedian: 9000, risk: 8},
	{host: "outlook.office365.com", cat: catOffice, app: "Outlook", class: "Business", paths: []string{"/owa/", "/owa/service.svc", "/mapi/emsmdb/"}, weight: 14, upload: 0.12, uploadMethod: "POST", uploadMedian: 9000, respMedian: 22000, risk: 6},
	{host: "teams.microsoft.com", cat: catOffice, app: "Microsoft Teams", class: "Business", paths: []string{"/api/chatsvc/", "/api/mt/", "/v2/"}, weight: 11, upload: 0.15, uploadMethod: "POST", uploadMedian: 3500, respMedian: 12000, risk: 6},
	{host: "northwindlabs.sharepoint.com", cat: catOffice, app: "SharePoint", class: "Business", paths: []string{"/sites/finance/", "/sites/all-hands/", "/_api/web/"}, weight: 5, upload: 0.1, uploadMethod: "PUT", uploadMedian: 180000, respMedian: 30000, risk: 5},
	{host: "app.slack.com", cat: "Professional Services", app: "Slack", class: "Business", paths: []string{"/api/conversations.history", "/api/chat.postMessage", "/api/client.counts"}, weight: 12, upload: 0.2, uploadMethod: "POST", uploadMedian: 1800, respMedian: 6000, risk: 6},
	{host: "mail.google.com", cat: "Web-based Email", app: "Gmail", class: "Business", paths: []string{"/mail/u/0/", "/sync/u/0/i/s"}, weight: 4, upload: 0.1, uploadMethod: "POST", uploadMedian: 6000, respMedian: 18000, risk: 8},
	{host: "docs.google.com", cat: "Professional Services", app: "Google Docs", class: "Business", paths: []string{"/document/d/", "/spreadsheets/d/", "/presentation/d/"}, weight: 6, upload: 0.25, uploadMethod: "POST", uploadMedian: 2600, respMedian: 26000, risk: 6},
	{host: "drive.google.com", cat: "Online Storage", app: "Google Drive", class: "Business", paths: []string{"/drive/my-drive", "/upload/drive/v3/files"}, weight: 3, upload: 0.12, uploadMethod: "POST", uploadMedian: 220000, respMedian: 20000, risk: 10},
	{host: "github.com", cat: "Professional Services", app: "GitHub", class: "Business", paths: []string{"/northwindlabs-example/platform", "/northwindlabs-example/platform/pulls", "/northwindlabs-example/web/issues"}, weight: 6, favors: "Engineering", upload: 0.06, uploadMethod: "POST", uploadMedian: 14000, respMedian: 40000, risk: 8},
	{host: "api.github.com", cat: "Professional Services", app: "GitHub", class: "Business", paths: []string{"/repos/northwindlabs-example/platform", "/graphql", "/notifications"}, weight: 4, favors: "Engineering", upload: 0.2, uploadMethod: "POST", uploadMedian: 1500, respMedian: 8000, risk: 8},
	{host: "stackoverflow.com", cat: "Professional Services", app: "General Browsing", class: "General Browsing", paths: []string{"/questions/", "/search"}, weight: 2, favors: "Engineering", respMedian: 45000, risk: 10},
	{host: "northwindlabs.my.salesforce.com", cat: "Corporate Marketing", app: "Salesforce", class: "Business", paths: []string{"/lightning/o/Opportunity/list", "/aura", "/services/data/v61.0/query"}, weight: 6, favors: "Sales", upload: 0.3, uploadMethod: "POST", uploadMedian: 3000, respMedian: 16000, risk: 6},
	{host: "app.hubspot.com", cat: "Corporate Marketing", app: "HubSpot", class: "Business", paths: []string{"/contacts/", "/email/", "/reports-dashboard/"}, weight: 4, favors: "Marketing", upload: 0.2, uploadMethod: "POST", uploadMedian: 2800, respMedian: 20000, risk: 8},
	{host: "wd5.myworkday.com", cat: "Professional Services", app: "Workday", class: "Business", paths: []string{"/northwindlabs/d/home.htmld", "/northwindlabs/d/task/"}, weight: 2, favors: "HR", upload: 0.2, uploadMethod: "POST", uploadMedian: 2400, respMedian: 24000, risk: 6},
	{host: "zoom.us", cat: "Professional Services", app: "Zoom", class: "Business", paths: []string{"/j/", "/wc/join/", "/api/v1/meeting"}, weight: 3, upload: 0.1, uploadMethod: "POST", uploadMedian: 1200, respMedian: 14000, risk: 8},
	{host: "www.linkedin.com", cat: "Professional Networking", app: "LinkedIn", class: "Social Networking", paths: []string{"/feed/", "/in/", "/jobs/"}, weight: 3, respMedian: 60000, risk: 12},
	{host: "www.nytimes.com", cat: "News and Media", app: "General Browsing", class: "General Browsing", paths: []string{"/", "/section/business", "/section/technology"}, weight: 2, respMedian: 90000, risk: 10},
	{host: "www.bbc.com", cat: "News and Media", app: "General Browsing", class: "General Browsing", paths: []string{"/news", "/news/business", "/sport"}, weight: 2, respMedian: 80000, risk: 10},
	{host: "www.reuters.com", cat: "News and Media", app: "General Browsing", class: "General Browsing", paths: []string{"/world/", "/markets/", "/technology/"}, weight: 1.5, respMedian: 70000, risk: 10},
	{host: "en.wikipedia.org", cat: "Reference Sites", app: "Wikipedia", class: "General Browsing", paths: []string{"/wiki/Main_Page", "/wiki/Special:Search", "/w/api.php"}, weight: 2, respMedian: 55000, risk: 5},
	{host: "www.google.com", cat: "Web Search", app: "Google Search", class: "General Browsing", paths: []string{"/search", "/complete/search"}, weight: 7, respMedian: 35000, risk: 5},
	{host: "cdn.jsdelivr.net", cat: "Content Delivery Networks", app: "General Browsing", class: "General Browsing", paths: []string{"/npm/", "/gh/"}, weight: 3, respMedian: 30000, risk: 12},
	{host: "fonts.gstatic.com", cat: "Content Delivery Networks", app: "General Browsing", class: "General Browsing", paths: []string{"/s/inter/v13/", "/s/roboto/v30/"}, weight: 3, respMedian: 24000, risk: 5},
	{host: "cdnjs.cloudflare.com", cat: "Content Delivery Networks", app: "General Browsing", class: "General Browsing", paths: []string{"/ajax/libs/"}, weight: 2, respMedian: 28000, risk: 12},
	// Routine policy blocks, so that "Blocked" alone is not suspicious.
	{host: "ad.doubleclick.net", cat: "Advertising", app: "General Browsing", class: "General Browsing", paths: []string{"/ddm/adj/", "/pagead/id"}, weight: 1.6, risk: 30, blocked: true, reason: reasonBlock},
	{host: "www.facebook.com", cat: "Social Networking", app: "Facebook", class: "Social Networking", paths: []string{"/", "/tr/"}, weight: 0.7, risk: 20, blocked: true, reason: reasonBlock},
	{host: "www.instagram.com", cat: "Social Networking", app: "Instagram", class: "Social Networking", paths: []string{"/"}, weight: 0.3, risk: 20, blocked: true, reason: reasonBlock},
	{host: "www.draftkings.com", cat: "Gambling", app: "General Browsing", class: "General Browsing", paths: []string{"/", "/lobby"}, weight: 0.1, risk: 40, blocked: true, reason: reasonBlock},
}

// background is machine noise that runs around the clock on every device.
var background = []struct {
	dest
	ua string
}{
	{dest{host: "ctldl.windowsupdate.com", cat: "Operating System and Software Updates", app: "Windows Update", class: "Business", paths: []string{"/msdownload/update/v3/static/trustedr/en/authrootstl.cab"}, respMedian: 60000, risk: 2}, "Microsoft-CryptoAPI/10.0"},
	{dest{host: "v10.events.data.microsoft.com", cat: "Operating System and Software Updates", app: "Microsoft Telemetry", class: "Business", paths: []string{"/OneCollector/1.0/"}, upload: 1, uploadMethod: "POST", uploadMedian: 2400, respMedian: 300, risk: 2}, "MSDW"},
	{dest{host: "wss-primary.slack.com", cat: "Professional Services", app: "Slack", class: "Business", paths: []string{"/websocket/keepalive"}, respMedian: 180, risk: 6}, "Slack/4.40.128 (Windows NT 10.0)"},
}

// personalWeights gives one user their own taste in destinations, as
// cumulative weights over catalog.
func personalWeights(rng *rand.Rand, dept string) []float64 {
	cum := make([]float64, len(catalog))
	total := 0.0
	for i, d := range catalog {
		w := d.weight * logNormal(rng, 0, 0.6)
		if d.favors != "" {
			if d.favors == dept {
				w *= 3
			} else {
				w *= 0.12
			}
		}
		total += w
		cum[i] = total
	}
	return cum
}

// serverIP gives each host a stable address in a documentation-reserved
// range, so no real address appears in the data.
func serverIP(host string) string {
	h := fnv.New32a()
	h.Write([]byte(host))
	sum := h.Sum32()
	ranges := []string{"192.0.2", "198.51.100", "203.0.113"}
	return fmt.Sprintf("%s.%d", ranges[sum%3], 1+(sum>>8)%254)
}

func logNormal(rng *rand.Rand, mu, sigma float64) float64 {
	return math.Exp(mu + sigma*rng.NormFloat64())
}

// around returns a value near median, log-normally spread.
func around(rng *rand.Rand, median, sigma float64) int64 {
	v := int64(median * logNormal(rng, 0, sigma))
	if v < 1 {
		v = 1
	}
	return v
}
