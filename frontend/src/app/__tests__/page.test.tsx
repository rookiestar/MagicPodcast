import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import Home from "../page";

const candidates = [
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
    time_basis: "published_date" as const,
    source: "最近更新" as const,
    show_notes: "<p>默认首页摘要来源</p>",
    show_notes_status: "available" as const,
    original_url: "https://example.com/1",
    image_url: "",
    decision_state: "pending" as const,
    pre_reads: [
      {
        kind: "summary" as const,
        label: "摘要",
        status: "available" as const,
        content: "默认首页摘要",
        sources: [{ kind: "show_notes", label: "Show Notes" }],
        generated_at: "2026-07-29T08:05:00+08:00",
        version: "evidence-v1",
      },
    ],
  },
];

vi.mock("swr", () => ({
  default: (
    _key: string,
    _fetcher: unknown,
    options?: { fallbackData?: typeof candidates },
  ) => ({
    data: options?.fallbackData,
    error: null,
    isLoading: true,
    mutate: vi.fn(),
  }),
}));

vi.mock("@/components/layout/PageLayout", () => ({
  SimplePageLayout: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}));

afterEach(() => {
  vi.unstubAllGlobals();
  window.sessionStorage.clear();
});

describe("default page", () => {
  it("renders server-provided candidates while client revalidation is pending", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue({
        success: true,
        data: candidates,
      }),
    });
    vi.stubGlobal("fetch", fetchMock);

    render(await Home());

    expect(screen.queryByText("你的播客书架")).not.toBeInTheDocument();
    expect(
      screen.queryByText("正在读取个人播客库…"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText("订阅单集，按发布时间排序。"),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("region", { name: "个人库最近更新" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("今日初筛工作区")).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "查看 默认首页最近更新" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Quick Actions" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("个人播客管理与自动化处理工具")).not.toBeInTheDocument();
    expect(screen.queryByText("我的订阅管理")).not.toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(
      "http://127.0.0.1:8080/api/v1/discovery/candidates?limit=5",
      expect.objectContaining({
        cache: "no-store",
      }),
    );
  });

  it("falls back to a structured skeleton when the server prefetch fails", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("unavailable")));

    render(await Home());

    expect(
      screen.getByRole("main", { name: "正在读取个人库最近更新" }),
    ).toHaveAttribute("aria-busy", "true");
    expect(
      screen.queryByText("暂时无法读取最近更新"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("region", { name: "个人库最近更新" }),
    ).not.toBeInTheDocument();
  });
});
