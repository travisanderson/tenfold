import { accessToken } from "@/lib/supabase/server";
import { apiUrl } from "@/lib/api/server";
export const dynamic = "force-dynamic";
export async function GET(
  request: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const token = await accessToken();
  if (!token)
    return Response.json({ detail: "Sign in to continue." }, { status: 401 });
  const { id } = await params;
  try {
    const upstream = await fetch(
      `${apiUrl()}/v1/uploads/${encodeURIComponent(id)}/progress`,
      {
        headers: { Authorization: `Bearer ${token}` },
        signal: request.signal,
        cache: "no-store",
      },
    );
    return new Response(upstream.body, {
      status: upstream.status,
      headers: {
        "Content-Type":
          upstream.headers.get("Content-Type") ?? "text/event-stream",
        "Cache-Control": "private, no-cache, no-transform",
        "X-Accel-Buffering": "no",
      },
    });
  } catch {
    return Response.json(
      { detail: "Progress connection interrupted. Retry to reconnect." },
      { status: 502 },
    );
  }
}
