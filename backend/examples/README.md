# Real responses

`attack-day/` holds responses captured from the deployed API on 2026-10-11,
after loading the `attack-day` demo sample. They are for checking that
frontend components draw live data. All of it is synthetic: Northwind Labs
does not exist.

They are a snapshot. IDs differ on every run, and the detectors changed in
session 5, so never read these files at run time.

The AI output here is the mock's. The capture predates Jev and Claude: every
decision has `provider: mock`, the narratives are the template text, and
`eval.json` scores the mock judge. For how the real judges rule, run
`TestLiveJudges`, as `../README.md` describes.

| File | Call |
| --- | --- |
| `samples.json` | `GET /v1/samples` |
| `upload-malformed.json` | `GET /v1/uploads/{id}`, stopped on malformed lines |
| `upload-completed.json` | `GET /v1/uploads/{id}`, finished |
| `uploads-page.json` | `GET /v1/uploads` |
| `progress-stream.json` | The first two frames and the last of `GET .../progress` |
| `incidents.json` | `GET /v1/uploads/{id}/incidents` |
| `incident-detail.json` | `GET /v1/incidents/{id}`, the top incident |
| `incident-context.json` | `GET /v1/incidents/{id}/context` |
| `timeseries-1h.json` | `GET .../timeseries?bucket=1h` |
| `events-page.json` | `GET .../events?limit=3&hasAnomaly=true&user=dana.k&host=files-sync-cdn.net` |
| `anomalies-page.json` | `GET .../anomalies?limit=5` |
| `eval.json` | `GET .../eval` |
