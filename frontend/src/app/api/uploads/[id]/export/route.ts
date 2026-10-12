import { accessToken } from "@/lib/supabase/server";
import { apiUrl } from "@/lib/api/server";
export const dynamic = "force-dynamic";
export async function GET(
  request: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const token = await accessToken();
  if (!token) return Response.redirect(new URL("/sign-in", request.url));
  const { id } = await params;
  const upstream = await fetch(
    `${apiUrl()}/v1/uploads/${encodeURIComponent(id)}/events/export`,
    {
      headers: { Authorization: `Bearer ${token}` },
      signal: request.signal,
      cache: "no-store",
    },
  );
  if (upstream.status === 401)
    return Response.redirect(new URL("/sign-in", request.url));
  return new Response(upstream.body, {
    status: upstream.status,
    headers: {
      "Content-Type":
        upstream.headers.get("Content-Type") ?? "application/x-ndjson",
      "Cache-Control": "private, no-store",
      ...(upstream.ok
        ? { "Content-Disposition": 'attachment; filename="events.ndjson"' }
        : {}),
    },
  });
}
