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
  getPeople: vi.fn(),
  correctName: vi.fn(),
  correctAttribution: vi.fn(),
 correctAppearance: vi.fn(),
 preparePeople: vi.fn(),
  isCancellation: vi.fn(),
  writeText: vi.fn(),
}));

vi.mock("@/lib/api/episodeCopilot", () => ({
  episodeCopilotApi: {
    getContext: copilotMocks.getContext,
    ask: copilotMocks.ask,
    getPeople: copilotMocks.getPeople,
    correctName: copilotMocks.correctName,
    correctAttribution: copilotMocks.correctAttribution,
 correctAppearance: copilotMocks.correctAppearance,
 preparePeople: copilotMocks.preparePeople,
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

  it("distinguishes namesakes by accessible identity descriptions and sends the selected ID", async () => {
    const people = [
      { id: 41, display_name: "陈晨", aliases: [], identity_note: "建筑师", role: "guest" as const, status: "confirmed" as const, status_reason: "" },
      { id: 42, display_name: "陈晨", aliases: [], identity_note: "急诊医生", role: "guest" as const, status: "confirmed" as const, status_reason: "" },
    ];
    copilotMocks.getContext.mockResolvedValue({ ...scopeForEpisode(201), people, index_ready: true });
    copilotMocks.getPeople.mockResolvedValue({ episode_id: 201, source_version: "v1", index_ready: true, people, attributions: [] });
    render(<EpisodeCopilotPanel item={item} />);
    const composer = await screen.findByRole("textbox", { name: "向单集助手提问" });
    fireEvent.change(composer, { target: { value: "@陈" } });
    expect(await screen.findByRole("option", { name: "选择陈晨", description: "嘉宾 · 建筑师" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("option", { name: "选择陈晨", description: "嘉宾 · 急诊医生" }));
    fireEvent.change(composer, { target: { value: "你如何安排工作顺序？" } });
    fireEvent.click(screen.getByRole("button", { name: "提问" }));
    await waitFor(() => expect(copilotMocks.ask).toHaveBeenCalledWith(201, expect.objectContaining({ target_person_id: 42 }), expect.any(Function), expect.any(AbortSignal)));
  });

  it("selects one @ person, sends a structured id, and blocks pending 模拟回答", async () => {
    const zhang = {
      id: 9,
      display_name: "张三",
      aliases: ["老张"],
      identity_note: "技术漫谈主播",
      role: "host" as const,
      status: "confirmed" as const,
      status_reason: "",
    };
    const pending = {
      id: 4,
      display_name: "匿名工程师",
      aliases: [],
      identity_note: "",
      role: "guest" as const,
      status: "pending" as const,
      status_reason: "anonymous",
    };
    vi.mocked(episodeCopilotApi.getContext).mockResolvedValue({
      ...scopeForEpisode(201),
      people: [zhang, pending],
      index_ready: true,
    });
    copilotMocks.getPeople.mockResolvedValue({
      episode_id: 201,
      source_version: "v1",
      index_ready: true,
      people: [zhang, pending],
      attributions: [
        {
          id: 1,
          source_kind: "transcript",
          source_version: "v1",
          fragment_order: 1,
          speaker_label: "Speaker 1",
          start_ms: 0,
          text: "待确认片段",
          status: "pending",
        },
      ],
    });

    render(<EpisodeCopilotPanel item={item} />);
    const composer = await screen.findByRole("textbox", {
      name: "向单集助手提问",
    });
    fireEvent.change(composer, { target: { value: "@张" } });
    fireEvent.click(await screen.findByRole("option", { name: "选择张三" }));
    expect(screen.getByTestId("copilot-person-chip")).toHaveTextContent("张三");
    fireEvent.change(composer, { target: { value: "加班怎么看" } });
    fireEvent.click(screen.getByRole("button", { name: "提问" }));
    await waitFor(() => expect(episodeCopilotApi.ask).toHaveBeenCalled());
    expect(episodeCopilotApi.ask).toHaveBeenCalledWith(
      201,
      expect.objectContaining({
        question: "加班怎么看",
        target_person_id: 9,
      }),
      expect.any(Function),
      expect.any(AbortSignal),
    );

    fireEvent.click(screen.getByRole("button", { name: "清除人物选择" }));
    fireEvent.change(composer, { target: { value: "@匿" } });
    fireEvent.click(
      await screen.findByRole("option", { name: "选择匿名工程师" }),
    );
    expect(
      screen.getByText(/待确认人物不能提交模拟回答/),
    ).toBeInTheDocument();
    fireEvent.change(composer, { target: { value: "降本方案" } });
    expect(screen.getByRole("button", { name: "提问" })).toBeDisabled();
    fireEvent.keyDown(composer, { key: "Enter" });
    expect(episodeCopilotApi.ask).toHaveBeenCalledTimes(1);
  });

  it("corrects role and supports reversible exclusion without silently changing the question target", async () => {
    const person = { id: 9, display_name: "张小珺", aliases: [], identity_note: "", role: "unknown" as const, status: "confirmed" as const, status_reason: "",
      evidence_locator: JSON.stringify({ name_evidence: { source: "podcast_author", quote: "张小珺", fragment: 0 }, source_names: [{ name: "小俊", evidence: { source: "transcript", fragment: 1, quote: "我是小俊。" } }] }),
    };
    const payload = { episode_id: 201, source_version: "v1", index_ready: true, preparation_state: "ready" as const, people: [person], excluded_people: [], attributions: [] };
    copilotMocks.getContext.mockResolvedValue({ ...scopeForEpisode(201), ...payload });
    copilotMocks.getPeople.mockResolvedValue(payload);
    render(<EpisodeCopilotPanel item={item} />);
    const composer = await screen.findByRole("textbox", { name: "向单集助手提问" });
    fireEvent.change(composer, { target: { value: "@张" } });
    fireEvent.click(await screen.findByRole("option", { name: "选择张小珺" }));
    expect(screen.getByTestId("copilot-person-chip")).toHaveTextContent("角色待确认");
    fireEvent.change(composer, { target: { value: "怎么看产业变化？" } });
    fireEvent.click(screen.getByRole("button", { name: "纠正发言归属" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "保存本集角色" })).toBeEnabled());
    fireEvent.click(screen.getByText("查看姓名、角色与原文依据"));
    expect(screen.getByText("转写称呼：小俊")).toBeInTheDocument();
    expect(screen.getByText("我是小俊。")).toBeInTheDocument();
    expect(screen.queryByText(/name_evidence/)).not.toBeInTheDocument();
    const host = { ...person, role: "host" as const, role_user_confirmed: true };
    copilotMocks.correctAppearance.mockResolvedValueOnce({ ...payload, people: [host] });
    fireEvent.change(screen.getByRole("combobox", { name: "纠正本集角色" }), { target: { value: "host" } });
    fireEvent.click(screen.getByRole("button", { name: "保存本集角色" }));
    await waitFor(() => expect(screen.getByTestId("copilot-person-chip")).toHaveTextContent("主播"));
    expect(copilotMocks.correctAppearance).toHaveBeenCalledWith(201, 9, { role: "host" }, expect.any(AbortSignal));
    copilotMocks.correctAppearance.mockResolvedValueOnce({ ...payload, people: [], excluded_people: [host] });
    fireEvent.click(screen.getByRole("button", { name: "不是本集人物" }));
    expect(await screen.findByText(/原选人物已失效/)).toBeInTheDocument();
    expect(composer).toHaveValue("怎么看产业变化？");
    expect(screen.getByRole("button", { name: "提问" })).toBeDisabled();
    fireEvent.keyDown(composer, { key: "Enter" });
    expect(copilotMocks.ask).not.toHaveBeenCalled();
    copilotMocks.correctAppearance.mockResolvedValueOnce({ ...payload, people: [host] });
    fireEvent.click(screen.getByText("已排除人物（1）"));
    fireEvent.click(screen.getByRole("button", { name: "撤销排除 张小珺" }));
    await waitFor(() => expect(screen.queryByText("已排除人物（1）")).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name: "提问" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "改为普通问答" }));
    expect(screen.getByRole("button", { name: "提问" })).toBeEnabled();
  });

  it("rebinds the selected person when a shared identity correction returns a new ID", async () => {
    const original = {
      id: 9,
      display_name: "王芳",
      aliases: [],
      identity_note: "产品负责人",
      role: "guest" as const,
      status: "confirmed" as const,
      status_reason: "",
    };
    const replacement = { ...original, id: 12, display_name: "王芳芳" };
    const initialPayload = {
      episode_id: 201,
      source_version: "v1",
      index_ready: true,
      preparation_state: "ready" as const,
      people: [original],
      excluded_people: [],
      attributions: [],
    };
    copilotMocks.getContext.mockResolvedValue({ ...scopeForEpisode(201), ...initialPayload, transcript_available: true });
    copilotMocks.getPeople.mockResolvedValue(initialPayload);
    copilotMocks.correctName.mockResolvedValue({ ...initialPayload, people: [replacement] });
    render(<EpisodeCopilotPanel item={item} />);
    const composer = await screen.findByRole("textbox", { name: "向单集助手提问" });
    fireEvent.change(composer, { target: { value: "@王" } });
    fireEvent.click(await screen.findByRole("option", { name: "选择王芳" }));
    fireEvent.click(screen.getByRole("button", { name: "纠正发言归属" }));
    const nameInput = await screen.findByRole("textbox", { name: "纠正人物姓名" });
    fireEvent.change(nameInput, { target: { value: "王芳芳" } });
    fireEvent.click(screen.getByRole("button", { name: "确认姓名与本集身份" }));
    await waitFor(() => expect(screen.getByTestId("copilot-person-chip")).toHaveTextContent("王芳芳"));
    expect(screen.queryByText(/原选人物已失效/)).not.toBeInTheDocument();
    fireEvent.change(composer, { target: { value: "你的产品观点是什么？" } });
    fireEvent.click(screen.getByRole("button", { name: "提问" }));
    await waitFor(() => expect(copilotMocks.ask).toHaveBeenCalledWith(201, expect.objectContaining({ target_person_id: 12 }), expect.any(Function), expect.any(AbortSignal)));
  });

  it("preserves preparation drafts and ignores cancelled or cross-episode results", async () => {
    const person = { id: 9, display_name: "曾鸣", aliases: [], identity_note: "", role: "guest" as const, status: "confirmed" as const, status_reason: "" };
    const fresh = { episode_id: 201, source_version: "v1", index_ready: true, preparation_state: "ready" as const, people: [person], attributions: [] };
    let resolveLate!: (value: typeof fresh) => void;
    copilotMocks.getContext.mockResolvedValue({ ...scopeForEpisode(201), transcript_available: true, index_ready: false, preparation_state: "outdated", people: [] });
    copilotMocks.preparePeople.mockReturnValueOnce(new Promise<typeof fresh>((resolve) => { resolveLate = resolve; }));
    const view = render(<EpisodeCopilotPanel item={item} />);
    const composer = await screen.findByRole("textbox", { name: "向单集助手提问" });
    expect(screen.getByText(/人物资料需要重新识别/)).toBeInTheDocument();
    fireEvent.change(composer, { target: { value: "保留这个问题" } });
    fireEvent.click(screen.getByRole("button", { name: "识别本集人物" }));
    expect(composer).toHaveValue("保留这个问题");
    const signal = copilotMocks.preparePeople.mock.calls[0][1] as AbortSignal;
    fireEvent.click(screen.getByRole("button", { name: "取消人物处理" }));
    expect(signal.aborted).toBe(true);
    await act(async () => resolveLate(fresh));
    expect(screen.getByText(/人物资料需要重新识别/)).toBeInTheDocument();
    copilotMocks.preparePeople.mockRejectedValueOnce(new Error("offline"));
    fireEvent.click(screen.getByRole("button", { name: "识别本集人物" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("人物资料处理失败");
    expect(composer).toHaveValue("保留这个问题");
    copilotMocks.preparePeople.mockReturnValueOnce(new Promise<typeof fresh>((resolve) => { resolveLate = resolve; }));
    fireEvent.click(screen.getByRole("button", { name: "识别本集人物" }));
    copilotMocks.getContext.mockResolvedValue({ ...scopeForEpisode(202), people: [], index_ready: false, preparation_state: "required" });
    view.rerender(<EpisodeCopilotPanel item={{ ...item, episode_id: 202 }} />);
    await screen.findByText(/人物资料尚未准备/);
    await act(async () => resolveLate(fresh));
    fireEvent.change(screen.getByRole("textbox", { name: "向单集助手提问" }), { target: { value: "@" } });
    expect(screen.queryByRole("option", { name: "选择曾鸣" })).not.toBeInTheDocument();
  });

  it("does not submit while IME is composing", async () => {
    render(<EpisodeCopilotPanel item={item} />);
    const composer = await screen.findByRole("textbox", {
      name: "向单集助手提问",
    });
    fireEvent.change(composer, { target: { value: "加班" } });
    fireEvent.keyDown(composer, { key: "Enter", isComposing: true, keyCode: 229 });
    expect(episodeCopilotApi.ask).not.toHaveBeenCalled();
  });

  it("clears the selected person when switching episodes", async () => {
    vi.mocked(episodeCopilotApi.getContext).mockResolvedValue({
      ...scopeForEpisode(201),
      people: [
        {
          id: 9,
          display_name: "张三",
          aliases: [],
          identity_note: "",
          role: "host",
          status: "confirmed",
          status_reason: "",
        },
      ],
    });
    const { rerender } = render(<EpisodeCopilotPanel item={item} />);
    const composer = await screen.findByRole("textbox", {
      name: "向单集助手提问",
    });
    fireEvent.change(composer, { target: { value: "@张" } });
    fireEvent.click(await screen.findByRole("option", { name: "选择张三" }));
    expect(screen.getByTestId("copilot-person-chip")).toBeInTheDocument();
    rerender(
      <EpisodeCopilotPanel
        item={{ ...item, episode_id: 202, episode_title: "另一集" }}
      />,
    );
    await waitFor(() =>
      expect(screen.queryByTestId("copilot-person-chip")).not.toBeInTheDocument(),
    );
  });

  it("jumps sourced answers to the transcript fragment and corrects pending 发言归属", async () => {
    const zhang = {
      id: 9,
      display_name: "张三",
      aliases: ["老张"],
      identity_note: "技术漫谈主播",
      role: "host" as const,
      status: "confirmed" as const,
      status_reason: "",
    };
    vi.mocked(episodeCopilotApi.getContext).mockResolvedValue({
      ...scopeForEpisode(201),
      people: [zhang],
      index_ready: true,
    });
    copilotMocks.getPeople.mockResolvedValue({
      episode_id: 201,
      source_version: "v1",
      index_ready: true,
      people: [zhang],
      attributions: [
        {
          id: 7,
          source_kind: "transcript",
          source_version: "v1",
          fragment_order: 7,
          speaker_label: "Speaker 2",
          start_ms: 120000,
          text: "待确认片段",
          status: "pending",
        },
      ],
    });
    copilotMocks.correctAttribution.mockResolvedValue({
      episode_id: 201,
      source_version: "v1",
      index_ready: true,
      people: [zhang],
      attributions: [
        {
          id: 7,
          source_kind: "transcript",
          source_version: "v1",
          fragment_order: 7,
          speaker_label: "Speaker 2",
          start_ms: 120000,
          text: "待确认片段",
          status: "confirmed",
        },
      ],
    });
    vi.mocked(episodeCopilotApi.ask).mockImplementation(
      async (_episodeId, _request, onEvent) => {
        onEvent({
          type: "answer_delta",
          message:
            "基于公开表达的 AI 模拟，非本人回复\n\n我不赞成无限制加班 [库内 S1]。\n\n## 库内来源\n\n- [库内 S1] 单集 201 · 2025-03-12 · 片段 3\n",
          transcript_used: true,
          private_note_included: false,
        });
        onEvent({
          type: "complete",
          message: "回答完成",
          transcript_used: true,
          private_note_included: false,
          first_content_ms: 120,
          total_ms: 420,
        });
      },
    );
    const transcriptTab = document.createElement("button");
    transcriptTab.id = "detail-tab-transcript";
    const artifactTab = document.createElement("button");
    artifactTab.id = "processing-artifact-tab-transcript";
    const root = document.createElement("div");
    root.setAttribute("data-copilot-source", "transcript");
    root.setAttribute("data-copilot-episode-id", "201");
    const fragment = document.createElement("article");
    fragment.setAttribute("data-fragment-order", "3");
    fragment.scrollIntoView = vi.fn();
    root.append(fragment);
    document.body.append(transcriptTab, artifactTab, root);
    const clickTranscript = vi.spyOn(transcriptTab, "click");
    const clickArtifact = vi.spyOn(artifactTab, "click");

    render(<EpisodeCopilotPanel item={item} />);
    const composer = await screen.findByRole("textbox", {
      name: "向单集助手提问",
    });
    fireEvent.change(composer, { target: { value: "@张" } });
    fireEvent.click(await screen.findByRole("option", { name: "选择张三" }));
    fireEvent.change(composer, { target: { value: "他对加班怎么看？" } });
    fireEvent.click(screen.getByRole("button", { name: "提问" }));
    fireEvent.click(
      await screen.findByRole("button", {
        name: "打开来源 · 单集 201 · 2025-03-12 · 片段 3",
      }),
    );
    expect(clickTranscript).toHaveBeenCalled();
    expect(clickArtifact).toHaveBeenCalled();
    expect(fragment.scrollIntoView).toHaveBeenCalledWith({ block: "center" });

    fireEvent.click(screen.getByRole("button", { name: "纠正发言归属" }));
    fireEvent.click(
      await screen.findByRole("button", { name: "将片段 7 归给 张三" }),
    );
    await waitFor(() =>
      expect(copilotMocks.correctAttribution).toHaveBeenCalledWith(201, {
        source_kind: "transcript",
        source_version: "v1",
        fragment_order: 7,
        assigned_person_id: 9,
        status: "confirmed",
      }),
    );
  });
});
