"use client";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useMemo } from "react";
import {
  useReactTable,
  getCoreRowModel,
  flexRender,
  type ColumnDef,
} from "@tanstack/react-table";
import type { Schema } from "@/lib/types";
import { evaluation } from "@/lib/evaluation-fixtures";
import { number, words, date } from "@/lib/format";
import { useWorkspace } from "./workspace";
export function Scorecard() {
  const { zone, uploads, incidents, selectedUploadId } = useWorkspace();
  const params = useSearchParams();
  const id = params.get("upload") ?? selectedUploadId;
  const upload = uploads.find((u) => u.id === id);
  const report = useMemo(
    () => ({
      ...evaluation,
      uploadId: id,
      scoresheet: evaluation.scoresheet.map((row) => ({
        ...row,
        incidentId: row.incidentId
          ? incidents.find((i) => i.uploadId === id && i.title === row.title)
              ?.id
          : undefined,
      })),
    }),
    [id, incidents],
  );
  const columns = useMemo<ColumnDef<Schema["ScoresheetRow"]>[]>(
    () => [
      {
        id: "incident",
        header: "Incident",
        cell: ({ row: { original: r } }) => (
          <>
            <strong>
              {r.incidentId ? (
                <Link href={`/incidents/${r.incidentId}?upload=${id}`}>
                  {r.title}
                </Link>
              ) : (
                r.title
              )}
            </strong>
            <small className="mono">{r.subtitle}</small>
          </>
        ),
      },
      {
        id: "rules",
        header: "Rules",
        cell: ({ row }) => (
          <span className="checkmark">
            {row.original.layers.includes("rules") ? "✓" : "–"}
          </span>
        ),
      },
      {
        id: "baselines",
        header: "Baselines",
        cell: ({ row }) => (
          <span className="checkmark">
            {row.original.layers.includes("baselines") ? "✓" : "–"}
          </span>
        ),
      },
      {
        id: "assessment",
        header: "Risk · AI assessment",
        cell: ({ row: { original: r } }) => (
          <span className="mono">
            {r.riskScore === undefined ? "—" : `Risk ${r.riskScore}`} ·{" "}
            {r.aiVerdict ? words(r.aiVerdict) : "not available"}
          </span>
        ),
      },
      { accessorKey: "answerKey", header: "Answer key" },
      {
        id: "mark",
        header: "Mark",
        cell: ({ row: { original: r } }) => (
          <span className={`score-mark ${r.mark}`}>
            {r.mark === "caught" && r.aiVerdict === "inconclusive"
              ? "Caught, sent to you"
              : words(r.mark)}
          </span>
        ),
      },
    ],
    [id],
  );
  const table = useReactTable({
    data: report.scoresheet,
    columns,
    getCoreRowModel: getCoreRowModel(),
  });
  if (upload?.source !== "sample")
    return (
      <section className="panel">
        <h1>This upload has no answer key.</h1>
        <p>Load demo data to compare detection results with planted attacks.</p>
        <Link className="button primary" href="/uploads">
          Open uploads
        </Link>
      </section>
    );
  const total = report.truePositives + report.falseNegatives;
  const tiles = [
    {
      label: "Attacks caught",
      value: report.truePositives,
      denominator: total,
      copy: report.falseNegatives
        ? "Planted attacks that became incidents."
        : "Every planted attack became an incident.",
      tone: "accent",
    },
    {
      label: "Attacks missed",
      value: report.falseNegatives,
      copy: report.falseNegatives
        ? "Planted attacks that did not become incidents."
        : "Nothing in the answer key was set aside.",
      tone: "",
    },
    {
      label: "False positives",
      value: report.falsePositives,
      denominator: report.incidentCount,
      copy: "Incidents raised that the answer key calls harmless.",
      tone: "warning",
    },
    {
      label: "Noise removed",
      value: report.incidentCount,
      copy: "Incidents an analyst has to read.",
      tone: "",
    },
  ];
  return (
    <>
      <span className="eyebrow">Detection scorecard</span>
      <div className="score-heading">
        <h1>Demo data, marked against its answer key.</h1>
        <small className="mono">
          {number(report.lineCount)} lines · {total} planted attacks ·{" "}
          {date(report.runAt, zone)}
        </small>
      </div>
      <section className="score-tiles">
        {tiles.map((t) => (
          <article key={t.label} className={`panel score-tile ${t.tone}`}>
            <h3>{t.label}</h3>
            <div className="score-value">
              {t.value}
              {t.denominator !== undefined && <span>/{t.denominator}</span>}
            </div>
            {t.denominator !== undefined && (
              <div className="score-segments">
                {Array.from({ length: t.denominator }, (_, i) => (
                  <span key={i} className={i < t.value ? "filled" : ""} />
                ))}
              </div>
            )}
            {t.label === "Noise removed" && (
              <p className="mono">
                {number(report.lineCount)} lines → {report.anomalyCount}{" "}
                anomalies → {report.incidentCount}
              </p>
            )}
            <p>{t.copy}</p>
          </article>
        ))}
      </section>
      <section className="panel section scoresheet">
        <div className="section-heading">
          <h2>Scoresheet</h2>
          <span>
            A tick means that layer flagged it. Missed attacks also appear.
          </span>
        </div>
        <div className="table-scroll">
          <table>
            <thead>
              {table.getHeaderGroups().map((g) => (
                <tr key={g.id}>
                  {g.headers.map((h) => (
                    <th key={h.id}>
                      {flexRender(h.column.columnDef.header, h.getContext())}
                    </th>
                  ))}
                </tr>
              ))}
            </thead>
            <tbody>
              {table.getRowModel().rows.map((r) => (
                <tr key={r.id}>
                  {r.getVisibleCells().map((c) => (
                    <td key={c.id}>
                      {flexRender(c.column.columnDef.cell, c.getContext())}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
      <section className="panel section">
        <div className="section-heading">
          <h2>Judge against judge</h2>
          <span>
            Both models ruled on {report.judgeAgreement.total} anomalies. They
            agreed on {report.judgeAgreement.agreed}, or{" "}
            {report.judgeAgreement.total
              ? Math.round(
                  (100 * report.judgeAgreement.agreed) /
                    report.judgeAgreement.total,
                )
              : 0}
            %.
          </span>
          <Link href={`/anomalies?upload=${id}&judgesDisagree=true`}>
            See the {report.judgeAgreement.total - report.judgeAgreement.agreed}{" "}
            they split on
          </Link>
        </div>
        <div className="judges">
          {report.providers.map((p) => (
            <article className={`judge ${p.role}`} key={p.provider}>
              <header>
                <h2>{p.label}</h2>
                <span className="pill eyebrow">
                  {p.role === "in_use" ? "In use" : "Shadow run"}
                </span>
              </header>
              <div className="judge-metrics">
                <div>
                  <strong>
                    {p.attacksCaught}/{p.attacksTotal}
                  </strong>
                  <small>caught</small>
                </div>
                <div>
                  <strong>{p.falsePositives}</strong>
                  <small>false positives</small>
                </div>
                <div>
                  <strong>
                    {number(p.avgLatencyMs)}
                    <small> ms</small>
                  </strong>
                  <small>latency</small>
                </div>
                <div>
                  <strong>${p.costPer1kUsd.toFixed(2)}</strong>
                  <small>per 1,000</small>
                </div>
              </div>
            </article>
          ))}
        </div>
      </section>
    </>
  );
}
