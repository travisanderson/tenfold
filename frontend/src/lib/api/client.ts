import "server-only";
import createClient from "openapi-fetch";
import type { paths } from "./schema";

/** Authenticated backend boundary. Never import into a browser module. */
export function createApiClient(baseUrl: string, accessToken: string) {
  return createClient<paths>({
    baseUrl,
    headers: { Authorization: `Bearer ${accessToken}` },
    cache: "no-store",
  });
}
