import { resolveApiBaseUrl } from "@/lib/apiBaseUrl";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const maxQuestionBodyBytes = 256 << 10;

class RequestBodyTooLargeError extends Error {
  constructor() {
    super("question request body is too large");
    this.name = "RequestBodyTooLargeError";
  }
}

async function readBoundedBody(request: Request): Promise<string> {
  const declaredLength = request.headers.get("content-length");
  if (declaredLength !== null) {
    const length = Number(declaredLength);
    if (!Number.isSafeInteger(length) || length < 0 || length > maxQuestionBodyBytes) {
      throw new RequestBodyTooLargeError();
    }
  }

  if (!request.body) return "";
  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  const cancelOnAbort = () => {
    void reader.cancel();
  };
  request.signal.addEventListener("abort", cancelOnAbort, { once: true });
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      if (!value) continue;
      total += value.byteLength;
      if (total > maxQuestionBodyBytes) {
        await reader.cancel();
        throw new RequestBodyTooLargeError();
      }
      chunks.push(value);
    }
  } finally {
    request.signal.removeEventListener("abort", cancelOnAbort);
    reader.releaseLock();
  }
  const bytes = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return new TextDecoder().decode(bytes);
}

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
    const body = await readBoundedBody(request);
    const response = await fetch(`${resolveApiBaseUrl(false)}/api/v1/episodes/${id}/copilot/questions`, {
      method: "POST", headers, body, cache: "no-store", signal: request.signal,
    });
    return new Response(response.body, {
      status: response.status,
      headers: { "Content-Type": response.headers.get("Content-Type") ?? "application/json", "Cache-Control": "no-cache, no-transform", "X-Accel-Buffering": "no" },
    });
  } catch (error) {
    if (error instanceof RequestBodyTooLargeError) {
      return Response.json({ success: false, error: { code: "REQUEST_BODY_TOO_LARGE", message: "问题内容过长，请缩短后重试。" } }, { status: 413 });
    }
    return Response.json({ success: false, error: { code: request.signal.aborted ? "CANCELLED" : "COPILOT_UNAVAILABLE", message: request.signal.aborted ? "回答已取消。" : "助手服务暂时不可用，请重试。" } }, { status: request.signal.aborted ? 499 : 502 });
  }
}
