# Tenfold frontend

Run `npm install`, copy `.env.example` to `.env.local`, set the Supabase publishable key, then run `npm run dev`. Open http://127.0.0.1:3000 and sign in with an existing Supabase email/password account. Keep passwords out of configuration files and commits.

Live mode is the default. The logo opens Triage; an account with no uploads lands on Uploads. Uploaded files, demo runs, notes, assignment, verdicts, and lessons are stored by the backend and survive refreshes. Display timezone stays selected while navigating.

## Live integration

- Supabase cookie sessions are verified on the server and refreshed by Next.js Proxy. Sign out clears the session. Backend `401` responses lead to sign-in.
- `src/lib/api/client.ts` is the server-only, typed `openapi-fetch` boundary. Server Components load screens and Server Actions perform mutations. The browser never calls the Go API directly. Independent screen reads run in parallel.
- Navigation keeps the current screen visible until the next server-rendered screen is ready, with a thin progress indicator for slower links. Events and Anomalies reuse the server-provided first page instead of fetching it again during hydration. Client-side filters and pagination use table skeletons; charts reserve their space while mounting. Skeleton animations respect reduced-motion preferences.
- `.log` files register as `text/plain`; `.csv` files register as `text/csv`. A browser PUT sends the file to the signed storage URL without an authorization header. Files are limited to 50 MB.
- **Clear data**, beside Recent uploads, deletes all account uploads and their results after confirmation. It stops running jobs and returns to an empty Uploads screen. Saved lessons are kept unless “Also delete saved lessons” is selected. The account stays signed in.
- Load demo data registers `attack-day` on the server. Progress passes through an authenticated SSE relay. Streams close on completion, error, or partial status; the client then reads the upload again. Interrupted streams offer Reconnect.
- Demo samples deliberately pause for malformed lines. “Analyse the good lines” sends `acceptParseErrors: true`. The completed run shows skipped-line counts in Triage.
- Triage uses incident summaries and 10-minute/hourly time series. Incident detail, notes, assignment, verdicts, and lesson previews use their contract endpoints.
- Event search, action/time/anomaly filters, and pagination run on the server. The global search opens Events with the same query. Export streams NDJSON through Next.js without buffering the whole file.
- Scorecard and anomalies use live responses. A single judge displays no invented comparison or agreement percentage. The deployed backend currently uses a mock AI provider; this is separate from frontend fixture mode.

`../api/openapi.yaml` remains the source of truth. Do not hand-edit generated `src/lib/api/schema.d.ts`.

## Screens

- `/sign-in`: Supabase email/password sign-in.
- `/uploads`: static pipeline, file picker, demo loading, resumable job states, recent uploads.
- `/triage`: risk timeline, event volume, incident cards.
- `/incidents/{id}`: linked citations, graph, signals, evidence, notes, assignment, verdicts, teaching.
- `/events`: virtualized results, server search and filters, columns, J/K navigation, raw log drawer, export.
- `/scorecard`: answer-key comparison and provider results.
- `/anomalies`: disposition and judge disagreement filters.

At phone width, evidence, graph, chart, timeline, and notes collapse. The upload pipeline has no pan, zoom, or view controls. The incident graph retains its buttons; wheel and pinch zoom remain disabled.

## Isolated fixture preview

Set `TENFOLD_FIXTURE_MODE=true` and restart Next.js to use the original design preview without sign-in. It starts empty; Load demo data creates three sample incidents. Changes stay in memory and reset on refresh. `/states` is available only in this mode. Live mode never substitutes fixture results for an API failure.

For independent contract checks, `npm run mock:api` starts Prism on http://127.0.0.1:4010. `npm run mock:check` checks its responses and request validation.

## Checks

```sh
npm run typecheck
npm run lint
npm run build
# Against a running live-mode dev server; set these in your shell:
# TENFOLD_TEST_EMAIL and TENFOLD_TEST_PASSWORD
npx playwright test tests/live.spec.ts --workers=1
# Read-only navigation/loading check against an existing completed demo:
npx playwright test tests/live.spec.ts -g "navigation keeps"
# Against a separate fixture-mode dev server:
TENFOLD_FIXTURE_MODE=true npx playwright test tests/preview.spec.ts
```

Browser tests use installed Chrome. Live tests create demo uploads in the test account and leave a completed investigation for review. They check auth, malformed-line handling, progress, refresh persistence, analyst actions, backend search/pagination, scorecard, anomalies, mobile layout, and sign-out. Fixtures have separate desktop/mobile design checks. Test screenshots and browser artifacts are ignored in `test-results/`.
