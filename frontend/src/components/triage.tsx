"use client";
import { useState } from "react";
import { NavigationLink as Link } from "./navigation-link";
import { redirect, useRouter, useSearchParams } from "next/navigation";
import { useWorkspace } from "./workspace";
import { LiveUploadState } from "./live-upload-state";
import { UploadState } from "./upload-state";
import { RiskTimeline, VolumeStrip } from "./charts";
import { riskSeries, volumeSeries } from "@/lib/incident-fixtures";
import { clock, date, number, words } from "@/lib/format";
import type { Schema } from "@/lib/types";
export function Severity({ value }: { value: Schema["Severity"] }) {
  return (
    <span className={`severity ${value}`}>
      {value === "medium" ? "■" : value === "low" ? "●" : "▲"} {value}
    </span>
  );
}
export function Triage() {
  const state = useWorkspace();
  const query = useSearchParams();
  const router = useRouter();
  const [chosenTab, setTab] = useState<"open" | "decided" | null>(null);
  const upload = state.uploads.find(
    (u) => u.id === (query.get("upload") ?? state.selectedUploadId),
  );
  if (!upload) redirect("/uploads");
  const incidents = (state.live?.summaries ?? state.incidents).filter(
    (i) => i.uploadId === upload.id,
  );
  if (
    upload.status !== "completed" &&
    !(upload.status === "partial" && upload.stop?.reason !== "parse_errors")
  )
    return !state.demo ? (
      <LiveUploadState upload={upload} />
    ) : (
      <UploadState
        modal
        upload={upload}
        resume={() =>
          state.setUploads((items) =>
            items.map((u) =>
              u.id === upload.id
                ? { ...u, status: "processing", stop: undefined }
                : u,
            ),
          )
        }
        cancel={() => {
          state.setUploads((items) => items.filter((u) => u.id !== upload.id));
          router.push("/uploads");
        }}
      />
    );
  const decided = incidents.filter(
    (i) => i.analystVerdict.status !== "open",
  ).length;
  const open = incidents.length - decided;
  // Open work comes first. With nothing left open, show what was decided.
  const tab = chosenTab ?? (open || !decided ? "open" : "decided");
  const justDecided = incidents.find((i) => i.id === query.get("decided"));
  return (
    <>
      {!state.demo && upload.status === "partial" && (
        <LiveUploadState upload={upload} />
      )}
      <section className="upload-bar">
        <span className="eyebrow pill">Latest upload</span>
        <label>
          Showing{" "}
          <select
            value={upload.id}
            onChange={(e) => {
              state.setSelectedUploadId(e.target.value);
              router.push(`/triage?upload=${e.target.value}`);
            }}
          >
            {state.uploads.map((u) => (
              <option key={u.id} value={u.id}>
                {u.filename}
              </option>
            ))}
          </select>
        </label>
        <span>
          Uploaded{" "}
          <span className="mono">{date(upload.createdAt, state.zone)}</span>
        </span>
        {!!upload.parseErrorCount && (
          <span className="partial-badge">
            ▲ Partial read · {number(upload.parseErrorCount)} malformed lines
            skipped
          </span>
        )}
        <Link href="/uploads">all uploads</Link>
      </section>
      <section className="hero triage-hero">
        <div className="hero-number">{number(incidents.length)}</div>
        <div>
          <h1>moments crossed the anomaly threshold.</h1>
          <p>
            The rest of the day stayed under it. {number(upload.eventCount)}{" "}
            events, {number(upload.anomalyCount)} anomalies,{" "}
            {number(incidents.length)} incidents.
          </p>
        </div>
        <div className="stats">
          <div>
            <strong>{number(upload.userCount)}</strong>
            <small>users</small>
          </div>
          <div>
            <strong>{number(upload.hostCount)}</strong>
            <small>hosts</small>
          </div>
          <div>
            <strong>
              {decided}/{incidents.length}
            </strong>
            <small>decided</small>
          </div>
        </div>
      </section>
      {incidents.length > 0 ? (
        <>
          <div className="section-heading">
            <strong>
              Peak risk score · times in{" "}
              {state.zone === "UTC" ? "UTC" : "your selected zone"}
            </strong>
            <span>
              The line is the highest risk score in each 10 minutes. The gray
              band is everything under the threshold of {upload.riskThreshold}.
            </span>
          </div>
          <div className="timeline-wrap">
            <div className="timeline-callouts">
              {incidents.map((i, n) => (
                <button
                  key={i.id}
                  onClick={() =>
                    router.push(`/incidents/${i.id}?upload=${upload.id}`)
                  }
                >
                  <span className={`marker ${i.severity}`}>{n + 1}</span>
                  <span>
                    <strong>{i.markerLabel}</strong>
                    <small className="mono">
                      {clock(i.timeRange.start, state.zone)}–
                      {clock(i.timeRange.end, state.zone)} · {i.markerDetail}
                    </small>
                  </span>
                </button>
              ))}
            </div>
            <RiskTimeline
              series={state.live?.riskSeries ?? riskSeries}
              incidents={incidents}
              threshold={upload.riskThreshold}
              zone={state.zone}
              onOpen={(id) =>
                router.push(`/incidents/${id}?upload=${upload.id}`)
              }
            />
          </div>
          <VolumeStrip
            series={state.live?.volumeSeries ?? volumeSeries}
            zone={state.zone}
          />
          <p className="caption">Bars: event volume per hour</p>
          {justDecided && (
            <p className="decision-saved" role="status">
              Decision saved: <strong>{justDecided.title}</strong> is{" "}
              {words(justDecided.analystVerdict.status)}.{" "}
              {open
                ? `${number(open)} still open.`
                : "Every incident is decided."}
            </p>
          )}
          <div className="tabs section" role="tablist" aria-label="Incidents">
            {(
              [
                ["open", "Open", open],
                ["decided", "Decided", decided],
              ] as const
            ).map(([id, label, count]) => (
              <button
                key={id}
                role="tab"
                aria-selected={tab === id}
                onClick={() => setTab(id)}
              >
                {label} <span className="mono">{count}</span>
              </button>
            ))}
          </div>
          <section className="incident-cards">
            {incidents.map((i, index) =>
              (i.analystVerdict.status === "open") !==
              (tab === "open") ? null : (
                <article
                  key={i.id}
                  className={`panel incident-card ${i.analystVerdict.status}`}
                >
                  <div className="card-top">
                    <span className={`marker ${i.severity}`}>{index + 1}</span>
                    <Severity value={i.severity} />
                    {i.analystVerdict.status !== "open" && (
                      <span
                        className={`verdict-badge ${i.analystVerdict.status}`}
                      >
                        {words(i.analystVerdict.status)}
                      </span>
                    )}
                    <small className="mono">
                      {clock(i.timeRange.start, state.zone)}–
                      {clock(i.timeRange.end, state.zone)}
                    </small>
                  </div>
                  <h2>{i.title}</h2>
                  <div className="card-risk">
                    <span className="risk-number">
                      {i.riskScore}
                      <small>/100 risk</small>
                    </span>
                    <span>
                      AI assessment:{" "}
                      {i.aiVerdict ? words(i.aiVerdict) : "not available"}
                    </span>
                  </div>
                  <div className="ai-panel">
                    <span className="eyebrow">AI interpretation</span>
                    <p>{i.summary ?? "Not available for this incident."}</p>
                  </div>
                  <p className="mono caption">
                    {i.principal.user} · {i.principal.host} ·{" "}
                    {number(i.eventCount)} events ·{" "}
                    {i.mitre.map((m) => `Possible ${m.id}`).join(" · ")}
                  </p>
                  <p className="caption">
                    Owner: {i.assignee?.name ?? "unassigned"}
                  </p>
                  {/* The link covers the whole card; see .incident-card in the CSS. */}
                  <Link
                    className="button accent-button"
                    href={`/incidents/${i.id}?upload=${upload.id}`}
                  >
                    {i.analystVerdict.status === "open"
                      ? "Open incident"
                      : "Review incident"}
                  </Link>
                </article>
              ),
            )}
          </section>
          {!(tab === "open" ? open : decided) && (
            <p className="panel caption">
              {tab === "open"
                ? "Nothing is open. Every incident in this upload has a decision."
                : "No decisions yet. Open an incident to confirm, dismiss or escalate it."}
            </p>
          )}
        </>
      ) : (
        <div className="panel">
          <h2>No incidents were raised.</h2>
          <p>
            That is a result, not an error. Review the anomalies and their
            scores below.
          </p>
        </div>
      )}
      <footer className="page-footer">
        <span>Every incident links back to its observed evidence.</span>
        <Link href={`/anomalies?upload=${upload.id}`}>
          See anomalies set aside, and why
        </Link>
        <Link href={`/events?upload=${upload.id}`}>
          Open all {number(upload.eventCount)} raw events
        </Link>
      </footer>
    </>
  );
}
