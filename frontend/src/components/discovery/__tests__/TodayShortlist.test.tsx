import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import TodayShortlist from "../TodayShortlist";
import type { DiscoveryCandidate, TodayShortlistData } from "@/types/discovery";

function candidate(
  episodeID: number,
  episodeTitle: string,
): DiscoveryCandidate {
  return {
    episode_id: episodeID,
    podcast_id: episodeID,
    podcast_title: `节目 ${episodeID}`,
    podcast_author: "作者",
    podcast_cover_url: "",
    episode_title: episodeTitle,
    episode_no: `E${episodeID}`,
    duration: 1800,
    candidate_time: `2026-07-29T0${episodeID}:00:00+08:00`,
    time_basis: "published_date",
    source: "最近更新",
    show_notes: "<p>可核对内容</p>",
    show_notes_status: "available",
    original_url: `https://example.com/${episodeID}`,
    image_url: "",
    decision_state: "shortlisted",
    decision_updated_at: `2026-07-29T1${episodeID}:00:00+08:00`,
    pre_reads: [
      {
        kind: "summary",
        label: "摘要",
        status: "available",
        content: `${episodeTitle}的必要摘要`,
        sources: [{ kind: "show_notes", label: "Show Notes" }],
        generated_at: "2026-07-29T12:00:00+08:00",
        version: "evidence-v1",
      },
      {
        kind: "viewpoints",
        label: "观点",
        status: "insufficient",
        content: "证据不足",
        sources: [],
        generated_at: "2026-07-29T12:00:00+08:00",
        version: "evidence-v1",
      },
      {
        kind: "relevant",
        label: "与我相关",
        status: "insufficient",
        content: "没有个人信号",
        sources: [],
        generated_at: "2026-07-29T12:00:00+08:00",
        version: "evidence-v1",
      },
      {
        kind: "challenge",
        label: "值得质疑",
        status: "available",
        content: "需核对原始链接",
        sources: [{ kind: "show_notes", label: "Show Notes" }],
        generated_at: "2026-07-29T12:00:00+08:00",
        version: "evidence-v1",
      },
    ],
  };
}

const shortlist: TodayShortlistData = {
  date: "2026-07-29",
  timezone: "Asia/Shanghai",
  candidates: [candidate(2, "第二条备选"), candidate(1, "第一条备选")],
};

describe("TodayShortlist", () => {
  it("shows the shared stable collection with necessary summaries only", () => {
    render(<TodayShortlist data={shortlist} />);

    expect(screen.getByText("2026-07-29")).toBeInTheDocument();
    expect(screen.getByText("Asia/Shanghai")).toBeInTheDocument();
    expect(screen.getAllByRole("article").map((item) => item.textContent)).toEqual([
      expect.stringContaining("第二条备选"),
      expect.stringContaining("第一条备选"),
    ]);
    expect(screen.getByText("第二条备选的必要摘要")).toBeInTheDocument();
    expect(screen.queryByText("深听")).not.toBeInTheDocument();
    expect(screen.queryByText("待播")).not.toBeInTheDocument();
    expect(screen.queryByText("导出")).not.toBeInTheDocument();
  });

  it("removes and undoes the most recent item through idempotent decisions", async () => {
    const onDecision = vi
      .fn()
      .mockResolvedValueOnce({
        state: "pending",
        decision_updated_at: "2026-07-29T13:00:00+08:00",
      })
      .mockResolvedValueOnce({
        state: "shortlisted",
        decision_updated_at: "2026-07-29T13:01:00+08:00",
      });
    render(<TodayShortlist data={shortlist} onDecision={onDecision} />);

    fireEvent.click(screen.getByRole("button", { name: "移出 第二条备选" }));
    await waitFor(() => {
      expect(screen.queryByText("第二条备选")).not.toBeInTheDocument();
    });
    expect(onDecision).toHaveBeenCalledWith(2, "pending");

    fireEvent.click(screen.getByRole("button", { name: "撤销最近移除" }));
    await waitFor(() => {
      expect(screen.getByText("第二条备选")).toBeInTheDocument();
    });
    expect(onDecision).toHaveBeenCalledWith(2, "shortlisted");
  });

  it("keeps return-to-triage available for empty and failed states", () => {
    const { rerender } = render(
      <TodayShortlist
        data={{ date: "2026-07-29", timezone: "Asia/Shanghai", candidates: [] }}
      />,
    );
    expect(screen.getByText("今日还没有备选")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "返回继续初筛" })).toHaveAttribute(
      "href",
      "/discovery",
    );

    rerender(
      <TodayShortlist
        data={{ date: "2026-07-29", timezone: "Asia/Shanghai", candidates: [] }}
        error="今日备选暂时读取失败"
      />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent(
      "今日备选暂时读取失败",
    );
    expect(screen.getByRole("link", { name: "返回继续初筛" })).toBeInTheDocument();
  });
});
