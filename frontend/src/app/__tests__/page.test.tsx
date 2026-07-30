import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import Home from "../page";

vi.mock("swr", () => ({
  default: () => ({
    data: [
      {
        episode_id: 1,
        podcast_id: 1,
        podcast_title: "默认首页节目",
        podcast_author: "作者",
        podcast_cover_url: "",
        episode_title: "默认首页最近更新",
        episode_no: "E1",
        duration: 1800,
        candidate_time: "2026-07-29T08:00:00+08:00",
        time_basis: "published_date",
        source: "最近更新",
        show_notes: "<p>默认首页摘要来源</p>",
        show_notes_status: "available",
        original_url: "https://example.com/1",
        image_url: "",
        decision_state: "pending",
        pre_reads: [
          {
            kind: "summary",
            label: "摘要",
            status: "available",
            content: "默认首页摘要",
            sources: [{ kind: "show_notes", label: "Show Notes" }],
            generated_at: "2026-07-29T08:05:00+08:00",
            version: "evidence-v1",
          },
        ],
      },
    ],
    error: null,
    isLoading: false,
    mutate: vi.fn(),
  }),
}));

vi.mock("@/components/layout/PageLayout", () => ({
  SimplePageLayout: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}));

describe("default page", () => {
  it("opens a content-first view of recent personal-library episodes", () => {
    render(<Home />);

    expect(screen.getByText("你的播客书架")).toBeInTheDocument();
    expect(
      screen.getByRole("region", { name: "个人库最近更新" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("今日初筛工作区")).not.toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "默认首页最近更新" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("个人播客管理与自动化处理工具")).not.toBeInTheDocument();
    expect(screen.queryByText("我的订阅管理")).not.toBeInTheDocument();
  });
});
