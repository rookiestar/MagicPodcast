import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import PodcastsPage from "../page";

vi.mock("../PodcastsContent", () => ({
  default: ({
    initialPage,
  }: {
    initialPage?: {
      podcasts: Array<{ id: number }>;
      pagination: { page_size: number; total: number };
    };
  }) => (
    <div
      data-testid="podcasts-content"
      data-initial-count={initialPage?.podcasts.length ?? 0}
      data-page-size={initialPage?.pagination.page_size ?? 0}
      data-total={initialPage?.pagination.total ?? 0}
    />
  ),
}));

function podcast(id: number) {
  return {
    id,
    xyz_id: `xyz-${id}`,
    title: `播客 ${id}`,
    description: "",
    author: "作者",
    cover_url: "",
    episode_count: 1,
    newest_episode_date: "2026-08-09T00:00:00Z",
    created_at: "2026-08-09T00:00:00Z",
    is_subscribed: true,
    is_dead: false,
  };
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("podcasts server prefetch", () => {
  it("passes the first 10 podcasts to the client component", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: Array.from({ length: 10 }, (_, index) => podcast(index + 1)),
          pagination: {
            page: 1,
            page_size: 10,
            total: 26,
            total_pages: 3,
          },
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    render(await PodcastsPage());

    expect(screen.getByTestId("podcasts-content")).toHaveAttribute(
      "data-initial-count",
      "10",
    );
    expect(screen.getByTestId("podcasts-content")).toHaveAttribute(
      "data-page-size",
      "10",
    );
    expect(screen.getByTestId("podcasts-content")).toHaveAttribute(
      "data-total",
      "26",
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "http://127.0.0.1:8080/api/v1/podcasts?page=1&page_size=10&sort_by=recent_update&view=summary",
      expect.objectContaining({
        cache: "no-store",
        headers: { Accept: "application/json" },
      }),
    );
  });

  it("bounds the server prefetch at 2.5 seconds and falls back to the client", async () => {
    const timeoutSignal = new AbortController().signal;
    const timeoutSpy = vi
      .spyOn(AbortSignal, "timeout")
      .mockReturnValue(timeoutSignal);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockRejectedValue(new DOMException("timed out", "TimeoutError")),
    );

    render(await PodcastsPage());

    expect(timeoutSpy).toHaveBeenCalledWith(2_500);
    expect(screen.getByTestId("podcasts-content")).toHaveAttribute(
      "data-initial-count",
      "0",
    );
  });

  it("does not serialize an invalid or failed response", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({ success: false, error: { message: "失败" } }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      ),
    );

    render(await PodcastsPage());

    expect(screen.getByTestId("podcasts-content")).toHaveAttribute(
      "data-initial-count",
      "0",
    );
  });
});
