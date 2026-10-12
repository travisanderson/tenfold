"use client";
import { Fragment, useMemo, useState } from "react";
import {
  useReactTable,
  getCoreRowModel,
  flexRender,
  type ColumnDef,
} from "@tanstack/react-table";
import type { LogEvent } from "@/lib/types";
import { time, bytes } from "@/lib/format";
export function CopyButton({
  text,
  label = "Copy",
}: {
  text: string;
  label?: string;
}) {
  const [status, setStatus] = useState("");
  return (
    <>
      <button
        className="small-button"
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(text);
            setStatus("Copied");
          } catch {
            setStatus("Copy unavailable");
          }
        }}
      >
        {status || label}
      </button>
      <span className="sr-only" role="status">
        {status}
      </span>
    </>
  );
}
export function RawLine({
  event,
  fields = false,
}: {
  event: LogEvent;
  fields?: boolean;
}) {
  return (
    <div className="raw-line">
      <div className="raw-heading">
        <span>Raw log line · {event.id} · stored in UTC</span>
        <CopyButton text={event.raw} />
      </div>
      <pre>{fields ? event.raw.split("\t").join("\n") : event.raw}</pre>
    </div>
  );
}
export function EvidenceTable({
  events,
  selected,
  select,
  zone,
}: {
  events: LogEvent[];
  selected: string[];
  select: (ids: string[]) => void;
  zone: string;
}) {
  const [open, setOpen] = useState<string | null>(null);
  const [filter, setFilter] = useState(false);
  const shown = useMemo(
    () => (filter ? events.filter((e) => selected.includes(e.id)) : events),
    [filter, events, selected],
  );
  const columns = useMemo<ColumnDef<LogEvent>[]>(
    () => [
      {
        id: "id",
        header: "Event",
        cell: ({ row: { original: e } }) => (
          <button
            className="text-button mono"
            aria-expanded={open === e.id}
            onClick={() => {
              setOpen(open === e.id ? null : e.id);
              select([e.id]);
            }}
          >
            {open === e.id ? "⌄" : "›"} {e.id}
          </button>
        ),
      },
      {
        accessorKey: "ts",
        header: "Time",
        cell: ({ getValue }) => time(getValue<string>(), zone, true),
      },
      { id: "user", header: "User", accessorFn: (e) => e.principal.user },
      { id: "ip", header: "Source IP", accessorFn: (e) => e.principal.srcIp },
      { id: "host", header: "Domain", accessorFn: (e) => e.target.host },
      {
        id: "category",
        header: "Category",
        accessorFn: (e) => e.target.category,
      },
      {
        id: "action",
        header: "Action",
        accessorFn: (e) => e.securityResult.action,
      },
      { id: "method", header: "Method", accessorFn: (e) => e.network.method },
      {
        id: "bytes",
        header: "Bytes out",
        cell: ({ row }) => bytes(row.original.network.bytesOut),
      },
      {
        id: "risk",
        header: "ZS risk",
        accessorFn: (e) => e.securityResult.riskScore ?? "—",
      },
    ],
    [zone, open, select],
  );
  const table = useReactTable({
    data: shown,
    columns,
    getCoreRowModel: getCoreRowModel(),
  });
  return (
    <>
      <div className="evidence-tools">
        <span aria-live="polite">{selected.length} events selected</span>
        <label>
          <input
            type="checkbox"
            checked={filter}
            onChange={(e) => setFilter(e.target.checked)}
          />{" "}
          Only selected
        </label>
        <button
          className="text-button"
          onClick={() => {
            select([]);
            setFilter(false);
          }}
        >
          Clear selection
        </button>
      </div>
      <div className="table-scroll panel evidence-table">
        <table className="mono">
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
            {table.getRowModel().rows.map((row) => (
              <Fragment key={row.id}>
                <tr
                  className={
                    selected.includes(row.original.id) ? "selected-row" : ""
                  }
                >
                  {row.getVisibleCells().map((c) => (
                    <td key={c.id}>
                      {flexRender(c.column.columnDef.cell, c.getContext())}
                    </td>
                  ))}
                </tr>
                {open === row.original.id && (
                  <tr>
                    <td colSpan={columns.length}>
                      <RawLine event={row.original} />
                    </td>
                  </tr>
                )}
              </Fragment>
            ))}
          </tbody>
        </table>
        {!shown.length && (
          <p className="empty">No evidence matches this selection.</p>
        )}
      </div>
    </>
  );
}
