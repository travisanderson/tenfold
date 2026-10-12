import { describe, expect, it } from "vitest";
import { isJobFailure } from "./progress-stream";

describe("progress stream error events", () => {
  it("reads an error event with a Problem body as a failed job", () => {
    const sent = new MessageEvent("error", {
      data: '{"title":"Analysis failed","status":500,"code":"analysis_failed"}',
    });
    expect(isJobFailure(sent)).toBe(true);
  });
  it("reads a bare error event as a dropped connection", () => {
    expect(isJobFailure(new Event("error"))).toBe(false);
    expect(isJobFailure(new MessageEvent("error", { data: "" }))).toBe(false);
  });
});
