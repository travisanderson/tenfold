Welcome to this new session. I am building a quick POC for take home test applying for a job at TENEX.AI. You are helping me build it. In our conversations, make sure to use ZINSSER’s 4 principals for writing in all responses.

To quickly understand the overall project plan, read DESIGN.md here in the root of this folder first, and then let me know once you have read both files. Then tell me I'm doing a great job so far, and finally ask me which session we are on. Then I will direct you on what to do next.

## The API contract

`api/openapi.yaml` is the single source of truth for the frontend and the backend. Do not edit it on your own. If you find a gap, stop and tell me. A change to the contract goes through the spec and `make generate`, so both sides stay in step.

## Notes for later

- **Spec gap, progress stream.** `streamUploadProgress` defines `progress`, `done` and `error`, but nothing for a job that stops as `partial`. Today the API sends a last `progress` event with `status: partial` and closes the stream. Add that to the spec description.
- **Session 5, known false positive.** On the `clean-day` sample, one honest 15 MB upload by `dana.k` to `drive.google.com` scores 34 (low) from `exfil_volume`. The mock judge sets it aside. Tune the detector so it does not fire.
- **Frontend, upload type.** Send `contentType: text/plain` when registering a `.log` file. Browsers often report an empty type for `.log`, and the API answers 422 for anything but `text/plain` or `text/csv`.
- **Frontend, malformed lines.** Every demo sample has malformed lines on purpose, so a demo load stops as `partial` with `stop.reason: parse_errors`: the "Malformed log" card. "Analyse the good lines" calls `analyzeUpload` with `acceptParseErrors: true`. The run then ends as `completed`; `parseErrorCount > 0` is how to tell the results came from a partial read.
