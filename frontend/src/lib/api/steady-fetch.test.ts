import { afterEach, describe, expect, it, vi } from "vitest";
import { steadyFetch } from "./steady-fetch";

const dropped = () => Promise.reject(new TypeError("fetch failed"));
const request = (method: string) =>
  new Request("https://api.test/v1/uploads", { method });

describe("steady fetch", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("tries a read once more after a dropped connection", async () => {
    const fetch = vi
      .fn()
      .mockImplementationOnce(dropped)
      .mockResolvedValueOnce(new Response("ok"));
    vi.stubGlobal("fetch", fetch);
    expect(await (await steadyFetch(request("GET"))).text()).toBe("ok");
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it("answers with a problem, not a crash, when the API stays down", async () => {
    vi.stubGlobal("fetch", vi.fn(dropped));
    const response = await steadyFetch(request("GET"));
    expect(response.status).toBe(503);
    expect((await response.json()).code).toBe("unavailable");
  });

  it("never repeats a write", async () => {
    const fetch = vi.fn(dropped);
    vi.stubGlobal("fetch", fetch);
    expect((await steadyFetch(request("POST"))).status).toBe(503);
    expect(fetch).toHaveBeenCalledTimes(1);
  });
});
