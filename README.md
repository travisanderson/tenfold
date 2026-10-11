# Tenfold

A log-investigation POC. See `DESIGN.md` for the plan.

## The API contract

`api/openapi.yaml` is the single source of truth. Go server types and
TypeScript client types are generated from it and committed.

| Command | What it does |
| --- | --- |
| `make lint` | Lints the spec with Redocly. |
| `make generate` | Lints, then regenerates the Go and TypeScript types. |
| `make check-drift` | Regenerates and fails if the committed files differ. CI runs this. |

Needs Go 1.24+ and Node 20+. Run `npm install` once.

Generated files, never edited by hand:

- `backend/internal/api/api.gen.go`: Go models and the strict server interface (`oapi-codegen`)
- `backend/internal/api/openapi.gen.yaml`: a copy of the spec, embedded in the server
- `frontend/src/lib/api/schema.d.ts`: TypeScript types (`openapi-typescript`)

The server serves the spec at `/openapi.yaml` and interactive docs at `/docs`:

```sh
cd backend && go run ./cmd/api   # http://localhost:8080/docs
```
