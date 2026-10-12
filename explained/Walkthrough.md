# Walkthrough

The written version of the video. The slides are in
[Slides.pdf](./Slides.pdf). To follow along, open
[tenfold-ui.vercel.app](https://tenfold-ui.vercel.app) and sign in.

### 1. Load a day of logs

Go to **Uploads** and press **Load demo data**. It loads one company's day
of web-proxy traffic: 48,002 lines, with four attacks planted in it.

### 2. It stops on lines it cannot read

48 of the 48,002 lines are broken. Tenfold counts them, shows them, and
waits. It does not guess. Press **Analyze the good lines**.

### 3. The pipeline runs

The stages light up in turn: read, detect, group, judge, narrate. It
takes well under a minute. The result: **5 incidents**.

### 4. Triage

Press **Open triage**. One day of logs is now five cards, worst first. Four
are the planted attacks. The fifth is a nightly backup that looks like one.

### 5. Open the top incident

It is a large upload to `files-sync-cdn.net` at two in the morning, with a
risk score of 91. Three things are on the screen:

- **Why it was flagged.** Four measured signals, each with its weight:
  +34, +22, +19 and +16. They add up to the score. No model wrote this
  panel.
- **What the AI says.** A short report. Every sentence ends in a chip that
  names the log lines it rests on.
- **The evidence.** The events themselves, and a graph of who talked to
  what.

### 6. Check the AI's work

Click a chip. The cited events light up in the evidence table. If a
sentence cannot point at real lines, it never reaches this screen.

### 7. Decide

Assign the incident to yourself, add a note, and press **Confirm
malicious**. It moves to the **Decided** tab with its evidence.

### 8. Teach it once

Open the backup incident and press **Dismiss as false positive**. Tenfold
offers to remember: a
narrow rule for that one pattern, with an expiry date, so it cannot hide a
real attack forever.

### 9. Search the raw events

**Events** holds every line that was read. Search `domain:files-sync-cdn.net`
and the 21 requests behind the first incident come back.

### 10. Scorecard

The demo file has an answer key. **Scorecard** shows what was planted, what
was caught, and how the AI judge ruled on each one.

### What I would do next

This was a one day vibe-coded build. Obviously lots could be done, given more time. The constraints were what they were.

Here are some things I would do next:

1. **Memory across days.** Today each file is judged alone, so a slow
   attacker can hide inside one day's noise. This also fixes the beacon
   rule, which caught 11 of 50 in testing.
2. **Group by destination.** A fast update agent raises one finding per
   workstation. Fleet-wide polling to one host should be a single finding.
3. **Learn from verdicts.** Every decision is stored with its signals.
   First show the judge past verdicts on similar cases, then learn the
   weights once there are enough labels.
4. **A job queue.** One process runs two jobs at a time and holds each
   upload in memory.
5. **Small fixes.** Reconnect the progress stream on its own after 300
   seconds, show the "waiting for another analysis" message on the uploads
   screen, and add a per-user limit on AI spend.

The numbers behind items 1 and 2 are in the
[evaluation](../backend/README.md#evaluation).
