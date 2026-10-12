import "server-only";
import { notFound } from "next/navigation";
import { serverApi, required } from "./server";
import type { LiveData } from "@/lib/live-data";

export async function loadScreen(
  screen: string[],
  query: Record<string, string | string[] | undefined>,
): Promise<LiveData> {
  const api = await serverApi();
  // These requests do not depend on one another. Start them together.
  const [uploads, incidentResponse, samples] = await Promise.all([
    api.GET("/v1/uploads", { params: { query: { limit: 100 } } }),
    screen[0] === "incidents"
      ? api.GET("/v1/incidents/{incidentId}", {
          params: { path: { incidentId: screen[1] } },
        })
      : null,
    screen[0] === "uploads" ? api.GET("/v1/samples") : null,
  ]);
  const page = required(uploads);
  const data: LiveData = {
    uploads: page.items,
    nextUploadCursor: page.nextCursor,
    selectedUploadId:
      typeof query.upload === "string"
        ? query.upload
        : (page.items[0]?.id ?? ""),
    incidents: [],
    summaries: [],
    samples: samples ? required(samples) : [],
  };
  if (incidentResponse) {
    if (incidentResponse.response.status === 404) notFound();
    const incident = required(incidentResponse);
    data.incidents = [incident];
    data.selectedUploadId = incident.uploadId;
  }
  const uploadId = data.selectedUploadId;
  if (uploadId && !data.uploads.some((u) => u.id === uploadId)) {
    const response = await api.GET("/v1/uploads/{uploadId}", {
      params: { path: { uploadId } },
    });
    if (response.response.status === 404) notFound();
    data.uploads.push(required(response));
  }
  const upload = data.uploads.find((u) => u.id === uploadId);
  if (!upload || !["completed", "partial"].includes(upload.status)) return data;
  const params = { path: { uploadId } };
  // All screen data is server-rendered; independent reads share one wait.
  const [summaries, risk, volume, evaluation, events, anomalies] =
    await Promise.all([
      ["triage", "incidents"].includes(screen[0])
        ? api.GET("/v1/uploads/{uploadId}/incidents", { params })
        : null,
      screen[0] === "triage"
        ? api.GET("/v1/uploads/{uploadId}/timeseries", {
            params: { ...params, query: { bucket: "10m" } },
          })
        : null,
      screen[0] === "triage"
        ? api.GET("/v1/uploads/{uploadId}/timeseries", {
            params: { ...params, query: { bucket: "1h" } },
          })
        : null,
      screen[0] === "scorecard" && upload.source === "sample"
        ? api.GET("/v1/uploads/{uploadId}/eval", { params })
        : null,
      screen[0] === "events"
        ? api.GET("/v1/uploads/{uploadId}/events", {
            params: {
              ...params,
              query: {
                limit: 100,
                // The API reads at most 200 characters of search.
                q:
                  typeof query.q === "string"
                    ? query.q.slice(0, 200) || undefined
                    : undefined,
              },
            },
          })
        : null,
      screen[0] === "anomalies"
        ? api.GET("/v1/uploads/{uploadId}/anomalies", {
            params: {
              ...params,
              query: {
                limit: 200,
                judgesDisagree: query.judgesDisagree === "true" || undefined,
              },
            },
          })
        : null,
    ]);
  if (summaries) data.summaries = required(summaries);
  if (risk) data.riskSeries = required(risk);
  if (volume) data.volumeSeries = required(volume);
  if (evaluation) data.evaluation = required(evaluation);
  if (events) data.events = required(events);
  if (anomalies) data.anomalies = required(anomalies);
  return data;
}
