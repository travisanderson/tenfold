# Tenfold API

The Go backend. It implements every operation in `../api/openapi.yaml`.

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

See `.env.example`. `AI_PROVIDER` is `mock` (an offline stand-in) or `none`.
The real providers plug in behind the `Judge` and `Narrator` interfaces in
`internal/pipeline/ai.go`.
