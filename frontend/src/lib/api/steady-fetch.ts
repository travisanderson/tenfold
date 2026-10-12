import type { components } from "./schema";

/**
 * A dropped connection must not crash a screen. A read is tried once more;
 * a request that still cannot connect comes back as a 503 problem, which
 * every screen already knows how to show. Writes are not repeated: the
 * first attempt may have reached the server.
 */
export async function steadyFetch(request: Request): Promise<Response> {
  const again = request.method === "GET" ? request.clone() : null;
  try {
    return await fetch(request);
  } catch {
    if (again)
      try {
        return await fetch(again);
      } catch {
        /* fall through to the problem below */
      }
    const problem: components["schemas"]["Problem"] = {
      type: "about:blank",
      title: "Service unavailable",
      status: 503,
      code: "unavailable",
      detail:
        "The analysis service could not be reached. Try again in a moment.",
    };
    return Response.json(problem, { status: 503 });
  }
}
