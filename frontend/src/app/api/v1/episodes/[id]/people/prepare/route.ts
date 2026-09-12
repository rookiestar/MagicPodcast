import { resolveApiBaseUrl } from "@/lib/apiBaseUrl";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// Identity preparation already has a 150-second backend budget. A dedicated
// route avoids the rewrite proxy's 30-second socket limit and forwards browser
// cancellation to the same backend request instead of leaving it running.
export async function POST(request: Request, context: { params: Promise<{ id: string }> }) {
  const { id } = await context.params;
  if (!/^[1-9]\d*$/.test(id)) return Response.json({ success: false }, { status: 400 });
  const headers = new Headers({ "Content-Type": "application/json", Accept: request.headers.get("accept") ?? "application/json" });
  for (const name of ["authorization", "cookie"]) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }
  try {
    const response = await fetch(`${resolveApiBaseUrl(false)}/api/v1/episodes/${id}/people/prepare`, {
      method: "POST", headers, body: "{}", cache: "no-store", signal: request.signal,
    });
    return new Response(response.body, {
      status: response.status,
      headers: { "Content-Type": response.headers.get("Content-Type") ?? "application/json", "Cache-Control": "no-store, no-transform", "X-Accel-Buffering": "no" },
    });
  } catch {
    return Response.json({ success: false, error: { code: request.signal.aborted ? "PERSON_PREPARATION_CANCELLED" : "PERSON_PREPARATION_UNAVAILABLE", message: request.signal.aborted ? "人物识别已取消。" : "人物服务暂时不可用，请重试。" } }, { status: request.signal.aborted ? 499 : 502 });
  }
}
