import type { Schema } from "./types";
export type LiveData = {
  uploads: Schema["Upload"][];
  nextUploadCursor?: string;
  selectedUploadId: string;
  incidents: Schema["IncidentDetail"][];
  summaries: Schema["IncidentSummary"][];
  samples: Schema["Sample"][];
  riskSeries?: Schema["TimeseriesResponse"];
  volumeSeries?: Schema["TimeseriesResponse"];
  evaluation?: Schema["EvalReport"];
  events?: Schema["EventPage"];
  anomalies?: Schema["AnomalyPage"];
};
