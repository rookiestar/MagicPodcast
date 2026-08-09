import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import DiscoveryPageClient from "../DiscoveryPageClient";
import { writeDiscoveryCandidatesCache } from "@/lib/discoveryCandidates";
import type { DiscoveryCandidate } from "@/types/discovery";

const useSWRMock = vi.hoisted(() => vi.fn());

vi.mock("swr", () => ({
  default: useSWRMock,
}));

vi.mock("@/components/layout/PageLayout", () => ({
  SimplePageLayout: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

vi.mock("@/components/discovery/DiscoveryDesk", () => ({
  default: ({ candidates }: { candidates: DiscoveryCandidate[] }) => (
    <main aria-label="个人库最近更新">
      {candidates.map((candidate) => (
        <article key={candidate.episode_id}>{candidate.episode_title}</article>
      ))}
    </main>
  ),
}));

const candidates: DiscoveryCandidate[] = [
  {
    episode_id: 1,
    podcast_id: 1,
    podcast_title: "默认首页节目",
    podcast_author: "作者",
    podcast_cover_url: "",
    episode_title: "保留的最近更新",
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
    pre_reads: [],
  },
];

describe("DiscoveryPageClient", () => {
  const mutate = vi.fn();

  beforeEach(() => {
    mutate.mockReset();
    useSWRMock.mockReset();
    window.sessionStorage.clear();
  });

  it("shows a structured skeleton while the first client request is retrying", () => {
    useSWRMock.mockReturnValue({
      data: undefined,
      error: new Error("retrying"),
      isLoading: true,
      isValidating: true,
      mutate,
    });

    render(<DiscoveryPageClient />);

    expect(
      screen.getByRole("main", { name: "正在读取个人库最近更新" }),
    ).toHaveAttribute("aria-busy", "true");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(useSWRMock).toHaveBeenCalledWith(
      "/api/v1/discovery/candidates?limit=30",
      expect.any(Function),
      expect.objectContaining({
        keepPreviousData: true,
        shouldRetryOnError: false,
      }),
    );
  });

  it("keeps existing content visible while refreshing in the background", () => {
    useSWRMock.mockReturnValue({
      data: candidates,
      error: null,
      isLoading: false,
      isValidating: true,
      mutate,
    });

    render(<DiscoveryPageClient initialCandidates={candidates} />);

    expect(screen.getByText("保留的最近更新")).toBeInTheDocument();
    expect(screen.getByText("正在后台更新最近内容…")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("restores recent session content while a reload keeps retrying", async () => {
    writeDiscoveryCandidatesCache(window.sessionStorage, candidates);
    useSWRMock.mockReturnValue({
      data: undefined,
      error: null,
      isLoading: true,
      isValidating: true,
      mutate,
    });

    render(<DiscoveryPageClient />);

    expect(await screen.findByText("保留的最近更新")).toBeInTheDocument();
    expect(
      screen.getByText("正在后台更新，当前显示上次加载结果…"),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("main", { name: "正在读取个人库最近更新" }),
    ).not.toBeInTheDocument();
  });

  it("keeps stale content and offers retry after background attempts fail", () => {
    useSWRMock.mockReturnValue({
      data: candidates,
      error: new Error("unavailable"),
      isLoading: false,
      isValidating: false,
      mutate,
    });

    render(<DiscoveryPageClient initialCandidates={candidates} />);

    expect(screen.getByText("保留的最近更新")).toBeInTheDocument();
    expect(
      screen.getByText("最近更新暂时无法刷新，当前显示上次加载结果。"),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "重新尝试" }));
    expect(mutate).toHaveBeenCalledTimes(1);
  });

  it("keeps the skeleton and only reports failure after retries are exhausted", () => {
    useSWRMock.mockReturnValue({
      data: undefined,
      error: new Error("unavailable"),
      isLoading: false,
      isValidating: false,
      mutate,
    });

    render(<DiscoveryPageClient />);

    expect(
      screen.getByRole("main", { name: "正在读取个人库最近更新" }),
    ).toHaveAttribute("aria-busy", "false");
    expect(screen.getByRole("alert")).toHaveTextContent(
      "最近更新暂时无法读取",
    );

    fireEvent.click(screen.getByRole("button", { name: "重新尝试" }));
    expect(mutate).toHaveBeenCalledTimes(1);
  });
});
