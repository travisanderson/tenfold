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

## Deploy

Railway builds `Dockerfile` and runs the binary. It needs three variables:
`DATABASE_URL`, `SUPABASE_URL` and `SUPABASE_SERVICE_KEY`. Railway sets
`PORT` itself.

```sh
railway up                    # from backend/
```

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
- **Raw log lines are customer data.** They are stored and returned, never
  logged.

## Settings

See `.env.example`.

## The AI layer

`AI_PROVIDER` picks who fills the two AI roles, both defined in
`internal/pipeline/ai.go`:

| Role | Runs | Asks |
| --- | --- | --- |
| `Judge` | once per anomaly in a group that reached the threshold | likely malicious, likely benign, or inconclusive? |
| `Narrator` | once per incident | what happened, in sentences that cite events |

- `mock` (the default) labels by risk score and writes the template text.
  It needs no key and costs nothing, so tests and offline demos use it.
- `claude` calls the Anthropic API (`internal/pipeline/claude.go`) and needs
  `ANTHROPIC_API_KEY`. The judge defaults to `claude-haiku-5-5`, the
  narrator to `claude-opus-5-5`. The mock judge runs beside Claude as a
  shadow, so the scorecard can compare the two.
- `none` runs no AI. The upload ends as `partial` with `ai_unavailable`.

What the model is shown: the detector's finding, its measurements, and a
sample of normalized events, 12 per anomaly. It is not shown the raw log
line, the risk score or the signal weights; the judge is a second opinion
and should not be anchored on the first.

What the model cannot do:

- Change a risk score, or set aside a group that scores 65 or more.
- Put an uncited sentence in front of the analyst. A draft that cites an
  event outside the incident is replaced by the template.
- Stop the job. If Claude cannot be reached, the upload ends as `partial`
  with scores and evidence complete. If it declines or returns something
  unusable, the anomaly is marked `inconclusive`, or the incident gets the
  template.

Log fields such as the URL and the user agent are written by whoever made
the request. Both prompts tell the model to treat them as data, never as
instructions, and the limits above hold even if it does not.
