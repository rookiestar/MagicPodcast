import { afterEach, expect, it, vi } from "vitest";
import { POST } from "@/app/api/v1/episodes/[id]/copilot/questions/route";
afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); });
it("forwards question fields intact and streams events before the answer is complete", async () => {
  vi.useFakeTimers();
  const bytes = new TextEncoder();
  const stream = new ReadableStream({start(controller) {
    controller.enqueue(bytes.encode('data: {"type":"context"}\n\n'));
    setTimeout(() => {controller.enqueue(bytes.encode('data: {"type":"complete"}\n\n'));controller.close();}, 45_000);
  }});
  const fetchMock = vi.fn(async (_url: string, _init: RequestInit) => new Response(stream, {headers:{"Content-Type":"text/event-stream"}}));
  vi.stubGlobal("fetch",fetchMock);
  const body = JSON.stringify({question:"测试问题", target_person_id:9, include_private_note:false, profile_id:"balanced"});
  const response = await POST(new Request("http://localhost/api",{method:"POST",body}),{params:Promise.resolve({id:"66314"})});
  expect(fetchMock.mock.calls[0][1].body).toBe(body);
  expect(response.headers.get("Content-Type")).toBe("text/event-stream");
  const reader=response.body!.getReader();
  expect(new TextDecoder().decode((await reader.read()).value)).toContain('"context"');
  await vi.advanceTimersByTimeAsync(45_000);
  expect(new TextDecoder().decode((await reader.read()).value)).toContain('"complete"');
  expect((await reader.read()).done).toBe(true);
});

it("rejects an oversized declared body before proxying", async () => {
  const fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
  const request = {
    headers: new Headers({ "content-length": String((256 << 10) + 1) }),
    body: null,
    signal: new AbortController().signal,
  } as unknown as Request;
  const response = await POST(request, { params: Promise.resolve({ id: "66314" }) });
  expect(response.status).toBe(413);
  expect(await response.json()).toEqual({ success: false, error: { code: "REQUEST_BODY_TOO_LARGE", message: "问题内容过长，请缩短后重试。" } });
  expect(fetchMock).not.toHaveBeenCalled();
});

it("rejects an oversized streamed body without proxying it", async () => {
  const fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
  const chunk = new Uint8Array(200 << 10);
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(chunk);
      controller.enqueue(chunk);
      controller.close();
    },
  });
  const response = await POST(new Request("http://localhost/api", { method: "POST", body, duplex: "half" } as RequestInit), { params: Promise.resolve({ id: "66314" }) });
  expect(response.status).toBe(413);
  expect(fetchMock).not.toHaveBeenCalled();
});
