"use client";
import { useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { anomalyFixtures } from "@/lib/evaluation-fixtures";
import { useWorkspace } from "./workspace";
import { words, time } from "@/lib/format";
export function Anomalies() {
  const { zone, selectedUploadId } = useWorkspace();
  const query = useSearchParams();
  const upload = query.get("upload") ?? selectedUploadId;
  const [filter, setFilter] = useState(
    query.get("judgesDisagree") === "true" ? "disagree" : "all",
  );
  const rows = anomalyFixtures.filter(
    (a) =>
      filter === "all" ||
      (filter === "disagree" &&
        a.decision?.label !== a.shadowDecision?.label) ||
      a.disposition === filter,
  );
  return (
    <>
      <Link href={`/triage?upload=${upload}`}>← Triage</Link>
      <h1 className="section">What was set aside, and why.</h1>
      <p>Measured risk, model assessments, and disposition remain separate.</p>
      <label className="anomaly-filter">
        Show{" "}
        <select value={filter} onChange={(e) => setFilter(e.target.value)}>
          <option value="all">All sample anomalies</option>
          <option value="below_threshold">Below threshold</option>
          <option value="judged_benign">Judged benign</option>
          <option value="lesson">Set aside by a lesson</option>
          <option value="disagree">Judges disagree</option>
        </select>
      </label>
      <div className="panel table-scroll section">
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
            {rows.map((a) => (
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
          </tbody>
        </table>
        {!rows.length && (
          <p className="empty">No anomalies match this filter.</p>
        )}
      </div>
    </>
  );
}
