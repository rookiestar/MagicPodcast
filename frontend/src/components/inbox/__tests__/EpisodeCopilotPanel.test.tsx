import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { StrictMode, useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import EpisodeCopilotPanel from "../EpisodeCopilotPanel";
import { episodeCopilotApi } from "@/lib/api/episodeCopilot";
import type { ConsumptionItem } from "@/types/consumption";
import type {
  EpisodeCopilotContextScope,
  EpisodeCopilotProfileID,
} from "@/types/episodeCopilot";

const copilotMocks = vi.hoisted(() => ({
  getContext: vi.fn(),
  ask: vi.fn(),
  isCancellation: vi.fn(),
  writeText: vi.fn(),
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

const scopeBase: EpisodeCopilotContextScope = {
  episode_id: 201,
  show_notes_available: true,
  transcript_available: false,
  private_note_available: true,
  profiles: [
    {
      id: "quick",
      model: "gpt-5.6-sol",
      effort: "medium",
      service_tier: "priority",
      service_tier_name: "Fast",
      is_default: false,
    },
    {
      id: "balanced",
      model: "gpt-5.6-luna",
      effort: "max",
      service_tier: "priority",
      service_tier_name: "Fast",
      is_default: true,
    },
    {
      id: "deep",
      model: "gpt-5.6-sol",
      effort: "xhigh",
      service_tier: "",
      service_tier_name: "Standard",
      is_default: false,
    },
  ],
  default_profile_id: "balanced",
};

function scopeForEpisode(episodeId: number, privateNoteAvailable = true) {
  return {
    ...scopeBase,
    episode_id: episodeId,
    private_note_available: privateNoteAvailable,
  };
}

function ControlledCopilotSession({ item }: { item: ConsumptionItem }) {
  const [rejectedProfileIDs, setRejectedProfileIDs] = useState<
    ReadonlySet<EpisodeCopilotProfileID>
  >(() => new Set());
  return (
    <EpisodeCopilotPanel
      item={item}
      rejectedProfileIDs={rejectedProfileIDs}
      onRejectedProfileID={(profileID) =>
        setRejectedProfileIDs((current) => new Set([...current, profileID]))
      }
    />
  );
}

async function openProfileMenu() {
  fireEvent.click(await screen.findByTestId("copilot-profiles"));
  return screen.findByRole("menu", { name: "选择回答档位" });
}

async function chooseProfile(name: string | RegExp) {
  const menu = await openProfileMenu();
  fireEvent.click(within(menu).getByRole("menuitemradio", { name }));
}

async function openContextMenu() {
  fireEvent.click(
    screen.getByRole("button", { name: "查看本次回答上下文" }),
  );
  return screen.findByRole("group", { name: "本次回答上下文" });
}

function mockSelectionCapture() {
  const source = screen.getByText("Runtime permissions are reduced per turn.");
  const range = document.createRange();
  range.selectNodeContents(source);
  vi.spyOn(window, "getSelection").mockReturnValue({
    isCollapsed: false,
    rangeCount: 1,
    getRangeAt: () => range,
    toString: () => "Runtime permissions are reduced per turn.",
  } as unknown as Selection);
  fireEvent(document, new Event("selectionchange"));
}

describe("EpisodeCopilotPanel", () => {
  beforeEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    vi.clearAllMocks();
    copilotMocks.isCancellation.mockReturnValue(false);
    copilotMocks.writeText.mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: copilotMocks.writeText },
    });
    vi.mocked(episodeCopilotApi.getContext).mockResolvedValue(
      scopeForEpisode(201),
    );
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

  it("welcomes with quick questions and submits one exactly once as an ordinary question", async () => {
    render(<EpisodeCopilotPanel item={item} />);

    expect(
      await screen.findByText("你好，我是这一集的单集助手。"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("group", { name: "快捷问题" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "总结这期节目的核心观点" }))
      .toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "向单集助手提问" }))
      .toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", { name: "总结这期节目的核心观点" }),
    );

    await waitFor(() =>
      expect(episodeCopilotApi.ask).toHaveBeenCalledTimes(1),
    );
    expect(episodeCopilotApi.ask).toHaveBeenCalledWith(
      201,
      expect.objectContaining({
        question: "总结这期节目的核心观点",
        selection: "",
        selection_source: "",
        include_private_note: false,
        profile_id: "balanced",
      }),
      expect.any(Function),
      expect.any(AbortSignal),
    );
    // The question becomes the user message; the draft keeps the text.
    expect(
      await screen.findByText("总结这期节目的核心观点", { selector: "p" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("group", { name: "快捷问题" }),
    ).not.toBeInTheDocument();
    expect(
      await screen.findByText("权限应按次收窄。"),
    ).toBeInTheDocument();
  });

  it("describes a transcript-only context without claiming Show Notes", async () => {
    vi.mocked(episodeCopilotApi.getContext).mockResolvedValue({
      ...scopeForEpisode(201),
      show_notes_available: false,
      transcript_available: true,
    });

    render(<EpisodeCopilotPanel item={item} />);

    expect(
      await screen.findByText(
        "我已读取本集逐字稿，围绕这一集提问即可。",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText(/我已读取本集 Show Notes/))
      .not.toBeInTheDocument();
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

    await screen.findByText("当前无成功逐字稿，将明确降级为 Show Notes。");
    mockSelectionCapture();

    expect(await screen.findByText("已选 Show Notes")).toBeInTheDocument();
    fireEvent.change(screen.getByRole("textbox", { name: "向单集助手提问" }), {
      target: { value: "为什么需要按次收窄权限？" },
    });
    const contextMenu = await openContextMenu();
    fireEvent.click(
      within(contextMenu).getByRole("checkbox", {
        name: /本次包含我的私有备注/,
      }),
    );
    // The context menu is informational elsewhere; Escape closes only it.
    fireEvent.keyDown(contextMenu, { key: "Escape" });
    expect(
      screen.queryByRole("group", { name: "本次回答上下文" }),
    ).not.toBeInTheDocument();
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
    const reopenedMenu = await openContextMenu();
    expect(
      within(reopenedMenu).getByRole("checkbox", {
        name: /本次包含我的私有备注/,
      }),
    ).not.toBeChecked();
  });

  it("offers the three tiers with task hints, a recommended default, and readable technical info", async () => {
    render(<EpisodeCopilotPanel item={item} />);

    const trigger = await screen.findByTestId("copilot-profiles");
    expect(trigger).toHaveAttribute("aria-label", "回答档位：均衡");
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(trigger);
    const menu = await screen.findByRole("menu", { name: "选择回答档位" });

    const options = within(menu).getAllByRole("menuitemradio");
    expect(options).toHaveLength(3);
    expect(options[1]).toHaveAttribute("aria-checked", "true");
    expect(
      within(menu).getByRole("menuitemradio", {
        name: "快速：快速查找与简短回答",
      }),
    ).toBeInTheDocument();
    expect(
      within(menu).getByRole("menuitemradio", {
        name: "均衡：速度与完整度兼顾（推荐）",
      }),
    ).toBeInTheDocument();
    expect(
      within(menu).getByRole("menuitemradio", {
        name: "深度：复杂问题与深入分析",
      }),
    ).toBeInTheDocument();
    expect(within(menu).getAllByText("推荐")).toHaveLength(1);
    expect(within(menu).getByText("快速查找与简短回答")).toBeInTheDocument();

    // Technical contract stays reachable, one interaction below the surface.
    fireEvent.click(screen.getByText("查看技术信息"));
    expect(screen.getByText(/gpt-5\.6-sol · medium · Fast/))
      .toBeInTheDocument();
    expect(screen.getByText(/gpt-5\.6-luna · max · Fast/))
      .toBeInTheDocument();
    expect(screen.getByText(/gpt-5\.6-sol · xhigh · Standard/))
      .toBeInTheDocument();
    expect(screen.getAllByText(/消耗更多 credits/)).toHaveLength(2);

    fireEvent.keyDown(menu, { key: "Escape" });
    expect(trigger).toHaveFocus();
    expect(
      screen.queryByRole("menu", { name: "选择回答档位" }),
    ).not.toBeInTheDocument();
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

    render(<EpisodeCopilotPanel item={item} />);
    await screen.findByText("当前无成功逐字稿，将明确降级为 Show Notes。");
    await chooseProfile(/快速/);
    const trigger = screen.getByTestId("copilot-profiles");
    expect(trigger).toHaveAttribute("aria-label", "回答档位：快速");
    const questionInput = screen.getByRole("textbox", {
      name: "向单集助手提问",
    });
    fireEvent.change(questionInput, {
      target: { value: "按快速档提问" },
    });
    const openMenu = await openProfileMenu();
    expect(openMenu).toBeInTheDocument();
    questionInput.focus();
    fireEvent.keyDown(questionInput, { key: "Enter", shiftKey: false });

    // Locked while the request runs: the tier cannot drift from the tier the
    // request is using, including when keyboard submission began with an open
    // menu.
    expect(
      screen.queryByRole("menu", { name: "选择回答档位" }),
    ).not.toBeInTheDocument();
    expect(trigger).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "取消" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "提问" }))
      .not.toBeInTheDocument();

    act(() => {
      releaseAnswer?.();
    });
    expect(
      await screen.findByText("首字 60ms · 完成 180ms · 快速"),
    ).toBeInTheDocument();
    expect(trigger).toBeEnabled();

    // After completion the menu unlocks and a new question carries the
    // newly selected tier.
    await chooseProfile(/深度/);
    expect(trigger).toHaveAttribute("aria-label", "回答档位：深度");
    expect(
      screen.getByRole("button", { name: "提问" }),
    ).toBeEnabled();
  });

  it("supports copy and regenerate without adopting a later-chosen tier", async () => {
    const profileSequence: string[] = [];
    vi.mocked(episodeCopilotApi.ask).mockImplementation(
      async (_episodeId, request, onEvent) => {
        profileSequence.push(request.profile_id);
        onEvent({
          type: "answer_delta",
          message: `## 回答\n\n回答版本 ${profileSequence.length}。`,
          transcript_used: false,
          private_note_included: false,
          profile_id: request.profile_id,
        });
        onEvent({
          type: "complete",
          message: "回答完成",
          transcript_used: false,
          private_note_included: false,
          profile_id: request.profile_id,
          first_content_ms: 90,
          total_ms: 260,
        });
      },
    );

    render(<EpisodeCopilotPanel item={item} />);
    await screen.findByText("当前无成功逐字稿，将明确降级为 Show Notes。");
    fireEvent.change(screen.getByRole("textbox", { name: "向单集助手提问" }), {
      target: { value: "第一版问题" },
    });
    fireEvent.click(screen.getByRole("button", { name: "提问" }));
    expect(
      await screen.findByText("回答版本 1。"),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "复制" }));
    await waitFor(() =>
      expect(copilotMocks.writeText).toHaveBeenCalledWith(
        "## 回答\n\n回答版本 1。",
      ),
    );
    expect(await screen.findByText("已复制")).toBeInTheDocument();

    // The user browses deep but regenerates: the completed request's own
    // tier is reused, not the current selection.
    expect(screen.getByTestId("copilot-run-summary")).toHaveTextContent(
      "档位 均衡",
    );
    await chooseProfile(/深度/);
    expect(screen.getByTestId("copilot-run-summary")).toHaveTextContent(
      "档位 均衡",
    );
    fireEvent.click(screen.getByRole("button", { name: "重新生成" }));
    expect(
      await screen.findByText("回答版本 2。"),
    ).toBeInTheDocument();
    expect(profileSequence).toEqual(["balanced", "balanced"]);
    expect(screen.getByTestId("copilot-profiles")).toHaveAttribute(
      "aria-label",
      "回答档位：均衡",
    );
  });

  it("sends with Enter, keeps Shift+Enter as a newline, and disables an empty send", async () => {
    render(<EpisodeCopilotPanel item={item} />);
    const questionInput = await screen.findByRole("textbox", {
      name: "向单集助手提问",
    });
    const send = screen.getByRole("button", { name: "提问" });
    expect(send).toBeDisabled();

    fireEvent.keyDown(questionInput, { key: "Enter", shiftKey: false });
    expect(episodeCopilotApi.ask).not.toHaveBeenCalled();

    fireEvent.change(questionInput, { target: { value: "第一行" } });
    expect(send).toBeEnabled();
    fireEvent.keyDown(questionInput, {
      key: "Enter",
      shiftKey: true,
    });
    expect(episodeCopilotApi.ask).not.toHaveBeenCalled();
    fireEvent.change(questionInput, { target: { value: "第一行\n第二行" } });
    fireEvent.keyDown(questionInput, { key: "Enter", shiftKey: false });
    await waitFor(() =>
      expect(episodeCopilotApi.ask).toHaveBeenCalledTimes(1),
    );
    expect(episodeCopilotApi.ask).toHaveBeenCalledWith(
      201,
      expect.objectContaining({ question: "第一行\n第二行" }),
      expect.any(Function),
      expect.any(AbortSignal),
    );
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
          profile_id: request.profile_id,
          first_content_ms: 70,
          total_ms: 210,
        });
      });

    render(<EpisodeCopilotPanel item={item} />);
    await screen.findByText("当前无成功逐字稿，将明确降级为 Show Notes。");
    await chooseProfile(/快速/);
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
    const askButton = screen.getByRole("button", { name: "提问" });
    expect(askButton).toBeDisabled();
    fireEvent.click(askButton);
    expect(episodeCopilotApi.ask).toHaveBeenCalledTimes(1);

    // The rejected tier is explicit in the menu, and switching to deep
    // allows a fresh question.
    const menu = await openProfileMenu();
    expect(
      within(menu).getByRole("menuitemradio", {
        name: "快速：快速查找与简短回答，已确认不可用",
      }),
    ).toHaveAttribute("aria-disabled", "true");
    fireEvent.click(within(menu).getByRole("menuitemradio", { name: /深度/ }));
    expect(askButton).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "提问" }));
    await waitFor(() =>
      expect(episodeCopilotApi.ask).toHaveBeenCalledTimes(2),
    );
    expect(profileSequence[1]).toBe("deep");
  });

  it("hides retry for any SSE failure explicitly marked non-retryable", async () => {
    vi.mocked(episodeCopilotApi.ask).mockImplementation(
      async (_episodeId, _request, onEvent) => {
        onEvent({
          type: "error",
          message: "当前请求不可重试",
          code: "request_rejected",
          retryable: false,
          transcript_used: false,
          private_note_included: false,
        });
        const failure = new Error("当前请求不可重试") as Error & {
          code?: string;
        };
        failure.code = "request_rejected";
        throw failure;
      },
    );

    render(<EpisodeCopilotPanel item={item} />);
    fireEvent.change(
      await screen.findByRole("textbox", { name: "向单集助手提问" }),
      { target: { value: "验证不可重试失败" } },
    );
    fireEvent.click(screen.getByRole("button", { name: "提问" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "当前请求不可重试",
    );
    expect(
      screen.queryByRole("button", { name: "重试" }),
    ).not.toBeInTheDocument();
  });

  it("keeps every unsupported tier blocked until the page session ends", async () => {
    vi.mocked(episodeCopilotApi.ask).mockImplementation(
      async (_episodeId, request, onEvent) => {
        onEvent({
          type: "error",
          message: `${request.profile_id} 不可用`,
          code: "profile_unavailable",
          retryable: false,
          transcript_used: false,
          private_note_included: false,
          profile_id: request.profile_id,
        });
        const failure = new Error(`${request.profile_id} 不可用`) as Error & {
          code?: string;
        };
        failure.code = "profile_unavailable";
        throw failure;
      },
    );

    render(<EpisodeCopilotPanel item={item} />);
    const question = await screen.findByRole("textbox", {
      name: "向单集助手提问",
    });
    fireEvent.change(question, { target: { value: "验证多个不可用档位" } });

    await chooseProfile(/快速/);
    fireEvent.click(screen.getByRole("button", { name: "提问" }));
    await screen.findByRole("alert");
    expect(screen.getByRole("button", { name: "提问" })).toBeDisabled();

    await chooseProfile(/深度/);
    fireEvent.click(screen.getByRole("button", { name: "提问" }));
    await waitFor(() =>
      expect(episodeCopilotApi.ask).toHaveBeenCalledTimes(2),
    );
    expect(screen.getByRole("button", { name: "提问" })).toBeDisabled();

    // The rejected tier cannot even be re-selected from the menu.
    const menu = await openProfileMenu();
    expect(
      within(menu).getByRole("menuitemradio", { name: /快速/ }),
    ).toHaveAttribute("aria-disabled", "true");
    expect(
      within(menu).getByRole("menuitemradio", { name: /深度/ }),
    ).toHaveAttribute("aria-disabled", "true");
    expect(
      within(menu).getByRole("menuitemradio", { name: /均衡/ }),
    ).toHaveFocus();
    fireEvent.click(within(menu).getByRole("menuitemradio", { name: /均衡/ }));
    expect(screen.getByRole("button", { name: "提问" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "提问" }));
    await waitFor(() =>
      expect(episodeCopilotApi.ask).toHaveBeenCalledTimes(3),
    );
  });

  it("keeps rejected tiers blocked when the controlled panel changes episodes", async () => {
    vi.mocked(episodeCopilotApi.getContext).mockImplementation(
      async (episodeId) => scopeForEpisode(episodeId, false),
    );
    vi.mocked(episodeCopilotApi.ask).mockImplementationOnce(
      async (_episodeId, request, onEvent) => {
        onEvent({
          type: "error",
          message: "快速档位不可用",
          code: "profile_unavailable",
          retryable: false,
          transcript_used: false,
          private_note_included: false,
          profile_id: request.profile_id,
        });
        const failure = new Error("快速档位不可用") as Error & {
          code?: string;
        };
        failure.code = "profile_unavailable";
        throw failure;
      },
    );

    const view = render(<ControlledCopilotSession item={item} />);
    await chooseProfile(/快速/);
    fireEvent.change(screen.getByRole("textbox", { name: "向单集助手提问" }), {
      target: { value: "先验证当前单集" },
    });
    fireEvent.click(screen.getByRole("button", { name: "提问" }));
    await screen.findByRole("alert");

    const nextItem = { ...item, episode_id: 202 };
    view.rerender(<ControlledCopilotSession item={nextItem} />);
    await waitFor(() =>
      expect(episodeCopilotApi.getContext).toHaveBeenCalledWith(202),
    );
    expect(
      await screen.findByText(
        "当前选择的快速档位已确认不可用；请切换其他档位后再提问。",
      ),
    ).toBeInTheDocument();
    const nextQuestion = screen.getByRole("textbox", {
      name: "向单集助手提问",
    });
    fireEvent.change(nextQuestion, { target: { value: "切换后仍验证" } });
    const askButton = screen.getByRole("button", { name: "提问" });
    expect(askButton).toBeDisabled();
    fireEvent.click(askButton);
    expect(episodeCopilotApi.ask).toHaveBeenCalledTimes(1);

    await chooseProfile(/深度/);
    expect(askButton).toBeEnabled();
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
    await screen.findByText("当前无成功逐字稿，将明确降级为 Show Notes。");
    mockSelectionCapture();

    const question = screen.getByRole("textbox", {
      name: "向单集助手提问",
    });
    fireEvent.change(question, { target: { value: "失败时保留什么？" } });
    const contextMenu = await openContextMenu();
    fireEvent.click(
      within(contextMenu).getByRole("checkbox", {
        name: /本次包含我的私有备注/,
      }),
    );
    fireEvent.keyDown(contextMenu, { key: "Escape" });
    await chooseProfile(/快速/);
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
    const reopenedMenu = await openContextMenu();
    expect(
      within(reopenedMenu).getByRole("checkbox", {
        name: /本次包含我的私有备注/,
      }),
    ).not.toBeChecked();
    fireEvent.keyDown(reopenedMenu, { key: "Escape" });

    await chooseProfile(/深度/);
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
        profile_id: "quick",
      }),
      expect.any(Function),
      expect.any(AbortSignal),
    );
    expect(screen.getByTestId("copilot-profiles")).toHaveAttribute(
      "aria-label",
      "回答档位：快速",
    );
  });

  it("keeps the selected tier when navigating to another episode", async () => {
    vi.mocked(episodeCopilotApi.getContext).mockImplementation(
      async (episodeId) => scopeForEpisode(episodeId, false),
    );
    const { rerender } = render(<EpisodeCopilotPanel item={item} />);
    await chooseProfile(/深度/);
    expect(screen.getByTestId("copilot-profiles")).toHaveAttribute(
      "aria-label",
      "回答档位：深度",
    );

    rerender(<EpisodeCopilotPanel item={{ ...item, episode_id: 202 }} />);

    await waitFor(() =>
      expect(episodeCopilotApi.getContext).toHaveBeenCalledWith(202),
    );
    await waitFor(() =>
      expect(screen.getByTestId("copilot-profiles")).toHaveAttribute(
        "aria-label",
        "回答档位：深度",
      ),
    );
  });

  it("keeps a page-session tier when the detail panel remounts", async () => {
    let pageProfileID: EpisodeCopilotProfileID | null = null;
    const onProfileChange = (profileID: EpisodeCopilotProfileID) => {
      pageProfileID = profileID;
    };
    const view = render(
      <EpisodeCopilotPanel
        item={item}
        selectedProfileID={pageProfileID}
        onSelectedProfileIDChange={onProfileChange}
      />,
    );
    await chooseProfile(/深度/);
    expect(pageProfileID).toBe("deep");

    view.rerender(
      <EpisodeCopilotPanel
        item={item}
        selectedProfileID={pageProfileID}
        onSelectedProfileIDChange={onProfileChange}
      />,
    );
    view.unmount();
    render(
      <EpisodeCopilotPanel
        item={{ ...item, episode_id: 202 }}
        selectedProfileID={pageProfileID}
        onSelectedProfileIDChange={onProfileChange}
      />,
    );

    await waitFor(() =>
      expect(screen.getByTestId("copilot-profiles")).toHaveAttribute(
        "aria-label",
        "回答档位：深度",
      ),
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
    await screen.findByText("当前无成功逐字稿，将明确降级为 Show Notes。");
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
    expect(screen.getByRole("button", { name: "重试" })).toBeEnabled();
    vi.useRealTimers();
  });

  it("shows a short actionable error with retry instead of a blank panel when the context fails", async () => {
    vi.mocked(episodeCopilotApi.getContext).mockRejectedValue(
      new Error("网络中断"),
    );
    render(<EpisodeCopilotPanel item={item} />);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "助手上下文暂时不可用，单集阅读不受影响：网络中断",
    );
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();

    vi.mocked(episodeCopilotApi.getContext).mockResolvedValue(
      scopeForEpisode(201),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "重试读取助手上下文" }),
    );
    expect(
      await screen.findByText("你好，我是这一集的单集助手。"),
    ).toBeInTheDocument();
  });
});
