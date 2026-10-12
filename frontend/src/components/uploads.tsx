'use client';
import { useState, useMemo } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useDropzone } from 'react-dropzone';
import { useReactTable, getCoreRowModel, flexRender, type ColumnDef } from '@tanstack/react-table';
import { useWorkspace } from './workspace';
import { Pipeline } from './pipeline';
import type { Upload } from '@/lib/types';
import { number, date, time } from '@/lib/format';
export function Uploads() {
  const { uploads, zone, loadDemo, setSelectedUploadId } = useWorkspace();
  const router = useRouter(); const [busy, setBusy] = useState(false); const [error, setError] = useState('');
  const active = uploads.find(u => u.status === 'processing') ?? uploads[0];
  const judging = active?.pipeline.find(s => s.stage === 'judging');
  const dropzone = useDropzone({ accept: { 'text/plain': ['.log'], 'text/csv': ['.csv'] }, maxSize: 52428800,
    onDropAccepted: () => setError('This preview uses sample evidence. Load demo data to explore it. File analysis is available when the API connection is configured.'),
    onDropRejected: () => setError('Choose a .log or .csv file up to 50 MB.'), noClick: true, noKeyboard: true });
  const columns = useMemo<ColumnDef<Upload>[]>(() => [
    { accessorKey: 'filename', header: 'File', cell: ({ row }) => <span className="mono">{row.original.filename}{row.original.source === 'sample' && <small> · sample</small>}</span> },
    { accessorKey: 'createdAt', header: 'Uploaded', cell: ({ getValue }) => <span className="mono">{date(getValue<string>(), zone)}</span> },
    { accessorKey: 'lineCount', header: 'Lines', cell: ({ getValue }) => number(getValue<number>()) },
    { accessorKey: 'anomalyCount', header: 'Anomalies', cell: ({ getValue }) => number(getValue<number>()) },
    { accessorKey: 'incidentCount', header: 'Incidents', cell: ({ getValue }) => number(getValue<number>()) },
    { id: 'status', header: 'Status', cell: ({ row: { original: u } }) => u.status === 'processing' ? <span className="accent">Processing · {u.progress}%</span> : <Link href={`/triage?upload=${u.id}`} onClick={() => setSelectedUploadId(u.id)}>{u.incidentCount ? `${(u.verdictCounts?.confirmed ?? 0) + (u.verdictCounts?.dismissed ?? 0) + (u.verdictCounts?.escalated ?? 0)} of ${u.incidentCount} decided` : 'Clear · no incidents'}</Link> },
  ], [zone, setSelectedUploadId]);
  const table = useReactTable({ data: uploads, columns, getCoreRowModel: getCoreRowModel() });
  return <>
    <section className="hero uploads-hero"><div className="hero-number">{active?.status === 'processing' ? number(judging?.processed) : number(active?.incidentCount ?? 0)}{active?.status === 'processing' && <span>/{number(judging?.countIn)}</span>}</div><div><h1>{active?.status === 'processing' ? 'anomalies judged so far.' : active ? 'incidents ready for review.' : 'Your investigation starts here.'}</h1><p>{active ? <> {active.status === 'processing' ? 'Stage 3 of 4.' : 'Analysis complete.'} Tenfold is reading <span className="mono">{active.filename}</span>, {number(active.lineCount)} lines. {active.startedAt && `Started ${time(active.startedAt, zone)}.`}</> : 'Drop a day of Zscaler logs, or explore the sample.'}</p></div></section>
    {active && <><Pipeline upload={active} /><p className="caption">{active.status === 'processing' ? 'Dashed lines are moving now. Counts update as each stage runs.' : 'All stages finished. Open triage to review the results.'}</p></>}
    <div className="upload-grid section"><section {...dropzone.getRootProps()} className={`dropzone ${dropzone.isDragActive ? 'dragging' : ''}`}><input {...dropzone.getInputProps()} aria-label="Upload log files" /><h2>Drop {active ? 'another' : 'a'} day of Zscaler logs</h2><p>NSS feed exports as .log or .csv, up to 50 MB each</p><div className="actions"><button className="primary" onClick={dropzone.open}>Choose files</button><button className="accent-button" disabled={busy} onClick={async () => { setBusy(true); setError(''); try { const u = await loadDemo(); router.push(`/triage?upload=${u.id}`); } catch { setError('Could not load demo data. Please try again.'); } finally { setBusy(false); } }}>{busy ? 'Loading…' : 'Load demo data'}</button></div><small className="mono">attack-day · 48,002 lines · 4 planted attacks · 3 decoys</small></section>
    <section className="panel recent-uploads"><h3>Recent uploads</h3><div className="table-scroll"><table><thead>{table.getHeaderGroups().map(group => <tr key={group.id}>{group.headers.map(h => <th key={h.id}>{flexRender(h.column.columnDef.header, h.getContext())}</th>)}</tr>)}</thead><tbody>{table.getRowModel().rows.map(row => <tr key={row.id}>{row.getVisibleCells().map(cell => <td key={cell.id}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</td>)}</tr>)}</tbody></table></div>{!uploads.length && <p>No uploads yet. Choose a file or load the sample to begin.</p>}</section></div>{error && <p role="alert" className="notice">{error}</p>}
  </>;
}
