"use client";
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  Filler,
  Tooltip,
  type ChartOptions,
} from "chart.js";
import annotationPlugin from "chartjs-plugin-annotation";
import { Line, Bar } from "react-chartjs-2";
import { useEffect, useState } from "react";
import type { Schema, Incident } from "@/lib/types";
import { ChartSkeleton } from "./skeleton";
import { time, bytes } from "@/lib/format";
ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  Filler,
  Tooltip,
  annotationPlugin,
);
function usePalette() {
  const [palette, setPalette] = useState<Record<string, string> | null>(null);
  useEffect(() => {
    const frame = requestAnimationFrame(() => {
      const css = getComputedStyle(document.documentElement);
      const names = [
        "text-secondary",
        "chart-line",
        "chart-band",
        "chart-bar",
        "chart-flagged-fill",
        "flagged",
        "accent",
        "selected",
        "panel",
      ];
      setPalette(
        Object.fromEntries(
          names.map((n) => [n, css.getPropertyValue(`--color-${n}`).trim()]),
        ),
      );
    });
    return () => cancelAnimationFrame(frame);
  }, []);
  return palette;
}
export function RiskTimeline({
  series,
  incidents,
  threshold,
  zone,
  onOpen,
}: {
  series: Schema["TimeseriesResponse"];
  incidents: Schema["IncidentSummary"][];
  threshold: number;
  zone: string;
  onOpen: (id: string) => void;
}) {
  const p = usePalette();
  if (!p)
    return <ChartSkeleton className="chart-panel" label="Loading risk chart" />;
  const points = series.points;
  const markers = incidents.map((i) =>
    Math.max(
      0,
      points.findLastIndex((point) => point.ts <= i.timeRange.start),
    ),
  );
  const options: ChartOptions<"line"> = {
    responsive: true,
    maintainAspectRatio: false,
    animation: false,
    layout: { padding: { top: 40, left: 0, right: 0, bottom: 0 } },
    plugins: {
      legend: { display: false },
      tooltip: {
        callbacks: { label: (ctx) => `Risk score ${ctx.parsed.y}/100` },
      },
      annotation: {
        annotations: {
          threshold: {
            type: "box",
            yMin: 0,
            yMax: threshold,
            backgroundColor: p["chart-band"],
            borderWidth: 0,
            drawTime: "beforeDatasetsDraw",
          },
        },
      },
    },
    scales: {
      x: {
        ticks: { maxTicksLimit: 7, color: p["text-secondary"], maxRotation: 0 },
        grid: { display: false },
      },
      y: { min: 0, max: 115, display: false },
    },
    onClick: (_, elements) => {
      if (elements.length) {
        const index = elements[0].index;
        const nearest = markers.reduce(
          (best, n, i) =>
            Math.abs(n - index) < Math.abs(markers[best] - index) ? i : best,
          0,
        );
        if (incidents[nearest]) onOpen(incidents[nearest].id);
      }
    },
  };
  return (
    <div className="chart-panel">
      <Line
        aria-label="Peak risk score over the day"
        role="img"
        options={options}
        data={{
          labels: points.map((v) => time(v.ts, zone)),
          datasets: [
            {
              data: points.map((v) => v.peakRiskScore),
              borderColor: p["chart-line"],
              borderWidth: 1.5,
              backgroundColor: p["chart-flagged-fill"],
              fill: {
                target: { value: threshold },
                above: p["chart-flagged-fill"],
                below: "transparent",
              },
              pointRadius: points.map((_, i) => (markers.includes(i) ? 5 : 0)),
              pointBackgroundColor: p.flagged,
              pointHitRadius: 12,
              tension: 0,
            },
          ],
        }}
      />
    </div>
  );
}
export function VolumeStrip({
  series,
  zone,
}: {
  series: Schema["TimeseriesResponse"];
  zone: string;
}) {
  const p = usePalette();
  if (!p)
    return (
      <ChartSkeleton className="volume-chart" label="Loading event volume" />
    );
  return (
    <div className="volume-chart">
      <Bar
        aria-label="Event volume per hour"
        role="img"
        data={{
          labels: series.points.map((v) => time(v.ts, zone)),
          datasets: [
            {
              data: series.points.map((v) => v.total),
              backgroundColor: p["chart-bar"],
              borderRadius: 2,
              barPercentage: 0.94,
              categoryPercentage: 1,
            },
          ],
        }}
        options={{
          responsive: true,
          maintainAspectRatio: false,
          animation: false,
          plugins: { legend: { display: false } },
          scales: {
            x: {
              grid: { display: false },
              ticks: {
                color: p["text-secondary"],
                maxTicksLimit: 5,
                maxRotation: 0,
              },
            },
            y: { display: false },
          },
        }}
      />
    </div>
  );
}
export function IncidentChart({
  incident,
  selected,
  select,
  zone,
}: {
  incident: Incident;
  selected: string[];
  select: (ids: string[]) => void;
  zone: string;
}) {
  const p = usePalette();
  if (!p)
    return (
      <ChartSkeleton className="incident-chart" label="Loading request chart" />
    );
  const events = incident.evidence;
  const baseline = incident.chart.baselineBytesOut;
  const options: ChartOptions<"bar"> = {
    responsive: true,
    maintainAspectRatio: false,
    animation: false,
    plugins: {
      legend: { display: false },
      tooltip: {
        callbacks: {
          label: (ctx) =>
            `${events[ctx.dataIndex].id} · ${bytes(ctx.parsed.y ?? 0)} out`,
          title: (items) =>
            items[0] ? time(events[items[0].dataIndex].ts, zone, true) : "",
        },
      },
      annotation: {
        annotations:
          baseline === undefined
            ? {}
            : {
                baseline: {
                  type: "box",
                  yMin: 0,
                  yMax: baseline,
                  backgroundColor: p["chart-band"],
                  borderWidth: 0,
                  drawTime: "beforeDatasetsDraw",
                },
              },
      },
    },
    scales: {
      x: {
        type: "linear",
        min: Date.parse(incident.chart.window.start),
        max: Date.parse(incident.chart.window.end),
        grid: { display: false },
        ticks: {
          color: p["text-secondary"],
          maxTicksLimit: 7,
          maxRotation: 0,
          callback: (value) =>
            time(new Date(Number(value)).toISOString(), zone),
        },
      },
      y: { display: false, beginAtZero: true },
    },
    onClick: (_, items) => {
      if (items[0]) select([events[items[0].index].id]);
    },
  };
  return (
    <div className="incident-chart">
      <Bar
        role="img"
        aria-label="Bytes out per request"
        options={options}
        data={{
          datasets: [
            {
              data: events.map((e) => ({
                x: Date.parse(e.ts),
                y: e.network.bytesOut,
              })),
              backgroundColor: events.map((e) =>
                selected.includes(e.id)
                  ? p.accent
                  : e.flags.length
                    ? p.flagged
                    : p["chart-line"],
              ),
              barPercentage: 0.6,
              minBarLength: 3,
            },
          ],
        }}
      />
      <span className="chart-baseline">
        {incident.chart.baselineLabel ?? "No baseline"}
      </span>
    </div>
  );
}
