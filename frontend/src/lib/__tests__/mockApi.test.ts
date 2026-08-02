import { describe, expect, it } from "vitest";
import { handleMockRequest } from "@/lib/mockApi";

describe("local mock API", () => {
  it("returns paginated podcasts for the library page", async () => {
    const response = await handleMockRequest({
      method: "GET",
      pathname: "/api/v1/podcasts",
      search: "?page=1&page_size=2&sort_by=recent_update&view=summary",
    });

    expect(response.status).toBe(200);
    expect(response.body.success).toBe(true);
    expect(response.body.data).toHaveLength(2);
    expect(response.body.pagination).toEqual({
      page: 1,
      page_size: 2,
      total: 4,
      total_pages: 2,
    });
  });

  it("uses distinct editorial cover images in the local library demo", async () => {
    const response = await handleMockRequest({
      method: "GET",
      pathname: "/api/v1/podcasts",
      search: "?page=1&page_size=10&sort_by=recent_update&view=summary",
    });
    const coverUrls = response.body.data.map(
      (podcast: { cover_url: string }) => podcast.cover_url,
    );

    expect(new Set(coverUrls).size).toBe(4);
    expect(
      coverUrls.every((coverUrl: string) =>
        coverUrl.startsWith("/api/mock-cover/"),
      ),
    ).toBe(true);
  });

  it("returns discovery candidates with all four pre-reads", async () => {
    const response = await handleMockRequest({
      method: "GET",
      pathname: "/api/v1/discovery/candidates",
      search: "?limit=1",
    });

    expect(response.status).toBe(200);
    expect(response.body.data).toHaveLength(1);
    expect(response.body.data[0].pre_reads).toHaveLength(4);
    expect(response.body.data[0].decision_state).toBe("pending");
  });

  it("persists a shortlist decision for the today view", async () => {
    const update = await handleMockRequest({
      method: "PUT",
      pathname: "/api/v1/discovery/candidates/101/decision",
      body: { state: "shortlisted" },
    });
    const today = await handleMockRequest({
      method: "GET",
      pathname: "/api/v1/discovery/shortlist/today",
    });

    expect(update.status).toBe(200);
    expect(update.body.data.state).toBe("shortlisted");
    expect(today.body.data.candidates.some((item) => item.episode_id === 101)).toBe(true);
  });

  it("returns a clear 404 for an unsupported mock endpoint", async () => {
    const response = await handleMockRequest({
      method: "GET",
      pathname: "/api/v1/not-supported",
    });

    expect(response.status).toBe(404);
    expect(response.body.success).toBe(false);
  });
});
