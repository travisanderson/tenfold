import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    // Playwright owns tests/; keep unit discovery out of browser tests and builds.
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
