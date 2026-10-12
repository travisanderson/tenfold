import type { createApiClient } from "./client";

type Api = ReturnType<typeof createApiClient>;
export type ClearDataResult = {
  deletedUploads: string[];
  deletedLessons: string[];
  error?: string;
};

/** Snapshot every page before deleting: deletion can invalidate pagination cursors. */
export async function clearWorkspaceData(
  api: Api,
  includeLessons: boolean,
): Promise<ClearDataResult> {
  const result: ClearDataResult = { deletedUploads: [], deletedLessons: [] };
  const uploads = new Set<string>();
  let cursor: string | undefined;
  do {
    const page = await api.GET("/v1/uploads", {
      params: { query: { limit: 100, cursor } },
    });
    if (!page.data)
      return {
        ...result,
        error:
          page.error?.detail || "Could not list uploads. No data was cleared.",
      };
    page.data.items.forEach((upload) => uploads.add(upload.id));
    cursor = page.data.nextCursor;
  } while (cursor);

  const lessons: string[] = [];
  if (includeLessons) {
    const response = await api.GET("/v1/lessons");
    if (!response.data)
      return {
        ...result,
        error:
          response.error?.detail ||
          "Could not list lessons. No data was cleared.",
      };
    lessons.push(...response.data.map((lesson) => lesson.id));
  }
  for (const uploadId of uploads) {
    const response = await api.DELETE("/v1/uploads/{uploadId}", {
      params: { path: { uploadId } },
    });
    if (!response.response.ok && response.response.status !== 404)
      return {
        ...result,
        error:
          response.error?.detail ||
          "An upload could not be deleted. Try again to clear the remaining data.",
      };
    result.deletedUploads.push(uploadId);
  }
  for (const lessonId of lessons) {
    const response = await api.DELETE("/v1/lessons/{lessonId}", {
      params: { path: { lessonId } },
    });
    if (!response.response.ok && response.response.status !== 404)
      return {
        ...result,
        error:
          response.error?.detail ||
          "A lesson could not be deleted. Try again to clear the remaining data.",
      };
    result.deletedLessons.push(lessonId);
  }
  return result;
}
