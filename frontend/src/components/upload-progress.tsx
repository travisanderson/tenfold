"use client";
import { useEffect, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { readUpload } from "@/lib/api/actions";
import { isJobFailure } from "@/lib/api/progress-stream";
import { unwrap } from "@/lib/api/result";
import type { Schema } from "@/lib/types";
import { useWorkspace } from "./workspace";

export function UploadProgress() {
  const { uploads, setUploads, demo, clearing } = useWorkspace();
  const router = useRouter();
  const pathname = usePathname();
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);
  const ids =
    demo || clearing
      ? ""
      : uploads
          .filter((u) => u.status === "processing")
          .map((u) => u.id)
          .sort()
          .join(",");
  useEffect(() => {
    if (!ids) return;
    let disposed = false;
    const sources = ids.split(",").map((id) => {
      const source = new EventSource(
        `/api/uploads/${encodeURIComponent(id)}/progress`,
      );
      async function finish(interrupted = false) {
        source.close();
        try {
          const upload = unwrap(await readUpload(id));
          if (disposed) return;
          setUploads((items) =>
            items.map((item) => (item.id === id ? upload : item)),
          );
          if (interrupted && upload.status === "processing")
            setError(
              "Progress connection interrupted. Reconnect to keep watching.",
            );
          if (upload.status === "completed" && pathname !== "/uploads")
            router.refresh();
        } catch {
          if (!disposed)
            setError("Could not refresh the upload. Reconnect to try again.");
        }
      }
      function progress(event: MessageEvent) {
        try {
          const update: Schema["ProgressEvent"] = JSON.parse(event.data);
          if (update.uploadId !== id) return;
          if (["partial", "completed", "failed"].includes(update.status)) {
            void finish();
            return;
          }
          setUploads((items) =>
            items.map((item) =>
              item.id === id
                ? {
                    ...item,
                    status: update.status,
                    stage: update.stage,
                    progress: update.progress,
                    pipeline: update.pipeline,
                    lineCount: update.counts.lines,
                    eventCount: update.counts.events,
                    anomalyCount: update.counts.anomalies,
                    incidentCount: update.counts.incidents,
                  }
                : item,
            ),
          );
        } catch {
          void finish(true);
        }
      }
      source.addEventListener("progress", progress);
      source.addEventListener("done", () => void finish());
      source.addEventListener(
        "error",
        (event) => void finish(!isJobFailure(event)),
      );
      return source;
    });
    return () => {
      disposed = true;
      sources.forEach((source) => source.close());
    };
  }, [ids, attempt, setUploads, pathname, router]);
  if (!error) return null;
  return (
    <p className="notice" role="alert">
      {error}{" "}
      <button
        onClick={() => {
          setError("");
          setAttempt((n) => n + 1);
        }}
      >
        Reconnect
      </button>
    </p>
  );
}
