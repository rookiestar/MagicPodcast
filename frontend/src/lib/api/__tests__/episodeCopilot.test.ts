import { afterEach, describe, expect, it, vi } from "vitest";
import {
  episodeCopilotApi,
  EpisodeCopilotCancellationError,
} from "../episodeCopilot";

const question = {
  question: "为什么需要收窄权限？",
  selection: "按次收窄",
  selection_source: "show_notes" as const,
  include_private_note: false,
};

function streamFromChunks(chunks: string[]) {
  const encoder = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(encoder.encode(chunk));
      }
      controller.close();
    },
  });
}

describe("episodeCopilotApi.ask", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it("parses split SSE events and sends only the question contract", async () => {
    const onEvent = vi.fn();
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        streamFromChunks([
          ": ping\n\n",
          'data: {"type":"answer_delta","message":"权',
          '限应收窄。","transcript_used":false,"private_note_included":false}\n\n',
          'data: {"type":"complete","message":"回答完成",',
          '"transcript_used":false,"private_note_included":false,',
          '"first_content_ms":120,"total_ms":420}\n\n',
        ]),
        { status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await episodeCopilotApi.ask(
      201,
      question,
      onEvent,
      new AbortController().signal,
    );

    expect(onEvent).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({
        type: "answer_delta",
        message: "权限应收窄。",
      }),
    );
    expect(onEvent).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        type: "complete",
        first_content_ms: 120,
        total_ms: 420,
      }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/episodes/201/copilot/questions"),
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify(question),
      }),
    );
    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(request.body))).toEqual(question);
  });

  it("surfaces a stable API error message", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            success: false,
            error: {
              code: "COPILOT_UNAVAILABLE",
              message: "本地 Runtime 未启用",
            },
          }),
          {
            status: 503,
            headers: { "Content-Type": "application/json" },
          },
        ),
      ),
    );

    await expect(
      episodeCopilotApi.ask(
        201,
        question,
        vi.fn(),
        new AbortController().signal,
      ),
    ).rejects.toThrow("本地 Runtime 未启用");
  });

  it("maps caller aborts to an explicit cancellation", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((_url, init) => {
        return new Promise<Response>((_resolve, reject) => {
          const signal = init?.signal as AbortSignal;
          signal.addEventListener(
            "abort",
            () => reject(new DOMException("aborted", "AbortError")),
            { once: true },
          );
        });
      }),
    );
    const controller = new AbortController();
    const request = episodeCopilotApi.ask(
      201,
      question,
      vi.fn(),
      controller.signal,
    );

    controller.abort();

    await expect(request).rejects.toBeInstanceOf(
      EpisodeCopilotCancellationError,
    );
  });

  it("rejects an already-cancelled request without calling fetch", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const controller = new AbortController();
    controller.abort();

    await expect(
      episodeCopilotApi.ask(201, question, vi.fn(), controller.signal),
    ).rejects.toBeInstanceOf(EpisodeCopilotCancellationError);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
