import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { StrictMode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import EpisodeCopilotPanel from "../EpisodeCopilotPanel";
import { episodeCopilotApi } from "@/lib/api/episodeCopilot";
import type { ConsumptionItem } from "@/types/consumption";

const copilotMocks = vi.hoisted(() => ({
  getContext: vi.fn(),
  ask: vi.fn(),
  isCancellation: vi.fn(),
}));

vi.mock("@/lib/api/episodeCopilot", () => ({
  episodeCopilotApi: {
    getContext: copilotMocks.getContext,
    ask: copilotMocks.ask,
  },
  isEpisodeCopilotCancellation: copilotMocks.isCancellation,
}));

const item: ConsumptionItem = {
  episode_id: 201,
  podcast_id: 20,
  podcast_title: "测试节目",
  podcast_author: "测试作者",
  podcast_cover_url: "",
  episode_title: "单集助手测试",
  episode_no: "201",
  duration: 2400,
  published_date: "2026-08-10T08:00:00Z",
  show_notes: "Runtime permissions are reduced per turn.",
  show_notes_document: {
    content: "Runtime permissions are reduced per turn.",
    format: "markdown",
  },
  original_url: "https://example.com/episode/201",
  image_url: "",
  notes: "私有备注",
  tags: [],
  queue_state: "focus",
};

describe("EpisodeCopilotPanel", () => {
  beforeEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    vi.clearAllMocks();
    copilotMocks.isCancellation.mockReturnValue(false);
    vi.mocked(episodeCopilotApi.getContext).mockResolvedValue({
      episode_id: 201,
      show_notes_available: true,
      transcript_available: false,
      private_note_available: true,
    });
    vi.mocked(episodeCopilotApi.ask).mockImplementation(
      async (_episodeId, _request, onEvent) => {
        onEvent({
          type: "context",
          message: "未使用逐字稿",
          transcript_used: false,
          private_note_included: true,
        });
        onEvent({
          type: "answer_delta",
          message: "## 回答\n\n权限应按次收窄。",
          transcript_used: false,
          private_note_included: true,
        });
        onEvent({
          type: "complete",
          message: "回答完成",
          transcript_used: false,
          private_note_included: true,
          first_content_ms: 120,
          total_ms: 420,
        });
      },
    );
  });

  it("captures current-episode Show Notes selection and streams a sourced answer", async () => {
    render(
      <StrictMode>
        <div
          data-copilot-source="show_notes"
          data-copilot-episode-id="201"
        >
          Runtime permissions are reduced per turn.
        </div>
        <EpisodeCopilotPanel item={item} />
      </StrictMode>,
    );

    expect(
      await screen.findByText("当前无成功逐字稿，将明确降级为 Show Notes。"),
    ).toBeInTheDocument();

    const source = screen.getByText(
      "Runtime permissions are reduced per turn.",
    );
    const range = document.createRange();
    range.selectNodeContents(source);
    vi.spyOn(window, "getSelection").mockReturnValue({
      isCollapsed: false,
      rangeCount: 1,
      getRangeAt: () => range,
      toString: () => "Runtime permissions are reduced per turn.",
    } as unknown as Selection);
    fireEvent(document, new Event("selectionchange"));

    expect(await screen.findByText("已选 Show Notes")).toBeInTheDocument();
    fireEvent.change(screen.getByRole("textbox", { name: "向单集助手提问" }), {
      target: { value: "为什么需要按次收窄权限？" },
    });
    fireEvent.click(
      screen.getByRole("checkbox", { name: /本次包含我的私有备注/ }),
    );
    fireEvent.click(screen.getByRole("button", { name: "提问" }));

    await waitFor(() =>
      expect(episodeCopilotApi.ask).toHaveBeenCalledWith(
        201,
        {
          question: "为什么需要按次收窄权限？",
          selection: "Runtime permissions are reduced per turn.",
          selection_source: "show_notes",
          include_private_note: true,
        },
        expect.any(Function),
        expect.any(AbortSignal),
      ),
    );
    expect(await screen.findByText("权限应按次收窄。")).toBeInTheDocument();
    expect(screen.getByText("首字 120ms · 完成 420ms")).toBeInTheDocument();
    expect(
      screen.getByRole("checkbox", { name: /本次包含我的私有备注/ }),
    ).not.toBeChecked();
  });

  it("keeps the question, selection, and partial answer after failure, then retries", async () => {
    vi.mocked(episodeCopilotApi.ask)
      .mockImplementationOnce(async (_episodeId, _request, onEvent) => {
        onEvent({
          type: "answer_delta",
          message: "## 回答\n\n已生成的部分。",
          transcript_used: false,
          private_note_included: false,
        });
        throw new Error("连接中断");
      })
      .mockImplementationOnce(async (_episodeId, _request, onEvent) => {
        onEvent({
          type: "answer_delta",
          message: "## 回答\n\n重试后的完整回答。",
          transcript_used: false,
          private_note_included: false,
        });
        onEvent({
          type: "complete",
          message: "回答完成",
          transcript_used: false,
          private_note_included: false,
          first_content_ms: 300,
          total_ms: 900,
        });
      });

    render(
      <StrictMode>
        <div
          data-copilot-source="show_notes"
          data-copilot-episode-id="201"
        >
          Runtime permissions are reduced per turn.
        </div>
        <EpisodeCopilotPanel item={item} />
      </StrictMode>,
    );
    await screen.findByText(
      "当前无成功逐字稿，将明确降级为 Show Notes。",
    );
    const source = screen.getByText(
      "Runtime permissions are reduced per turn.",
    );
    const range = document.createRange();
    range.selectNodeContents(source);
    vi.spyOn(window, "getSelection").mockReturnValue({
      isCollapsed: false,
      rangeCount: 1,
      getRangeAt: () => range,
      toString: () => "Runtime permissions are reduced per turn.",
    } as unknown as Selection);
    fireEvent(document, new Event("selectionchange"));

    const question = screen.getByRole("textbox", {
      name: "向单集助手提问",
    });
    fireEvent.change(question, { target: { value: "失败时保留什么？" } });
    fireEvent.click(
      screen.getByRole("checkbox", { name: /本次包含我的私有备注/ }),
    );
    fireEvent.click(screen.getByRole("button", { name: "提问" }));

    expect(await screen.findByText("已生成的部分。")).toBeInTheDocument();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "问题、选区和已有答案已保留",
    );
    expect(
      screen.queryByText("正在继续生成回答与来源…"),
    ).not.toBeInTheDocument();
    expect(question).toHaveValue("失败时保留什么？");
    expect(screen.getByText("已选 Show Notes")).toBeInTheDocument();
    expect(
      screen.getByRole("checkbox", { name: /本次包含我的私有备注/ }),
    ).not.toBeChecked();

    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    expect(
      await screen.findByText("重试后的完整回答。"),
    ).toBeInTheDocument();
    expect(screen.queryByText("已生成的部分。")).not.toBeInTheDocument();
    expect(question).toHaveValue("失败时保留什么？");
    expect(episodeCopilotApi.ask).toHaveBeenNthCalledWith(
      2,
      201,
      expect.objectContaining({ include_private_note: true }),
      expect.any(Function),
      expect.any(AbortSignal),
    );
  });

  it("keeps reading usable during a slow answer and supports cancellation", async () => {
    copilotMocks.isCancellation.mockImplementation(
      (error: unknown) =>
        error instanceof Error && error.message === "cancelled",
    );
    vi.mocked(episodeCopilotApi.ask).mockImplementation(
      async (_episodeId, _request, _onEvent, signal) =>
        new Promise<void>((_resolve, reject) => {
          signal.addEventListener(
            "abort",
            () => reject(new Error("cancelled")),
            { once: true },
          );
        }),
    );

    render(
      <>
        <div>单集正文仍然可读</div>
        <EpisodeCopilotPanel item={item} />
      </>,
    );
    await screen.findByText(
      "当前无成功逐字稿，将明确降级为 Show Notes。",
    );
    fireEvent.change(screen.getByRole("textbox", { name: "向单集助手提问" }), {
      target: { value: "慢请求测试" },
    });
    vi.useFakeTimers();
    fireEvent.click(screen.getByRole("button", { name: "提问" }));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2500);
    });

    expect(
      screen.getByText("响应较慢；单集仍可阅读，可随时取消。"),
    ).toBeInTheDocument();
    expect(screen.getByText("单集正文仍然可读")).toBeInTheDocument();
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "取消" }));
      await Promise.resolve();
    });
    expect(
      screen.getByText("已取消；问题、选区和已有答案已保留。"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("textbox", { name: "向单集助手提问" }),
    ).toHaveValue("慢请求测试");
    vi.useRealTimers();
  });
});
