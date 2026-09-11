import { afterEach, expect, it, vi } from "vitest";
import { POST } from "@/app/api/v1/episodes/[id]/people/prepare/route";

afterEach(() => vi.unstubAllGlobals());

it("keeps a real preparation response pending beyond the rewrite timeout and preserves backend status", async () => {
  vi.useFakeTimers();
  const fetchMock = vi.fn((_url: string) => new Promise<Response>((resolve) => {
    setTimeout(() => resolve(Response.json({ success: true, data: { people: [] } })), 45_000);
  }));
  vi.stubGlobal("fetch", fetchMock);
  try {
    let settled = false;
    const result = POST(new Request("http://localhost/api", { method: "POST" }), { params: Promise.resolve({ id: "66314" }) }).then((value) => { settled = true; return value; });
    await vi.advanceTimersByTimeAsync(31_000);
    expect(settled).toBe(false);
    await vi.advanceTimersByTimeAsync(14_000);
    expect((await result).status).toBe(200);
    expect(fetchMock.mock.calls[0][0]).toContain("/api/v1/episodes/66314/people/prepare");
  } finally { vi.useRealTimers(); }
});

it("forwards cancellation without starting another request", async () => {
  const controller = new AbortController();
  const fetchMock = vi.fn((_url: string, init: RequestInit) => new Promise<Response>((_resolve, reject) => {
    init.signal!.addEventListener("abort", () => reject(new Error("aborted")), { once: true });
  }));
  vi.stubGlobal("fetch", fetchMock);
  const result = POST(new Request("http://localhost/api", { method: "POST", signal: controller.signal }), { params: Promise.resolve({ id: "66314" }) });
  await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledOnce());
  controller.abort();
  expect((await result).status).toBe(499);
  expect(fetchMock.mock.calls[0][1].signal!.aborted).toBe(true);
});

it("does not hide backend conflicts or allow arbitrary proxy paths", async () => {
  const fetchMock = vi.fn(async () => Response.json({ error: { code: "PERSON_SOURCE_CHANGED" } }, { status: 409 }));
  vi.stubGlobal("fetch", fetchMock);
  const result = await POST(new Request("http://localhost/api", { method: "POST" }), { params: Promise.resolve({ id: "66314" }) });
  expect(result.status).toBe(409);
  expect(await result.json()).toEqual({ error: { code: "PERSON_SOURCE_CHANGED" } });
  const invalid = await POST(new Request("http://localhost/api", { method: "POST" }), { params: Promise.resolve({ id: "../other" }) });
  expect(invalid.status).toBe(400);
  expect(fetchMock).toHaveBeenCalledOnce();
});
