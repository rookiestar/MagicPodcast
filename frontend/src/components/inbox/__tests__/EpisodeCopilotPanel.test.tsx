import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
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
      profiles: [
        {
          id: "quick",
          model: "gpt-5.6-sol",
          effort: "medium",
          service_tier: "fast",
          is_default: false,
        },
        {
          id: "balanced",
          model: "gpt-5.6-luna",
          effort: "max",
          service_tier: "fast",
          is_default: true,
        },
        {
          id: "deep",
          model: "gpt-5.6-sol",
          effort: "xhigh",
          service_tier: "",
          is_default: false,
        },
      ],
      default_profile_id: "balanced",
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
          profile_id: "balanced",
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

  it("offers exactly the three tiers with technical meaning and defaults to balanced", async () => {
    render(
      <>
        <div>单集正文仍然可读</div>
        <EpisodeCopilotPanel item={item} />
      </>,
    );

    const group = await screen.findByTestId("copilot-profiles");
    const radios = within(group).getAllByRole("radio");
    expect(radios).toHaveLength(3);
    expect(radios.map((radio) => (radio as HTMLInputElement).value)).toEqual([
      "quick",
      "balanced",
      "deep",
    ]);
    expect(radios[1]).toBeChecked();

    expect(within(group).getByText("快速")).toBeInTheDocument();
    expect(within(group).getByText("均衡")).toBeInTheDocument();
    expect(within(group).getByText("深度")).toBeInTheDocument();
    expect(
      within(group).getByText(/gpt-5\.6-sol · medium · fast/),
    ).toBeInTheDocument();
    expect(
      within(group).getByText(/gpt-5\.6-luna · max · fast/),
    ).toBeInTheDocument();
    expect(
      within(group).getByText(/gpt-5\.6-sol · xhigh · standard/),
    ).toBeInTheDocument();
    expect(
      within(group).getAllByText(/Fast 消耗更多 credits/),
    ).toHaveLength(2);
    expect(within(group).getByText("默认")).toBeInTheDocument();
  });

  it("applies the selected tier per question and locks the choice during the request", async () => {
    let releaseAnswer: (() => void) | null = null;
    vi.mocked(episodeCopilotApi.ask)
      .mockImplementationOnce(
        (_episodeId, request, onEvent, signal) =>
          new Promise((resolve, reject) => {
            expect(request.profile_id).toBe("quick");
            signal.addEventListener("abort", () =>
              reject(new Error("cancelled")),
              { once: true },
            );
            releaseAnswer = () => {
              onEvent({
                type: "answer_delta",
                message: "## 回答\n\n快速档回答。",
                transcript_used: false,
                private_note_included: false,
                profile_id: "quick",
              });
              onEvent({
                type: "complete",
                message: "回答完成",
                transcript_used: false,
                private_note_included: false,
                profile_id: "quick",
                first_content_ms: 60,
                total_ms: 180,
              });
              resolve();
            };
          }),
      );

    render(
      <>
        <div>单集正文仍然可读</div>
        <EpisodeCopilotPanel item={item} />
      </>,
    );
    await screen.findByText("当前无成功逐字稿，将明确降级为 Show Notes。");
    const group = screen.getByTestId("copilot-profiles");
    fireEvent.click(within(group).getByRole("radio", { name: /快速/ }));
    fireEvent.change(screen.getByRole("textbox", { name: "向单集助手提问" }), {
      target: { value: "按快速档提问" },
    });
    fireEvent.click(screen.getByRole("button", { name: "提问" }));

    // Locked while the request runs: every option is disabled, so the
    // selection cannot drift from the tier the request is using.
    expect(
      within(group).getByRole("radio", { name: /快速/ }),
    ).toBeDisabled();
    expect(
      within(group).getByRole("radio", { name: /均衡/ }),
    ).toBeDisabled();
    expect(
      within(group).getByRole("radio", { name: /深度/ }),
    ).toBeDisabled();
    expect(
      within(group).getByRole("radio", { name: /快速/ }),
    ).toBeChecked();

    act(() => {
      releaseAnswer?.();
    });
    expect(
      await screen.findByText("首字 60ms · 完成 180ms · 快速"),
    ).toBeInTheDocument();

    // After completion the group unlocks and a new question carries the
    // newly selected tier.
    fireEvent.click(within(group).getByRole("radio", { name: /深度/ }));
    expect(
      within(group).getByRole("radio", { name: /深度/ }),
    ).toBeEnabled();
    expect(
      within(group).getByRole("radio", { name: /深度/ }),
    ).toBeChecked();
  });

  it("hides retry after an unsupported profile and treats a changed tier as a new request", async () => {
    const profileSequence: Array<string | undefined> = [];
    vi.mocked(episodeCopilotApi.ask)
      .mockImplementationOnce(async (_episodeId, request, onEvent) => {
        profileSequence.push(request.profile_id);
        onEvent({
          type: "error",
          message: "当前账号或 Runtime 不支持所选档位，请更换档位后重新提问。",
          code: "profile_unavailable",
          retryable: false,
          transcript_used: false,
          private_note_included: false,
          profile_id: "quick",
        });
        // Mirror the API client: the thrown error carries the SSE error code.
        const failure = new Error(
          "当前账号或 Runtime 不支持所选档位，请更换档位后重新提问。",
        ) as Error & { code?: string };
        failure.code = "profile_unavailable";
        throw failure;
      })
      .mockImplementation(async (_episodeId, request, onEvent) => {
        profileSequence.push(request.profile_id);
        onEvent({
          type: "complete",
          message: "回答完成",
          transcript_used: false,
          private_note_included: false,
          profile_id: request.profile_id ?? "",
          first_content_ms: 70,
          total_ms: 210,
        });
      });

    render(
      <>
        <div>单集正文仍然可读</div>
        <EpisodeCopilotPanel item={item} />
      </>,
    );
    await screen.findByText("当前无成功逐字稿，将明确降级为 Show Notes。");
    const group = screen.getByTestId("copilot-profiles");
    fireEvent.click(within(group).getByRole("radio", { name: /快速/ }));
    fireEvent.change(screen.getByRole("textbox", { name: "向单集助手提问" }), {
      target: { value: "先快速，再换深度" },
    });
    fireEvent.click(screen.getByRole("button", { name: "提问" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "不支持所选档位",
    );
    expect(screen.getByRole("textbox", { name: "向单集助手提问" })).toHaveValue(
      "先快速，再换深度",
    );
    // Retrying the identical unsupported request is impossible by design:
    // the action is hidden while the work and the tier switch stay usable.
    expect(
      screen.queryByRole("button", { name: "重试" }),
    ).not.toBeInTheDocument();

    // User switches to deep and asks again: a new request carrying deep.
    fireEvent.click(within(group).getByRole("radio", { name: /深度/ }));
    fireEvent.click(screen.getByRole("button", { name: "提问" }));
    await waitFor(() =>
      expect(episodeCopilotApi.ask).toHaveBeenCalledTimes(2),
    );
    expect(profileSequence[1]).toBe("deep");
  });

  it("omits profile_id when the scope predates the profile contract", async () => {
    vi.mocked(episodeCopilotApi.getContext).mockResolvedValue({
      episode_id: 201,
      show_notes_available: true,
      transcript_available: false,
      private_note_available: false,
      profiles: [],
      default_profile_id: "",
    });

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
      target: { value: "旧后端也要能提问" },
    });
    fireEvent.click(screen.getByRole("button", { name: "提问" }));

    await waitFor(() => expect(episodeCopilotApi.ask).toHaveBeenCalled());
    const request = vi.mocked(episodeCopilotApi.ask).mock.calls[0]?.[1];
    expect(request).not.toHaveProperty("profile_id");
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
      expect.objectContaining({
        include_private_note: true,
        profile_id: "balanced",
      }),
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
