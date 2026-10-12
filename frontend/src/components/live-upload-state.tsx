"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { analyzeUpload, deleteUpload } from "@/lib/api/actions";
import { unwrap } from "@/lib/api/result";
import { safeError } from "@/lib/format";
import type { Upload } from "@/lib/types";
import { useWorkspace } from "./workspace";
import { UploadState } from "./upload-state";
export function LiveUploadState({ upload }: { upload: Upload }) {
  const { setUploads, setSelectedUploadId } = useWorkspace();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const router = useRouter();
  async function run(cancel: boolean, accept = false) {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      if (cancel) {
        unwrap(await deleteUpload(upload.id));
        setUploads((items) => items.filter((u) => u.id !== upload.id));
        setSelectedUploadId("");
        router.replace("/uploads");
        router.refresh();
      } else {
        const updated = unwrap(await analyzeUpload(upload.id, accept));
        setUploads((items) =>
          items.map((u) => (u.id === upload.id ? updated : u)),
        );
      }
    } catch (e) {
      setError(safeError(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <fieldset
        disabled={busy}
        className="upload-state-actions"
        aria-busy={busy}
      >
        <UploadState
          modal
          upload={upload}
          resume={(accept) => void run(false, accept)}
          cancel={() => void run(true)}
        />
        {upload.status === "queued" && (
          <button className="primary" onClick={() => void run(false)}>
            Start analysis
          </button>
        )}
      </fieldset>
      {error && (
        <p className="notice" role="alert">
          {error}
        </p>
      )}
    </>
  );
}
