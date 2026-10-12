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
  const [loading, setLoading] = useState<"replace" | "append" | null>(null);
  const busy = loading !== null;
  const initialRequest = useRef(fetchPage);
  const [error, setError] = useState("");
  const generation = useRef(0);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    if (!enabled) return;
    // Hydration already has the server-rendered first page, even in Strict Mode.
    if (initial && fetchPage === initialRequest.current && retry === 0) return;
    const current = ++generation.current;
    const timer = setTimeout(async () => {
      setLoading("replace");
      setError("");
      try {
        const data = await fetchPage();
        if (generation.current === current) setPage(data);
      } catch (e) {
        if (generation.current === current) setError(safeError(e));
      } finally {
        if (generation.current === current) setLoading(null);
      }
    }, 250);
    return () => {
      clearTimeout(timer);
      generation.current = current + 1;
    };
  }, [fetchPage, enabled, retry, initial]);
  async function more() {
    if (busy || !page.nextCursor) return;
    const current = generation.current;
    setLoading("append");
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
      if (generation.current === current) setLoading(null);
    }
  }
  return {
    page,
    busy,
    loadingMore: loading === "append",
    error,
    more,
    retry: () => setRetry((n) => n + 1),
  };
}
