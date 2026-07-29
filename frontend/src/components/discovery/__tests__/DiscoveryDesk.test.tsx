import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import DiscoveryDesk from "../DiscoveryDesk";
import type { DiscoveryCandidate } from "@/types/discovery";

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
  });

  it("keeps the list in place while switching the selected candidate", () => {
    render(<DiscoveryDesk candidates={candidates} />);

    expect(
      screen.getByRole("heading", { name: "模型能力如何转向真实应用" }),
    ).toBeInTheDocument();
    const list = screen.getByTestId("discovery-candidate-list");

    fireEvent.click(
      screen.getByRole("button", {
        name: "查看 缺少 Show Notes 的边界项",
      }),
    );

    expect(
      screen.getByRole("heading", { name: "缺少 Show Notes 的边界项" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Show Notes 暂缺")).toBeInTheDocument();
    expect(screen.getByTestId("discovery-candidate-list")).toBe(list);
  });

  it("shows only verifiable recent-update metadata without recommendation scores", () => {
    render(<DiscoveryDesk candidates={candidates} />);

    expect(screen.getAllByText("最近更新")).toHaveLength(3);
    expect(
      screen.getAllByText((_, element) => element?.textContent?.includes("52 分钟") ?? false)
        .length,
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

    fireEvent.click(screen.getByRole("button", { name: "加入备选" }));
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "撤销备选" })).toBeInTheDocument();
    });
    expect(onDecision).toHaveBeenCalledWith(11, "shortlisted");

    fireEvent.click(screen.getByRole("button", { name: "撤销备选" }));
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "加入备选" })).toBeInTheDocument();
    });
    expect(onDecision).toHaveBeenCalledWith(11, "pending");

    fireEvent.click(screen.getByRole("button", { name: "舍弃" }));
    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent("决定保存失败");
    });
    expect(screen.getByRole("button", { name: "舍弃" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
  });

  it("keeps four pre-reads independent and makes personal relevance distinct", () => {
    render(<DiscoveryDesk candidates={candidates} />);

    expect(screen.getByRole("button", { name: "摘要" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByText("这是一段基于 Show Notes 的摘要。")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "与我相关" }));

    expect(screen.getByRole("button", { name: "与我相关" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByText("明确相关")).toBeInTheDocument();
    expect(
      screen.getByText("个人标签「AI 工具」与标题直接重合。"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("这是一段基于 Show Notes 的摘要。"),
    ).not.toBeInTheDocument();
  });

  it("shows pending, insufficient, failed, and missing pre-reads without blocking decisions", () => {
    const onDecision = vi.fn().mockResolvedValue({
      state: "shortlisted",
      decision_updated_at: "2026-07-29T08:20:00Z",
    });
    render(<DiscoveryDesk candidates={candidates} onDecision={onDecision} />);
    fireEvent.click(
      screen.getByRole("button", { name: "查看 缺少 Show Notes 的边界项" }),
    );

    expect(screen.getByText("信息缺失")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "观点" }));
    expect(screen.getByText("尚未完成")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "与我相关" }));
    expect(screen.getByText("证据不足")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "值得质疑" }));
    expect(screen.getByText("生成失败")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "加入备选" })).toBeEnabled();
  });

  it("keeps the candidate usable when pre-read data has not arrived yet", () => {
    const candidateWithoutPreReads = {
      ...candidates[0],
      pre_reads: undefined,
    } as unknown as DiscoveryCandidate;

    render(<DiscoveryDesk candidates={[candidateWithoutPreReads]} />);

    expect(
      screen.getByRole("heading", { name: "模型能力如何转向真实应用" }),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText("四类预读")).not.toBeInTheDocument();
    expect(screen.getByText("预读内容尚未就绪")).toBeInTheDocument();
  });

  it("moves through one mobile candidate at a time with explicit progress", () => {
    render(<DiscoveryDesk candidates={candidates} />);

    expect(screen.getByText("1 / 2")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "上一项" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "下一项" }));

    expect(
      screen.getByRole("heading", { name: "缺少 Show Notes 的边界项" }),
    ).toBeInTheDocument();
    expect(screen.getByText("2 / 2")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "下一项" })).toBeDisabled();
  });

  it("restores the current candidate after refresh and keeps raw evidence in place", () => {
    window.history.replaceState({ magicpodcastDiscoveryEpisodeID: 12 }, "");
    const { unmount } = render(<DiscoveryDesk candidates={candidates} />);

    expect(
      screen.getByRole("heading", { name: "缺少 Show Notes 的边界项" }),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByText("查看原始信息"));
    expect(screen.getByText("Show Notes 暂缺")).toBeInTheDocument();
    expect(screen.getByText("待判断")).toBeInTheDocument();

    unmount();
    render(<DiscoveryDesk candidates={candidates} />);
    expect(
      screen.getByRole("heading", { name: "缺少 Show Notes 的边界项" }),
    ).toBeInTheDocument();
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
