# Tenfold frontend preview

Run `npm install`, then `npm run dev` from this folder. Open http://127.0.0.1:3000/uploads.

The preview implements all eight mockup views with typed fixtures. It works independently of the backend. Decisions, assignment, notes, and lessons remain in memory while navigating in the tab; refreshing resets them. Loading demo data creates a separate sample investigation. Real file analysis and Supabase sign-in are not connected yet. The file picker validates extensions and size, then explains that limitation.

## Screens

- `/uploads`: React Flow pipeline, drop zone, recent uploads.
- `/triage`: Chart.js risk timeline, hourly volume, incident cards.
- `/incidents/3f2b8c1e-5d4a-4e6f-9a1b-2c3d4e5f6a7b`: linked citations, chart, graph, signals, evidence, notes, assignment, verdicts, teaching dialog.
- `/events`: TanStack Table and Virtual, search, time/action filters, columns, J/K navigation, raw log drawer.
- `/states`: processing, completed, empty, malformed, partial, unavailable AI, and failure scenarios.
- `/scorecard`: answer-key comparison and two judges.
- `/anomalies`: disposition and judge-disagreement review.

At phone width, the incident puts interpretation and measured signals first. Evidence, activity graph, request chart, timeline, and notes collapse. Wheel and pinch zoom are disabled on both React Flow views; buttons still zoom.

## API contract and Prism

`../api/openapi.yaml` remains the source of truth. The existing generated types in `src/lib/api/schema.d.ts` type-check every fixture. `src/lib/api/client.ts` provides a server-only `openapi-fetch` client for the backend handoff. It is not connected to the preview yet; the browser must never send real API requests directly to the Go service.

In a second terminal, run:

```sh
npm run mock:api
```

Prism reads the existing contract without modifying it and listens on http://127.0.0.1:4010. `npm run mock:check` checks health, uploads, incident detail, and request validation. The UI keeps linked, stateful fixtures because independently generated Prism examples do not maintain the relationships needed for an investigation.

The authored preview contains three visible incidents and 208 evidence events. Its upload-level totals and illustrative evaluation scores are fixtures, not measured backend performance. The scorecard includes a fourth planted attack marked as missed. Prism and all browser checks run locally.

## Design choices

- Search appears on every screen.
- Times default to Eastern, with a UTC selector. Raw logs remain in UTC and retain all 21 tab-separated fields.
- Critical/low severity use the proposed token colors with an icon and text.
- The API wins over drawings: `.log`/`.csv`, 50 MB, five stages displayed as four nodes, and no unsupported pipeline drilldown or “See them” lesson link.
- One incident selection drives the chart, graph, and table. No detection or score calculation lives in UI components.
- The lesson dialog uses the scopes belonging to the incident being dismissed. Preview counts are authored API-shaped fixture responses.
- Operational scenarios demonstrate their actions locally; the processing snapshot is not a running detection job.

## Checks

```sh
npm run typecheck
npm run lint
npm run build
npm run test:e2e
npm run mock:check
```

Browser tests require the dev server and installed Chrome. They check desktop/mobile overflow, the investigation flow, citations, decisions, notes, filters, raw fields, keyboard navigation, lessons, graph scrolling, and collapsed mobile panels. Screenshots are written to the ignored `test-results/` folder. `npm run format` formats source while excluding generated API types.
