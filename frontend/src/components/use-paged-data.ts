"use client";
import { useEffect, useRef, useState } from "react";
import { safeError } from "@/lib/format";
type Page<T> = { items: T[]; total: number; nextCursor?: string };
export function usePagedData<T>(
  initial: Page<T> | undefined,
  fetchPage: (cursor?: string) => Promise<Page<T>>,
  enabled: boolean,
) {
  const [page, setPage] = useState<Page<T>>(initial ?? { items: [], total: 0 });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const generation = useRef(0);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    if (!enabled) return;
    const current = ++generation.current;
    const timer = setTimeout(async () => {
      setBusy(true);
      setError("");
      try {
        const data = await fetchPage();
        if (generation.current === current) setPage(data);
      } catch (e) {
        if (generation.current === current) setError(safeError(e));
      } finally {
        if (generation.current === current) setBusy(false);
      }
    }, 250);
    return () => {
      clearTimeout(timer);
      generation.current = current + 1;
    };
  }, [fetchPage, enabled, retry]);
  async function more() {
    if (busy || !page.nextCursor) return;
    const current = generation.current;
    setBusy(true);
    setError("");
    try {
      const data = await fetchPage(page.nextCursor);
      if (generation.current === current)
        setPage((previous) => ({
          ...data,
          items: [...previous.items, ...data.items],
        }));
    } catch (e) {
      if (generation.current === current) setError(safeError(e));
    } finally {
      if (generation.current === current) setBusy(false);
    }
  }
  return { page, busy, error, more, retry: () => setRetry((n) => n + 1) };
}
