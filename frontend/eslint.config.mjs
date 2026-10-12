import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";
export default defineConfig([
  ...nextVitals,
  ...nextTs,
  // React Compiler is not enabled; TanStack's mutable table/virtual APIs are intentional.
  { rules: { "react-hooks/incompatible-library": "off" } },
  globalIgnores([
    ".next/**",
    "src/lib/api/schema.d.ts",
    "next-env.d.ts",
    "test-results/**",
    "playwright-report/**",
  ]),
]);
