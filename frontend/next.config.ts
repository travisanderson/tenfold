import type { NextConfig } from "next";
const config: NextConfig = {
  poweredByHeader: false,
  devIndicators: false,
  turbopack: { root: process.cwd() },
  // Only the Docker image asks for a standalone build (see Dockerfile).
  // Vercel does not set this, so its build is unchanged.
  output: process.env.NEXT_OUTPUT === "standalone" ? "standalone" : undefined,
};
export default config;
