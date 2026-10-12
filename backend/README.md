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

## Test it

```sh
go test ./...                       # unit tests; database tests are skipped
```

The database tests need a throwaway Postgres. They create a fresh schema
per test and drop it afterwards, so they never touch real data.

```sh
docker run -d --name tenfold-db -p 127.0.0.1:54329:5432 \
  -e POSTGRES_USER=tenfold -e POSTGRES_PASSWORD=tenfold -e POSTGRES_DB=tenfold postgres:17
TEST_DATABASE_URL=postgres://tenfold:tenfold@localhost:54329/tenfold go test -race ./...
```

Two kinds of test are skipped unless their input exists:

- Tests that read the demo logs, until `make samples` has generated them.
- `SUPABASE_STORAGE_TEST=1 go test ./internal/blob/` writes one small file
  to the real bucket in `.env` and deletes it.

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
| `internal/samples` | Reads the demo logs and their answer keys. |
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
