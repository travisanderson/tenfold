"use client";
import { useCallback, useState } from "react";
import { NavigationLink as Link } from "./navigation-link";
import { useSearchParams } from "next/navigation";
import { anomalyPage } from "@/lib/api/actions";
import { unwrap } from "@/lib/api/result";
import { SkeletonRows } from "./skeleton";
import { usePagedData } from "./use-paged-data";
import type { Schema } from "@/lib/types";
import { anomalyFixtures } from "@/lib/evaluation-fixtures";
import { useWorkspace } from "./workspace";
import { words, time } from "@/lib/format";
export function Anomalies() {
  const { zone, selectedUploadId, live, demo } = useWorkspace();
  const query = useSearchParams();
  const upload = query.get("upload") ?? selectedUploadId;
  const [filter, setFilter] = useState(
    query.get("judgesDisagree") === "true" ? "disagree" : "all",
  );
  const fetchPage = useCallback(
    async (cursor?: string) =>
      unwrap(
        await anomalyPage(upload, {
          limit: 200,
          cursor,
          disposition:
            filter !== "all" && filter !== "disagree"
              ? (filter as Schema["AnomalyDisposition"])
              : undefined,
          judgesDisagree: filter === "disagree" || undefined,
        }),
      ),
    [upload, filter],
  );
  const remote = usePagedData(live?.anomalies, fetchPage, !demo);
  const replacing = !demo && remote.busy && !remote.loadingMore;
  const rows = demo
    ? anomalyFixtures.filter(
        (a) =>
          filter === "all" ||
          (filter === "disagree" &&
            a.decision?.label !== a.shadowDecision?.label) ||
          a.disposition === filter,
      )
    : remote.page.items;
  return (
    <>
      <Link href={`/triage?upload=${upload}`}>← Triage</Link>
      <h1 className="section">What was set aside, and why.</h1>
      <p>Measured risk, model assessments, and disposition remain separate.</p>
      <label className="anomaly-filter">
        Show{" "}
        <select value={filter} onChange={(e) => setFilter(e.target.value)}>
          <option value="all">All anomalies</option>
          <option value="below_threshold">Below threshold</option>
          <option value="judged_benign">Judged benign</option>
          <option value="lesson">Set aside by a lesson</option>
          <option value="disagree">Judges disagree</option>
        </select>
      </label>
      {!demo && remote.error && (
        <p role="alert">
          {remote.error} <button onClick={remote.retry}>Try again</button>
        </p>
      )}
      {!demo && remote.busy && (
        <p className="sr-only" role="status">
          Loading anomalies…
        </p>
      )}
      <div className="panel table-scroll section" aria-busy={remote.busy}>
        <table>
          <thead>
            <tr>
              <th>Time</th>
              <th>Measured finding</th>
              <th>Risk</th>
              <th>Disposition</th>
              <th>AI assessment</th>
              <th>Shadow assessment</th>
              <th>Evidence</th>
            </tr>
          </thead>
          <tbody>
            {replacing && <SkeletonRows columns={7} />}
            {!replacing &&
              rows.map((a) => (
                <tr key={a.id}>
                  <td className="mono">{time(a.firstSeen, zone)}</td>
                  <td>{a.reason}</td>
                  <td className="mono">{a.riskScore}/100</td>
                  <td>{words(a.disposition)}</td>
                  <td>
                    {a.decision ? words(a.decision.label) : "Not available"}
                  </td>
                  <td>
                    {a.shadowDecision
                      ? words(a.shadowDecision.label)
                      : "Not available"}
                  </td>
                  <td>
                    <Link
                      className="mono"
                      href={`/events?upload=${upload}&q=${encodeURIComponent(a.eventIds.map((id) => `id:${id}`).join(" OR "))}`}
                    >
                      {a.eventIds.join(", ")}
                    </Link>
                  </td>
                </tr>
              ))}
            {!demo && remote.loadingMore && (
              <SkeletonRows columns={7} rows={2} />
            )}
          </tbody>
        </table>
        {!replacing && !rows.length && (
          <p className="empty">No anomalies match this filter.</p>
        )}
      </div>
      {!demo && (
        <div className="actions">
          <span>
            {rows.length} of {remote.page.total} anomalies
          </span>
          {remote.page.nextCursor && (
            <button disabled={remote.busy} onClick={remote.more}>
              Load more anomalies
            </button>
          )}
        </div>
      )}
    </>
  );
}
