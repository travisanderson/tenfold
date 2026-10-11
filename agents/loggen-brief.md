# Brief: demo log generator (`tools/loggen`)

Build a small **Go** command-line tool that generates realistic, synthetic Zscaler web-proxy logs for a fictional company, with planted attacks and benign decoys, plus an **answer key** listing exactly which lines belong to each scenario.

These files drive everything downstream: the parser, detectors, demo samples, and the detection scorecard. Build this first.

---

## 1. Requirements

- **Deterministic:** `--seed` makes output byte-for-byte identical. Tests depend on this.
- **Sorted by time**, one event per line.
- **Fast:** 100k lines in a few seconds.
- **No real customer data.** Everything is invented.
- Output pairs: `samples/<name>.log` + `samples/<name>.answers.json`.

CLI:
```
go run ./tools/loggen --profile attack-day --seed 42 --out samples/
go run ./tools/loggen --all --out samples/     # writes all three profiles
```

## 2. Line format

Modeled on Zscaler NSS web-log custom feeds: `key=value` pairs, **separated by a single tab**. Values never contain tabs. Fields always appear in this order. Missing values are `None` (Zscaler's convention).

| Key | Type | Example |
| --- | --- | --- |
| `time` | RFC 3339 UTC | `2026-10-06T02:31:07Z` |
| `recordid` | int, increasing | `1078` |
| `login` | user email | `dana.k@northwind-labs.com` |
| `dept` | string | `Finance` |
| `cip` | client internal IP | `10.20.4.17` |
| `sip` | server IP | `203.0.113.45` |
| `host` | hostname | `files-sync-cdn.net` |
| `url` | full URL | `https://files-sync-cdn.net/api/v2/put` |
| `urlcat` | URL category | `Miscellaneous or Unknown` |
| `action` | `Allowed` or `Blocked` | `Allowed` |
| `reason` | policy reason | `Allowed` |
| `reqmethod` | HTTP method | `POST` |
| `respcode` | HTTP status | `200` |
| `reqsize` | bytes sent (outbound) | `4194304` |
| `respsize` | bytes received | `512` |
| `ua` | user agent | `Mozilla/5.0 (Windows NT 10.0; Win64; x64) ...` |
| `appname` | cloud app | `General Browsing` |
| `appclass` | app class | `General Browsing` |
| `devicehostname` | workstation | `WS-107` |
| `threatname` | threat or `None` | `Phish.Generic` |
| `riskscore` | 0–100 | `12` |

Use documentation-reserved IP ranges for public IPs (`192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24`) and `10.20.0.0/16` internally.

**Malformed lines:** about 0.1% of lines are deliberately broken (truncated, missing `time`, garbage). Record their line numbers in the answer key under `malformedLines`. The parser must count and skip them, not crash.

## 3. The fictional company

- **Northwind Labs**, domain `northwind-labs.com`.
- About **25 users** across Engineering, Finance, Sales, Marketing, HR, IT. Each has a stable workstation (`WS-101`–`WS-125`) and internal IP.
- Named users the scenarios need: `dana.k` (Finance), `marcus.l` (Engineering, `WS-114`), `priya.s` (Marketing), `ben.t` (Sales), plus a service account `svc-backup` (IT, host `SRV-BACKUP-01`).
- Each user gets a **personal profile**: typical daily volume, favorite sites, typical upload sizes. Statistics-based detectors need per-user baselines to be meaningful.

## 4. Normal traffic

- **Daily shape:** busy 08:00–18:00 local (ET), a lunch dip, light evening, sparse overnight. Overnight still has realistic background noise: OS updates, Slack keepalives, telemetry.
- **Destination catalog** (real, well-known SaaS is fine for benign traffic): Office 365, Outlook, Teams, Slack, Google Workspace, GitHub, Salesforce, Zoom, news sites, Wikipedia, CDNs. Each has a category, typical methods, and **log-normal** byte-size distributions.
- **Mostly GET**, with POST/PUT for SaaS uploads and form submissions.
- **Ordinary blocks:** a little routine policy blocking (ads, social media, gambling) so `Blocked` isn't automatically suspicious.

## 5. Benign decoys (label: `benign`)

These exist to produce *false positives*, which make the "teach it once" and scorecard demos meaningful.

| ID | Decoy | Looks like |
| --- | --- | --- |
| `decoy-backup` | `svc-backup` uploads ~2 GB in chunks to `backup.northwind-labs.com` at 01:00–01:40 nightly | Exfiltration (volume + off-hours) |
| `decoy-dev-push` | An engineer pushes a large repo to `github.com` mid-afternoon (~300 MB) | Volume spike |
| `decoy-updater` | A legitimate updater polls `update.vendor-agent.example` every 15 minutes, all day | Beaconing (but slow, and seen across many hosts) |

## 6. Planted attacks (label: `malicious`)

| ID | Scenario | Exact pattern | Intended detector | MITRE (verify) |
| --- | --- | --- | --- | --- |
| `atk-exfil` | Data exfiltration | `dana.k`, 02:14 login-like activity from her host, then 02:20–02:41 repeated POSTs to `files-sync-cdn.net` (uncategorized, first-seen), ~40 MB total (≈30× her daily baseline) | `exfil_volume` + `rare_destination` | T1567 |
| `atk-beacon` | Malware beaconing | `WS-114` (`marcus.l`) requests `update-chk-svc.io/c` every 60s ±2s for ~3 hours, small requests (~200–400 B), same user agent, starting mid-morning | `beaconing` | T1071.001 |
| `atk-phish` | Phishing click | `priya.s` clicks a link at ~10:42; request to `secure-login-verify.net` blocked with `threatname=Phish.Generic`, `urlcat=Phishing`, high `riskscore` | `threat_hit` | T1566 |
| `atk-burst` | Policy-evasion burst | `ben.t` makes ~80 requests in 4 minutes to anonymizer/proxy sites, all `Blocked` with category `Anonymizer` | `blocked_burst` | T1090 |

**Domain safety:** make sure the attack domains above are fictional and not real, live businesses. If in doubt, swap to an obviously invented name. A public demo shouldn't label a real site malicious.

## 7. Profiles

| Profile | Lines (≈) | Contents |
| --- | --- | --- |
| `clean-day` | 30,000 | Normal traffic + decoys, **no attacks** (tests false-positive rate) |
| `attack-day` | 48,000 | Normal traffic + decoys + all four attacks (the main demo) |
| `large-100k` | 100,000 | ~60 users, same scenarios (performance demo for the virtualized table and chart) |

Exact line counts come from the seed; record them in the answer key, and update the OpenAPI example values if they differ.

## 8. Answer key (`<name>.answers.json`)

```json
{
  "sample": "attack-day",
  "seed": 42,
  "generatorVersion": "1",
  "lineCount": 48212,
  "timeRange": { "start": "2026-10-06T04:00:00Z", "end": "2026-10-07T03:59:59Z" },
  "malformedLines": [512, 9003],
  "scenarios": [
    {
      "id": "atk-exfil",
      "label": "malicious",
      "type": "exfiltration",
      "expectedDetectors": ["exfil_volume", "rare_destination"],
      "mitre": ["T1567"],
      "users": ["dana.k"],
      "hosts": ["WS-103"],
      "domains": ["files-sync-cdn.net"],
      "timeRange": { "start": "...", "end": "..." },
      "lineNos": [31877, 31879, 31884]
    }
  ]
}
```

**Matching rule for the scorecard** (document it in the README): a scenario counts as *caught* if some incident's evidence covers **≥ 50%** of that scenario's `lineNos`. An incident that matches no malicious scenario is a *false positive*. A decoy that surfaces as an incident is a false positive too.

## 9. Tests

- **Determinism:** same seed → identical file hash.
- **Format:** every non-malformed line parses with the real parser; field order is stable.
- **Golden end-to-end** (once detectors exist): generate `attack-day` → parse → detect → group → assert all four attacks caught and decoys below an agreed threshold. This is your regression net when tuning detectors.

## 10. Out of scope

Other log formats, live streaming, real Zscaler field completeness. Note in the README that the format is **modeled on** Zscaler NSS web logs, with a subset of fields, for a synthetic dataset.
