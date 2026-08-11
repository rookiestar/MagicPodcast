import {
  fireEvent,
  render as testingRender,
  screen,
  waitFor,
} from "@testing-library/react";
import type { ReactElement } from "react";
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
    time_basis: "published_date",
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
    time_basis: "updated_date",
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
    apiMocks.tagCreate.mockImplementation(async ({ name }: { name: string }) => ({
      id: 3,
      name,
      color: "#746b60",
    }));
  });

  it("leads with recent podcast content without duplicating global navigation", () => {
    render(<DiscoveryDesk candidates={candidates} />);

    expect(
      screen.getByRole("region", { name: "个人库最近更新" }),
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
  });

  it("keeps the recent-update header focused on the section title", () => {
    render(<DiscoveryDesk candidates={candidates} />);

    const recentUpdates = screen.getByRole("region", {
      name: "个人库最近更新",
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
    expect(
      screen.getByRole("heading", { name: "最近更新" }),
    ).toHaveClass("editorial-section-title");
    expect(
      recentUpdates.querySelector(".discovery-workbench-copy"),
    ).toHaveClass("editorial-title-group");
  });

  it("marks the end of a short list and exposes an adjustable desktop split", () => {
    render(<DiscoveryDesk candidates={candidates} />);

    expect(screen.getByText("本轮 02 集")).toBeInTheDocument();
    expect(screen.getByText("最近更新已到底")).toBeInTheDocument();

    const resizer = screen.getByRole("separator", {
      name: "调整 Episodes 列表与 Quick Actions 区域宽度",
    });
    expect(resizer).toHaveAttribute("aria-valuenow", "60");

    fireEvent.keyDown(resizer, { key: "ArrowLeft" });
    expect(resizer).toHaveAttribute("aria-valuenow", "57");
  });

  it("uses a compact directional icon for the current candidate", () => {
    render(<DiscoveryDesk candidates={candidates} />);

    const currentCandidate = screen.getByRole("button", {
      name: "查看 模型能力如何转向真实应用",
    });
    const openState = currentCandidate.querySelector(".discovery-open-state");

    expect(openState).toHaveAttribute("data-state", "current");
    expect(openState).toHaveAttribute("title", "当前单集");
    expect(openState?.querySelector("svg")).toBeInTheDocument();
    expect(openState?.querySelector(".sr-only")).toHaveTextContent("当前单集");
  });

  it("keeps the Quick Actions header concise and defers metadata loading", () => {
    render(<DiscoveryDesk candidates={candidates} onDecision={vi.fn()} />);

    expect(
      screen.getByRole("heading", { name: "Episodes" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Quick Actions" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("heading", { name: "Show Notes" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("单集预读")).not.toBeInTheDocument();
    expect(screen.queryByText("摘要、观点、关联与质疑")).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "编辑标签与备注" }),
    ).toHaveAttribute("aria-expanded", "false");
    expect(apiMocks.episodeGetTags).not.toHaveBeenCalled();
    expect(apiMocks.episodeGetNotes).not.toHaveBeenCalled();
    expect(
      screen.getByRole("button", { name: "加入今日备选" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "留到今天" }),
    ).not.toBeInTheDocument();
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
      screen.getByRole("button", { name: "查看 缺少 Show Notes 的边界项" }),
    );
    expect(screen.getByText("70 分钟")).toBeInTheDocument();
    expect(
      Array.from(document.querySelectorAll(".discovery-candidate-details"))
        .map((element) => element.textContent)
        .join(" "),
    ).not.toContain("单集");
  });

  it("moves the three source and triage actions into the Quick Actions toolbar", () => {
    render(<DiscoveryDesk candidates={candidates} />);

    expect(document.querySelector(".discovery-preview-identity")).toBeNull();
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
    expect(toolbar?.children).toHaveLength(3);
    expect(toolbar).toContainElement(
      screen.getByRole("link", { name: "打开节目页面" }),
    );
    expect(toolbar).toContainElement(
      screen.getByRole("button", { name: "忽略" }),
    );
    expect(toolbar).toContainElement(
      screen.getByRole("button", { name: "加入今日备选" }),
    );
  });

  it("uses icon-only decision controls with accessible hover labels", () => {
    render(<DiscoveryDesk candidates={candidates} onDecision={vi.fn()} />);

    const discard = screen.getByRole("button", { name: "忽略" });
    const shortlist = screen.getByRole("button", { name: "加入今日备选" });

    expect(discard).toHaveAttribute("title", "忽略");
    expect(shortlist).toHaveAttribute("title", "加入今日备选");
    expect(discard).toHaveTextContent("");
    expect(shortlist).toHaveTextContent("");
    expect(discard.querySelector("svg")).toBeInTheDocument();
    expect(shortlist.querySelector("svg")).toBeInTheDocument();
  });

  it("keeps the list in place while switching the selected candidate", () => {
    render(<DiscoveryDesk candidates={candidates} />);

    expect(
      screen.getByRole("button", { name: "查看 模型能力如何转向真实应用" }),
    ).toHaveAttribute("aria-pressed", "true");
    const list = screen.getByTestId("discovery-candidate-list");

    fireEvent.click(
      screen.getByRole("button", {
        name: "查看 缺少 Show Notes 的边界项",
      }),
    );

    expect(
      screen.getByRole("button", { name: "查看 缺少 Show Notes 的边界项" }),
    ).toHaveAttribute("aria-pressed", "true");
    expect(
      screen.getByRole("button", { name: "节目链接暂缺" }),
    ).toBeDisabled();
    expect(screen.getByTestId("discovery-candidate-list")).toBe(list);
  });

  it("shows only verifiable recent-update metadata without recommendation scores", async () => {
    render(<DiscoveryDesk candidates={candidates} />);

    expect(
      screen.getByRole("heading", { name: "最近更新" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "查看 模型能力如何转向真实应用" }),
    ).toBeInTheDocument();
    expect(screen.getByText("07/29 16:00")).toBeInTheDocument();
    expect(
      screen.getAllByText("这是一段基于 Show Notes 的摘要。"),
    ).toHaveLength(1);
    expect(await screen.findByText("第一条可核对内容")).toBeInTheDocument();
    expect(
      screen.getAllByText(
        (_, element) => element?.textContent?.includes("52 分钟") ?? false,
      ).length,
    ).toBeGreaterThan(0);
    expect(screen.queryByText("推荐分数")).not.toBeInTheDocument();
    expect(screen.queryByText("编辑精选")).not.toBeInTheDocument();
    expect(screen.queryByText("兴趣推荐")).not.toBeInTheDocument();
  });

  it("updates, undoes, and rolls back decisions against server state", async () => {
    const onDecision = vi
      .fn()
      .mockResolvedValueOnce({
        state: "shortlisted",
        decision_updated_at: "2026-07-29T08:10:00Z",
      })
      .mockResolvedValueOnce({
        state: "pending",
        decision_updated_at: "2026-07-29T08:11:00Z",
      })
      .mockRejectedValueOnce(new Error("server failed"));

    render(<DiscoveryDesk candidates={candidates} onDecision={onDecision} />);

    fireEvent.click(screen.getByRole("button", { name: "加入今日备选" }));
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "移出今日备选" }),
      ).toBeInTheDocument();
    });
    expect(onDecision).toHaveBeenCalledWith(11, "shortlisted");

    fireEvent.click(screen.getByRole("button", { name: "移出今日备选" }));
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "加入今日备选" }),
      ).toBeInTheDocument();
    });
    expect(onDecision).toHaveBeenCalledWith(11, "pending");

    fireEvent.click(screen.getByRole("button", { name: "忽略" }));
    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent("决定保存失败");
    });
    expect(screen.getByRole("button", { name: "忽略" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
  });

  it("uses source-backed Show Notes instead of AI pre-read tabs", async () => {
    render(<DiscoveryDesk candidates={candidates} />);

    expect(await screen.findByText("第一条可核对内容")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "摘要" })).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "与我相关" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "打开节目页面" }),
    ).toHaveAttribute("href", "https://example.com/episodes/11");
  });

  it("keeps decisions available when Show Notes are missing", () => {
    const onDecision = vi.fn().mockResolvedValue({
      state: "shortlisted",
      decision_updated_at: "2026-07-29T08:20:00Z",
    });
    render(<DiscoveryDesk candidates={candidates} onDecision={onDecision} />);
    fireEvent.click(
      screen.getByRole("button", { name: "查看 缺少 Show Notes 的边界项" }),
    );

    expect(
      screen.getByRole("region", { name: "Show Notes" }),
    ).toHaveTextContent("暂无 Show Notes");
    expect(screen.getByRole("button", { name: "加入今日备选" })).toBeEnabled();
  });

  it("opens and closes metadata editing while preserving the reading surface", async () => {
    render(<DiscoveryDesk candidates={candidates} />);

    fireEvent.click(screen.getByRole("button", { name: "编辑标签与备注" }));
    expect(
      screen.getAllByRole("button", { name: "收起编辑" })[0],
    ).toHaveAttribute("aria-expanded", "true");
    expect(
      screen.getByLabelText("标签与备注编辑"),
    ).toBeInTheDocument();
    expect(apiMocks.episodeGetTags).toHaveBeenCalledWith(11);
    expect(apiMocks.episodeGetNotes).toHaveBeenCalledWith(11);
    expect(await screen.findByText("单集旧备注")).toBeInTheDocument();

    fireEvent.click(screen.getAllByRole("button", { name: "收起编辑" })[0]);
    expect(
      screen.queryByLabelText("标签与备注编辑"),
    ).not.toBeInTheDocument();
    expect(await screen.findByText("第一条可核对内容")).toBeInTheDocument();
  });

  it("keeps editing open across episodes and loads Podcast metadata on demand", async () => {
    render(<DiscoveryDesk candidates={candidates} />);

    fireEvent.click(screen.getByRole("button", { name: "编辑标签与备注" }));
    await screen.findByText("单集旧备注");
    fireEvent.click(
      screen.getByRole("button", { name: "查看 缺少 Show Notes 的边界项" }),
    );

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
      expect(apiMocks.episodeUpdateNotes).toHaveBeenCalledWith(11, "单集新备注");
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
      screen.getByRole("button", { name: "查看 缺少 Show Notes 的边界项" }),
    ).toHaveAttribute("aria-pressed", "true");
    expect(
      screen.getByRole("button", { name: "节目链接暂缺" }),
    ).toBeDisabled();
    expect(screen.queryByText("节目原文")).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "加入今日备选" }),
    ).toHaveAttribute("aria-pressed", "false");

    unmount();
    render(<DiscoveryDesk candidates={candidates} />);
    expect(
      screen.getByRole("button", { name: "查看 缺少 Show Notes 的边界项" }),
    ).toHaveAttribute("aria-pressed", "true");
  });

  it("ignores edge swipes but supports a center swipe shortcut", () => {
    render(<DiscoveryDesk candidates={candidates} />);
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
