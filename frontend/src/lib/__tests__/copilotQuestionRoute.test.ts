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
