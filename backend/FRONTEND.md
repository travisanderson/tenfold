# Connecting the frontend

Everything here was run against the deployed API with a real Supabase
sign-in on 2026-10-11. Real responses from that run are in
`examples/attack-day/`; use them to check that components draw live data.

## Settings

| Variable | Value |
| --- | --- |
| `TENFOLD_API_URL` | `https://tenfold-api.up.railway.app` (or `http://localhost:8080` with `go run ./cmd/api` in `backend/`) |
| `NEXT_PUBLIC_SUPABASE_URL` | `https://qftygikbzgytxrmfvrch.supabase.co` |
| `NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY` | Supabase dashboard → Settings → API Keys → Publishable key |

The project has **no users yet**. Add one in the dashboard (Authentication →
Users → Add user, with "Auto Confirm User" on), or build a sign-up page.

## Auth

1. Sign the analyst in with Supabase (`@supabase/ssr`, email and password).
2. In server code, read the session and pass its `access_token` to
   `createApiClient(process.env.TENFOLD_API_URL, session.access_token)`.
3. The token lasts one hour. Let the Supabase middleware refresh it. When
   the API answers `401`, send the analyst back to sign-in.

The browser never calls the API. Every call goes through a Server
Component, a Server Action or a Route Handler, which adds the token.

## The demo, call by call

| Step | Call | What comes back |
| --- | --- | --- |
| List demos | `GET /v1/samples` | `attack-day`, `clean-day`, `large-100k` |
| Load one | `POST /v1/samples/attack-day/load` | `202`, an `Upload` with `status: processing` |
| Watch it | `GET /v1/uploads/{id}/progress` | An event stream; see below |
| It stops | `GET /v1/uploads/{id}` | `status: partial`, `stop.reason: parse_errors`: the **Malformed log** card |
| "Analyse the good lines" | `POST /v1/uploads/{id}/analyze` with `{"acceptParseErrors": true}` | `202`; watch the stream again |
| "Cancel this upload" | `DELETE /v1/uploads/{id}` | `204` |
| Done | `GET /v1/uploads/{id}` | `status: completed`; 48,002 lines → 41 anomalies → 5 incidents |
| Triage | `GET /v1/uploads/{id}/incidents`, `GET .../timeseries?bucket=10m` and `?bucket=1h` | Five incidents, most severe first |
| Incident | `GET /v1/incidents/{id}` | Everything the incident screen draws |
| Events | `GET /v1/uploads/{id}/events?limit=200` | A page; pass `nextCursor` back as `cursor` |
| Scorecard | `GET /v1/uploads/{id}/eval` | 4 of 4 attacks caught, 1 false positive |

Every demo sample has malformed lines on purpose, so every demo load stops
once on the Malformed log card. After the analyst accepts, the run ends as
`completed`. `parseErrorCount > 0` is how to tell the results came from a
partial read.

## The analyst's own file

1. `POST /v1/uploads` with `{filename, sizeBytes, contentType}`. Send
   `contentType: "text/plain"` for a `.log` file; browsers often report an
   empty type, and the API answers `422` for anything but `text/plain` or
   `text/csv`. Over 50 MB is `413`.
2. **The browser** sends the file straight to `uploadUrl`:
   `fetch(uploadUrl, { method: "PUT", headers: { "Content-Type": "text/plain" }, body: file })`.
   No token, no key. This is the one request the browser makes to something
   other than Next.js. Storage allows it from any origin. The URL works for
   two hours.
3. `POST /v1/uploads/{id}/analyze`, then watch the stream.

If the `PUT` never happened, analysis ends as `status: failed` with
`error.code: file_missing`.

## The progress stream

A browser `EventSource` cannot send an `Authorization` header, so relay the
stream through a Route Handler:

```ts
// app/api/uploads/[id]/progress/route.ts
export const dynamic = "force-dynamic";

export async function GET(request: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const token = await accessToken(); // from the Supabase session
  const upstream = await fetch(`${process.env.TENFOLD_API_URL}/v1/uploads/${id}/progress`, {
    headers: { Authorization: `Bearer ${token}` },
    signal: request.signal, // stop the upstream call when the browser leaves
    cache: "no-store",
  });
  return new Response(upstream.body, {
    status: upstream.status,
    headers: {
      "Content-Type": upstream.headers.get("Content-Type") ?? "text/event-stream",
      "Cache-Control": "no-cache, no-transform",
    },
  });
}
```

| Event | Data | Then |
| --- | --- | --- |
| `progress` | `ProgressEvent` | Keep listening, unless `status` is `partial` |
| `done` | `ProgressEvent`, `status: completed` | The server closes the stream |
| `error` | `Problem` | The server closes the stream |

Two things to get right:

- **Close the `EventSource` yourself** on `done`, on `error`, and on a
  `progress` whose `status` is `partial`. The server closes its end, and an
  `EventSource` left open reconnects for ever.
- **A job that stops early has no event of its own.** It ends with a last
  `progress` carrying `status: "partial"`. Then read the upload to see why.

The stream opens with the current state, so reconnecting is always safe.
Each `ProgressEvent` carries the same `pipeline` array as the upload. While
judging runs, its entry has `countIn` and `processed` ("judging 132 of
198"), and `processed - countOut` is the count set aside so far.

## Upload status to mockup card

| `status` | Also check | Card |
| --- | --- | --- |
| `queued` | | Registered, not yet analyzed |
| `processing` | `stage`, `pipeline` | Processing |
| `completed` | `incidentCount > 0` | Completed |
| `completed` | `incidentCount === 0` | Empty results |
| `partial` | `stop.reason === "parse_errors"` | Malformed log; `firstParseError` has the line |
| `partial` | `stop.reason === "ai_unavailable"` | AI unavailable; "Try the AI stages again" is `analyzeUpload` with no body |
| `partial` | `stop.reason === "stage_failed"` | Partial results; "Resume" is `analyzeUpload` with no body |
| `failed` | `error.code`, `error.message` | Failed |

`analyzeUpload` with `{"restart": true}` runs everything again. It answers
`409` while a job is running.

## Where live data differs from the fixtures

- **Incident IDs are new on every run.** Take them from `listIncidents`;
  never hard-code a route like `/incidents/3f2b…`.
- **Five incidents, not three**, on `attack-day`: four planted attacks and
  the backup decoy. The engineer's large GitHub push is set aside by the
  judge and appears under `GET .../anomalies?disposition=judged_benign`.
- **The AI is a stand-in for now.** `decision.provider` and
  `narrative.provider` are `mock`. `shadowDecision` is absent, so the "two
  judges" view has one judge and `judgeAgreement.total` is 0.
- **Absent means absent.** Optional fields are left out, never `null`.
  Arrays are always present and may be empty.
- **Errors are Problem Details** (`application/problem+json`) with a
  stable `code`. `404` means "missing or not yours". `422` lists the bad
  fields in `errors[]`.
- **`events/export` is 53 MB** for `attack-day`. Stream it; do not buffer.
- **Rate limiting (`429`) is not built.**

## Measured on Railway, for sizing loading states

| Call | Time |
| --- | --- |
| Demo load, to the Malformed log card | about 7 s |
| After accepting, to completed | about 1 s |
| Incident list, incident detail, scorecard | 150 to 250 ms |
| Timeseries | 200 to 300 ms |
| A page of 200 events | about 430 ms |
| A page of 1,000 events | about 540 ms |
