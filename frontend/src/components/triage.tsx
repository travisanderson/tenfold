"use client";
import Link from "next/link";
import { redirect, useRouter, useSearchParams } from "next/navigation";
import { useWorkspace } from "./workspace";
import { UploadState } from "./upload-state";
import { RiskTimeline, VolumeStrip } from "./charts";
import { riskSeries, volumeSeries } from "@/lib/incident-fixtures";
import { date, number, time, words } from "@/lib/format";
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
  const upload = state.uploads.find(
    (u) => u.id === (query.get("upload") ?? state.selectedUploadId),
  );
  if (!upload) redirect("/uploads");
  const incidents = state.incidents.filter((i) => i.uploadId === upload.id);
  if (upload.status !== "completed")
    return (
      <UploadState
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
  return (
    <>
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
              The line is the highest risk score in each 10 minutes. The grey
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
                      {time(i.timeRange.start, state.zone)}–
                      {time(i.timeRange.end, state.zone)} · {i.markerDetail}
                    </small>
                  </span>
                </button>
              ))}
            </div>
            <RiskTimeline
              series={riskSeries}
              incidents={incidents}
              threshold={upload.riskThreshold}
              zone={state.zone}
              onOpen={(id) =>
                router.push(`/incidents/${id}?upload=${upload.id}`)
              }
            />
          </div>
          <VolumeStrip series={volumeSeries} zone={state.zone} />
          <p className="caption">Bars: event volume per hour</p>
          <section className="incident-cards section">
            {incidents.map((i, index) => (
              <article key={i.id} className="panel incident-card">
                <div className="card-top">
                  <span className={`marker ${i.severity}`}>{index + 1}</span>
                  <Severity value={i.severity} />
                  <small className="mono">
                    {time(i.timeRange.start, state.zone)}–
                    {time(i.timeRange.end, state.zone)}
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
                  Owner: {i.assignee?.name ?? "unassigned"} ·{" "}
                  {i.analystVerdict.status === "open"
                    ? "New"
                    : words(i.analystVerdict.status)}
                </p>
                <Link
                  className={`button ${index === 0 ? "primary" : "accent-button"}`}
                  href={`/incidents/${i.id}?upload=${upload.id}`}
                >
                  Open incident
                </Link>
              </article>
            ))}
          </section>
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
