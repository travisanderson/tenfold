"use client";
import { useState, useMemo } from "react";
import { NavigationLink as Link } from "./navigation-link";
import { useRouter } from "next/navigation";
import { useDropzone } from "react-dropzone";
import {
  useReactTable,
  getCoreRowModel,
  flexRender,
  type ColumnDef,
} from "@tanstack/react-table";
import { useWorkspace } from "./workspace";
import { Pipeline } from "./pipeline";
import type { Upload } from "@/lib/types";
import { ClearDataDialog } from "./clear-data-dialog";
import { LiveUploadState } from "./live-upload-state";
import {
  registerUpload,
  analyzeUpload,
  deleteUpload,
  uploadPage,
} from "@/lib/api/actions";
import { unwrap } from "@/lib/api/result";
import { safeError, number, date, time } from "@/lib/format";
export function Uploads() {
  const {
    uploads,
    zone,
    loadDemo,
    setSelectedUploadId,
    selectedUploadId,
    setUploads,
    demo,
    live,
  } = useWorkspace();
  const router = useRouter();
  const [clearing, setClearing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [nextCursor, setNextCursor] = useState(live?.nextUploadCursor);
  const active = uploads.find((u) => u.id === selectedUploadId) ?? uploads[0];
  const sample = live?.samples.find((s) => s.name === "attack-day");
  const progress = active?.pipeline.find((s) => s.stage === active.stage);
  const stageNames = {
    uploaded: "Waiting to start",
    parsing: "Parsing",
    detecting: "Detecting",
    judging: "Judging",
    grouping: "Writing report",
    narrating: "Writing report",
    done: "Complete",
  };
  const stageNumbers = {
    uploaded: 0,
    parsing: 1,
    detecting: 2,
    judging: 3,
    grouping: 4,
    narrating: 4,
    done: 4,
  };
  const progressLabels = {
    uploaded: "items queued.",
    parsing: "lines parsed so far.",
    detecting: "events checked so far.",
    judging: "anomalies judged so far.",
    grouping: "anomalies grouped so far.",
    narrating: "incidents narrated so far.",
    done: "incidents ready for review.",
  };
  const dropzone = useDropzone({
    accept: { "text/plain": [".log"], "text/csv": [".csv"] },
    maxSize: 52428800,
    disabled: busy,
    multiple: false,
    onDropAccepted: async ([file]) => {
      if (demo) {
        setError("File analysis requires live mode.");
        return;
      }
      setBusy(true);
      setError("");
      let registeredId = "";
      try {
        const contentType = file.name.toLowerCase().endsWith(".csv")
          ? "text/csv"
          : "text/plain";
        const { upload, uploadUrl } = unwrap(
          await registerUpload({
            filename: file.name,
            sizeBytes: file.size,
            contentType,
          }),
        );
        registeredId = upload.id;
        const stored = await fetch(uploadUrl, {
          method: "PUT",
          headers: { "Content-Type": contentType },
          body: file,
        });
        if (!stored.ok)
          throw new Error(
            "The file could not be stored. Please choose it again.",
          );
        setUploads((items) => [upload, ...items]);
        setSelectedUploadId(upload.id);
        registeredId = ""; // Keep the stored file if starting analysis fails; it can be retried.
        const running = unwrap(await analyzeUpload(upload.id));
        setUploads((items) =>
          items.map((u) => (u.id === upload.id ? running : u)),
        );
      } catch (e) {
        if (registeredId)
          await deleteUpload(registeredId).catch(() => undefined);
        setError(safeError(e));
      } finally {
        setBusy(false);
      }
    },
    onDropRejected: () => setError("Choose a .log or .csv file up to 50 MB."),
    noClick: true,
    noKeyboard: true,
  });
  const columns = useMemo<ColumnDef<Upload>[]>(
    () => [
      {
        accessorKey: "filename",
        header: "File",
        cell: ({ row }) => (
          <span className="mono">
            {row.original.filename}
            {row.original.source === "sample" && <small> · sample</small>}
          </span>
        ),
      },
      {
        accessorKey: "createdAt",
        header: "Uploaded",
        cell: ({ getValue }) => (
          <span className="mono">{date(getValue<string>(), zone)}</span>
        ),
      },
      {
        accessorKey: "lineCount",
        header: "Lines",
        cell: ({ getValue }) => number(getValue<number>()),
      },
      {
        accessorKey: "anomalyCount",
        header: "Anomalies",
        cell: ({ getValue }) => number(getValue<number>()),
      },
      {
        accessorKey: "incidentCount",
        header: "Incidents",
        cell: ({ getValue }) => number(getValue<number>()),
      },
      {
        id: "status",
        header: "Status",
        cell: ({ row: { original: u } }) =>
          u.status === "processing" ? (
            <span className="accent">
              <span className="spinner" />
              Processing · {u.progress}%
            </span>
          ) : u.status !== "completed" ? (
            <button
              className="text-button"
              onClick={() => setSelectedUploadId(u.id)}
            >
              {u.status === "partial"
                ? "Needs attention"
                : u.status === "failed"
                  ? "Failed"
                  : "Waiting to start"}
            </button>
          ) : (
            <Link
              href={`/triage?upload=${u.id}`}
              onClick={() => setSelectedUploadId(u.id)}
            >
              {u.incidentCount
                ? `${(u.verdictCounts?.confirmed ?? 0) + (u.verdictCounts?.dismissed ?? 0) + (u.verdictCounts?.escalated ?? 0)} of ${u.incidentCount} decided`
                : "Clear · no incidents"}
            </Link>
          ),
      },
    ],
    [zone, setSelectedUploadId],
  );
  const table = useReactTable({
    data: uploads,
    columns,
    getCoreRowModel: getCoreRowModel(),
  });
  return (
    <>
      <section className="hero uploads-hero">
        {active && (
          <div className="hero-number">
            {active?.status === "processing"
              ? number(progress?.processed ?? 0)
              : number(active?.incidentCount ?? 0)}
            {active?.status === "processing" && (
              <span>/{number(progress?.countIn)}</span>
            )}
          </div>
        )}
        <div>
          <h1>
            {active?.status === "processing"
              ? progressLabels[active.stage]
              : active
                ? active.status === "completed"
                  ? "incidents ready for review."
                  : "Your upload needs attention."
                : "Your investigation starts here."}
          </h1>
          <p>
            {active ? (
              <>
                {" "}
                {active.status === "processing" && (
                  <span className="spinner accent" />
                )}
                {active.status === "processing"
                  ? `${stageNames[active.stage]} · Stage ${stageNumbers[active.stage]} of 4.`
                  : active.status === "completed"
                    ? "Analysis complete."
                    : active.status === "partial"
                      ? "Analysis paused."
                      : active.status === "failed"
                        ? "Analysis failed."
                        : "Ready to analyze."}{" "}
                {active.status === "completed"
                  ? "Tenfold read"
                  : "Tenfold is reading"}{" "}
                <span className="mono">{active.filename}</span>,{" "}
                {number(active.lineCount)} lines.{" "}
                {active.startedAt && `Started ${time(active.startedAt, zone)}.`}
              </>
            ) : (
              "Drop a day of Zscaler logs, or explore the sample."
            )}
          </p>
        </div>
      </section>
      {active && (
        <>
          <Pipeline upload={active} />
          <p className="caption">
            {active.status === "processing"
              ? "The moving line leads into the stage at work. Counts update as each stage runs."
              : active.status === "completed"
                ? "All stages finished. Open triage to review the results."
                : "Review the upload status below to continue."}
          </p>
        </>
      )}
      {!demo && active && active.status !== "processing" && (
        <LiveUploadState upload={active} />
      )}
      <div className="upload-grid section">
        <section
          {...dropzone.getRootProps()}
          className={`dropzone ${dropzone.isDragActive ? "dragging" : ""}`}
        >
          <input {...dropzone.getInputProps()} aria-label="Upload log files" />
          <h2>Drop {active ? "another" : "a"} day of Zscaler logs</h2>
          <p>NSS feed exports as .log or .csv, up to 50 MB each</p>
          <div className="actions">
            <button className="primary" disabled={busy} onClick={dropzone.open}>
              Choose files
            </button>
            <button
              className="accent-button"
              disabled={busy}
              onClick={async () => {
                setBusy(true);
                setError("");
                try {
                  const u = await loadDemo();
                  if (demo) router.push(`/triage?upload=${u.id}`);
                } catch (e) {
                  setError(safeError(e));
                } finally {
                  setBusy(false);
                }
              }}
            >
              {busy && <span className="spinner" />}
              {busy ? "Loading…" : "Load demo data"}
            </button>
          </div>
          <small className="mono">
            {demo
              ? "attack-day · 48,002 lines · 4 planted attacks · 3 decoys"
              : sample
                ? `${sample.name} · ${number(sample.lineCount)} lines · ${sample.description}`
                : "Load a bundled sample to try the investigation flow."}
          </small>
        </section>
        <section className="panel recent-uploads">
          <div className="section-heading">
            <h3>Recent uploads</h3>
            <button
              className="small-button"
              disabled={busy}
              onClick={() => setClearing(true)}
            >
              Clear data
            </button>
          </div>
          <div className="table-scroll">
            <table>
              <thead>
                {table.getHeaderGroups().map((group) => (
                  <tr key={group.id}>
                    {group.headers.map((h) => (
                      <th key={h.id}>
                        {flexRender(h.column.columnDef.header, h.getContext())}
                      </th>
                    ))}
                  </tr>
                ))}
              </thead>
              <tbody>
                {table.getRowModel().rows.map((row) => (
                  <tr key={row.id}>
                    {row.getVisibleCells().map((cell) => (
                      <td key={cell.id}>
                        {flexRender(
                          cell.column.columnDef.cell,
                          cell.getContext(),
                        )}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {nextCursor && (
            <button
              disabled={busy}
              onClick={async () => {
                setBusy(true);
                setError("");
                try {
                  const page = unwrap(await uploadPage(nextCursor));
                  setUploads((items) => [
                    ...items,
                    ...page.items.filter(
                      (u) => !items.some((item) => item.id === u.id),
                    ),
                  ]);
                  setNextCursor(page.nextCursor);
                } catch (e) {
                  setError(safeError(e));
                } finally {
                  setBusy(false);
                }
              }}
            >
              Load older uploads
            </button>
          )}
          {!uploads.length && (
            <p>No uploads yet. Choose a file or load the sample to begin.</p>
          )}
        </section>
      </div>
      {error && (
        <p role="alert" className="notice">
          {error}
        </p>
      )}
      {clearing && (
        <ClearDataDialog
          close={() => {
            setClearing(false);
            if (!demo) router.refresh();
          }}
        />
      )}
      {demo && (
        <footer className="page-footer">
          <Link href="/states">
            Preview processing, empty, partial, and failure states
          </Link>
        </footer>
      )}
    </>
  );
}
