import {
  fireEvent,
  render as testingRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { ReactElement } from "react";
import { renderToString } from "react-dom/server";
import { SWRConfig } from "swr";
import { beforeEach, describe, expect, it, vi } from "vitest";
import DiscoveryDesk from "../DiscoveryDesk";
import { availableTagCache } from "@/lib/tagAvailabilityCache";
import type { Tag } from "@/types";
import type { DiscoveryCandidate } from "@/types/discovery";

const apiMocks = vi.hoisted(() => ({
  episodeGetTags: vi.fn(),
  episodeGetNotes: vi.fn(),
  episodeAddTag: vi.fn(),
  episodeRemoveTag: vi.fn(),
  episodeUpdateNotes: vi.fn(),
  podcastGetTags: vi.fn(),
  podcastGetNotes: vi.fn(),
  podcastAddTag: vi.fn(),
  podcastRemoveTag: vi.fn(),
  podcastUpdateNotes: vi.fn(),
  tagList: vi.fn(),
  tagCreate: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  episodeApi: {
    getTags: apiMocks.episodeGetTags,
    getNotes: apiMocks.episodeGetNotes,
    addTag: apiMocks.episodeAddTag,
    removeTag: apiMocks.episodeRemoveTag,
    updateNotes: apiMocks.episodeUpdateNotes,
  },
  podcastApi: {
    getTags: apiMocks.podcastGetTags,
    getNotes: apiMocks.podcastGetNotes,
    addTag: apiMocks.podcastAddTag,
    removeTag: apiMocks.podcastRemoveTag,
    updateNotes: apiMocks.podcastUpdateNotes,
  },
  tagApi: {
    list: apiMocks.tagList,
    create: apiMocks.tagCreate,
  },
}));

vi.mock("@/lib/toast", () => ({
  toast: {
    error: vi.fn(),
  },
}));

const aiTag: Tag = { id: 1, name: "AI", color: "#7a2f27" };
const productTag: Tag = { id: 2, name: "产品", color: "#1768d0" };

function render(ui: ReactElement) {
  return testingRender(
    <SWRConfig value={{ provider: () => new Map() }}>{ui}</SWRConfig>,
  );
}

const candidates: DiscoveryCandidate[] = [
  {
    episode_id: 11,
    podcast_id: 1,
    podcast_title: "声东击西",
    podcast_author: "声动活泼",
    podcast_cover_url: "https://example.com/cover-a.jpg",
    episode_title: "模型能力如何转向真实应用",
    episode_no: "E11",
    duration: 3120,
    candidate_time: "2026-07-29T08:00:00Z",
    time_basis: "fetched_at",
    source: "最近更新",
    show_notes: "<p>第一条可核对内容</p>",
    show_notes_status: "available",
    original_url: "https://example.com/episodes/11",
    image_url: "",
    decision_state: "pending",
    pre_reads: [
      {
        kind: "summary",
        label: "摘要",
        status: "available",
        content: "这是一段基于 Show Notes 的摘要。",
        sources: [
          { kind: "show_notes", label: "Show Notes" },
          {
            kind: "original_link",
            label: "原始链接",
            url: "https://example.com/episodes/11",
          },
        ],
        generated_at: "2026-07-29T08:05:00Z",
        version: "evidence-v1",
      },
      {
        kind: "viewpoints",
        label: "观点",
        status: "available",
        content: "Show Notes 中可核对的观点。",
        sources: [{ kind: "show_notes", label: "Show Notes" }],
        generated_at: "2026-07-29T08:05:00Z",
        version: "evidence-v1",
      },
      {
        kind: "relevant",
        label: "与我相关",
        status: "available",
        content: "个人标签「AI 工具」与标题直接重合。",
        relation_strength: "明确相关",
        sources: [{ kind: "episode_tag", label: "单集标签：AI 工具" }],
        generated_at: "2026-07-29T08:05:00Z",
        version: "evidence-v1",
      },
      {
        kind: "challenge",
        label: "值得质疑",
        status: "available",
        content: "关键主张仍需回到原始链接或音频核对。",
        sources: [{ kind: "show_notes", label: "Show Notes" }],
        generated_at: "2026-07-29T08:05:00Z",
        version: "evidence-v1",
      },
    ],
  },
  {
    episode_id: 12,
    podcast_id: 2,
    podcast_title: "商业就是这样",
    podcast_author: "商业就是这样",
    podcast_cover_url: "https://example.com/cover-b.jpg",
    episode_title: "缺少 Show Notes 的边界项",
    episode_no: "E12",
    duration: 1800,
    candidate_time: "2026-07-28T08:00:00Z",
    time_basis: "fetched_at",
    source: "最近更新",
    show_notes: "",
    show_notes_status: "missing",
    original_url: "",
    image_url: "",
    decision_state: "pending",
    pre_reads: [
      {
        kind: "summary",
        label: "摘要",
        status: "missing",
        content: "Show Notes 暂缺，无法生成摘要。",
        sources: [],
        generated_at: "2026-07-29T08:05:00Z",
        version: "evidence-v1",
      },
      {
        kind: "viewpoints",
        label: "观点",
        status: "pending",
        content: "观点预读尚未完成。",
        sources: [],
        generated_at: "2026-07-29T08:05:00Z",
        version: "evidence-v1",
      },
      {
        kind: "relevant",
        label: "与我相关",
        status: "insufficient",
        content: "未发现个人标签或备注证据，不生成个人关联。",
        sources: [],
        generated_at: "2026-07-29T08:05:00Z",
        version: "evidence-v1",
      },
      {
        kind: "challenge",
        label: "值得质疑",
        status: "failed",
        content: "预读失败，请直接核对原始信息。",
        sources: [],
        generated_at: "2026-07-29T08:05:00Z",
        version: "evidence-v1",
      },
    ],
  },
];

describe("DiscoveryDesk", () => {
  beforeEach(() => {
    window.history.replaceState(null, "");
    vi.clearAllMocks();
    availableTagCache.clear();
    apiMocks.episodeGetTags.mockImplementation(async (id: number) =>
      id === 11 ? [aiTag] : [],
    );
    apiMocks.episodeGetNotes.mockImplementation(async (id: number) =>
      id === 11 ? "单集旧备注" : "",
    );
    apiMocks.podcastGetTags.mockResolvedValue([productTag]);
    apiMocks.podcastGetNotes.mockResolvedValue("节目旧备注");
    apiMocks.episodeAddTag.mockResolvedValue(undefined);
    apiMocks.episodeRemoveTag.mockResolvedValue(undefined);
    apiMocks.episodeUpdateNotes.mockResolvedValue(undefined);
    apiMocks.podcastAddTag.mockResolvedValue(undefined);
    apiMocks.podcastRemoveTag.mockResolvedValue(undefined);
    apiMocks.podcastUpdateNotes.mockResolvedValue(undefined);
    apiMocks.tagList.mockResolvedValue([aiTag, productTag]);
    apiMocks.tagCreate.mockImplementation(
      async ({ name }: { name: string }) => ({
        id: 3,
        name,
        color: "#746b60",
      }),
    );
  });

  it("leads with recent podcast content without duplicating global navigation", () => {
    const { container } = render(<DiscoveryDesk candidates={candidates} />);

    expect(
      screen.getByRole("region", { name: "工作流最近更新" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("个人播客知识库")).not.toBeInTheDocument();
    expect(screen.queryByText("你的播客书架")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("navigation", { name: "知识管理入口" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("今日初筛工作区")).not.toBeInTheDocument();
    expect(screen.queryByText("先看 1 分钟")).not.toBeInTheDocument();
    expect(screen.queryByText("正在预览")).not.toBeInTheDocument();
    expect(screen.getByTestId("candidate-excerpt-11")).toHaveTextContent(
      "这是一段基于 Show Notes 的摘要。",
    );
    expect(container.querySelectorAll(".discovery-unread-dot")).toHaveLength(2);
    expect(
      container
        .querySelector(".discovery-index")
        ?.querySelector(".discovery-unread-dot"),
    ).not.toBeNull();
    expect(container.querySelector(".discovery-unread-indicator")).toBeNull();
  });

  it("states that recent updates are limited to workflow syncs", () => {
    render(<DiscoveryDesk candidates={[]} />);

    expect(
      screen.getByRole("region", { name: "工作流最近更新" }),
    ).toHaveTextContent("按工作流同步时间 · 最近 14 天");
    expect(
      screen.getByRole("heading", {
        name: "工作流暂时没有同步到新单集",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "工作流抓取到的新单集会按系统同步时间显示在这里。",
      ),
    ).toBeInTheDocument();
  });

  it("removes an opened episode from unread results while keeping its preview open", async () => {
    const onRead = vi.fn().mockResolvedValue({
      episode_id: 11,
      queue_state: null,
      read_at: "2026-07-29T08:10:00Z",
    });
    render(<DiscoveryDesk candidates={candidates} onRead={onRead} />);

    fireEvent.click(screen.getByRole("button", { name: "未读 2" }));
    fireEvent.click(
      screen.getByRole("button", {
        name: "预读 模型能力如何转向真实应用",
      }),
    );

    expect(onRead).toHaveBeenCalledWith(11);
    expect(
      screen.queryByRole("button", {
        name: "预读 模型能力如何转向真实应用",
      }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("dialog")).toHaveAccessibleName(
      "模型能力如何转向真实应用",
    );
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "未读 1" }),
      ).toBeInTheDocument(),
    );
  });

  it("uses Discovery as the section title and explains the rolling window", () => {
    render(<DiscoveryDesk candidates={candidates} />);

    const sidebar = screen.getByRole("complementary", {
      name: "Discovery 导航与筛选",
    });

    expect(
      screen.queryByText("订阅单集，按发布时间排序。"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText(
        "订阅更新、单集摘录、标签与备注，按原始内容留在同一处。",
      ),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText("按发布时间陈列；日期缺失时，以更新时间补位。"),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Discovery" })).toHaveClass(
      "editorial-section-title",
    );
    expect(screen.getByText("最近更新 · 14 天")).toBeInTheDocument();
    expect(
      screen.getByText("按工作流同步时间 · 最近 14 天"),
    ).toBeInTheDocument();
    expect(sidebar.querySelector(".discovery-workbench-copy")).toHaveClass(
      "editorial-title-group",
    );
  });

  it("keeps Focus beside the filters in the shared left rail", () => {
    const { container } = render(
      <DiscoveryDesk
        candidates={candidates}
        reportContent={<section aria-label="精选报告">报告</section>}
        focusContent={<section aria-label="Focus 快捷摘要">Focus</section>}
      />,
    );

    expect(screen.getByText("共 02 集")).toBeInTheDocument();
    expect(screen.getByText("1 / 1")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "上一页" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "下一页" })).toBeDisabled();
    expect(screen.getByLabelText("精选报告")).toBeInTheDocument();
    const sidebar = screen.getByRole("complementary", {
      name: "Discovery 导航与筛选",
    });
    const focusRail = screen.getByRole("region", {
      name: "Focus 快捷区域",
    });
    expect(sidebar).toContainElement(focusRail);
    expect(focusRail).toContainElement(
      screen.getByLabelText("Focus 快捷摘要"),
    );
    expect(container.querySelector(".discovery-stream")).not.toContainElement(
      focusRail,
    );
    expect(screen.queryByRole("separator")).not.toBeInTheDocument();
  });

  it("keeps recent updates in a fixed viewport and paginates four at a time", () => {
    const pagedCandidates = Array.from({ length: 5 }, (_, index) => ({
      ...candidates[0],
      episode_id: 100 + index,
      episode_title: `分页单集 ${index + 1}`,
      candidate_time: `2026-07-29T0${8 - index}:00:00Z`,
    }));

    render(<DiscoveryDesk candidates={pagedCandidates} />);

    expect(screen.getByTestId("discovery-candidate-viewport")).toBeInTheDocument();
    expect(screen.getByText("分页单集 1")).toBeInTheDocument();
    expect(screen.getByText("分页单集 4")).toBeInTheDocument();
    expect(screen.queryByText("分页单集 5")).not.toBeInTheDocument();
    expect(screen.getByText("1 / 2")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "下一页" }));

    expect(screen.queryByText("分页单集 1")).not.toBeInTheDocument();
    expect(screen.getByText("分页单集 5")).toBeInTheDocument();
    expect(screen.getByText("2 / 2")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "下一页" })).toBeDisabled();
  });

  it("opens pre-read from the full card instead of a separate preview action", () => {
    render(<DiscoveryDesk candidates={candidates} />);

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    const candidate = screen.getByRole("button", {
      name: "预读 模型能力如何转向真实应用",
    });
    fireEvent.click(candidate);
    expect(screen.getByRole("dialog")).toHaveAccessibleName(
      "模型能力如何转向真实应用",
    );
    expect(candidate).toHaveAttribute("aria-expanded", "true");
    expect(
      screen.queryByRole("button", { name: "预读 Show Notes" }),
    ).not.toBeInTheDocument();
  });

  it("defers pre-read and metadata until the episode card is opened", async () => {
    render(<DiscoveryDesk candidates={candidates} onDecision={vi.fn()} />);

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(apiMocks.episodeGetTags).not.toHaveBeenCalled();
    expect(apiMocks.episodeGetNotes).not.toHaveBeenCalled();

    fireEvent.click(
      screen.getByRole("button", {
        name: "预读 模型能力如何转向真实应用",
      }),
    );
    expect(
      screen.getByRole("button", { name: "编辑标签与备注" }),
    ).toHaveAttribute("aria-expanded", "false");
    await screen.findByText("第一条可核对内容");
    expect(
      screen.getByRole("region", { name: "Show Notes" }),
    ).toHaveTextContent("第一条可核对内容");
  });

  it("loads full Show Notes only after a lightweight candidate is opened", async () => {
    let resolveDetails:
      | ((candidate: DiscoveryCandidate) => void)
      | undefined;
    const loadDetails = vi.fn(
      () =>
        new Promise<DiscoveryCandidate>((resolve) => {
          resolveDetails = resolve;
        }),
    );
    const summaryCandidate: DiscoveryCandidate = {
      ...candidates[0],
      excerpt: "列表保留的轻量摘要",
      show_notes: "",
      pre_reads: [],
      metadata_only: true,
    };

    render(
      <DiscoveryDesk
        candidates={[summaryCandidate]}
        onLoadCandidateDetails={loadDetails}
      />,
    );

    expect(screen.getByTestId("candidate-excerpt-11")).toHaveTextContent(
      "列表保留的轻量摘要",
    );
    fireEvent.click(
      screen.getByRole("button", {
        name: "预读 模型能力如何转向真实应用",
      }),
    );

    expect(loadDetails).toHaveBeenCalledWith(11);
    expect(
      screen.getByRole("region", { name: "Show Notes" }),
    ).toHaveTextContent("正在加载 Show Notes");

    resolveDetails?.({ ...candidates[0], metadata_only: false });
    expect(await screen.findByText("第一条可核对内容")).toBeInTheDocument();
  });

  it("keeps the list usable and retries when one detail request fails", async () => {
    const loadDetails = vi
      .fn()
      .mockRejectedValueOnce(new Error("detail unavailable"))
      .mockResolvedValueOnce({ ...candidates[0], metadata_only: false });
    const summaryCandidate: DiscoveryCandidate = {
      ...candidates[0],
      excerpt: "失败时仍保留的列表摘要",
      show_notes: "",
      pre_reads: [],
      metadata_only: true,
    };

    render(
      <DiscoveryDesk
        candidates={[summaryCandidate]}
        onLoadCandidateDetails={loadDetails}
      />,
    );
    fireEvent.click(
      screen.getByRole("button", {
        name: "预读 模型能力如何转向真实应用",
      }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Show Notes 暂时无法加载",
    );
    expect(screen.getByText("失败时仍保留的列表摘要")).toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", { name: "重新加载 Show Notes" }),
    );
    expect(await screen.findByText("第一条可核对内容")).toBeInTheDocument();
    expect(loadDetails).toHaveBeenCalledTimes(2);
  });

  it("uses the same episode-number display contract in the compact list", () => {
    const numbered = candidates[0];
    const unnumbered = {
      ...candidates[1],
      episode_no: "",
      duration: 4200,
    };

    render(<DiscoveryDesk candidates={[numbered, unnumbered]} />);

    expect(screen.getByText("#11 · 52 分钟")).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "预读 缺少 Show Notes 的边界项" }),
    );
    expect(screen.getAllByText("70 分钟")).toHaveLength(2);
    expect(
      Array.from(document.querySelectorAll(".discovery-candidate-details"))
        .map((element) => element.textContent)
        .join(" "),
    ).not.toContain("单集");
  });

  it("keeps progress outside the balanced three-action toolbar", () => {
    render(<DiscoveryDesk candidates={candidates} />);

    expect(document.querySelector(".discovery-preview-identity")).toBeNull();
    fireEvent.click(
      screen.getByRole("button", {
        name: "预读 模型能力如何转向真实应用",
      }),
    );
    const headingTools = document.querySelector<HTMLElement>(
      ".discovery-preview-heading-tools",
    );
    const currentCount = document.querySelector<HTMLElement>(
      ".discovery-current-count",
    );
    const toolbar = document.querySelector<HTMLElement>(
      ".discovery-quick-actions",
    );
    const editToggle = screen.getByRole("button", {
      name: "编辑标签与备注",
    });

    expect(headingTools).toBeTruthy();
    expect(currentCount).toHaveTextContent("01 / 02");
    expect(toolbar).toBeTruthy();
    expect(toolbar).not.toContainElement(currentCount);
    expect(headingTools?.children[0]).toBe(currentCount);
    expect(headingTools?.children[1]).toBe(toolbar);
    expect(headingTools?.children[2]).toBe(editToggle);
    expect(headingTools?.children[3]).toBe(
      document.querySelector(".discovery-preview-close"),
    );
    expect(toolbar?.children).toHaveLength(3);
    expect(toolbar).toContainElement(
      screen.getByRole("link", { name: "打开节目页面" }),
    );
    expect(toolbar).toContainElement(
      screen.getByRole("button", { name: "不感兴趣" }),
    );
    expect(toolbar).toContainElement(
      within(toolbar!).getByRole("button", { name: "收集到 Inbox" }),
    );
  });

  it("uses icon-only decision controls with accessible hover labels", () => {
    render(<DiscoveryDesk candidates={candidates} onDecision={vi.fn()} />);

    fireEvent.click(
      screen.getByRole("button", {
        name: "预读 模型能力如何转向真实应用",
      }),
    );
    const toolbar = document.querySelector<HTMLElement>(
      ".discovery-quick-actions",
    );
    expect(toolbar).toBeTruthy();
    const discard = within(toolbar!).getByRole("button", {
      name: "不感兴趣",
    });
    const shortlist = within(toolbar!).getByRole("button", {
      name: "收集到 Inbox",
    });

    expect(discard).toHaveAttribute("title", "不感兴趣");
    expect(shortlist).toHaveAttribute("title", "收集到 Inbox");
    expect(discard).toHaveTextContent("");
    expect(shortlist).toHaveTextContent("");
    expect(discard.querySelector("svg")).toBeInTheDocument();
    expect(shortlist.querySelector("svg")).toBeInTheDocument();
  });

  it("keeps the list in place while switching the selected candidate", () => {
    render(<DiscoveryDesk candidates={candidates} />);

    const list = screen.getByTestId("discovery-candidate-list");
    fireEvent.click(
      screen.getByRole("button", {
        name: "预读 模型能力如何转向真实应用",
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "下一项" }));
    expect(
      screen.getByRole("dialog", { name: "缺少 Show Notes 的边界项" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "节目链接暂缺" })).toBeDisabled();
    expect(screen.getByTestId("discovery-candidate-list")).toBe(list);
  });

  it("shows only verifiable recent-update metadata without recommendation scores", async () => {
    render(<DiscoveryDesk candidates={candidates} />);

    expect(
      screen.getByRole("heading", { name: "Discovery" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "预读 模型能力如何转向真实应用" }),
    ).toBeInTheDocument();
    const expectedCandidateDate = new Intl.DateTimeFormat("zh-CN", {
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    }).format(new Date(candidates[0].candidate_time));
    expect(screen.getByText(expectedCandidateDate)).toBeInTheDocument();
    expect(
      screen.getAllByText("这是一段基于 Show Notes 的摘要。"),
    ).toHaveLength(1);
    expect(screen.queryByText("第一条可核对内容")).not.toBeInTheDocument();
    expect(
      screen.getAllByText(
        (_, element) => element?.textContent?.includes("52 分钟") ?? false,
      ).length,
    ).toBeGreaterThan(0);
    expect(screen.queryByText("推荐分数")).not.toBeInTheDocument();
    expect(screen.queryByText("编辑精选")).not.toBeInTheDocument();
    expect(screen.queryByText("兴趣推荐")).not.toBeInTheDocument();
  });

  it("collects into Inbox and rolls back a failed dismissal", async () => {
    const onDecision = vi
      .fn()
      .mockResolvedValueOnce({
        state: "shortlisted",
        decision_updated_at: "2026-07-29T08:10:00Z",
      })
      .mockRejectedValueOnce(new Error("server failed"));

    render(<DiscoveryDesk candidates={candidates} onDecision={onDecision} />);

    fireEvent.click(screen.getAllByRole("button", { name: "收集到 Inbox" })[0]);
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "已在 Inbox" }),
      ).toBeInTheDocument();
    });
    expect(onDecision).toHaveBeenCalledWith(11, "shortlisted");
    expect(screen.getByRole("button", { name: "已在 Inbox" })).toBeDisabled();

    fireEvent.click(
      screen.getByRole("button", { name: "预读 缺少 Show Notes 的边界项" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "不感兴趣" }));
    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent("状态保存失败");
    });
    expect(screen.getByRole("button", { name: "不感兴趣" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
  });

  it("uses source-backed Show Notes instead of AI pre-read tabs", async () => {
    render(<DiscoveryDesk candidates={candidates} />);

    fireEvent.click(
      screen.getByRole("button", {
        name: "预读 模型能力如何转向真实应用",
      }),
    );
    expect(await screen.findByText("第一条可核对内容")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "摘要" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "与我相关" }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "打开节目页面" })).toHaveAttribute(
      "href",
      "https://example.com/episodes/11",
    );
  });

  it("keeps decisions available when Show Notes are missing", () => {
    const onDecision = vi.fn().mockResolvedValue({
      state: "shortlisted",
      decision_updated_at: "2026-07-29T08:20:00Z",
    });
    render(<DiscoveryDesk candidates={candidates} onDecision={onDecision} />);
    fireEvent.click(
      screen.getByRole("button", { name: "预读 缺少 Show Notes 的边界项" }),
    );

    expect(
      screen.getByRole("region", { name: "Show Notes" }),
    ).toHaveTextContent("暂无 Show Notes");
    expect(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: "收集到 Inbox",
      }),
    ).toBeEnabled();
  });

  it("opens and closes metadata editing while preserving the reading surface", async () => {
    render(<DiscoveryDesk candidates={candidates} />);

    fireEvent.click(
      screen.getByRole("button", {
        name: "预读 模型能力如何转向真实应用",
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "编辑标签与备注" }));
    expect(
      screen.getAllByRole("button", { name: "收起编辑" })[0],
    ).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByLabelText("标签与备注编辑")).toBeInTheDocument();
    expect(apiMocks.episodeGetTags).toHaveBeenCalledWith(11);
    expect(apiMocks.episodeGetNotes).toHaveBeenCalledWith(11);
    expect(await screen.findByText("单集旧备注")).toBeInTheDocument();

    fireEvent.click(screen.getAllByRole("button", { name: "收起编辑" })[0]);
    expect(screen.queryByLabelText("标签与备注编辑")).not.toBeInTheDocument();
    expect(await screen.findByText("第一条可核对内容")).toBeInTheDocument();
  });

  it("keeps editing open across episodes and loads Podcast metadata on demand", async () => {
    render(<DiscoveryDesk candidates={candidates} />);

    fireEvent.click(
      screen.getByRole("button", {
        name: "预读 模型能力如何转向真实应用",
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "编辑标签与备注" }));
    await screen.findByText("单集旧备注");
    fireEvent.click(screen.getByRole("button", { name: "下一项" }));

    expect(
      screen.getAllByRole("button", { name: "收起编辑" })[0],
    ).toHaveAttribute("aria-expanded", "true");
    await waitFor(() => {
      expect(apiMocks.episodeGetTags).toHaveBeenCalledWith(12);
      expect(apiMocks.episodeGetNotes).toHaveBeenCalledWith(12);
    });

    fireEvent.click(screen.getByRole("tab", { name: "Podcast" }));
    await waitFor(() => {
      expect(apiMocks.podcastGetTags).toHaveBeenCalledWith(2);
      expect(apiMocks.podcastGetNotes).toHaveBeenCalledWith(2);
    });
    expect(await screen.findByText("节目旧备注")).toBeInTheDocument();
  });

  it("reuses existing tag and note editing behavior", async () => {
    render(<DiscoveryDesk candidates={candidates} />);

    fireEvent.click(
      screen.getByRole("button", {
        name: "预读 模型能力如何转向真实应用",
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "编辑标签与备注" }));
    await screen.findByText("单集旧备注");

    fireEvent.click(screen.getByRole("button", { name: "删除标签 AI" }));
    await waitFor(() => {
      expect(apiMocks.episodeRemoveTag).toHaveBeenCalledWith(11, 1);
    });

    const tagInput = screen.getByPlaceholderText(
      "点击输入框从列表选择，或输入新标签名按回车添加",
    );
    fireEvent.focus(tagInput);
    fireEvent.click(await screen.findByRole("button", { name: "产品" }));
    await waitFor(() => {
      expect(apiMocks.episodeAddTag).toHaveBeenCalledWith(11, 2);
    });

    fireEvent.click(screen.getByRole("button", { name: "编辑" }));
    fireEvent.change(screen.getByPlaceholderText("添加备注..."), {
      target: { value: "单集新备注" },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() => {
      expect(apiMocks.episodeUpdateNotes).toHaveBeenCalledWith(
        11,
        "单集新备注",
      );
    });
  });

  it("keeps the list cover fallback without duplicating it in Actions", () => {
    render(<DiscoveryDesk candidates={[candidates[0]]} />);

    for (const cover of screen.getAllByRole("img", { name: "声东击西封面" })) {
      fireEvent.error(cover);
    }

    expect(screen.getAllByText("暂无封面")).toHaveLength(1);
    expect(document.querySelector(".discovery-preview-cover")).toBeNull();
    expect(
      screen.queryByRole("img", { name: "声东击西封面" }),
    ).not.toBeInTheDocument();
  });

  it("moves through one mobile candidate at a time with explicit progress", () => {
    render(<DiscoveryDesk candidates={candidates} />);

    fireEvent.click(
      screen.getByRole("button", {
        name: "预读 模型能力如何转向真实应用",
      }),
    );
    expect(screen.getByText("1 / 2")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "上一项" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "下一项" }));

    expect(
      screen.getByRole("region", { name: "Show Notes" }),
    ).toHaveTextContent("暂无 Show Notes");
    expect(screen.getByText("2 / 2")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "下一项" })).toBeDisabled();
  });

  it("restores the current candidate after refresh and keeps the source-link state", () => {
    window.history.replaceState({ magicpodcastDiscoveryEpisodeID: 12 }, "");
    const { unmount } = render(<DiscoveryDesk candidates={candidates} />);

    expect(
      screen.getByRole("button", { name: "预读 缺少 Show Notes 的边界项" }),
    ).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("button", { name: "节目链接暂缺" })).toBeDisabled();
    expect(screen.queryByText("节目原文")).not.toBeInTheDocument();
    expect(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: "收集到 Inbox",
      }),
    ).toHaveAttribute("aria-pressed", "false");

    unmount();
    render(<DiscoveryDesk candidates={candidates} />);
    expect(
      screen.getByRole("button", { name: "预读 缺少 Show Notes 的边界项" }),
    ).toHaveAttribute("aria-expanded", "true");
  });

  it("keeps server markup independent of browser history before restoring a preview", () => {
    window.history.replaceState({ magicpodcastDiscoveryEpisodeID: 12 }, "");

    const markup = renderToString(<DiscoveryDesk candidates={candidates} />);

    expect(markup).not.toContain('role="dialog"');
    expect(markup).not.toContain("discovery-preview-overlay");
  });

  it("ignores edge swipes but supports a center swipe shortcut", () => {
    render(<DiscoveryDesk candidates={candidates} />);
    fireEvent.click(
      screen.getByRole("button", {
        name: "预读 模型能力如何转向真实应用",
      }),
    );
    const preview = screen.getByTestId("discovery-mobile-card");
    vi.spyOn(preview, "getBoundingClientRect").mockReturnValue({
      bottom: 700,
      height: 600,
      left: 0,
      right: 390,
      top: 100,
      width: 390,
      x: 0,
      y: 100,
      toJSON: () => ({}),
    });

    fireEvent.touchStart(preview, { touches: [{ clientX: 20 }] });
    fireEvent.touchEnd(preview, { changedTouches: [{ clientX: 180 }] });
    expect(screen.getByText("1 / 2")).toBeInTheDocument();

    fireEvent.touchStart(preview, { touches: [{ clientX: 300 }] });
    fireEvent.touchEnd(preview, { changedTouches: [{ clientX: 180 }] });
    expect(screen.getByText("2 / 2")).toBeInTheDocument();
  });
});
