import type { Incident, LogEvent, Schema } from "./types";
import { uploadId, incidentId } from "./fixtures";

const transfers = Array.from({ length: 19 }, (_, i) => `e${1048 + i}`);
function event(
  lineNo: number,
  ts: string,
  host: string,
  bytesOut: number,
  label: string,
): LogEvent {
  const e: LogEvent = {
    id: `e${lineNo}`,
    lineNo,
    ts,
    principal: {
      user: "dana.k",
      srcIp: "10.20.99.47",
      device: "LT-022",
      dept: "Finance",
    },
    target: {
      host,
      url: `https://${host}/api/v2/put`,
      category: host.startsWith("login.")
        ? "Office 365"
        : "Miscellaneous or Unknown",
    },
    network: {
      method: bytesOut > 1000 ? "POST" : "GET",
      status: 200,
      bytesOut,
      bytesIn: 418,
      userAgent: "curl/8.4.0",
    },
    securityResult: {
      action: "allowed",
      riskScore: host.startsWith("login.") ? 12 : 78,
    },
    incidentId,
    flags: label
      ? [
          {
            anomalyId: "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
            label,
            disposition: "in_incident",
          },
        ]
      : [],
    raw: "",
  };
  e.raw = Object.entries({
    time: ts,
    recordid: lineNo,
    login: "dana.k@northwindlabs.example",
    dept: "Finance",
    cip: e.principal.srcIp,
    sip: "192.0.2.36",
    host,
    url: e.target.url,
    urlcat: e.target.category,
    action: "Allowed",
    reason: "Allowed",
    reqmethod: e.network.method,
    respcode: 200,
    reqsize: bytesOut,
    respsize: 418,
    ua: e.network.userAgent,
    appname: "General Browsing",
    appclass: "General Browsing",
    devicehostname: "LT-022",
    threatname: "None",
    riskscore: e.securityResult.riskScore,
  })
    .map(([k, v]) => `${k}=${v}`)
    .join("\t");
  return e;
}
const evidence = [
  event(1041, "2026-10-08T06:13:52Z", "login.microsoftonline.com", 1126, ""),
  event(
    1042,
    "2026-10-08T06:14:07Z",
    "login.microsoftonline.com",
    2150,
    "new source IP",
  ),
  event(
    1047,
    "2026-10-08T06:17:22Z",
    "files-sync-cdn.net",
    410,
    "first-seen domain",
  ),
  ...transfers.map((_, i) =>
    event(
      1048 + i,
      new Date(Date.parse("2026-10-08T06:17:41Z") + i * 76800).toISOString(),
      "files-sync-cdn.net",
      [2306867, 2202009, 2411725, 2097152, 2306867, 1992294][i % 6],
      "bytes out 30× baseline",
    ),
  ),
  event(1067, "2026-10-08T06:41:12Z", "files-sync-cdn.net", 307, ""),
];
export const mainIncident: Incident = {
  id: incidentId,
  uploadId,
  title: "Off-hours upload to a first-seen domain",
  severity: "high",
  riskScore: 91,
  aiVerdict: "likely_malicious",
  summary:
    "Requests for dana.k came from an address new to this account. Then 40.2 MB went to files-sync-cdn.net in 24 minutes.",
  markerLabel: "dana.k · 40 MB out",
  markerDetail: "30× median",
  principal: { user: "dana.k", host: "LT-022" },
  timeRange: { start: "2026-10-08T06:14:07Z", end: "2026-10-08T06:41:12Z" },
  eventCount: 23,
  anomalyCount: 4,
  analystVerdict: { status: "open" },
  mitre: [
    {
      id: "T1567",
      name: "Exfiltration Over Web Service",
      tactic: "Exfiltration",
    },
    { id: "T1078", name: "Valid Accounts", tactic: "Initial Access" },
  ],
  narrative: {
    status: "generated",
    provider: "mock",
    nextSteps: ["Ask dana.k whether the 02:14 transfer was expected."],
    sentences: [
      {
        text: "A sign-in request for dana.k reached Office 365 at 02:14 from 10.20.99.47, an address this account uses nowhere else in this file.",
        citations: ["e1042"],
      },
      {
        text: "Three minutes later the same address reached files-sync-cdn.net, a domain Zscaler has not categorized and no other user in this file contacted.",
        citations: ["e1047"],
      },
      {
        text: "Over the next 23 minutes, 19 POST requests carried 40.2 MB out, about 30 times this user’s median hour of 1.3 MB.",
        citations: transfers,
      },
      {
        text: "The requests stopped at 02:41 and no other user contacted the domain that day.",
        citations: ["e1067"],
      },
    ],
  },
  signals: [
    {
      name: "bytes_out_vs_baseline",
      description: "Outbound bytes 30× this user’s in-file median",
      weight: 34,
      layer: "baselines",
      observed: "40.2 MB",
      observedDetail: "in 23 minutes",
      baselineText: "1.3 MB",
      baselineDetail: "per hour, median in this file",
      baselineSource: "in_file",
      eventIds: transfers,
    },
    {
      name: "rare_destination",
      description: "First-seen domain, uncategorized",
      weight: 22,
      layer: "baselines",
      observed: "files-sync-cdn.net",
      observedDetail: "Uncategorized",
      baselineText: "0 other visits",
      baselineDetail: "in this file",
      baselineSource: "in_file",
      eventIds: ["e1047", ...transfers],
    },
    {
      name: "new_source_ip",
      description: "Sign-in request from a new source IP",
      weight: 19,
      layer: "baselines",
      observed: "10.20.99.47",
      baselineText: "10.20.10.12",
      baselineDetail: "every other request in this file",
      baselineSource: "in_file",
      eventIds: ["e1042"],
    },
    {
      name: "off_hours",
      description: "Off-hours activity",
      weight: 16,
      layer: "rules",
      observed: "02:14–02:41",
      baselineText: "08:30–18:00",
      baselineDetail: "this user’s other activity in this file",
      baselineSource: "in_file",
      eventIds: evidence.map((e) => e.id),
    },
  ],
  timeline: [
    {
      ts: evidence[1].ts,
      eventId: "e1042",
      label: "Sign-in request, new IP",
      kind: "first_seen",
    },
    {
      ts: evidence[2].ts,
      eventId: "e1047",
      label: "First request to files-sync-cdn.net",
      kind: "first_seen",
    },
    {
      ts: evidence[3].ts,
      eventId: "e1048",
      label: "Outbound transfers begin",
      kind: "transfer",
    },
    {
      ts: evidence.at(-1)!.ts,
      eventId: "e1067",
      label: "Requests stop",
      kind: "transfer",
    },
  ],
  graph: {
    nodes: [
      {
        id: "user",
        type: "user",
        label: "dana.k",
        sublabel: "user · 1,214 events today",
        flagged: false,
        neighbour: false,
        eventIds: evidence.map((e) => e.id),
        x: 0,
        y: 140,
      },
      {
        id: "ip",
        type: "ip",
        label: "10.20.99.47",
        sublabel: "LT-022 · new source IP",
        flagged: true,
        neighbour: false,
        eventIds: evidence.map((e) => e.id),
        x: 270,
        y: 140,
      },
      {
        id: "login",
        type: "domain",
        label: "login.microsoftonline.com",
        sublabel: "Office 365 sign-in request",
        flagged: false,
        neighbour: false,
        eventIds: ["e1041", "e1042"],
        x: 540,
        y: 0,
      },
      {
        id: "domain",
        type: "domain",
        label: "files-sync-cdn.net",
        sublabel: "uncategorized · first seen",
        flagged: true,
        neighbour: false,
        eventIds: ["e1047", ...transfers, "e1067"],
        x: 540,
        y: 140,
      },
      {
        id: "data",
        type: "data",
        label: "40.2 MB out",
        sublabel: "19 POST · 02:17–02:40",
        flagged: true,
        neighbour: false,
        eventIds: transfers,
        x: 810,
        y: 140,
      },
      {
        id: "usual",
        type: "ip",
        label: "10.20.10.12",
        sublabel: "LT-022 · usual IP in this file",
        flagged: false,
        neighbour: true,
        eventIds: [],
        x: 270,
        y: 280,
      },
      {
        id: "others",
        type: "host",
        label: "0 other hosts",
        sublabel: "contacted this domain",
        flagged: false,
        neighbour: true,
        eventIds: [],
        x: 540,
        y: 280,
      },
    ],
    edges: [
      {
        id: "user-ip",
        source: "user",
        target: "ip",
        label: "02:14",
        flagged: true,
        eventCount: 23,
        eventIds: evidence.map((e) => e.id),
      },
      {
        id: "ip-login",
        source: "ip",
        target: "login",
        label: "e1042",
        flagged: false,
        eventCount: 2,
        eventIds: ["e1041", "e1042"],
      },
      {
        id: "ip-domain",
        source: "ip",
        target: "domain",
        label: "02:17",
        flagged: true,
        eventCount: 21,
        eventIds: ["e1047", ...transfers, "e1067"],
      },
      {
        id: "domain-data",
        source: "domain",
        target: "data",
        label: "40.2 MB",
        bytes: 42153574,
        flagged: true,
        eventCount: 19,
        eventIds: transfers,
      },
      {
        id: "user-usual",
        source: "user",
        target: "usual",
        label: "usual",
        flagged: false,
        eventCount: 1191,
        eventIds: [],
      },
    ],
  },
  indicators: [
    { type: "domain", value: "files-sync-cdn.net", eventCount: 21 },
    { type: "ip", value: "10.20.99.47", eventCount: 23 },
    { type: "user", value: "dana.k", eventCount: 1214 },
  ],
  chart: {
    window: { start: "2026-10-08T06:00:00Z", end: "2026-10-08T07:00:00Z" },
    baselineBytesOut: 98304,
    baselineLabel: "median: under 0.1 MB",
  },
  notes: [
    {
      id: "c4d5e6f7-a8b9-4c0d-9e1f-2a3b4c5d6e7f",
      author: "j.ortiz",
      text: "Seen at handover from the night shift. Not yet investigated.",
      createdAt: "2026-10-09T11:58:00Z",
    },
  ],
  lessonSuggestions: [
    { scope: { type: "user", value: "dana.k" }, label: "This user dana.k" },
    {
      scope: { type: "domain", value: "files-sync-cdn.net" },
      label: "This domain files-sync-cdn.net",
    },
  ],
  evidenceEventIds: evidence.map((e) => e.id),
  evidence,
};
// Separate typed cases keep every displayed citation attached to actual evidence.
function secondary(
  index: number,
  title: string,
  user: string,
  host: string,
  domain: string,
  start: string,
  count: number,
  riskScore: number,
  severity: Schema["Severity"],
  aiVerdict: Schema["AiVerdict"],
  summary: string,
): Incident {
  const id = `3f2b8c1e-5d4a-4e6f-9a1b-2c3d4e5f6a7${index}`;
  const events = Array.from({ length: count }, (_, i) => {
    const e = event(
      2000 * index + i,
      new Date(Date.parse(start) + i * 60000).toISOString(),
      domain,
      1024,
      "regular requests",
    );
    e.principal = { user, device: host, srcIp: `10.20.10.${index + 20}` };
    e.incidentId = id;
    e.raw = e.raw
      .replace("dana.k@northwindlabs.example", `${user}@northwindlabs.example`)
      .replace("LT-022", host)
      .replace("10.20.99.47", e.principal.srcIp);
    if (index === 3) {
      e.securityResult = {
        action: "blocked",
        threatName: "Phish.Generic",
        riskScore: 84,
      };
      e.flags[0].label = "Phish.Generic blocked";
      e.raw = e.raw
        .replace("action=Allowed", "action=Blocked")
        .replace("threatname=None", "threatname=Phish.Generic");
    }
    return e;
  });
  const ids = events.map((e) => e.id);
  return {
    ...mainIncident,
    id,
    title,
    principal: { user, host },
    riskScore,
    severity,
    aiVerdict,
    summary,
    markerLabel: `${host} · ${index === 2 ? "every 60 s" : "phish blocked"}`,
    markerDetail: `${count} requests`,
    timeRange: { start, end: events.at(-1)!.ts },
    eventCount: count,
    anomalyCount: index === 2 ? 3 : 2,
    mitre: [
      {
        id: index === 2 ? "T1071.001" : "T1566.002",
        name: index === 2 ? "Web Protocols" : "Spearphishing Link",
        tactic: index === 2 ? "Command and Control" : "Initial Access",
      },
    ],
    narrative: {
      status: "generated",
      provider: "mock",
      sentences: [{ text: summary, citations: ids }],
      nextSteps: [],
    },
    signals: [
      {
        name: index === 2 ? "beaconing" : "threat_hit",
        description:
          index === 2
            ? "Requests repeat at a regular 60-second interval"
            : "Proxy named and blocked Phish.Generic",
        weight: riskScore,
        layer: "rules",
        observed: `${count} requests`,
        baselineSource: "none",
        eventIds: ids,
      },
    ],
    timeline: [
      {
        ts: start,
        eventId: ids[0],
        label: "First observed request",
        kind: "first_seen",
      },
    ],
    graph: {
      nodes: [
        {
          id: "user",
          type: "user",
          label: user,
          sublabel: host,
          flagged: false,
          neighbour: false,
          eventIds: ids,
          x: 0,
          y: 0,
        },
        {
          id: "domain",
          type: "domain",
          label: domain,
          flagged: true,
          neighbour: false,
          eventIds: ids,
          x: 400,
          y: 0,
        },
      ],
      edges: [
        {
          id: "request",
          source: "user",
          target: "domain",
          flagged: true,
          eventCount: count,
          eventIds: ids,
          label: `${count} requests`,
        },
      ],
    },
    indicators: [
      { type: "user", value: user, eventCount: count },
      { type: "domain", value: domain, eventCount: count },
    ],
    chart: {
      window: {
        start,
        end: new Date(Date.parse(events.at(-1)!.ts) + 60000).toISOString(),
      },
    },
    notes: [],
    lessonSuggestions: [
      {
        scope: { type: "domain", value: domain },
        label: `This domain ${domain}`,
      },
    ],
    evidenceEventIds: ids,
    evidence: events,
  };
}
export const initialIncidents = [
  mainIncident,
  secondary(
    2,
    "Clockwork requests from WS-114",
    "marcus.l",
    "WS-114",
    "update-chk-svc.io",
    "2026-10-08T13:12:00Z",
    181,
    87,
    "high",
    "likely_malicious",
    "WS-114 called update-chk-svc.io about every 60 seconds for three hours. The timing is too regular for a person.",
  ),
  secondary(
    3,
    "Phishing link clicked, then blocked",
    "priya.s",
    "WS-107",
    "account-review.example",
    "2026-10-08T18:37:00Z",
    4,
    54,
    "medium",
    "inconclusive",
    "priya.s followed a link that Zscaler blocked as Phish.Generic. The logs cannot show whether it reached other staff.",
  ),
];
export const riskSeries: Schema["TimeseriesResponse"] = {
  bucketSeconds: 600,
  points: Array.from({ length: 144 }, (_, i) => ({
    ts: new Date(Date.parse("2026-10-08T04:00:00Z") + i * 600000).toISOString(),
    total: 300,
    blocked: 0,
    anomalous: 0,
    peakRiskScore:
      i >= 13 && i <= 16
        ? 91 - (i % 3) * 4
        : i >= 55 && i <= 73
          ? 87 - (i % 5) * 2
          : i === 87
            ? 54
            : [12, 14, 10, 15, 11, 9][i % 6],
  })),
};
export const volumeSeries: Schema["TimeseriesResponse"] = {
  bucketSeconds: 3600,
  points: [
    80, 60, 190, 65, 72, 76, 220, 680, 1870, 2710, 2990, 2820, 2250, 2800, 3100,
    2930, 2660, 1940, 1120, 700, 490, 420, 310, 170,
  ].map((total, i) => ({
    ts: new Date(
      Date.parse("2026-10-08T04:00:00Z") + i * 3600000,
    ).toISOString(),
    total,
    blocked: 0,
    anomalous: 0,
    peakRiskScore: 0,
  })),
};
export const allEvents = initialIncidents
  .flatMap((i) => i.evidence)
  .sort((a, b) => a.ts.localeCompare(b.ts));
