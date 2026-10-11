# Brief: OpenAPI contract (`api/openapi.yaml`)

> **Revised to match the mockups.** Where this brief and `mockups/` once disagreed, the mockups decide what the analyst sees and this brief decides the plumbing. The scoring model is a rule-based **risk score (0–100)** plus a separate **AI label with no percentage**; there is no blended confidence.

You are writing the API contract for a log-investigation POC. This file is the **single source of truth** for both sides:
- **Go API** (backend): types and server interfaces generated with `oapi-codegen` (strict server, std `net/http`).
- **Next.js** (frontend): types generated with `openapi-typescript`, calls made with `openapi-fetch`.

Write the spec first, lint it, and do not write handler code in this step.

---

## 1. Product context (for naming and descriptions)

Analysts upload Zscaler web-proxy logs. The Go API parses them, normalizes each line into a vendor-neutral event (modeled after Google SecOps UDM), runs layered detection (rules → statistics → fast AI decision → grouping → AI narrative), and produces a few **incidents**: grouped, evidence-backed stories. Every AI narrative sentence cites event IDs. The analyst confirms, dismisses, or escalates. An agent-ready context endpoint exposes each incident in a compact, typed form.

All AI work happens in a background job. **No endpoint calls an AI model synchronously.** GET endpoints read stored results only.

---

## 2. Global conventions

| Topic | Rule |
| --- | --- |
| OpenAPI version | **3.0.3** (best `oapi-codegen` compatibility). Do not use 3.1-only features. |
| Base path | `/v1` for all resources; `/healthz`, `/openapi.yaml`, `/docs` unversioned |
| JSON casing | camelCase |
| Timestamps | RFC 3339, UTC (`format: date-time`) |
| IDs | UUID for uploads, incidents, anomalies, lessons. **Event IDs are strings `e{lineNo}`** (e.g. `e1042`), unique within an upload, so citations stay human-readable. |
| operationId | Required on every operation, camelCase verbs (`listUploads`, `getIncident`). These become Go and TS function names. |
| Tags | `Uploads`, `Events`, `Detections`, `Incidents`, `Lessons`, `Samples`, `Evaluation`, `System` |
| Enums | Define once under `components/schemas` and `$ref` them. No inline enums. |
| Nullability | Prefer omitting optional fields over `nullable`. Use `nullable: true` only where null carries meaning. |
| Pagination | Cursor-based: `?cursor=&limit=`; response `{ items, nextCursor, total }`. `nextCursor` absent on the last page. |
| Errors | RFC 9457 Problem Details, `application/problem+json` (section 6) |
| Examples | Include realistic `example` values using the fictional dataset in section 9 |

## 3. Security

- `securitySchemes.bearerAuth`: HTTP bearer, `bearerFormat: JWT` (Supabase-issued access token). Applied globally; `/healthz`, `/openapi.yaml`, `/docs` override with `security: []`.
- Every resource is owned by the calling user. **Accessing another user's resource returns 404, not 403**, so existence isn't leaked. State this in the operation descriptions.
- The Next.js server forwards the user's token; the browser never calls the Go API directly.

## 4. Schemas (`components/schemas`)

### Enums
- `UploadStatus`: `queued | processing | completed | partial | failed` (`partial` = stopped early, can resume)
- `PipelineStage`: `uploaded | parsing | detecting | judging | grouping | narrating | done`
- `UploadSource`: `upload | sample`
- `Severity`: `critical | high | medium | low`
- `AiVerdict`: `likely_malicious | likely_benign | inconclusive`
- `AnalystStatus`: `open | confirmed | dismissed | escalated`
- `DetectorType`: `threat_hit | blocked_burst | exfil_volume | beaconing | rare_destination | new_source_ip | off_hours`
- `DecisionProvider`: `jev | claude | mock | rules`
- `NarrativeStatus`: `generated | template | unavailable`
- `LessonScopeType`: `user | domain | pattern`
- `GraphNodeType`: `user | host | domain | ip | data`
- `EventAction`: `allowed | blocked`
- `VerdictStatus`: `confirmed | dismissed | escalated`
- `SignalLayer`: `rules | baselines`
- `BaselineSource`: `in_file | earlier_uploads | none`
- `StopReason`: `parse_errors | ai_unavailable | stage_failed`
- `StageState`: `pending | running | done | stopped | skipped`
- `AnomalyDisposition`: `in_incident | below_threshold | judged_benign | lesson`
- `AnswerKeyLabel`: `attack | benign`; `ScoreMark`: `caught | missed | false_positive`; `ProviderRole`: `in_use | shadow`
- `EventSort`: `ts | -ts`; `TimeseriesBucket`: `1m | 5m | 10m | 15m | 1h`

### Uploads
- `Upload`: `id, filename, source, sampleName?, status, stage, progress (int 0–100), pipeline[], stop?, firstParseError?, riskThreshold, sizeBytes, lineCount?, eventCount?, parseErrorCount?, anomalyCount?, incidentCount?, userCount?, hostCount?, verdictCounts?, timeRange? {start, end}, error? {code, message}, createdAt, startedAt?, completedAt?`
- `StageProgress`: `stage, state, countIn?, processed?, countOut?, elapsedMs?` (one per pipeline node)
- `UploadStop`: `stage, reason (StopReason), message`. Present when `status` is `partial`. One field drives the Malformed log, Partial results and AI unavailable cards.
- `ParseError`: `lineNo, reason, sample`
- `VerdictCounts`: `open, confirmed, dismissed, escalated`
- `AnalyzeRequest`: `acceptParseErrors?, restart?`
- `CreateUploadRequest`: `filename, sizeBytes (max 52428800), contentType`
- `CreateUploadResponse`: `upload, uploadUrl (signed URL for a direct PUT to storage), expiresAt`
- `ProgressEvent` (SSE payload): `uploadId, status, stage, progress, message, counts {lines, events, anomalies, incidents}, pipeline[], at`

### Events
- `Event`:
  - `id` (string, `e{lineNo}`), `lineNo`, `ts`
  - `principal { user, dept?, device?, srcIp }`
  - `target { host, url, category, ip? }`
  - `network { method, status, bytesOut, bytesIn, userAgent? }`
  - `securityResult { action, reason?, threatName?, riskScore? }`
  - `incidentId?`
  - `flags[] { anomalyId, label, disposition }` (empty if none; feeds the "Why flagged" column)
  - `raw` (original log line)
- `EventPage`: `items: Event[], nextCursor?, total`
- `TimeseriesResponse`: `bucketSeconds, points[] { ts, total, blocked, anomalous, peakRiskScore }`

### Detections
- `Signal`: `name, description (plain language, deterministic), weight, layer, observed, observedDetail?, baselineText?, baselineDetail?, baselineSource, baselineUploadCount?, value?, baseline?, unit?, eventIds[]`. Weights of the signals that fired sum to the risk score, capped at 100.
- `Decision`: `label (AiVerdict), confidence (0–1), provider, latencyMs, model?`. `confidence` is the model's own figure: for ranking and the scorecard only, never shown as a probability, never part of the risk score.
- `Anomaly`: `id, uploadId, detector, reason, signals[], riskScore (int 0–100), decision?, shadowDecision?, disposition, incidentId?, eventIds[], user?, host?, firstSeen, lastSeen, suppressedByLessonId?`
- `AnomalyPage`: `items, nextCursor?, total`

### Incidents
- `MitreTechnique`: `id (e.g. T1567), name, tactic`
- `AnalystVerdict`: `status, note?, decidedAt?`
- `IncidentSummary`: `id, uploadId, title, severity, riskScore (int 0–100), aiVerdict?, summary?, markerLabel, markerDetail?, assignee? { id, name }, principal { user?, host? }, timeRange { start, end }, eventCount, anomalyCount, mitre[], analystVerdict`. `aiVerdict` and `summary` are absent when the AI did not run.
- `NarrativeSentence`: `text, citations[] (event IDs; minItems 1)`
- `Narrative`: `status, provider?, sentences[], nextSteps[], generatedAt?`. Description: *"Every citation is validated server-side to be a subset of `evidenceEventIds`. If validation fails, a deterministic template narrative is returned with `status: template`."*
- `TimelineItem`: `ts, eventId, label, kind`
- `GraphNode`: `id, type, label, sublabel?, flagged, neighbour, eventIds[], x?, y?`; `GraphEdge`: `id, source, target, label?, flagged, bytes?, eventCount, eventIds[]`; `IncidentGraph`: `nodes[], edges[]`
- `IncidentDetail`: all of `IncidentSummary` (use `allOf`) plus `narrative, signals[], timeline[], graph, indicators[] { type, value, eventCount }, chart { window, baselineBytesOut?, baselineLabel? }, notes[] (IncidentNote), lessonSuggestions[] { scope, label }, evidenceEventIds[], evidence: Event[] (max 500)`
- `IncidentNote`: `id, author, text, createdAt`; `AddNoteRequest`: `text`; `UpdateIncidentRequest`: `assignedToMe`
- `VerdictRequest`: `status (confirmed | dismissed | escalated), note?, lesson? (LessonCreate)`

### Agent context
- `IncidentContext`: `schemaVersion ("1"), incidentId, summary (1–2 sentences), riskScore, aiVerdict?, severity, analystStatus, entities { users[], hosts[], domains[], ips[] }, timeline[] { ts, eventId, label }, evidence[] { id, ts, user, host, action, bytesOut, threatName? }, mitre[], recommendedActions[], provenance { detectors[], decisionProvider, narrativeProvider?, generatedAt }, tokenEstimate`
  - Description: *"Compact, cited, typed representation of an incident for downstream AI agents. Minimal fields to reduce tokens."*

### Lessons ("teach it once")
- `LessonScope`: `type, value`
- `LessonCreate`: `text, scope, expiresAt`
- `Lesson`: `id, text, scope, createdAt, expiresAt, matchCount`
- `LessonPreviewRequest`: `uploadId, scope`; `LessonPreviewResponse`: `wouldSuppressAnomalies, wouldResolveIncidents`

### Samples and evaluation
- `Sample`: `name, title, description, lineCount, sizeBytes, hasAnswerKey`
- `ScoresheetRow`: `title, subtitle, incidentId?, layers[] (SignalLayer), riskScore?, aiVerdict?, answerKey, mark`. One row per incident raised, plus one per missed attack.
- `JudgeAgreement`: `total, agreed`
- `ProviderScore`: `provider, label, role, decisions, attacksCaught, attacksTotal, falsePositives, agreementWithAnswerKey (0–1), avgLatencyMs, costPer1kUsd`
- `EvalReport`: `uploadId, sampleName, runAt, lineCount, anomalyCount, incidentCount, scoresheet[], truePositives, falsePositives, falseNegatives, precision, recall, judgeAgreement, providers[]`

### Problem
- `Problem`: `type, title, status, detail?, instance?, code` (machine-readable, e.g. `upload_too_large`), `errors?[] { field, message }`

## 5. Endpoints

| Pri | Method | Path | operationId | Request | Success | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| P0 | GET | `/healthz` | `getHealth` | | 200 `{status, version}` | No auth |
| P0 | POST | `/v1/uploads` | `createUpload` | `CreateUploadRequest` | 201 `CreateUploadResponse` | 413 if too large; 422 bad content type |
| P0 | GET | `/v1/uploads` | `listUploads` | `cursor, limit` | 200 page of `Upload` | Newest first |
| P0 | GET | `/v1/uploads/{uploadId}` | `getUpload` | | 200 `Upload` | |
| P0 | DELETE | `/v1/uploads/{uploadId}` | `deleteUpload` | | 204 | "Cancel this upload". Stops any job; deletes file, events, results. |
| P0 | POST | `/v1/uploads/{uploadId}/analyze` | `analyzeUpload` | `AnalyzeRequest` (optional) | 202 `Upload` | Starts or resumes the background job. 409 if already processing. Resumes at the stopped stage; `restart: true` reruns everything. Stops after parsing with `partial / parse_errors` if lines are malformed, until called with `acceptParseErrors: true`. |
| P0 | GET | `/v1/uploads/{uploadId}/progress` | `streamUploadProgress` | | 200 `text/event-stream` | Server-sent events: `progress` (data = `ProgressEvent` JSON), `done`, `error`. Document the event names and payload in the description. |
| P0 | GET | `/v1/uploads/{uploadId}/events` | `listEvents` | `cursor, limit (≤1000, default 200), user, srcIp, host, category, action, hasAnomaly, from, to, q, sort (ts | -ts)` | 200 `EventPage` | |
| P0 | GET | `/v1/uploads/{uploadId}/timeseries` | `getTimeseries` | `bucket` (enum `1m, 5m, 10m, 15m, 1h`, default `5m`) | 200 `TimeseriesResponse` | `10m` feeds the risk line, `1h` the volume bars |
| P0 | GET | `/v1/uploads/{uploadId}/anomalies` | `listAnomalies` | `cursor, limit, detector, disposition, judgesDisagree` | 200 `AnomalyPage` | Includes anomalies set aside; highest risk score first |
| P0 | GET | `/v1/uploads/{uploadId}/incidents` | `listIncidents` | | 200 `IncidentSummary[]` | Sorted by severity, then risk score |
| P0 | GET | `/v1/incidents/{incidentId}` | `getIncident` | | 200 `IncidentDetail` | |
| P0 | PATCH | `/v1/incidents/{incidentId}` | `updateIncident` | `UpdateIncidentRequest` | 200 `IncidentSummary` | "Assign to me" |
| P0 | POST | `/v1/incidents/{incidentId}/notes` | `addIncidentNote` | `AddNoteRequest` | 201 `IncidentNote` | Append-only |
| P0 | POST | `/v1/incidents/{incidentId}/verdict` | `setIncidentVerdict` | `VerdictRequest` | 200 `IncidentSummary` | Optional lesson creation. `note` is saved as an analyst note. |
| P0 | GET | `/v1/samples` | `listSamples` | | 200 `Sample[]` | |
| P0 | POST | `/v1/samples/{sampleName}/load` | `loadSample` | | 202 `Upload` | Creates an upload and starts analysis |
| P1 | GET | `/v1/incidents/{incidentId}/context` | `getIncidentContext` | | 200 `IncidentContext` | Agent-ready |
| P1 | GET | `/v1/uploads/{uploadId}/eval` | `getEvalReport` | | 200 `EvalReport` | 404 if the upload has no answer key |
| P1 | GET | `/v1/lessons` | `listLessons` | | 200 `Lesson[]` | |
| P1 | POST | `/v1/lessons` | `createLesson` | `LessonCreate` | 201 `Lesson` | |
| P1 | DELETE | `/v1/lessons/{lessonId}` | `deleteLesson` | | 204 | |
| P1 | POST | `/v1/lessons/preview` | `previewLesson` | `LessonPreviewRequest` | 200 `LessonPreviewResponse` | Powers "would suppress 37 anomalies" |
| P2 | GET | `/v1/uploads/{uploadId}/events/export` | `exportEvents` | | 200 `application/x-ndjson` | Bulk stream for the browser IndexedDB cache |

## 6. Errors

Every operation documents these as `$ref`'d shared responses (`components/responses`), all `application/problem+json` with the `Problem` schema:

| Status | Response name | When |
| --- | --- | --- |
| 400 | `BadRequest` | Malformed parameters |
| 401 | `Unauthorized` | Missing or invalid token |
| 404 | `NotFound` | Doesn't exist **or isn't yours** |
| 409 | `Conflict` | Job already running |
| 413 | `PayloadTooLarge` | Upload over the size cap |
| 422 | `Unprocessable` | Valid JSON, invalid values (with `errors[]`) |
| 429 | `TooManyRequests` | Rate limited; include a `Retry-After` header |
| 500 | `InternalError` | Unexpected |
| 503 | `Unavailable` | Dependency down |

## 7. Reusable parameters (`components/parameters`)
`UploadId`, `IncidentId`, `LessonId`, `SampleName` (path) · `Cursor`, `Limit` (query)

## 8. Implementation notes to put in the spec descriptions

- **SSE and auth:** browsers' `EventSource` can't send an `Authorization` header. The browser connects to a Next.js Route Handler, which opens the SSE stream to Go with the user's token and relays it. Note this in the `streamUploadProgress` description.
- **Uploads:** the file goes directly from the browser to storage via `uploadUrl`. The API never receives file bytes.
- **Scoring:** the risk score is rule-based and deterministic; no model touches it. It measures how unusual the activity is, not the odds of an attack. The AI label is separate and has no percentage. Say so on `riskScore`, `AiVerdict` and `Decision.confidence`.
- **Search:** `q` accepts `user:`, `domain:`, `ip:`, `id:` terms joined by `OR`, or bare text. Nothing more.
- **Raw logs** are customer data. Mark `Event.raw` as sensitive in its description.

## 9. Example data (use in `example` values)

Fictional company "Northwind Labs."
- Upload `northwind-zia-2026-10-08.log`, 48,212 lines → 214 anomalies → 3 incidents. Sample: `sample-day` (50,000 lines, 3 planted attacks). Times on screen are ET; the API is UTC (02:14 ET = 06:14Z).
- Incident 1: data exfiltration, user `dana.k` on `LT-022` from `203.0.113.47`, 02:14–02:41 ET, 40.2 MB to `files-sync-cdn.net`, severity `high`, risk score 91 (signals 34 + 22 + 19 + 16), `likely_malicious`, MITRE `T1567`
- Incident 2: beaconing, host `WS-114` (user `marcus.l`), `update-chk-svc.io` every ~60s, `high`, risk score 87, `likely_malicious`
- Incident 3: phishing click, user `priya.s`, threat `Phish.Generic`, blocked, `medium`, risk score 54, `inconclusive`
- Event ID examples: `e1042`, `e1047`, `e1048`

## 10. Deliverables for this step

1. `api/openapi.yaml`, complete and lint-clean (`npx @redocly/cli lint api/openapi.yaml`)
2. `backend/internal/api/oapi.cfg.yaml` for `oapi-codegen` (strict server, std-http, models + server)
3. A `make generate` (or npm script) that regenerates Go and TS types from the spec
4. Generated files committed; a check that fails if the spec and generated code drift
5. Serve the spec at `/openapi.yaml` and interactive docs at `/docs` (Scalar or Swagger UI)

Do **not** add GraphQL or non-REST patterns. Keep it boring, explicit, and easy to explain.
