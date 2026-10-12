import "server-only";
import { notFound } from "next/navigation";
import { serverApi, required } from "./server";
import type { LiveData } from "@/lib/live-data";

export async function loadScreen(
  screen: string[],
  query: Record<string, string | string[] | undefined>,
): Promise<LiveData> {
  const api = await serverApi();
  const page = required(
    await api.GET("/v1/uploads", { params: { query: { limit: 100 } } }),
  );
  const data: LiveData = {
    uploads: page.items,
    nextUploadCursor: page.nextCursor,
    selectedUploadId:
      typeof query.upload === "string"
        ? query.upload
        : (page.items[0]?.id ?? ""),
    incidents: [],
    summaries: [],
    samples: [],
  };
  if (screen[0] === "incidents") {
    const response = await api.GET("/v1/incidents/{incidentId}", {
      params: { path: { incidentId: screen[1] } },
    });
    if (response.response.status === 404) notFound();
    const incident = required(response);
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
  if (screen[0] === "uploads")
    data.samples = required(await api.GET("/v1/samples"));
  const upload = data.uploads.find((u) => u.id === uploadId);
  if (!upload || !["completed", "partial"].includes(upload.status)) return data;
  const params = { path: { uploadId } };
  if (["triage", "incidents"].includes(screen[0]))
    data.summaries = required(
      await api.GET("/v1/uploads/{uploadId}/incidents", { params }),
    );
  if (screen[0] === "triage") {
    const [risk, volume] = await Promise.all([
      api.GET("/v1/uploads/{uploadId}/timeseries", {
        params: { ...params, query: { bucket: "10m" } },
      }),
      api.GET("/v1/uploads/{uploadId}/timeseries", {
        params: { ...params, query: { bucket: "1h" } },
      }),
    ]);
    data.riskSeries = required(risk);
    data.volumeSeries = required(volume);
  }
  if (screen[0] === "scorecard" && upload.source === "sample")
    data.evaluation = required(
      await api.GET("/v1/uploads/{uploadId}/eval", { params }),
    );
  if (screen[0] === "events")
    data.events = required(
      await api.GET("/v1/uploads/{uploadId}/events", {
        params: {
          ...params,
          query: {
            limit: 200,
            q: typeof query.q === "string" ? query.q : undefined,
          },
        },
      }),
    );
  if (screen[0] === "anomalies")
    data.anomalies = required(
      await api.GET("/v1/uploads/{uploadId}/anomalies", {
        params: {
          ...params,
          query: {
            limit: 200,
            judgesDisagree: query.judgesDisagree === "true" || undefined,
          },
        },
      }),
    );
  return data;
}
