# Samples

Synthetic web-proxy logs for a fictional company, Northwind Labs, and the
generator that writes them. This folder stands alone. The generator is in
version control; the logs are not, because it rebuilds every file from a
seed.

```sh
cd samples
go run ./loggen --all                              # all three profiles
go run ./loggen --profile attack-day --seed 42     # one profile
go test ./...
```

Or, from the repo root: `make samples`.

| Sample | Lines | Contents |
| --- | --- | --- |
| `clean-day` | 30,000 | Normal traffic and decoys, no attacks. Tests the false-positive rate. |
| `attack-day` | 48,002 | Normal traffic, decoys, and four attacks. The main demo. |
| `large-100k` | 100,002 | About 60 users, same scenarios. The performance demo. |

Each profile writes `<name>.log` and `<name>.answers.json`. The same `--seed`
always gives byte-for-byte identical files.

## Format

Modeled on Zscaler NSS web logs: 21 `key=value` fields separated by tabs,
always in the same order, with `None` for a missing value. It is a subset of
Zscaler's fields, built for a synthetic dataset. No real customer data is
involved, and every attack domain is invented.

About 0.1% of lines are broken on purpose. The answer key lists them under
`malformedLines`; a parser must count and skip them.

`loggen/line.go` is a copy of `backend/internal/zscaler/line.go`. The
backend's copy is the real parser, and its tests read the files in this
folder when they exist. Change both together.

## Scenarios

| ID | Label | What happens |
| --- | --- | --- |
| `atk-exfil` | malicious | `dana.k`, 02:14 ET: a sign-in from a new address, then 41 MB to a first-seen domain |
| `atk-beacon` | malicious | `WS-114` calls one host every 60 seconds for three hours |
| `atk-phish` | malicious | `priya.s` follows a link at 14:37 ET; the proxy blocks it as `Phish.Generic` |
| `atk-burst` | malicious | `ben.t` tries 80 anonymizer sites in four minutes; all blocked |
| `decoy-backup` | benign | `svc-backup` sends 2 GB to online storage, 01:00 to 02:40 ET |
| `decoy-dev-push` | benign | `ravi.p` pushes 300 MB to GitHub mid-afternoon |
| `decoy-updater` | benign | An agent polls its vendor every 15 minutes from every workstation |

## Holdout

The demo samples replay one fixed script, and the detectors were built
against it. The `holdout` profile plays the same seven scenarios with their
users, times, sizes, hosts and intervals drawn from the seed, so it shows
how the detectors do on a day they have not seen.

```sh
make holdout                                                    # from the repo root
cd backend && go test ./internal/detect -run TestHoldout -v     # the scorecard
```

It writes to `samples/holdout/`, apart from the demos, and `--all` leaves it
out. Each scenario's `note` in the answer key says what was drawn. The
ranges are in `loggen/script.go`. Do not narrow a range, or change a
detector, to turn a miss into a catch: report the miss.

## Scoring rule

A scenario counts as *caught* if some incident's evidence covers at least 50%
of that scenario's `lineNos`. An incident that matches no malicious scenario
is a *false positive*, and so is a decoy that surfaces as an incident.
