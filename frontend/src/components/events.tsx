"use client";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { NavigationLink as Link } from "./navigation-link";
import { useSearchParams } from "next/navigation";
import {
  useReactTable,
  getCoreRowModel,
  flexRender,
  type ColumnDef,
  type VisibilityState,
} from "@tanstack/react-table";
import { useVirtualizer } from "@tanstack/react-virtual";

import { eventPage } from "@/lib/api/actions";
import { unwrap } from "@/lib/api/result";
import { SkeletonRows } from "./skeleton";
import { usePagedData } from "./use-paged-data";
import type { Schema } from "@/lib/types";
import { matchesQuery } from "@/lib/event-filter";
import type { LogEvent } from "@/lib/types";
import { date, number, time, bytes } from "@/lib/format";
import { useWorkspace } from "./workspace";
import { RawLine, CopyButton } from "./evidence";
export function Events() {
  const { zone, uploads, incidents, selectedUploadId, live, demo } =
    useWorkspace();
  const params = useSearchParams();
  const upload = params.get("upload") ?? selectedUploadId;
  const allEvents = useMemo(
    () =>
      incidents
        .filter((i) => i.uploadId === upload)
        .flatMap((i) => i.evidence)
        .filter(
          (event, index, all) =>
            all.findIndex((e) => e.id === event.id) === index,
        )
        .sort((a, b) => a.ts.localeCompare(b.ts)),
    [incidents, upload],
  );
  // The API reads at most 200 characters of search.
  const [query, setQuery] = useState((params.get("q") ?? "").slice(0, 200));
  const [action, setAction] = useState("");
  const [anomalous, setAnomalous] = useState(false);
  const [hour, setHour] = useState("");
  const [selectedId, setSelectedId] = useState(params.get("event") ?? "e1048");
  const [drawer, setDrawer] = useState(true);
  const [visibility, setVisibility] = useState<VisibilityState>({});
  const range = uploads.find((u) => u.id === upload)?.timeRange;
  const hours = useMemo(() => {
    if (!range) return [];
    const start =
      Math.floor(new Date(range.start).getTime() / 3600000) * 3600000;
    const end = new Date(range.end).getTime();
    return Array.from(
      { length: Math.min(744, Math.floor((end - start) / 3600000) + 1) },
      (_, i) => new Date(start + i * 3600000).toISOString(),
    );
  }, [range]);
  const fetchPage = useCallback(
    async (cursor?: string) =>
      unwrap(
        await eventPage(upload, {
          limit: 100,
          cursor,
          q: query || undefined,
          action: action ? (action as Schema["EventAction"]) : undefined,
          hasAnomaly: anomalous || undefined,
          from: hour || undefined,
          to: hour
            ? new Date(new Date(hour).getTime() + 3600000).toISOString()
            : undefined,
        }),
      ),
    [upload, query, action, anomalous, hour],
  );
  const remote = usePagedData(live?.events, fetchPage, !demo);
  const localFiltered = useMemo(
    () =>
      allEvents.filter(
        (e) =>
          matchesQuery(e, query) &&
          (!action || e.securityResult.action === action) &&
          (!anomalous || e.flags.length > 0) &&
          (!hour || time(e.ts, zone).startsWith(hour + ":")),
      ),
    [allEvents, query, action, anomalous, hour, zone],
  );
  const filtered = demo ? localFiltered : remote.page.items;
  const replacing = !demo && remote.busy && !remote.loadingMore;
  const selected = filtered.find((e) => e.id === selectedId);
  const scroll = useRef<HTMLDivElement>(null);
  const columns = useMemo<ColumnDef<LogEvent>[]>(
    () => [
      {
        id: "time",
        header: "Time",
        cell: ({ row: { original: e } }) => (
          <>
            <span>{time(e.ts, zone, true)}</span>
            <small>{e.id}</small>
          </>
        ),
      },
      {
        id: "user",
        header: "User · source IP",
        cell: ({ row: { original: e } }) => (
          <>
            {e.principal.user}
            <small>{e.principal.srcIp}</small>
          </>
        ),
      },
      {
        id: "domain",
        header: "Domain · category",
        cell: ({ row: { original: e } }) => (
          <>
            {e.target.host}
            <small>{e.target.category}</small>
          </>
        ),
      },
      {
        id: "action",
        header: "Action · request",
        cell: ({ row: { original: e } }) => (
          <>
            {e.securityResult.action}
            <small>
              {e.network.method} · {e.network.status}
            </small>
          </>
        ),
      },
      {
        id: "bytes",
        header: "Bytes out · in",
        cell: ({ row: { original: e } }) => (
          <>
            {bytes(e.network.bytesOut)}
            <small>{bytes(e.network.bytesIn)}</small>
          </>
        ),
      },
      {
        id: "risk",
        header: "ZS risk",
        accessorFn: (e) => e.securityResult.riskScore ?? "—",
      },
      {
        id: "flags",
        header: "Why flagged",
        cell: ({ row: { original: e } }) => (
          <span className={e.flags.length ? "flag-text" : ""}>
            {e.flags.map((f) => `▲ ${f.label}`).join(" · ") || "—"}
          </span>
        ),
      },
    ],
    [zone],
  );
  const table = useReactTable({
    data: filtered,
    columns,
    state: { columnVisibility: visibility },
    onColumnVisibilityChange: setVisibility,
    getCoreRowModel: getCoreRowModel(),
  });
  const rows = table.getRowModel().rows;
  const virtual = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scroll.current,
    estimateSize: () => 70,
    overscan: 8,
  });
  const items = virtual.getVirtualItems();
  // Fetch the next page before the reader reaches the end of this one.
  const lastShown = items.at(-1)?.index ?? 0;
  const nearEnd = rows.length > 0 && lastShown >= rows.length - 20;
  const { more, busy: loadingPage, error: pageError } = remote;
  const hasMore = Boolean(remote.page.nextCursor);
  useEffect(() => {
    if (!demo && nearEnd && hasMore && !loadingPage && !pageError) void more();
  }, [demo, nearEnd, hasMore, loadingPage, pageError, more]);
  const top = items[0]?.start ?? 0;
  const bottom = items.length ? virtual.getTotalSize() - items.at(-1)!.end : 0;
  function choose(index: number) {
    const e = filtered[index];
    if (e) {
      setSelectedId(e.id);
      setDrawer(true);
      virtual.scrollToIndex(index, { align: "auto" });
    }
  }
  return (
    <>
      <header className="events-heading">
        <h1>Every event, as logged.</h1>
        <div className="stats">
          <div>
            <strong>
              {number(demo ? filtered.length : remote.page.total)}
            </strong>
            <small>match your filters</small>
          </div>
          <div>
            <strong>
              {number(uploads.find((u) => u.id === upload)?.eventCount)}
            </strong>
            <small>in this upload</small>
          </div>
        </div>
      </header>
      <section className="filter-bar">
        <label className="event-search">
          Search
          <input
            aria-label="Search events"
            value={query}
            maxLength={200}
            placeholder="user:dana.k OR domain:backblazeb2.com"
            onChange={(e) => setQuery(e.target.value)}
          />
        </label>
        <label>
          Time
          <select
            aria-label="Time filter"
            value={hour}
            onChange={(e) => setHour(e.target.value)}
          >
            <option value="">All day</option>
            {demo
              ? Array.from({ length: 24 }, (_, i) => (
                  <option key={i} value={String(i).padStart(2, "0")}>
                    {String(i).padStart(2, "0")}:00–
                    {String(i + 1).padStart(2, "0")}
                    :00
                  </option>
                ))
              : hours.map((value) => (
                  <option key={value} value={value}>
                    {date(value, zone)}
                  </option>
                ))}
          </select>
        </label>
        <label>
          Action
          <select
            aria-label="Action filter"
            value={action}
            onChange={(e) => setAction(e.target.value)}
          >
            <option value="">Any</option>
            <option value="allowed">Allowed</option>
            <option value="blocked">Blocked</option>
          </select>
        </label>
        <label className="checkbox-label">
          <input
            type="checkbox"
            checked={anomalous}
            onChange={(e) => setAnomalous(e.target.checked)}
          />
          Anomalous only
        </label>
        <details className="column-picker">
          <summary>
            Columns · {table.getVisibleLeafColumns().length} of {columns.length}
          </summary>
          <div>
            {table.getAllLeafColumns().map((c) => (
              <label key={c.id}>
                <input
                  type="checkbox"
                  checked={c.getIsVisible()}
                  disabled={
                    c.getIsVisible() &&
                    table.getVisibleLeafColumns().length === 1
                  }
                  onChange={c.getToggleVisibilityHandler()}
                />
                {String(c.columnDef.header)}
              </label>
            ))}
          </div>
        </details>
        <button
          className="text-button"
          onClick={() => {
            setQuery("");
            setAction("");
            setAnomalous(false);
            setHour("");
          }}
        >
          Clear filters
        </button>
      </section>
      {!demo && remote.error && (
        <p role="alert" className="notice">
          {remote.error} <button onClick={remote.retry}>Try again</button>
        </p>
      )}
      {!demo && remote.busy && (
        <p className="sr-only" role="status">
          Loading events…
        </p>
      )}
      <div
        className={`explorer-grid ${drawer && selected ? "has-drawer" : ""}`}
      >
        <section
          className="panel explorer-panel"
          aria-busy={!demo && remote.busy}
        >
          <div
            ref={scroll}
            className="explorer-scroll"
            role="region"
            aria-label="Event results. J and K move the selection; Enter opens the raw line."
            tabIndex={0}
            onKeyDown={(e) => {
              if (e.target !== e.currentTarget) return;
              const index = filtered.findIndex((v) => v.id === selectedId);
              if (e.key === "j" || e.key === "ArrowDown") {
                e.preventDefault();
                choose(Math.min(rows.length - 1, index + 1));
              }
              if (e.key === "k" || e.key === "ArrowUp") {
                e.preventDefault();
                choose(Math.max(0, index - 1));
              }
              if (e.key === "Enter") {
                e.preventDefault();
                if (selected) setDrawer(true);
                else choose(0);
              }
            }}
          >
            <table className="event-table">
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
                {replacing && (
                  <SkeletonRows
                    columns={table.getVisibleLeafColumns().length}
                  />
                )}
                {!replacing && top > 0 && (
                  <tr aria-hidden="true">
                    <td
                      colSpan={columns.length}
                      style={{ height: top, padding: 0 }}
                    />
                  </tr>
                )}
                {!replacing &&
                  items.map((v) => {
                    const row = rows[v.index];
                    return (
                      <tr
                        key={row.original.id}
                        className={
                          selectedId === row.original.id ? "active-event" : ""
                        }
                        aria-selected={selectedId === row.original.id}
                        onClick={() => choose(v.index)}
                      >
                        {row.getVisibleCells().map((c, i) => (
                          <td key={c.id}>
                            {i === 0 ? (
                              <button
                                className="event-row-button mono"
                                aria-label={`Open event ${row.original.id}`}
                                onClick={() => choose(v.index)}
                              >
                                {flexRender(
                                  c.column.columnDef.cell,
                                  c.getContext(),
                                )}
                              </button>
                            ) : (
                              flexRender(
                                c.column.columnDef.cell,
                                c.getContext(),
                              )
                            )}
                          </td>
                        ))}
                      </tr>
                    );
                  })}
                {!replacing && bottom > 0 && (
                  <tr aria-hidden="true">
                    <td
                      colSpan={columns.length}
                      style={{ height: bottom, padding: 0 }}
                    />
                  </tr>
                )}
                {!demo && remote.loadingMore && (
                  <SkeletonRows
                    columns={table.getVisibleLeafColumns().length}
                    rows={2}
                  />
                )}
              </tbody>
            </table>
            {!replacing && !rows.length && (
              <div className="empty">
                <h2>No events match these filters.</h2>
                <p>Try another user, domain, IP, or event ID.</p>
              </div>
            )}
          </div>
          <footer className="table-footer">
            <span>
              {number(filtered.length)}{" "}
              {demo
                ? "matching preview events"
                : `of ${number(remote.page.total)} matching events loaded`}
              . More load as you scroll.
            </span>
            <span>J and K move the selection · Enter opens the raw line</span>
          </footer>
          {!demo && (
            <div className="actions">
              {remote.page.nextCursor && (
                <button disabled={remote.busy} onClick={remote.more}>
                  Load more events
                </button>
              )}
              <a className="button" href={`/api/uploads/${upload}/export`}>
                Export all events
              </a>
            </div>
          )}
        </section>
        {drawer && selected && (
          <aside className="panel raw-drawer">
            <div className="drawer-title">
              <CopyButton text={selected.raw} />
              <h3>
                Event {selected.id} · all {selected.raw.split("\t").length}{" "}
                fields
              </h3>
              <button
                className="small-button"
                onClick={() => setDrawer(false)}
                aria-label="Close raw line"
              >
                ×
              </button>
            </div>
            <p>
              Raw log line, stored in UTC. Display time:{" "}
              {time(selected.ts, zone, true)}.
            </p>
            <RawLine event={selected} fields />
            {selected.incidentId && (
              <>
                <p className="section">Part of</p>
                <Link
                  href={`/incidents/${selected.incidentId}?upload=${upload}`}
                >
                  Open incident →
                </Link>
              </>
            )}
            <div className="actions section">
              <button
                onClick={() => setQuery(`domain:${selected.target.host}`)}
              >
                All events for this domain
              </button>
              <button
                onClick={() => setQuery(`ip:${selected.principal.srcIp}`)}
              >
                All events for this IP
              </button>
            </div>
          </aside>
        )}
      </div>
    </>
  );
}
