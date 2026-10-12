import "server-only";
import createClient from "openapi-fetch";
import type { paths } from "./schema";

/** The HTTP boundary for the backend handoff. Use only from a server module.
 * The browser-facing preview currently uses typed, session-local fixtures.
 */
export function createApiClient(baseUrl: string, accessToken: string) {
  return createClient<paths>({
    baseUrl,
    headers: { Authorization: `Bearer ${accessToken}` },
    cache: "no-store",
  });
}
