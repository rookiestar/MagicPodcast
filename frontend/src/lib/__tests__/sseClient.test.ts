import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { sseRequest } from "../sseClient";

function streamFromChunks(chunks: string[]) {
  const encoder = new TextEncoder();

  return new ReadableStream({
    start(controller) {
      chunks.forEach((chunk) => controller.enqueue(encoder.encode(chunk)));
      controller.close();
    },
  });
}

function streamThatErrorsAfterFirstMessage() {
  const encoder = new TextEncoder();
  let reads = 0;

  return new ReadableStream({
    pull(controller) {
      if (reads === 0) {
        reads += 1;
        controller.enqueue(
          encoder.encode('data: {"type":"info","message":"started"}\n\n'),
        );
        return;
      }

      controller.error(new Error("stream broken"));
    },
  });
}

describe("sseRequest", () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    vi.spyOn(console, "log").mockImplementation(() => {});
    vi.spyOn(console, "warn").mockImplementation(() => {});
    vi.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
    globalThis.fetch = originalFetch;
  });

  it("parses split SSE messages and resolves on DONE", async () => {
    const onProgress = vi.fn();
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(
        streamFromChunks([
          ': ping\n\n',
          'data: {"type":"progress","message":"hel',
          'lo","current":1,"total":2}\n\n',
          "data: [DONE]\n\n",
        ]),
        { status: 200 },
      ),
    );

    await sseRequest(
      { endpoint: "/api/test", timeout: 1000, logPrefix: "[Test]" },
      onProgress,
    );

    expect(onProgress).toHaveBeenCalledWith(
      "progress",
      "hello",
      1,
      2,
      expect.objectContaining({ type: "progress" }),
    );
  });

  it("can preserve old sync behavior when a stream breaks after messages", async () => {
    const onProgress = vi.fn();
    globalThis.fetch = vi
      .fn()
      .mockResolvedValue(
        new Response(streamThatErrorsAfterFirstMessage(), { status: 200 }),
      );

    await expect(
      sseRequest(
        {
          endpoint: "/api/test",
          timeout: 1000,
          logPrefix: "[Test]",
          resolveOnStreamErrorAfterMessage: true,
        },
        onProgress,
      ),
    ).resolves.toBeUndefined();

    expect(onProgress).toHaveBeenCalledWith(
      "info",
      "started",
      undefined,
      undefined,
      expect.objectContaining({ type: "info" }),
    );
  });

  it("rejects when completion is required but the stream ends early", async () => {
    const onProgress = vi.fn();
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(
        streamFromChunks(['data: {"type":"info","message":"started"}\n\n']),
        { status: 200 },
      ),
    );

    await expect(
      sseRequest(
        {
          endpoint: "/api/test",
          timeout: 1000,
          logPrefix: "[Test]",
          requireCompletion: true,
          incompleteMessage: "missing completion",
        },
        onProgress,
      ),
    ).rejects.toThrow("missing completion");

    expect(onProgress).toHaveBeenCalledWith(
      "info",
      "started",
      undefined,
      undefined,
      expect.objectContaining({ type: "info" }),
    );
  });

  it("resolves required-completion streams when the custom completion event arrives", async () => {
    const onProgress = vi.fn();
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(
        streamFromChunks([
          'data: {"type":"summary","message":"finished"}\n\n',
          "data: [DONE]\n\n",
        ]),
        { status: 200 },
      ),
    );

    await expect(
      sseRequest(
        {
          endpoint: "/api/test",
          timeout: 1000,
          logPrefix: "[Test]",
          requireCompletion: true,
          isComplete: (data) => data.type === "summary",
        },
        onProgress,
      ),
    ).resolves.toBeUndefined();

    expect(onProgress).toHaveBeenCalledWith(
      "summary",
      "finished",
      undefined,
      undefined,
      expect.objectContaining({ type: "summary" }),
    );
  });

  it("uses the configured timeout message when the response never arrives", async () => {
    vi.useFakeTimers();

    const onProgress = vi.fn();
    globalThis.fetch = vi.fn((_url, init) => {
      return new Promise<Response>((_resolve, reject) => {
        const signal = init?.signal as AbortSignal | undefined;
        signal?.addEventListener("abort", () => {
          reject(new DOMException("aborted", "AbortError"));
        });
      });
    }) as typeof fetch;

    const request = sseRequest(
      {
        endpoint: "/api/test",
        timeout: 1000,
        logPrefix: "[Test]",
        timeoutMessage: "自定义超时",
      },
      onProgress,
    );

    const assertion = expect(request).rejects.toThrow("自定义超时");
    await vi.advanceTimersByTimeAsync(1000);
    await assertion;
  });
});
