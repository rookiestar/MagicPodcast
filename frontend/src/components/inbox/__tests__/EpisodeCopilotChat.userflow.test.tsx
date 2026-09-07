import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ConsumptionDetailPanel from "../ConsumptionDetailPanel";
import type { ConsumptionItem } from "@/types/consumption";
import type {
  EpisodeCopilotActivity,
  EpisodeCopilotStreamEvent,
} from "@/types/episodeCopilot";

/**
 * The single new automation seam for the chat-first copilot (#307): the real
 * Focus Detail and its copilot subtree are rendered, only the public frontend
 * API/SSE layer and browser media capabilities are simulated, and each test
 * drives from opening the assistant through asking, reading, and closing or
 * returning. Assertions observe user-visible copy, accessible roles, request
 * arguments, and DOM order of accessible elements — never the component tree
 * or CSS class names.
 */

const apiMocks = vi.hoisted(() => ({
  getItem: vi.fn(),
  markInProgress: vi.fn(),
  getConsumptionErrorDetails: vi.fn(() => ({ message: "保存失败" })),
  getNotes: vi.fn(),
  getTags: vi.fn(),
  updateNotes: vi.fn(),
  addTag: vi.fn(),
  removeTag: vi.fn(),
  getShowNotes: vi.fn(),
  listTags: vi.fn(),
  listEpisodeRuns: vi.fn(),
  getLatestAudio: vi.fn(),
  getScheduleStatus: vi.fn(),
  getRun: vi.fn(),
  startProcessing: vi.fn(),
  cancelProcessing: vi.fn(),
  retryProcessing: vi.fn(),
  getArtifactContent: vi.fn(),
  getCopilotContext: vi.fn(),
  askCopilot: vi.fn(),
  isCancellation: vi.fn(),
  writeText: vi.fn(),
}));

vi.mock("@/lib/api/consumption", () => ({
  consumptionApi: {
    getItem: apiMocks.getItem,
    markInProgress: apiMocks.markInProgress,
  },
  getConsumptionErrorDetails: apiMocks.getConsumptionErrorDetails,
}));

vi.mock("@/lib/api", () => ({
  episodeApi: {
    getShowNotes: apiMocks.getShowNotes,
    getNotes: apiMocks.getNotes,
    getTags: apiMocks.getTags,
    updateNotes: apiMocks.updateNotes,
    addTag: apiMocks.addTag,
    removeTag: apiMocks.removeTag,
  },
  tagApi: {
    list: apiMocks.listTags,
  },
}));

vi.mock("@/lib/api/processing", () => ({
  processingApi: {
    listEpisodeRuns: apiMocks.listEpisodeRuns,
    getLatestAudio: apiMocks.getLatestAudio,
    getScheduleStatus: apiMocks.getScheduleStatus,
    getRun: apiMocks.getRun,
    start: apiMocks.startProcessing,
    cancel: apiMocks.cancelProcessing,
    retry: apiMocks.retryProcessing,
    getArtifactContent: apiMocks.getArtifactContent,
  },
  getProcessingErrorDetails: vi.fn((error: unknown) => ({
    message: error instanceof Error ? error.message : "加工状态读取失败",
    status: (error as { response?: { status?: number } })?.response?.status,
  })),
}));

vi.mock("@/lib/api/episodeCopilot", () => ({
  episodeCopilotApi: {
    getContext: apiMocks.getCopilotContext,
    ask: apiMocks.askCopilot,
  },
  isEpisodeCopilotCancellation: apiMocks.isCancellation,
}));

const item: ConsumptionItem = {
  episode_id: 201,
  podcast_id: 20,
  podcast_title: "测试节目",
  podcast_author: "测试作者",
  podcast_cover_url: "",
  episode_title: "对话优先助手用户流",
  episode_no: "201",
  duration: 2400,
  published_date: "2026-08-10T08:00:00Z",
  show_notes:
    "<p>公开来源核对依赖这一段正文：Runtime permissions are reduced per turn.</p>",
  original_url: "https://example.com/episode/201",
  image_url: "",
  notes: "我的私有判断",
  tags: [],
  queue_state: "focus",
  queue_updated_at: "2026-08-10T08:00:00Z",
};

const selectionText = "Runtime permissions are reduced per turn.";

function stageActivity(
  stage: string,
  state: string,
  text: string,
): EpisodeCopilotActivity {
  return {
    id: `stage:${stage}`,
    ordinal: 1,
    stage: stage as EpisodeCopilotActivity["stage"],
    category: "stage",
    state: state as EpisodeCopilotActivity["state"],
    text,
    observed_at: "2026-09-06T12:00:00Z",
  };
}

/** Emit one full real-stage run through the mocked API event callback. */
function emitFullRun(
  onEvent: (event: EpisodeCopilotStreamEvent) => void,
  profileID: string,
  answerText = "## 回答\n\n权限确实按次收窄。",
) {
  const status = (activity: EpisodeCopilotActivity) =>
    onEvent({
      type: "status",
      transcript_used: false,
      private_note_included: false,
      activity,
    });
  onEvent({
    type: "context",
    message: "将使用当前单集的 Show Notes；未使用逐字稿",
    transcript_used: false,
    private_note_included: false,
  });
  status(stageActivity("read_context", "completed", "上下文就绪"));
  status(stageActivity("research_runtime", "started", ""));
  status(stageActivity("research_runtime", "completed", ""));
  status(stageActivity("public_research", "started", ""));
  status(stageActivity("public_research", "completed", ""));
  status(stageActivity("source_validation", "started", ""));
  status(
    stageActivity("source_validation", "completed", "已验证 2 个公开来源"),
  );
  status(stageActivity("answer_runtime", "started", ""));
  onEvent({
    type: "answer_delta",
    message: answerText,
    transcript_used: false,
    private_note_included: false,
    profile_id: profileID as EpisodeCopilotStreamEvent["profile_id"],
  });
  status(stageActivity("citation_validation", "started", ""));
  onEvent({
    type: "complete",
    message: "回答完成",
    transcript_used: false,
    private_note_included: false,
    profile_id: profileID as EpisodeCopilotStreamEvent["profile_id"],
    first_content_ms: 420,
    total_ms: 1980,
  });
}

async function openDetailAndCopilot() {
  render(<ConsumptionDetailPanel item={item} isQueueBusy={false} onClose={vi.fn()} onItemChange={vi.fn()} onMove={vi.fn()} />);
  const dialog = await screen.findByRole("dialog", { name: item.episode_title });
  fireEvent.click(within(dialog).getByRole("button", { name: "单集助手" }));
  const workspace = await within(dialog).findByRole("complementary", {
    name: "单集助手双栏工作台",
  });
  return { dialog, workspace };
}

function captureShowNotesSelection(dialog: HTMLElement) {
  const tabpanel = within(dialog).getByRole("tabpanel", {
    name: "Show Notes",
  });
  const source = within(tabpanel).getByText(
    new RegExp(selectionText.replace(".", "\\.")),
  );
  const range = document.createRange();
  range.selectNodeContents(source);
  vi.spyOn(window, "getSelection").mockReturnValue({
    isCollapsed: false,
    rangeCount: 1,
    getRangeAt: () => range,
    toString: () => selectionText,
  } as unknown as Selection);
  fireEvent(document, new Event("selectionchange"));
  return tabpanel;
}

describe("EpisodeCopilotChat user flow (real Focus Detail seam)", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.clearAllMocks();
    apiMocks.isCancellation.mockReturnValue(false);
    apiMocks.writeText.mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: apiMocks.writeText },
    });
    apiMocks.getItem.mockResolvedValue(item);
    apiMocks.markInProgress.mockResolvedValue(item);
    apiMocks.getShowNotes.mockResolvedValue({
      episode_id: item.episode_id,
      show_notes_document: { content: item.show_notes, format: "html" },
    });
    apiMocks.getNotes.mockResolvedValue("我的私有判断");
    apiMocks.getTags.mockResolvedValue([]);
    apiMocks.listTags.mockResolvedValue([]);
    apiMocks.listEpisodeRuns.mockResolvedValue([]);
    apiMocks.getLatestAudio.mockRejectedValue({ response: { status: 404 } });
    apiMocks.getScheduleStatus.mockResolvedValue({
      enabled: false,
      cron: "",
      timezone: "",
      batch_size: 0,
    });
    apiMocks.getCopilotContext.mockResolvedValue({
      episode_id: item.episode_id,
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
    });
    apiMocks.askCopilot.mockImplementation(
      async (
        _episodeId: number,
        _request: unknown,
        onEvent: (event: EpisodeCopilotStreamEvent) => void,
      ) => {
        onEvent({
          type: "context",
          message: "将使用当前单集的 Show Notes；未使用逐字稿",
          transcript_used: false,
          private_note_included: false,
        });
        onEvent({
          type: "answer_delta",
          message: "## 回答\n\n权限确实按次收窄。",
          transcript_used: false,
          private_note_included: false,
        });
        onEvent({
          type: "complete",
          message: "回答完成",
          transcript_used: false,
          private_note_included: false,
          first_content_ms: 420,
          total_ms: 1980,
        });
      },
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("drives welcome, quick questions, tier and context contracts, and answer actions to a completed close/reopen cycle", async () => {
    apiMocks.askCopilot.mockImplementationOnce(
      (
        _episodeId: number,
        _request: unknown,
        onEvent: (event: EpisodeCopilotStreamEvent) => void,
      ) => {
        emitFullRun(onEvent, "quick");
      },
    );
    const { dialog, workspace } = await openDetailAndCopilot();

    // First visit: identity stays, welcome + three fixed quick questions and
    // the always-visible composer appear.
    expect(
      await within(workspace).findByText("你好，我是这一集的单集助手。"),
    ).toBeInTheDocument();
    const suggestions = within(workspace).getByRole("group", {
      name: "快捷问题",
    });
    expect(
      within(suggestions).getByRole("button", { name: "总结这期节目的核心观点" }),
    ).toBeInTheDocument();
    expect(
      within(suggestions).getByRole("button", { name: "解释这期内容的关键转折" }),
    ).toBeInTheDocument();
    expect(
      within(suggestions).getByRole("button", { name: "列出提到的工具与人物" }),
    ).toBeInTheDocument();
    expect(
      within(workspace).getByRole("textbox", { name: "向单集助手提问" }),
    ).toBeInTheDocument();
    expect(
      within(workspace).getByText("只读回答 · 不会修改你的内容"),
    ).toBeInTheDocument();

    // No successful transcript: the degradation stays explicit and quiet.
    expect(
      within(workspace).getByText(
        "当前无成功逐字稿，将明确降级为 Show Notes。",
      ),
    ).toBeInTheDocument();

    // Tier contract: default balanced, task hints, recommended marker, and
    // the technical section one interaction below the surface.
    const trigger = within(workspace).getByTestId("copilot-profiles");
    expect(trigger).toHaveAttribute("aria-label", "回答档位：均衡");
    fireEvent.click(trigger);
    const menu = within(workspace).getByRole("menu", {
      name: "选择回答档位",
    });
    const options = within(menu).getAllByRole("menuitemradio");
    expect(options).toHaveLength(3);
    expect(options[1]).toHaveAttribute("aria-checked", "true");
    expect(
      within(menu).getByRole("menuitemradio", {
        name: "均衡：速度与完整度兼顾（推荐）",
      }),
    ).toBeInTheDocument();
    // Arrow-key traversal moves between the tier options.
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    expect(
      within(menu).getByRole("menuitemradio", {
        name: "深度：复杂问题与深入分析",
      }),
    ).toHaveFocus();
    fireEvent.keyDown(menu, { key: "Home" });
    expect(options[0]).toHaveFocus();
    fireEvent.click(
      within(menu).getByRole("menuitemradio", { name: /快速/ }),
    );
    expect(trigger).toHaveAttribute("aria-label", "回答档位：快速");

    // Context menu: availability plus the per-question private note
    // authorization, defaulting to off.
    fireEvent.click(
      within(workspace).getByRole("button", { name: "查看本次回答上下文" }),
    );
    const contextMenu = within(workspace).getByRole("group", {
      name: "本次回答上下文",
    });
    expect(
      within(contextMenu).getByText("当前无成功逐字稿，将明确降级为 Show Notes。"),
    ).toBeInTheDocument();
    const privateNote = within(contextMenu).getByRole("checkbox", {
      name: /本次包含我的私有备注/,
    });
    expect(privateNote).not.toBeChecked();
    fireEvent.click(privateNote);
    expect(privateNote).toBeChecked();
    fireEvent.keyDown(contextMenu, { key: "Escape" });

    // A Show Notes selection in the real reading pane becomes a removable
    // attachment near the composer.
    captureShowNotesSelection(dialog);
    expect(
      await within(workspace).findByText("已选 Show Notes"),
    ).toBeInTheDocument();

    fireEvent.change(
      within(workspace).getByRole("textbox", { name: "向单集助手提问" }),
      { target: { value: "为什么需要按次收窄权限？" } },
    );
    fireEvent.click(within(workspace).getByRole("button", { name: "提问" }));

    const questionMessage = await within(workspace).findByText(
      "为什么需要按次收窄权限？",
      { selector: "p" },
    );
    const card = await within(workspace).findByRole("region", {
      name: "助手执行活动",
    });
    const answer = await within(workspace).findByText("权限确实按次收窄。");
    // The activity card keeps its stable seat between the user's question
    // and the assistant answer.
    expect(
      questionMessage.compareDocumentPosition(card) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      card.compareDocumentPosition(answer) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();

    await waitFor(() =>
      expect(apiMocks.askCopilot).toHaveBeenCalledWith(
        item.episode_id,
        {
          question: "为什么需要按次收窄权限？",
          selection: selectionText,
          selection_source: "show_notes",
          include_private_note: true,
          profile_id: "quick",
        },
        expect.any(Function),
        expect.any(AbortSignal),
      ),
    );

    // Completion: verified-source count, secondary run info, copy, and
    // regenerate stay below the answer.
    expect(
      await within(workspace).findByText(/已验证公开来源 2 个/),
    ).toBeInTheDocument();
    expect(
      within(workspace).getByText("首字 420ms · 完成 1980ms · 快速"),
    ).toBeInTheDocument();
    fireEvent.click(within(workspace).getByRole("button", { name: "复制" }));
    await waitFor(() =>
      expect(apiMocks.writeText).toHaveBeenCalledWith(
        "## 回答\n\n权限确实按次收窄。",
      ),
    );

    // Closing returns focus to the detail trigger; reopening keeps this
    // working area (question, attachment state, answer) intact.
    fireEvent.click(
      within(workspace).getByRole("button", { name: "关闭助手" }),
    );
    await waitFor(() =>
      expect(
        within(dialog).getByRole("button", { name: "单集助手" }),
      ).toHaveFocus(),
    );
    fireEvent.click(within(dialog).getByRole("button", { name: "单集助手" }));
    const reopened = await within(dialog).findByRole("complementary", {
      name: "单集助手双栏工作台",
    });
    expect(
      await within(reopened).findByText("为什么需要按次收窄权限？", {
        selector: "p",
      }),
    ).toBeInTheDocument();
    expect(
      within(reopened).getByText("权限确实按次收窄。"),
    ).toBeInTheDocument();
  });

  it("submits a quick question exactly once as an ordinary question", async () => {
    const { workspace } = await openDetailAndCopilot();
    await within(workspace).findByText("你好，我是这一集的单集助手。");

    fireEvent.click(
      within(workspace).getByRole("button", {
        name: "解释这期内容的关键转折",
      }),
    );

    await waitFor(() =>
      expect(apiMocks.askCopilot).toHaveBeenCalledTimes(1),
    );
    expect(apiMocks.askCopilot).toHaveBeenCalledWith(
      item.episode_id,
      expect.objectContaining({
        question: "解释这期内容的关键转折",
        selection: "",
        selection_source: "",
        include_private_note: false,
        profile_id: "balanced",
      }),
      expect.any(Function),
      expect.any(AbortSignal),
    );
    expect(
      await within(workspace).findByText("权限确实按次收窄。"),
    ).toBeInTheDocument();
    expect(
      within(workspace).queryByRole("group", { name: "快捷问题" }),
    ).not.toBeInTheDocument();
  });

  it("keeps reading, real stages, and cancel available during a slow answer, then retries to completion", async () => {
    apiMocks.askCopilot.mockImplementationOnce(
      (_episodeId: number, _request: unknown, onEvent: (event: EpisodeCopilotStreamEvent) => void, signal: AbortSignal) =>
        new Promise<void>((resolve, reject) => {
          onEvent({
            type: "context",
            message: "将使用当前单集的 Show Notes；未使用逐字稿",
            transcript_used: false,
            private_note_included: false,
          });
          onEvent({
            type: "status",
            message: "正在核对公开资料…",
            transcript_used: false,
            private_note_included: false,
            activity: stageActivity("public_research", "started", ""),
          });
          signal.addEventListener(
            "abort",
            () => reject(new Error("cancelled")),
            { once: true },
          );
          void resolve;
        }),
    );
    apiMocks.isCancellation.mockImplementation(
      (error: unknown) =>
        error instanceof Error && error.message === "cancelled",
    );

    const { dialog, workspace } = await openDetailAndCopilot();
    fireEvent.change(
      await within(workspace).findByRole("textbox", {
        name: "向单集助手提问",
      }),
      { target: { value: "这期在讲什么？" } },
    );
    fireEvent.click(within(workspace).getByRole("button", { name: "提问" }));

    const card = await within(workspace).findByRole("region", {
      name: "助手执行活动",
    });
    expect(await within(card).findByText("核对公开资料")).toBeInTheDocument();
    // The reading pane keeps every word readable while the request runs.
    const tabpanel = within(dialog).getByRole("tabpanel", {
      name: "Show Notes",
    });
    expect(
      within(tabpanel).getByText(
        new RegExp(selectionText.replace(".", "\\.")),
      ),
    ).toBeInTheDocument();

    fireEvent.click(within(workspace).getByRole("button", { name: "取消" }));
    expect(
      await within(workspace).findByText(
        "已取消；问题、选区和已有答案已保留。",
      ),
    ).toBeInTheDocument();
    expect(
      within(workspace).getByRole("textbox", { name: "向单集助手提问" }),
    ).toHaveValue("这期在讲什么？");

    apiMocks.askCopilot.mockImplementationOnce(async (
      _episodeId: number,
      request: { question: string },
      onEvent: (event: EpisodeCopilotStreamEvent) => void,
    ) => {
      expect(request.question).toBe("这期在讲什么？");
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
        first_content_ms: 100,
        total_ms: 700,
      });
    });
    fireEvent.click(within(workspace).getByRole("button", { name: "重试" }));
    expect(
      await within(workspace).findByText("重试后的完整回答。"),
    ).toBeInTheDocument();
  });

  it("keeps work through failure and close, and makes retry a new independent request", async () => {
    apiMocks.askCopilot
      .mockImplementationOnce(async (
        _episodeId: number,
        _request: unknown,
        onEvent: (event: EpisodeCopilotStreamEvent) => void,
      ) => {
        onEvent({
          type: "answer_delta",
          message: "## 回答\n\n已生成的部分回答。",
          transcript_used: false,
          private_note_included: false,
        });
        throw new Error("连接中断");
      })
      .mockImplementationOnce(async (
        _episodeId: number,
        request: { question: string; profile_id: string },
        onEvent: (event: EpisodeCopilotStreamEvent) => void,
      ) => {
        expect(request.question).toBe("失败后还能继续吗？");
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
          first_content_ms: 90,
          total_ms: 520,
        });
      });

    const { dialog, workspace } = await openDetailAndCopilot();
    captureShowNotesSelection(dialog);
    fireEvent.change(
      await within(workspace).findByRole("textbox", {
        name: "向单集助手提问",
      }),
      { target: { value: "失败后还能继续吗？" } },
    );
    fireEvent.click(within(workspace).getByRole("button", { name: "提问" }));

    expect(
      await within(workspace).findByText("已生成的部分回答。"),
    ).toBeInTheDocument();
    expect(
      await within(workspace).findByRole("alert"),
    ).toHaveTextContent("问题、选区和已有答案已保留");
    expect(
      within(workspace).getByText("已选 Show Notes"),
    ).toBeInTheDocument();

    // Closing and reopening keeps the failed question and partial answer.
    fireEvent.click(
      within(workspace).getByRole("button", { name: "关闭助手" }),
    );
    fireEvent.click(within(dialog).getByRole("button", { name: "单集助手" }));
    const reopened = await within(dialog).findByRole("complementary", {
      name: "单集助手双栏工作台",
    });
    expect(
      await within(reopened).findByText("失败后还能继续吗？", {
        selector: "p",
      }),
    ).toBeInTheDocument();
    expect(
      within(reopened).getByText("已生成的部分回答。"),
    ).toBeInTheDocument();

    fireEvent.click(within(reopened).getByRole("button", { name: "重试" }));
    expect(
      await within(reopened).findByText("重试后的完整回答。"),
    ).toBeInTheDocument();
    expect(apiMocks.askCopilot).toHaveBeenCalledTimes(2);
  });

  it("never retries an unavailable tier and continues with a switched tier as a new request", async () => {
    apiMocks.askCopilot
      .mockImplementationOnce(async (
        _episodeId: number,
        _request: unknown,
        onEvent: (event: EpisodeCopilotStreamEvent) => void,
      ) => {
        onEvent({
          type: "error",
          message: "当前账号或 Runtime 不支持所选档位，请更换档位后重新提问。",
          code: "profile_unavailable",
          retryable: false,
          transcript_used: false,
          private_note_included: false,
          profile_id: "quick",
        });
        const failure = new Error(
          "当前账号或 Runtime 不支持所选档位，请更换档位后重新提问。",
        ) as Error & { code?: string };
        failure.code = "profile_unavailable";
        throw failure;
      })
      .mockImplementationOnce(async (
        _episodeId: number,
        request: { profile_id: string },
        onEvent: (event: EpisodeCopilotStreamEvent) => void,
      ) => {
        expect(request.profile_id).toBe("deep");
        onEvent({
          type: "answer_delta",
          message: "## 回答\n\n深度档回答。",
          transcript_used: false,
          private_note_included: false,
        });
        onEvent({
          type: "complete",
          message: "回答完成",
          transcript_used: false,
          private_note_included: false,
          profile_id: "deep",
          first_content_ms: 120,
          total_ms: 640,
        });
      });

    const { workspace } = await openDetailAndCopilot();
    const trigger = await within(workspace).findByTestId("copilot-profiles");
    fireEvent.click(trigger);
    const menu = within(workspace).getByRole("menu", {
      name: "选择回答档位",
    });
    fireEvent.click(
      within(menu).getByRole("menuitemradio", { name: /快速/ }),
    );
    fireEvent.change(
      within(workspace).getByRole("textbox", { name: "向单集助手提问" }),
      { target: { value: "快速档可用吗？" } },
    );
    fireEvent.click(within(workspace).getByRole("button", { name: "提问" }));

    expect(
      await within(workspace).findByRole("alert"),
    ).toHaveTextContent("不支持所选档位");
    expect(
      within(workspace).queryByRole("button", { name: "重试" }),
    ).not.toBeInTheDocument();
    expect(
      within(workspace).getByText(
        "当前选择的快速档位已确认不可用；请切换其他档位后再提问。",
      ),
    ).toBeInTheDocument();

    fireEvent.click(trigger);
    const rejectedMenu = within(workspace).getByRole("menu", {
      name: "选择回答档位",
    });
    expect(
      within(rejectedMenu).getByRole("menuitemradio", { name: /快速/ }),
    ).toHaveAttribute("aria-disabled", "true");
    fireEvent.click(
      within(rejectedMenu).getByRole("menuitemradio", { name: /深度/ }),
    );
    fireEvent.change(
      within(workspace).getByRole("textbox", { name: "向单集助手提问" }),
      { target: { value: "换深度档再问一次" } },
    );
    fireEvent.click(within(workspace).getByRole("button", { name: "提问" }));
    expect(
      await within(workspace).findByText("深度档回答。"),
    ).toBeInTheDocument();
  });

  it("regenerates from the completed request and ignores a later tier switch", async () => {
    const { workspace } = await openDetailAndCopilot();
    await within(workspace).findByText("你好，我是这一集的单集助手。");
    fireEvent.change(
      within(workspace).getByRole("textbox", { name: "向单集助手提问" }),
      { target: { value: "换一版回答" } },
    );
    fireEvent.click(within(workspace).getByRole("button", { name: "提问" }));
    await within(workspace).findByText("权限确实按次收窄。");

    // Switch to deep after completion, then regenerate: the original
    // balanced request conditions are reused.
    const trigger = within(workspace).getByTestId("copilot-profiles");
    fireEvent.click(trigger);
    const menu = within(workspace).getByRole("menu", {
      name: "选择回答档位",
    });
    fireEvent.click(
      within(menu).getByRole("menuitemradio", { name: /深度/ }),
    );
    fireEvent.click(
      within(workspace).getByRole("button", { name: "重新生成" }),
    );

    await waitFor(() =>
      expect(apiMocks.askCopilot).toHaveBeenCalledTimes(2),
    );
    expect(apiMocks.askCopilot).toHaveBeenNthCalledWith(
      2,
      item.episode_id,
      expect.objectContaining({
        question: "换一版回答",
        profile_id: "balanced",
      }),
      expect.any(Function),
      expect.any(AbortSignal),
    );
    expect(trigger).toHaveAttribute("aria-label", "回答档位：均衡");
  });

  it.each([800, 390])(
    "keeps the narrow-viewport interaction branch usable at %ipx and returns to the same detail state",
    async (viewportWidth) => {
      const previousWidth = window.innerWidth;
      Object.defineProperty(window, "innerWidth", {
        configurable: true,
        value: viewportWidth,
      });
      try {
        render(
          <ConsumptionDetailPanel
            item={item}
            isQueueBusy={false}
            onClose={vi.fn()}
            onItemChange={vi.fn()}
            onMove={vi.fn()}
          />,
        );
        fireEvent(window, new Event("resize"));
        const detailDialog = await screen.findByRole("dialog", {
          name: item.episode_title,
        });
        fireEvent.click(
          within(detailDialog).getByRole("button", { name: "单集助手" }),
        );
        const mobileDialog = await screen.findByRole("dialog", {
          name: "单集助手",
        });
        const workspace = within(mobileDialog).getByRole("complementary", {
          name: "移动端单集助手",
        });

        // This jsdom test covers the narrow-screen branch and interaction
        // contract; browser evidence covers actual CSS sizing and overflow.
        expect(
          await within(workspace).findByText("你好，我是这一集的单集助手。"),
        ).toBeInTheDocument();
        expect(
          within(workspace).getByRole("group", { name: "快捷问题" }),
        ).toBeInTheDocument();
        const composer = within(workspace).getByRole("textbox", {
          name: "向单集助手提问",
        });
        expect(composer).toBeInTheDocument();
        expect(
          within(workspace).getByRole("button", { name: "提问" }),
        ).toBeInTheDocument();
        fireEvent.click(
          within(workspace).getByTestId("copilot-profiles"),
        );
        expect(
          within(workspace).getByRole("menu", { name: "选择回答档位" }),
        ).toBeInTheDocument();
        fireEvent.keyDown(
          within(workspace).getByRole("menu", { name: "选择回答档位" }),
          { key: "Escape" },
        );
        expect(
          within(workspace).getByRole("button", {
            name: "查看本次回答上下文",
          }),
        ).toBeInTheDocument();

        fireEvent.change(composer, {
          target: { value: "窄屏也要能提问" },
        });
        fireEvent.click(
          within(workspace).getByRole("button", { name: "提问" }),
        );
        expect(
          await within(workspace).findByText("权限确实按次收窄。"),
        ).toBeInTheDocument();
        expect(
          within(workspace).getByRole("button", { name: "复制" }),
        ).toBeInTheDocument();
        expect(
          within(workspace).getByRole("button", { name: "重新生成" }),
        ).toBeInTheDocument();

        fireEvent.click(
          within(workspace).getByRole("button", { name: "返回单集" }),
        );
        const restored = await screen.findByRole("dialog", {
          name: item.episode_title,
        });
        expect(
          within(restored).getByRole("button", { name: "单集助手" }),
        ).toHaveFocus();
      } finally {
        Object.defineProperty(window, "innerWidth", {
          configurable: true,
          value: previousWidth,
        });
        fireEvent(window, new Event("resize"));
      }
    },
  );
});
