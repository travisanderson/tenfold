# Tenfold API

The Go backend. It implements every operation in `../api/openapi.yaml`.

Connecting the frontend? Read `FRONTEND.md`.

## Run it

```sh
cp .env.example .env    # then fill in the three values
go run ./cmd/api        # http://localhost:8080, docs at /docs
```

At startup the API applies the database schema, creates the private `logs`
storage bucket if it is missing, and marks any job a previous process left
running as stopped, so it can be resumed.

## Database

Postgres on Supabase. The schema is plain SQL in
`internal/store/migrations/`, one numbered file per change. The API applies
any file it has not run yet at startup and records it in the
`schema_migrations` table. To change the schema, add `0002_<what>.sql`;
never edit a file that has already run.

There is no seed data. The demo comes from the sample logs below.

## Demo samples

The three demo logs and their answer keys live in a private storage bucket
named `samples`, which keeps about 90 MB of generated files out of git and
out of the server image.

```sh
make samples                  # from the repo root: generates ../samples
go run ./cmd/uploadsamples    # from backend/: copies them to the bucket
```

Run both again whenever the generator changes. Set `SAMPLES_DIR` to read
the files from a folder instead.

`make holdout` writes a fourth log to `samples/holdout/`: the same scenarios
with users, times, sizes, hosts and intervals drawn at random. It is not a
demo. `go test ./internal/detect -run TestHoldout -v` scores the detectors
on it and asserts nothing; a miss there is a finding, not a bug to tune out.

## Test it

```sh
go test ./...                 # tests that need a database are skipped
```

The database tests run against whatever `TEST_DATABASE_URL` points to. For
this project that is the Supabase database. Each test creates its own
schema, named `test_` plus eight random characters, and drops it when it
ends, so the tests never see or touch the app's tables.

```sh
TEST_DATABASE_URL="$(grep '^DATABASE_URL=' .env | cut -d= -f2-)" go test -p 1 ./...
```

Expect a few minutes: the tests load the 48,000-line sample several times
over the network. `-p 1` runs one package at a time, to stay under the
session pooler's connection limit.

Two kinds of test are skipped unless their input exists:

- Tests that read the demo logs from `../samples`, until `make samples`
  has generated them.
- `SUPABASE_STORAGE_TEST=1 go test ./internal/blob/` writes one small file
  to the real bucket in `.env` and deletes it.

## Evaluation

Every number here comes from one run: 50 holdout days, seeds 101 to 150,
scored on 12 October 2026. Its full output is in
[`eval/holdout-sweep.txt`](eval/holdout-sweep.txt).

A holdout day plays the demo's seven scenarios with their users, times,
sizes, hosts and intervals drawn at random. Only the detectors ran. No model
was called, and no detector, threshold or prompt was changed in response.

An attack counts as caught when a group at or above the threshold of 30
holds at least half of its lines. A decoy counts as raised when such a group
is made of it.

**The detectors caught 161 of 200 attacks.**

| Scenario | Result |
| --- | --- |
| Exfiltration | caught on 50 of 50 days |
| Phishing click | caught on 50 of 50 |
| Blocked burst | caught on 50 of 50 |
| Beacon | caught on **11 of 50** |
| Backup (decoy) | raised on 50 of 50 |
| Code push (decoy) | raised on 48 of 50 |
| Update agent (decoy) | raised on 4 of 50 |

Nothing was raised that the generator did not plant. The median day raised
5 groups.

### The beacon rule has two blind spots

The rule needs 30 calls to one host before it looks for a rhythm, and then
it wants 80% of the gaps within 10% of the median gap. So it misses:

1. **A beacon that makes fewer than 30 calls in a day.**
2. **A beacon whose timing varies by more than 10%.** Real malware adds
   timing variation on purpose, so this is the larger gap.

| Beacon drawn | Caught |
| --- | --- |
| Fewer than 30 calls in the day | 0 of 16 |
| 30 or more, timing varies by under 10% of the beat | 11 of 12 |
| 30 or more, timing varies by 10% or more | 0 of 22 |

The interval alone does not decide it. Fast beacons were missed about as
often as slow ones:

| Beacon interval | Caught |
| --- | --- |
| Under 1 minute | 1 of 8 |
| 1 to 5 minutes | 4 of 13 |
| 5 to 15 minutes | 4 of 13 |
| 15 to 30 minutes | 2 of 16 |

Proposed fix, not built: a rhythm test that tolerates jitter and counts
calls across several days, validated on seeds it has not seen.

### A fast update agent floods the list

The benign agent polls its vendor from every workstation. On the 4 days it
polled every 5 minutes, the detectors raised a group per workstation: 74
groups in all, and up to 21 on one day, which made that day's list 27 groups
long. On the 46 days it polled every 10 minutes or slower, it was never
raised.

Proposed fix, not built: group by destination, so that fleet-wide polling to
one host becomes a single finding.

### What the numbers do not say

- **These are detector results, before the judge.** On the demo the judge
  sets the code push aside. The backup scores above the judge's limit and
  stays, by design; a lesson is the answer to it.
- **They are not field rates.** They depend on the ranges in
  `samples/loggen/script.go`, chosen by the same person who wrote the
  detectors. The blind spots are the finding.
- **The system does not learn from verdicts yet.** Verdicts are stored with
  their signals. The next step is precedents, which show the judge past
  verdicts on similar anomalies, then learned weights once there are enough
  labels.

### Run it again

```sh
make holdout-sweep            # from the repo root; about 20 seconds
```

It generates the 50 days, scores them, and rewrites `eval/holdout-sweep.txt`.
The same code gives the same bytes, so `git diff` shows whether anything
moved. `scripts/holdout-sweep.sh 201 260` scores another range. Both need
the generator in `samples/loggen`, which is kept out of version control.

## Deploy

A push to `main` deploys the API. Railway watches `backend/`, builds
`Dockerfile` and runs the binary. A push that changes nothing under
`backend/` leaves the API alone.

The service needs `DATABASE_URL`, `SUPABASE_URL` and `SUPABASE_SERVICE_KEY`,
plus the AI settings described below and the keys they call for. Railway
sets `PORT` itself. The server refuses to start without a setting it needs,
so check the variables before you push.

A deploy restarts the server. An analysis running at that moment shows as
stopped and can be resumed.

## How it fits together

| Package | Job |
| --- | --- |
| `cmd/api` | Wires everything and starts the server. |
| `internal/api` | Generated from the contract. Do not edit `api.gen.go`. |
| `internal/server` | One method per operation. Auth, validation, error shape. |
| `internal/pipeline` | The background job: parse, detect, judge, group, narrate. |
| `internal/detect` | Deterministic detectors, scoring and grouping. No model, no database. |
| `internal/store` | Every SQL query. Each one filters by the calling user. |
| `internal/blob` | Signed upload URLs and file reads in Supabase Storage. |
| `internal/auth` | Verifies Supabase access tokens. |
| `internal/samples` | Reads the demo logs and their answer keys from the bucket. |
| `cmd/uploadsamples` | Copies the generated demo logs into the bucket. |
| `internal/zscaler` | Parses one log line. |

## Rules the code keeps

- **Yours or 404.** Every query takes the caller's user ID. Someone else's
  resource and a missing one get the same answer.
- **The risk score is arithmetic.** It is the sum of the weights of the
  signals that fired, capped at 100. Weights and thresholds are constants at
  the top of `internal/detect`. No model touches the score.
- **The AI can only set aside what scores low.** A group the judge calls
  benign is dropped only below a risk score of 65. Above that, it reaches
  the analyst whatever the model says.
- **Every narrative citation is checked** against the incident's evidence.
  A draft that cites anything else is replaced by a fixed template.
- **GET never calls a model.** All AI work happens in the background job.
- **Two jobs work at once.** A job holds its upload's events in memory, so
  the rest wait their turn and say so in `message`. The cap is `maxRunningJobs`
  in `internal/pipeline/runner.go`. It is per server, not per user.
- **Raw log lines are customer data.** They are stored and returned, never
  logged.

## Settings

See `.env.example`.

## The AI layer

There are three AI roles, defined in `internal/pipeline/ai.go`. Each has
its own setting, so any provider that can do a job can be given it.

| Role | Setting | Runs | Job |
| --- | --- | --- | --- |
| Judge | `AI_JUDGE_PROVIDER` | once per anomaly in a group that reached the threshold | Labels it likely malicious, likely benign or inconclusive. A benign label can set a low-scoring group aside. |
| Shadow judge | `AI_SHADOW_JUDGE_PROVIDER` | on the same anomalies | Labels them too. Its labels are saved for the scorecard and change nothing. |
| Narrator | `AI_NARRATIVE_PROVIDER` | once per incident | Writes what happened, in sentences that cite events. |

Each setting is `jev`, `claude`, `mock` or `none`; Jev cannot narrate. The
defaults are `mock`, `none`, `mock`, which need no key. The full setup:

```
AI_JUDGE_PROVIDER=jev
AI_SHADOW_JUDGE_PROVIDER=claude
AI_NARRATIVE_PROVIDER=claude
```

A role set to `jev` needs `JEV_API_KEY`; one set to `claude` needs
`ANTHROPIC_API_KEY`. The server refuses to start without a key it needs.

- **Jev** (`internal/pipeline/jev.go`) is TypeSafe's decision model. It
  does not write text. It reads the facts and one typed question, malicious
  or benign, and returns its choice with a confidence, in about 0.2
  seconds. Below a confidence of 0.5 the code, not the model, calls the
  anomaly inconclusive. Jev cannot narrate, so Claude does.
- **Claude** (`internal/pipeline/claude.go`) judges with `claude-haiku-5-5`
  and narrates with `claude-opus-5-5`. Both return JSON that follows a
  schema.
- **The mock** labels by risk score and writes the template text. It needs
  no key and costs nothing, so tests and offline demos use it.
- **Why a shadow judge?** It is how you try a second model on real traffic
  without trusting it: both rule, only the judge counts, and the scorecard
  shows where they split and how each did against the answer key.
- With the judge set to `none`, the upload ends as `partial` with
  `ai_unavailable`.

What a model is shown: the detector's finding, its measurements, and a
sample of normalized events, 12 per anomaly. It is not shown the raw log
line, the risk score or the signal weights; the judge is a second opinion
and should not be anchored on the first. Jev and Claude get the same facts.

To see how the real judges rule on a demo sample, beside the answer key:

```sh
LIVE_AI=1 JEV_API_KEY=... ANTHROPIC_API_KEY=... \
  go test ./internal/pipeline/ -run TestLiveJudges -v
```

It calls paid APIs, so it never runs unless `LIVE_AI` is set.

What the model cannot do:

- Change a risk score, or set aside a group that scores 65 or more.
- Put an uncited sentence in front of the analyst. A draft that cites an
  event outside the incident is replaced by the template.
- Stop the job. If a model cannot be reached, the upload ends as `partial`
  with scores and evidence complete. If it declines or returns something
  unusable, the anomaly is marked `inconclusive`, or the incident gets the
  template.

Log fields such as the URL and the user agent are written by whoever made
the request. Every prompt tells the model to treat them as data, never as
instructions, and the limits above hold even if it does not.
