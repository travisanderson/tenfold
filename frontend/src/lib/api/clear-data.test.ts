import { describe, expect, it, vi } from "vitest";
import { clearWorkspaceData } from "./clear-data";
import type { createApiClient } from "./client";
type Api = ReturnType<typeof createApiClient>;
const ok = (data: unknown) => ({
  data,
  response: new Response(null, { status: 200 }),
});
const deleted = (status = 204) => ({
  response: new Response(null, { status }),
});

describe("clear workspace data", () => {
  it("snapshots every upload page before deleting and keeps lessons by default", async () => {
    const calls: string[] = [];
    const api = {
      GET: vi.fn(async (_path, options) => {
        calls.push("list");
        return options.params.query.cursor
          ? ok({ items: [{ id: "older" }], total: 2 })
          : ok({ items: [{ id: "latest" }], total: 2, nextCursor: "page2" });
      }),
      DELETE: vi.fn(async (_path, options) => {
        calls.push(options.params.path.uploadId);
        return deleted();
      }),
    };
    expect(await clearWorkspaceData(api as unknown as Api, false)).toEqual({
      deletedUploads: ["latest", "older"],
      deletedLessons: [],
    });
    expect(calls).toEqual(["list", "list", "latest", "older"]);
    expect(api.GET).toHaveBeenCalledTimes(2);
  });
  it("only deletes lessons when requested, and tolerates already-deleted items", async () => {
    const api = {
      GET: vi
        .fn()
        .mockResolvedValueOnce(ok({ items: [{ id: "u1" }], total: 1 }))
        .mockResolvedValueOnce(ok([{ id: "l1" }])),
      DELETE: vi
        .fn()
        .mockResolvedValueOnce(deleted(404))
        .mockResolvedValueOnce(deleted()),
    };
    expect(await clearWorkspaceData(api as unknown as Api, true)).toEqual({
      deletedUploads: ["u1"],
      deletedLessons: ["l1"],
    });
    expect(api.DELETE).toHaveBeenLastCalledWith("/v1/lessons/{lessonId}", {
      params: { path: { lessonId: "l1" } },
    });
  });
  it("does not delete anything if a later listing fails", async () => {
    const api = {
      GET: vi
        .fn()
        .mockResolvedValueOnce(
          ok({ items: [{ id: "u1" }], total: 2, nextCursor: "page2" }),
        )
        .mockResolvedValueOnce({
          error: { detail: "Unavailable" },
          response: new Response(null, { status: 503 }),
        }),
      DELETE: vi.fn(),
    };
    const result = await clearWorkspaceData(api as unknown as Api, false);
    expect(result.error).toBe("Unavailable");
    expect(result.deletedUploads).toEqual([]);
    expect(api.DELETE).not.toHaveBeenCalled();
  });
  it("reports partial deletion without claiming that everything was cleared", async () => {
    const api = {
      GET: vi
        .fn()
        .mockResolvedValue(
          ok({ items: [{ id: "u1" }, { id: "u2" }, { id: "u3" }], total: 3 }),
        ),
      DELETE: vi
        .fn()
        .mockResolvedValueOnce(deleted())
        .mockResolvedValueOnce({
          error: { detail: "Try later" },
          response: new Response(null, { status: 503 }),
        }),
    };
    const result = await clearWorkspaceData(api as unknown as Api, false);
    expect(result.deletedUploads).toEqual(["u1"]);
    expect(result.error).toBe("Try later");
    expect(api.DELETE).toHaveBeenCalledTimes(2);
  });
});
