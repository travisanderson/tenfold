"use client";
import { useCallback, useState } from "react";
import { NavigationLink as Link } from "./navigation-link";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { useWorkspace } from "./workspace";
import { ResponsiveDisclosure } from "./responsive-disclosure";
import { TeachDialog } from "./teach-dialog";
import { Severity } from "./triage";
import { IncidentChart } from "./charts";
import { ActivityGraph } from "./activity-graph";
import { EvidenceTable, CopyButton } from "./evidence";
import { saveVerdict, assignIncident, addNote } from "@/lib/api/actions";
import { unwrap } from "@/lib/api/result";
import {
  safeError,
  citationLabel,
  clock,
  date,
  time,
  words,
} from "@/lib/format";
import type { Schema } from "@/lib/types";
export function IncidentScreen() {
  const {
    incidents,
    setIncidents,
    setLessons,
    zone,
    selectedUploadId,
    live,
    demo,
  } = useWorkspace();
  const params = useParams<{ screen: string[] }>();
  const query = useSearchParams();
  const router = useRouter();
  const incident = incidents.find((i) => i.id === params.screen[1]);
  const [teaching, setTeaching] = useState(false);
  const [selected, setSelected] = useState<string[]>([]);
  const [note, setNote] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const select = useCallback((ids: string[]) => setSelected(ids), []);
  if (!incident)
    return (
      <div className="panel">
        <h1>Incident not found.</h1>
        <Link href="/triage">Return to triage</Link>
      </div>
    );
  const upload = incident.uploadId ?? query.get("upload") ?? selectedUploadId;
  const summaries = (live?.summaries ?? incidents).filter(
    (i) => i.uploadId === incident.uploadId,
  );
  async function run(action: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await action();
    } catch (e) {
      setError(safeError(e));
    } finally {
      setBusy(false);
    }
  }
  const verdict = async (
    status: Schema["VerdictRequest"]["status"],
    lesson?: Schema["LessonCreate"],
  ) => {
    if (!demo) {
      const saved = unwrap(await saveVerdict(incident.id, { status, lesson }));
      setIncidents((items) =>
        items.map((i) => (i.id === incident.id ? { ...i, ...saved } : i)),
      );
    } else {
      setIncidents((items) =>
        items.map((i) =>
          i.id === incident.id
            ? {
                ...i,
                analystVerdict: { status, decidedAt: new Date().toISOString() },
              }
            : i,
        ),
      );
    }
    // The decision is recorded. Back to triage, where the card now shows
    // it and the rest of the open incidents wait.
    router.push(`/triage?upload=${upload}&decided=${incident.id}`);
  };
  return (
    <div className="incident-screen">
      <div className="incident-toolbar">
        <div>
          Owner <strong>{incident.assignee?.name ?? "Unassigned"}</strong>{" "}
          <button
            className="small-button"
            disabled={busy}
            onClick={() =>
              void run(async () => {
                if (!demo) {
                  const saved = unwrap(
                    await assignIncident(incident.id, !incident.assignee),
                  );
                  setIncidents((items) =>
                    items.map((i) =>
                      i.id === incident.id
                        ? { ...i, ...saved, assignee: saved.assignee }
                        : i,
                    ),
                  );
                } else {
                  setIncidents((items) =>
                    items.map((i) =>
                      i.id === incident.id
                        ? {
                            ...i,
                            assignee: i.assignee
                              ? undefined
                              : {
                                  id: "5a1c3e7b-9d2f-4b6a-8c0e-1f3a5b7d9e2c",
                                  name: "sam.r",
                                },
                          }
                        : i,
                    ),
                  );
                }
              })
            }
          >
            {incident.assignee ? "Unassign" : "Assign to me"}
          </button>
        </div>
        <div>
          Status{" "}
          <strong>
            {incident.analystVerdict.status === "open"
              ? "New"
              : words(incident.analystVerdict.status)}
          </strong>
        </div>
        <span>All times {zone === "UTC" ? "UTC" : "Eastern"}</span>
        <div className="indicators">
          Indicators{" "}
          {incident.indicators.map((i) => (
            <span key={i.value}>
              <span className="mono">{i.value}</span>{" "}
              <CopyButton text={i.value} />
              <Link
                href={`/events?upload=${upload}&q=${encodeURIComponent(`${i.type === "user" ? "user" : i.type === "ip" ? "ip" : "domain"}:${i.value}`)}`}
              >
                all {i.eventCount} events
              </Link>
            </span>
          ))}
        </div>
      </div>
      <Link className="back-link" href={`/triage?upload=${upload}`}>
        ← Triage · incident{" "}
        {summaries.findIndex((i) => i.id === incident.id) + 1} of{" "}
        {summaries.length}
      </Link>
      <header className="incident-heading">
        <h1>{incident.title}</h1>
        <div className="mobile-risk-summary">
          <div>
            <strong>{incident.riskScore}</strong>
            <span className="mono"> /100 risk</span>
          </div>
          <p>
            AI assessment:{" "}
            {incident.aiVerdict ? words(incident.aiVerdict) : "not available"}
          </p>
        </div>
        <div className="incident-heading-row">
          <div>
            <div className="badges">
              <Severity value={incident.severity} />
              <span className="badge">Risk score {incident.riskScore}/100</span>
              <span className="badge">
                AI assessment:{" "}
                {incident.aiVerdict
                  ? words(incident.aiVerdict)
                  : "not available"}
              </span>
            </div>
            <div className="badges mitre">
              {incident.mitre.map((m) => (
                <span className="badge mono" key={m.id}>
                  Possible {m.id} {m.name}
                </span>
              ))}
            </div>
          </div>
          <div className="actions verdict-actions">
            <button
              className="primary"
              disabled={busy}
              onClick={() => void run(() => verdict("confirmed"))}
            >
              Confirm malicious
            </button>
            <button disabled={busy} onClick={() => setTeaching(true)}>
              Dismiss as false positive
            </button>
            <button
              disabled={busy}
              onClick={() => void run(() => verdict("escalated"))}
            >
              Escalate
            </button>
          </div>
        </div>
      </header>
      {error && (
        <p className="notice" role="alert">
          {error}
        </p>
      )}
      {message && (
        <p className="status-message" role="status">
          {message}
        </p>
      )}
      <ResponsiveDisclosure
        title="Request chart · bytes out"
        className="incident-chart-section section"
      >
        <section>
          <div className="section-heading">
            <strong>
              {incident.principal.user} · bytes out per request ·{" "}
              {clock(incident.chart.window.start, zone)} to{" "}
              {clock(incident.chart.window.end, zone)}
            </strong>
            <span>
              Each bar is one request.{" "}
              {incident.chart.baselineBytesOut === undefined
                ? "No baseline is available."
                : "The gray band is this user’s median request size in this file."}
            </span>
          </div>
          <IncidentChart
            incident={incident}
            selected={selected}
            select={select}
            zone={zone}
          />
        </section>
      </ResponsiveDisclosure>
      <ResponsiveDisclosure
        title={`Activity graph · ${incident.graph.nodes.length} nodes`}
        className="incident-graph-section section"
      >
        <section>
          <ActivityGraph
            graph={incident.graph}
            selected={selected}
            select={select}
          />
        </section>
      </ResponsiveDisclosure>
      <div className="analysis-grid section">
        <section className="signals-section">
          <div className="section-heading">
            <strong>Why flagged · measured</strong>
            <span>
              {incident.signals.length} measured signals. No model wrote this
              panel.
            </span>
          </div>
          <div className="panel signal-panel">
            <div className="risk-summary">
              <div className="risk-number">
                {incident.riskScore}
                <small>/100</small>
              </div>
              <div>
                <div className="risk-bar">
                  {incident.signals.map((s, i) => (
                    <span
                      key={s.name}
                      style={{
                        width: `${s.weight}%`,
                        background: `var(--color-accent${i % 4 ? `-${(i % 4) + 1}` : ""})`,
                      }}
                    >
                      {s.weight}
                    </span>
                  ))}
                </div>
                <p className="caption">
                  Fixed rules set each weight, and the weights add up to the
                  risk score. No model is involved. The AI assessment is shown
                  separately.
                </p>
              </div>
            </div>
            <div className="table-scroll">
              <table className="signals-table">
                <thead>
                  <tr>
                    <th>Weight</th>
                    <th>Layer</th>
                    <th>Signal</th>
                    <th>Observed</th>
                    <th>Baseline</th>
                  </tr>
                </thead>
                <tbody>
                  {incident.signals.map((s) => (
                    <tr
                      key={s.name}
                      className={
                        s.eventIds.length &&
                        s.eventIds.every((id) => selected.includes(id))
                          ? "selected-row"
                          : ""
                      }
                    >
                      <td className="signal-weight">+{s.weight}</td>
                      <td>{s.layer}</td>
                      <td>
                        <button
                          className="text-button signal-link"
                          onClick={() => select(s.eventIds)}
                          disabled={!s.eventIds.length}
                        >
                          {s.description}
                        </button>
                        {!s.eventIds.length && <small>Uncited</small>}
                      </td>
                      <td className="mono">
                        {s.observed}
                        <small>{s.observedDetail}</small>
                        <small className="mobile-baseline">
                          Baseline: {s.baselineText ?? "not available"}
                        </small>
                      </td>
                      <td className="mono">
                        {s.baselineText ?? "No baseline"}
                        <small>
                          {s.baselineDetail}
                          {s.baselineSource === "earlier_uploads" &&
                            ` · ${s.baselineUploadCount ?? 0} earlier uploads`}
                        </small>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </section>
        <aside className="narrative-section">
          <section className="ai-panel narrative">
            <span className="eyebrow">
              {incident.narrative.status === "template"
                ? "Template summary · cited evidence"
                : incident.narrative.provider === "mock"
                  ? "AI stand-in · cited evidence"
                  : "AI interpretation · every sentence cites its evidence"}
            </span>
            {incident.narrative.status === "unavailable" ? (
              <p>Not available for this incident.</p>
            ) : (
              incident.narrative.sentences.map((sentence, index) => (
                <p
                  key={index}
                  className={
                    sentence.citations.length &&
                    sentence.citations.every((id) => selected.includes(id))
                      ? "selected-sentence"
                      : ""
                  }
                >
                  {sentence.text}{" "}
                  {sentence.citations.length ? (
                    <button
                      className="citation mono"
                      onClick={() => select(sentence.citations)}
                      aria-pressed={sentence.citations.every((id) =>
                        selected.includes(id),
                      )}
                    >
                      {citationLabel(sentence.citations)}
                    </button>
                  ) : (
                    <span className="uncited">Uncited</span>
                  )}
                </p>
              ))
            )}
            <p className="citation-disclaimer">
              Each chip opens the cited events. A citation shows where a
              sentence came from. It does not prove the reading is right.
            </p>
          </section>
          <div className="handoff">
            <CopyButton
              label="Copy handoff note"
              text={`${incident.title}\nRisk score ${incident.riskScore}/100\n${incident.narrative.sentences.map((s) => `${s.text} [${s.citations.join(", ")}]`).join("\n")}`}
            />
          </div>
          <ResponsiveDisclosure
            title={`Notes · ${incident.notes.length}`}
            className="notes-disclosure"
          >
            <section className="panel notes-section">
              <h3>Analyst notes</h3>
              {incident.notes.map((n) => (
                <div className="analyst-note" key={n.id}>
                  <small className="mono">
                    {n.author} · {date(n.createdAt, zone)}
                  </small>
                  <p>{n.text}</p>
                </div>
              ))}
              {!incident.notes.length && <p>No analyst notes yet.</p>}
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  if (!note.trim()) return;
                  void run(async () => {
                    const saved = demo
                      ? {
                          id: crypto.randomUUID(),
                          author: "sam.r",
                          text: note.trim(),
                          createdAt: new Date().toISOString(),
                        }
                      : unwrap(await addNote(incident.id, note.trim()));
                    setIncidents((items) =>
                      items.map((i) =>
                        i.id === incident.id
                          ? { ...i, notes: [...i.notes, saved] }
                          : i,
                      ),
                    );
                    setNote("");
                    setMessage("Note saved.");
                  });
                }}
              >
                <label htmlFor="note">Add a note</label>
                <textarea
                  id="note"
                  value={note}
                  onChange={(e) => setNote(e.target.value)}
                  maxLength={2000}
                  rows={3}
                  placeholder="What you checked, who you called, what you decided"
                />
                <button
                  className="small-button"
                  disabled={busy || !note.trim()}
                >
                  Save note
                </button>
              </form>
            </section>
          </ResponsiveDisclosure>
        </aside>
      </div>
      <ResponsiveDisclosure
        title={`Evidence · ${incident.eventCount} events, ${selected.length} selected`}
        className="evidence-section section"
      >
        <section>
          <div className="section-heading">
            <strong>Evidence · {incident.eventCount} events · observed</strong>
            <Link href={`/events?upload=${upload}`}>
              Open in event explorer
            </Link>
          </div>
          <EvidenceTable
            events={incident.evidence}
            selected={selected}
            select={select}
            zone={zone}
          />
        </section>
      </ResponsiveDisclosure>
      <details className="timeline-details section">
        <summary>Timeline · {incident.timeline.length} steps</summary>
        <ol>
          {incident.timeline.map((item) => (
            <li key={item.eventId}>
              <button
                className="text-button"
                onClick={() => select([item.eventId])}
              >
                <span className="mono">
                  {time(item.ts, zone, true)} · {item.eventId}
                </span>{" "}
                {item.label}
              </button>
            </li>
          ))}
        </ol>
      </details>
      {teaching && (
        <TeachDialog
          incident={incident}
          close={() => setTeaching(false)}
          save={async (lesson) => {
            await verdict("dismissed", lesson);
            if (!demo) return;
            if (lesson) {
              setLessons((items) => [
                ...items,
                {
                  ...lesson,
                  id: crypto.randomUUID(),
                  createdAt: new Date().toISOString(),
                  matchCount: 0,
                },
              ]);
              setMessage(
                "Incident dismissed and lesson saved in this preview.",
              );
            }
          }}
        />
      )}
    </div>
  );
}
