"use server";
import { serverApi, result } from "./server";
import type { Schema } from "@/lib/types";
import type { operations } from "./schema";

export async function loadSample(name = "attack-day") {
  const api = await serverApi();
  return result(
    await api.POST("/v1/samples/{sampleName}/load", {
      params: { path: { sampleName: name } },
    }),
  );
}
export async function readUpload(uploadId: string) {
  const api = await serverApi();
  return result(
    await api.GET("/v1/uploads/{uploadId}", { params: { path: { uploadId } } }),
  );
}
export async function registerUpload(body: Schema["CreateUploadRequest"]) {
  const api = await serverApi();
  return result(await api.POST("/v1/uploads", { body }));
}
export async function analyzeUpload(
  uploadId: string,
  acceptParseErrors = false,
  restart = false,
) {
  const api = await serverApi();
  return result(
    await api.POST("/v1/uploads/{uploadId}/analyze", {
      params: { path: { uploadId } },
      body: { acceptParseErrors, restart },
    }),
  );
}
export async function deleteUpload(uploadId: string) {
  const api = await serverApi();
  return result(
    await api.DELETE("/v1/uploads/{uploadId}", {
      params: { path: { uploadId } },
    }),
  );
}
export async function eventPage(
  uploadId: string,
  query: operations["listEvents"]["parameters"]["query"],
) {
  const api = await serverApi();
  return result(
    await api.GET("/v1/uploads/{uploadId}/events", {
      params: { path: { uploadId }, query },
    }),
  );
}
export async function anomalyPage(
  uploadId: string,
  query: operations["listAnomalies"]["parameters"]["query"],
) {
  const api = await serverApi();
  return result(
    await api.GET("/v1/uploads/{uploadId}/anomalies", {
      params: { path: { uploadId }, query },
    }),
  );
}
export async function saveVerdict(
  incidentId: string,
  body: Schema["VerdictRequest"],
) {
  const api = await serverApi();
  return result(
    await api.POST("/v1/incidents/{incidentId}/verdict", {
      params: { path: { incidentId } },
      body,
    }),
  );
}
export async function assignIncident(
  incidentId: string,
  assignedToMe: boolean,
) {
  const api = await serverApi();
  return result(
    await api.PATCH("/v1/incidents/{incidentId}", {
      params: { path: { incidentId } },
      body: { assignedToMe },
    }),
  );
}
export async function addNote(incidentId: string, text: string) {
  const api = await serverApi();
  return result(
    await api.POST("/v1/incidents/{incidentId}/notes", {
      params: { path: { incidentId } },
      body: { text },
    }),
  );
}
export async function previewLesson(
  uploadId: string,
  scope: Schema["LessonScope"],
) {
  const api = await serverApi();
  return result(
    await api.POST("/v1/lessons/preview", { body: { uploadId, scope } }),
  );
}

export async function uploadPage(cursor: string) {
  const api = await serverApi();
  return result(
    await api.GET("/v1/uploads", { params: { query: { cursor, limit: 100 } } }),
  );
}
