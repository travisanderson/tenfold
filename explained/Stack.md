# Stack

Where the build ended up, and why. The decisions come first, because the
parts list is the least interesting thing here.

## The parts

| Part | Built with | Runs on |
| --- | --- | --- |
| Screen | Next.js 16, React 19, TypeScript | Vercel |
| API and pipeline | Go 1.27 | Railway |
| Database, sign-in, files | Postgres, Auth, Storage | Supabase |
| Judge | Jev, TypeSafe's decision model | TypeSafe API |
| Narrator | Claude Opus | Anthropic API |
| Contract | One OpenAPI file, [`api/openapi.yaml`](../api/openapi.yaml) | Generates the types for both sides |

A request travels one way: browser → Next.js server → Go API → Supabase.

## The pipeline

1. **Read.** Each log line becomes one row. Broken lines are counted and
   shown, never guessed at.
2. **Detect.** Seven plain rules add fixed points for odd behavior.
3. **Group.** Findings that rest on the same lines become one story. Under
   30 points, a story is set aside.
4. **Judge.** Jev labels each story malicious, benign or unsure.
5. **Narrate.** Claude writes a short report. Every sentence cites log lines.
6. **Decide.** The analyst confirms, dismisses or escalates.

On the demo file, 48,002 lines become 5 incidents.

## Design decisions and tradeoffs

### 1. A decision model makes the decision

The judge has one job: answer a typed question, malicious or benign. That
is not a writing task, so it does not go to a writing model.

**Jev** is built for exactly this. It reads the facts and the question, and
returns a choice and a confidence. It writes no prose, so there is nothing
to parse and nothing to talk its way around. It answers in about 0.2
seconds. Below a confidence of 0.5, the code calls the story inconclusive;
the model does not get to.

Claude Opus does the one job that needs language: the report.

This split is the best decision in the build. Each model does what it is
good at, the judge is fast enough to run on every flagged story, and its
answer is a value the code can check.

- **Proof so far:** on three sample files, Jev ruled on 34 flagged
  behaviors and matched the answer key on all 34. So did Claude Haiku,
  running beside it as a second judge. The cases are synthetic and clear-cut.
- **Tradeoff:** two vendors and two keys. Jev cannot narrate.
- **Hedge:** each AI role has its own setting. A "shadow judge" can rule on
  the same stories without its answer counting, which is how a second model
  gets tried on real traffic before it is trusted.

### 2. The risk score is arithmetic

A score is the sum of the weights of the signals that fired, capped at 100.
The weights are constants in one file. No model touches the score, so an
analyst can trace any number to the rule behind it.

- **Tradeoff:** the weights are set by hand, and the rules miss what no
  rule describes. The beacon rule caught 11 of 50 in testing.

### 3. The AI is on a leash

- It can set a story aside only when the score is under 65.
- Every sentence it writes must cite lines in the incident. One bad
  citation, and a template replaces the report.
- If a model cannot be reached, the job still finishes, with scores and
  evidence intact.

- **Tradeoff:** more reaches the analyst. A nightly backup scores 69 and
  always shows up. The analyst can teach it once, with a narrow rule that
  expires.

### 4. Go for the pipeline, Next.js for the screen

Go parses and scores a 48,000-line file in seconds and ships as one small
binary. Next.js renders the screens on the server.

- **Tradeoff:** two languages and two deploys.

### 5. One contract keeps the two in step

Both sides generate their types from one OpenAPI file. CI fails if either
drifts from it.

- **Tradeoff:** every API change starts in the spec, which is slower than
  editing a handler.

### 6. The browser never calls the Go API

Sign-in tokens stay on the Next.js server. The browser talks only to
Next.js.

- **Tradeoff:** one more hop. The progress stream passes through Vercel,
  which cuts a connection at 300 seconds.

### 7. Files go straight to storage

The browser sends a log to Supabase Storage through a signed link, then
tells the API where it is. A large file never passes through a web server.

- **Tradeoff:** an upload is two steps, and the storage plan caps a file
  at 50 MB.

### 8. Each user sees only their own data

Every SQL query filters by the caller's user ID. Someone else's record and
a missing one get the same answer: 404. Row-level security is on with no
policies, which shuts Supabase's own REST door.

- **Tradeoff:** tenancy rests on each query being right, not on a database
  policy.

### 9. A job survives a restart

Each stage reads from and writes to Postgres. A job that stops, or a
deploy that restarts the server, resumes where it left off.

- **Tradeoff:** it is one process. Two jobs run at a time, and each holds
  its upload in memory. The next step is a queue.

### 10. "Normal" comes from the file itself

Baselines are computed from the one upload. Nothing needs to be trained or
stored first, so a new user gets results on the first file.

- **Tradeoff:** no memory across days. A slow, patient attacker can hide
  inside one day's noise.

### 11. Test data comes from a seed

A generator writes the logs and an answer key. The same seed gives the same
bytes, so every number in the evaluation can be rerun.

- **Tradeoff:** the same person wrote the attacks and the detectors. To
  offset that, the detectors were scored on 50 days drawn at random, with
  nothing tuned afterwards.

## Does it work?

On 50 unseen days the detectors caught 161 of 200 planted attacks. Three
kinds of attack were caught every time; the beacon was not. Apart from
decoys built to look suspicious, nothing was raised on ordinary traffic. The tables, the blind spots and the command that
reproduces them are in the
[evaluation](../backend/README.md#evaluation).

## Running it locally

`npx supabase start`, then `docker compose up`. The API applies its own
database schema at startup. The AI roles default to `mock`, an offline
stand-in, so a local run needs no keys. See
[`.env.example`](../.env.example) and
[`docker-compose.yml`](../docker-compose.yml).

## Read more

- [`backend/README.md`](../backend/README.md): the API, the AI layer, the evaluation.
- [`frontend/README.md`](../frontend/README.md): the screens and their checks.
- [`samples/README.md`](../samples/README.md): the log generator.
- [`Slides.pdf`](./Slides.pdf): the walkthrough deck.
