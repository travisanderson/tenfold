# Tenfold frontend: instructions and reference

You are building the Tenfold frontend from the mockups in this folder. Part 1 tells you what to do. Part 2 is the reference you look things up in.

The rule for this build: simplify the implementation, not the experience. Keep every visualization. Build each one from a stock library and from data the backend already returns.

## What is here

| File                | Use it for                                                         |
| ------------------- | ------------------------------------------------------------------ |
| `01-` to `08-*.png` | The target. Each screen must match its PNG.                        |
| `tokens.css`        | Every colour, type size, radius and spacing value. Import it once. |
| This file           | Build order, rules, and the reference for every visualization.     |

Source canvas: https://claude.ai/artifact/YPjf6oC2H81n2djLNpaJ26

---

# Part 1: Instructions

## Build order

Build all nine steps, in this order. Finish each step before starting the next. Steps 2 to 5 are the critical journey (Upload, Triage, Incident, Evidence, Human decision), so they come first.

1. **Shell.** Next.js app, `tokens.css`, the two fonts, the top nav.
2. **Uploads** (`01`). Drop zone, "Load demo data", recent uploads table, pipeline.
3. **Triage** (`02`). Upload bar, headline count, timeline, three incident cards.
4. **Incident** (`03`). Header and decision buttons, owner, status, chart, activity graph, why flagged, AI interpretation, notes, evidence table.
5. **Event explorer** (`04`). Filter bar, virtualized table, raw-line drawer.
6. **Operational states** (`08`). One component per state, shown by upload `status`.
7. **Teach it once dialog** (`05`). Opens from "Dismiss as false positive". Lesson text, scope, expiry, and the count of anomalies it would set aside.
8. **Detection scorecard** (`06`). Four score tiles, the scoresheet, and the two AI judges side by side.
9. **Mobile incident** (`07`). The incident screen at phone width, with evidence, graph, timeline and notes collapsed.

## Rules

- **Use the libraries named in the component inventory.** Chart.js for charts, React Flow for the pipeline and the activity graph, TanStack Table and Virtual for tables. Do not write a custom chart or graph renderer.
- **Use tokens, not hex values.** If a value is missing from `tokens.css`, add it there.
- **Put no detection logic in the frontend.** Risk scores, signals, baselines, severity and AI assessments come from the API. Components draw what they are given.
- **Keep the four kinds of statement apart:** observed, measured, AI interpretation, analyst decision. AI text always sits in the tinted panel under its label.
- **Keep the wording in the mockups.** "Sign-in request", not "signed in". "Risk score 91/100" and "AI assessment: likely malicious" are two separate things. The AI assessment has no percentage.
- **Every citation chip and every signal links to event IDs.** Clicking one selects those events in the chart, the graph and the evidence table.
- **Show times in the user's time zone,** Eastern by default. Raw log lines stay in UTC and say so.
- **Keep one selection state per screen.** The chart, graph and table read the same value.
- **Build against mock data first.** Write fixtures that match the API contract you are given, then swap in the API client.
- **Field names in this file are illustrative.** Use the names in the API contract you are given.

## Check each screen before moving on

- It matches its PNG at 1440 pixels wide: layout, spacing, type and colour.
- It does not break at 390 pixels wide. Wide tables scroll inside their own box.
- It handles its empty, loading and error states.
- Every button and link is a real `<button>` or `<a>`, reachable by keyboard.
- Severity is never shown by colour alone.

## Known gaps in the mockups

Decide these as you build, and note what you chose.

- The search box appears in the nav on triage and incident only. Put it on every screen or none.
- Critical and low severity are not drawn. Proposed colours are in `tokens.css`.
- The signal weights (34, 22, 19, 16) and the anomaly threshold of 30 are placeholders.
- The dismiss dialog in `05` is drawn over the wrong incident. Its layout is correct.

---

# Part 2: Reference

## Four kinds of statement

Every value on screen belongs to one of four kinds. The UI labels them and never blends them.

| Kind | Source | Examples | UI treatment |
|---|---|---|---|
| Observed | The log line | time, user, source IP, domain, bytes, action | Monospace. Sections tagged "observed". |
| Measured | Deterministic rules and statistics | signals, weights, risk score | "Why flagged · measured" |
| AI interpretation | The model | assessment label, narrative | Tinted panel, "AI INTERPRETATION" label |
| Analyst decision | A person | Confirm malicious, Dismiss, Escalate, notes | Buttons and notes only |

Wording rules that follow from proxy logs being the only source:

- A request to a login host is a "sign-in request". Never "signed in". The proxy cannot see whether authentication succeeded.
- Regular timing is "beacon-like requests". Never "malware".
- A large upload is a "large outbound transfer". Exfiltration is a possible reading, stated only in the AI interpretation.
- MITRE tags carry the word "Possible".
- The graph is the "activity graph", not the "attack graph".
- A citation shows where a sentence came from. It does not show the sentence is right. The UI says so under the narrative.

## Risk score and AI assessment are separate

**Risk score.** An integer from 0 to 100. Deterministic. It is the sum of the weights of the signals that fired, capped at 100. The backend returns the signals, so the UI can show the sum.

**AI assessment.** One of `likely_malicious`, `likely_benign`, `inconclusive`, plus a narrative whose sentences each carry event IDs. It has no percentage. Do not display a model confidence as a probability.

**Anomaly threshold.** An event group with a risk score of 30 or more becomes an incident.

The weights in the mockups (34, 22, 19, 16) and the threshold of 30 are placeholders. The backend sets the real values.

## Baselines

One upload is one day. There is no 14-day history, and the UI must not claim one.

- **In-file baseline.** A user's own median within the same file. This is the default.
- **Earlier uploads.** Used only where they exist, and counted in the label ("in this file or 3 earlier uploads").
- **No baseline.** If neither exists, the signal does not fire and the UI shows "no baseline".

## Component inventory

| Component | Library | Interaction | Screen |
|---|---|---|---|
| Processing pipeline | React Flow (`@xyflow/react`): custom node, built-in animated edge | Node state and counts update from the status endpoint. Click a stage for its kept and dropped lines. | Uploads |
| Triage timeline | Chart.js via `react-chartjs-2`: one line dataset, one filled band, annotation labels | Click an incident marker to open the incident | Triage |
| Hourly volume strip | Chart.js bar, or plain flex divs | None | Triage |
| Incident cards | Plain React | Open incident | Triage |
| Incident chart | Chart.js bar, one bar per request, with a band | Selecting a citation highlights its bars | Incident |
| Activity graph | React Flow: custom node, default edges, built-in Controls and MiniMap | Click a node to filter the evidence table. Double-click a node, or use "Expand all neighbours", to add the entities connected to it. | Incident |
| Risk bar and signal table | Plain CSS flex and grid | None | Incident |
| AI interpretation | Plain React | Click a citation chip to select its events | Incident |
| Evidence table | TanStack Table | Rows highlight for the selected citation. A row opens its raw line. | Incident |
| Event explorer | TanStack Table with TanStack Virtual | Filter, select a row, raw-line drawer, column picker, J and K keys | Events |
| Upload drop zone | `react-dropzone` | Drop, choose files, Load demo data | Uploads |

No custom drawing engine. Layout for both graphs comes from the backend as `x` and `y`, or from a fixed left-to-right arrangement in the component. Do not add a layout library.

**State.** One selection per screen: `selectedCitationId` or `selectedEventIds` on the incident, `selectedEventId` in the explorer. The chart, graph and table all read it. Keep it in the URL query or in one context. Do not sync between stores.

## Data each visualization needs

| Visualization | Source | Metric and unit | Calculation |
|---|---|---|---|
| Triage timeline, line | Anomalies of one upload | Peak risk score, 0 to 100 | Bucket the day into 144 ten-minute bins. Each bin takes the highest risk score of any anomaly in it, or 0. |
| Triage timeline, band | Config | Risk score | A flat band from 0 to the anomaly threshold. It is a threshold, not a baseline. |
| Triage timeline, markers | Incidents | Time range | `start_time` and `end_time` of each incident |
| Hourly volume strip | Events | Events per hour, count | `count(*) group by hour` |
| Reduction counts | Upload record | Count | `lines_total`, `anomalies_total`, `incidents_total` |
| Incident chart, bars | Events of one incident | Bytes out per request | `request_size` of each event, at its timestamp |
| Incident chart, band | Events of that user in the file | Bytes out | The user's median `request_size` in the file, outside the incident window |
| Activity graph, nodes | Events of one incident | Entity | Distinct user, source IP with host, domain. Add one node for total bytes out. |
| Activity graph, edges | Same | Count, bytes | One edge per distinct pair seen in an event. Width from total bytes. An edge is "flagged" when a signal names it. |
| Risk bar | Signals | Weight, points | One segment per signal, width equal to its weight |
| Signal table | Signals | Mixed | `observed_value`, `baseline_value`, `baseline_source`, `weight`, `layer` |
| Pipeline | Upload status | Count, seconds | Per stage: `state`, `count_in`, `count_out`, `elapsed_ms` |
| Score tiles and scoresheet | Answer key | Count | Planted attacks matched to incidents by event ID |

Times are stored in UTC and shown in the user's zone, Eastern by default. The raw log line stays in UTC and is labelled so.

## Operational states

Each is drawn in `08-operational-states.png`.

| State | Condition | UI |
|---|---|---|
| Processing | `status` is parsing, detecting, judging or writing | Pipeline on Uploads. Compact four-step strip elsewhere. |
| Completed | `status = complete` | Counts, and an "Open triage" link |
| Empty results | Complete, `incidents_total = 0` | A large zero. Link to the anomalies that scored under the threshold. Not an error. |
| Malformed log | `lines_malformed > 0` | Count, first bad line and reason. Analyse the good lines, or cancel. |
| Partial results | `status = partial` | Which stage stopped and what is complete. Resume, or open what is ready. |
| AI unavailable | `ai_available = false` | Risk scores, signals and evidence shown as normal. The AI panel says "Not available". No placeholder text. |
| Failed | `status = failed` | What stopped and what was saved. Upload again. |

## Architecture boundary

The frontend renders structured, evidence-backed data. It does not compute it.

- No detection logic, thresholds, weights or scoring in components. The risk score, signals, baselines and assessment label all arrive from the API.
- A chart component receives numbers and draws them. It does not decide what is anomalous.
- Severity colour and label come from the `severity` field. Components do not derive severity from the score.
- The frontend may sort, filter and format. It may convert UTC to local time and bytes to MB.
- Every narrative sentence and every signal arrives with event IDs. If the IDs are missing, the UI shows the item as uncited and does not hide the gap.

## Design system

All values are in `tokens.css`. The rules that the values alone do not express:

- **Type.** Bricolage Grotesque for text and large numerals. Spline Sans Mono for log data, IPs, domains, event IDs and counts.
- **Shape.** Panels have a 12px radius. Buttons, chips, nav items and selects are pills. No gradients and no glow.
- **Severity.** High takes a triangle icon and medium a square. Severity always carries a label and an icon as well as a colour.
- **AI text.** Always inside the tinted panel, under the "AI INTERPRETATION" label.
- **Accent.** One accent colour, used for interactive elements and selection only.
