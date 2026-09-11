import { resolveApiBaseUrl } from "@/lib/apiBaseUrl";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// Pass the existing SSE contract through without the generic rewrite socket
// timeout or buffering the full answer. The backend remains the question and
// source authority; caller cancellation also terminates its request.
export async function POST(request: Request, context: { params: Promise<{ id: string }> }) {
  const { id } = await context.params;
  if (!/^[1-9]\d*$/.test(id)) return Response.json({ success: false }, { status: 400 });
  const headers = new Headers({ "Content-Type": "application/json", Accept: "text/event-stream" });
  for (const name of ["authorization", "cookie"]) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }
  try {
    const body = await request.text();
    const response = await fetch(`${resolveApiBaseUrl(false)}/api/v1/episodes/${id}/copilot/questions`, {
      method: "POST", headers, body, cache: "no-store", signal: request.signal,
    });
    return new Response(response.body, {
      status: response.status,
      headers: { "Content-Type": response.headers.get("Content-Type") ?? "application/json", "Cache-Control": "no-cache, no-transform", "X-Accel-Buffering": "no" },
    });
  } catch {
    return Response.json({ success: false, error: { code: request.signal.aborted ? "CANCELLED" : "COPILOT_UNAVAILABLE", message: request.signal.aborted ? "回答已取消。" : "助手服务暂时不可用，请重试。" } }, { status: request.signal.aborted ? 499 : 502 });
  }
}
