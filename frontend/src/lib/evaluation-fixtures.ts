import type { Schema } from "./types";
import { uploadId } from "./fixtures";
import { initialIncidents } from "./incident-fixtures";
export const evaluation: Schema["EvalReport"] = {
  uploadId,
  sampleName: "attack-day",
  runAt: "2026-10-09T11:02:48Z",
  lineCount: 48002,
  anomalyCount: 214,
  incidentCount: 3,
  truePositives: 3,
  falsePositives: 0,
  falseNegatives: 1,
  precision: 1,
  recall: 0.75,
  scoresheet: [
    ...initialIncidents.map((i) => ({
      title: i.title,
      subtitle: `${i.principal.user} · ${i.mitre[0].id}`,
      incidentId: i.id,
      layers: [...new Set(i.signals.map((s) => s.layer))],
      riskScore: i.riskScore,
      aiVerdict: i.aiVerdict,
      answerKey: "attack" as const,
      mark: "caught" as const,
    })),
    {
      title: "Blocked request burst",
      subtitle: "Planted attack · not raised as an incident",
      layers: ["rules"],
      answerKey: "attack",
      mark: "missed",
    },
  ],
  judgeAgreement: { total: 214, agreed: 201 },
  providers: [
    {
      provider: "mock",
      label: "Fast typed-decision model",
      role: "in_use",
      decisions: 214,
      attacksCaught: 3,
      attacksTotal: 4,
      falsePositives: 0,
      agreementWithAnswerKey: 0.96,
      avgLatencyMs: 41,
      costPer1kUsd: 0.09,
    },
    {
      provider: "claude",
      label: "General LLM",
      role: "shadow",
      decisions: 214,
      attacksCaught: 4,
      attacksTotal: 4,
      falsePositives: 1,
      agreementWithAnswerKey: 0.97,
      avgLatencyMs: 1180,
      costPer1kUsd: 2.4,
    },
  ],
};
export const anomalyFixtures: Schema["Anomaly"][] = Array.from(
  { length: 18 },
  (_, index) => ({
    id: `a1b2c3d4-e5f6-4a7b-8c9d-${String(index).padStart(12, "0")}`,
    uploadId,
    detector: index % 2 ? "rare_destination" : "off_hours",
    reason:
      index % 2
        ? "A user reached a domain rarely seen in this file."
        : "A request fell outside the user’s usual hours in this file.",
    signals: [],
    riskScore: 12 + index,
    disposition: index % 3 === 0 ? "judged_benign" : "below_threshold",
    eventIds: ["e1041"],
    firstSeen: "2026-10-08T06:13:52Z",
    lastSeen: "2026-10-08T06:13:52Z",
    user: "dana.k",
    host: "login.microsoftonline.com",
    decision: {
      label: "likely_benign",
      confidence: 0.8,
      provider: "mock",
      latencyMs: 41,
    },
    shadowDecision: {
      label: index < 13 ? "inconclusive" : "likely_benign",
      confidence: 0.5,
      provider: "claude",
      latencyMs: 1180,
    },
  }),
);
