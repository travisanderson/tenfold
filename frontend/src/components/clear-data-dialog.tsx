"use client";
import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { clearData } from "@/lib/api/actions";
import { useWorkspace } from "./workspace";

export function ClearDataDialog({ close }: { close: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const cancelButton = useRef<HTMLButtonElement>(null);
  const [includeLessons, setIncludeLessons] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const {
    demo,
    setUploads,
    setIncidents,
    setLessons,
    setSelectedUploadId,
    setClearing,
  } = useWorkspace();
  const router = useRouter();
  useEffect(() => {
    const element = dialog.current;
    const prior = document.activeElement as HTMLElement | null;
    element?.showModal();
    cancelButton.current?.focus();
    return () => {
      element?.close();
      prior?.focus();
    };
  }, []);
  async function submit() {
    if (busy) return;
    setBusy(true);
    setClearing(true);
    setError("");
    try {
      if (!demo) {
        const result = await clearData(includeLessons);
        setUploads((items) =>
          items.filter((u) => !result.deletedUploads.includes(u.id)),
        );
        setIncidents((items) =>
          items.filter((i) => !result.deletedUploads.includes(i.uploadId)),
        );
        setLessons((items) =>
          items.filter((l) => !result.deletedLessons.includes(l.id)),
        );
        if (result.error) {
          setError(
            `${result.deletedUploads.length} uploads and ${result.deletedLessons.length} lessons cleared. ${result.error}`,
          );
          return;
        }
      }
      setUploads([]);
      setIncidents([]);
      setSelectedUploadId("");
      if (includeLessons) setLessons([]);
      if (demo) {
        router.replace("/uploads");
        close();
      }
    } catch {
      setError(
        "Could not finish clearing data. Some items may already be deleted. Try again to clear what remains.",
      );
    } finally {
      setBusy(false);
      setClearing(false);
    }
  }
  return (
    <dialog
      ref={dialog}
      className="teach-dialog"
      aria-labelledby="clear-data-title"
      aria-describedby="clear-data-description"
      onCancel={(event) => {
        event.preventDefault();
        if (!busy) close();
      }}
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          void submit();
        }}
      >
        <h2 id="clear-data-title">Clear your data?</h2>
        <p id="clear-data-description">
          This permanently deletes all your uploaded files and demo runs,
          including their events, incidents, notes, and verdicts. Running
          analyses will stop. This cannot be undone.
        </p>
        <label className="checkbox-label">
          <input
            type="checkbox"
            checked={includeLessons}
            disabled={busy}
            onChange={(event) => setIncludeLessons(event.target.checked)}
          />
          Also delete saved lessons
        </label>
        <p>
          {includeLessons
            ? "Saved lessons will also be deleted, so future uploads start fresh."
            : "Saved lessons will still apply to future uploads."}{" "}
          Your account stays signed in.
        </p>
        {error && (
          <p className="notice" role="alert">
            {error}
          </p>
        )}
        <div className="dialog-actions">
          <button
            type="button"
            ref={cancelButton}
            disabled={busy}
            onClick={close}
          >
            Cancel
          </button>
          <button className="danger-button" disabled={busy}>
            {busy ? "Clearing data…" : "Clear data"}
          </button>
        </div>
      </form>
    </dialog>
  );
}
