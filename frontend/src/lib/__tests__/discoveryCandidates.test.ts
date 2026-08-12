import { describe, expect, it, vi } from "vitest";
import {
  clearDiscoveryCandidateDetailsCache,
  DISCOVERY_CANDIDATES_CACHE_TTL_MS,
  DISCOVERY_CANDIDATES_PATH,
  fetchDiscoveryCandidateDetails,
  fetchDiscoveryCandidatesWithRetry,
  readDiscoveryCandidatesCache,
  writeDiscoveryCandidatesCache,
} from "../discoveryCandidates";

function successResponse(data: unknown) {
  return {
    ok: true,
    status: 200,
    json: vi.fn().mockResolvedValue({ success: true, data }),
  } as unknown as Response;
}

function errorResponse(status: number) {
  return {
    ok: false,
    status,
    json: vi.fn(),
  } as unknown as Response;
}

describe("fetchDiscoveryCandidatesWithRetry", () => {
  it("requests the complete rolling window as lightweight summaries", () => {
    expect(DISCOVERY_CANDIDATES_PATH).toBe(
      "/api/v1/discovery/candidates?limit=1000&view=summary",
    );
  });

  it("retries transient failures twice before returning data", async () => {
    const request = vi
      .fn()
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce(errorResponse(503))
      .mockResolvedValueOnce(successResponse([{ episode_id: 1 }]));

    await expect(
      fetchDiscoveryCandidatesWithRetry(request as typeof fetch, [0, 0]),
    ).resolves.toEqual([{ episode_id: 1 }]);

    expect(request).toHaveBeenCalledTimes(3);
    expect(request).toHaveBeenCalledWith(
      DISCOVERY_CANDIDATES_PATH,
      expect.objectContaining({
        cache: "no-store",
        headers: { Accept: "application/json" },
      }),
    );
  });

  it("stops retrying after three transient failures", async () => {
    const request = vi.fn().mockRejectedValue(new Error("offline"));

    await expect(
      fetchDiscoveryCandidatesWithRetry(request as typeof fetch, [0, 0]),
    ).rejects.toThrow("offline");
    expect(request).toHaveBeenCalledTimes(3);
  });

  it("does not retry a clear client error", async () => {
    const request = vi.fn().mockResolvedValue(errorResponse(404));

    await expect(
      fetchDiscoveryCandidatesWithRetry(request as typeof fetch, [0, 0]),
    ).rejects.toThrow("HTTP 404");
    expect(request).toHaveBeenCalledTimes(1);
  });
});

describe("fetchDiscoveryCandidateDetails", () => {
  it("deduplicates concurrent detail requests and reuses the successful result", async () => {
    clearDiscoveryCandidateDetailsCache();
    const detail = {
      episode_id: 42,
      podcast_id: 1,
      episode_title: "按需详情",
      podcast_title: "节目",
      show_notes: "<p>完整正文</p>",
      show_notes_status: "available",
      pre_reads: [],
    };
    const request = vi.fn().mockResolvedValue(successResponse(detail));

    const first = fetchDiscoveryCandidateDetails(42, request as typeof fetch);
    const second = fetchDiscoveryCandidateDetails(42, request as typeof fetch);

    await expect(Promise.all([first, second])).resolves.toEqual([detail, detail]);
    await expect(
      fetchDiscoveryCandidateDetails(42, request as typeof fetch),
    ).resolves.toEqual(detail);
    expect(request).toHaveBeenCalledTimes(1);
    expect(request).toHaveBeenCalledWith(
      "/api/v1/discovery/candidates/42",
      expect.objectContaining({
        cache: "no-store",
        headers: { Accept: "application/json" },
      }),
    );
  });

  it("does not cache a failed detail request", async () => {
    clearDiscoveryCandidateDetailsCache();
    const detail = {
      episode_id: 43,
      podcast_id: 1,
      episode_title: "重试详情",
      podcast_title: "节目",
      show_notes: "<p>重试后正文</p>",
      show_notes_status: "available",
      pre_reads: [],
    };
    const request = vi
      .fn()
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce(successResponse(detail));

    await expect(
      fetchDiscoveryCandidateDetails(43, request as typeof fetch),
    ).rejects.toThrow("offline");
    await expect(
      fetchDiscoveryCandidateDetails(43, request as typeof fetch),
    ).resolves.toEqual(detail);
    expect(request).toHaveBeenCalledTimes(2);
  });

  it("accepts missing Show Notes when the backend omits the empty field", async () => {
    clearDiscoveryCandidateDetailsCache();
    const detail = {
      episode_id: 44,
      podcast_id: 1,
      episode_title: "暂无正文",
      podcast_title: "节目",
      show_notes_status: "missing",
      pre_reads: [],
    };
    const request = vi.fn().mockResolvedValue(successResponse(detail));

    await expect(
      fetchDiscoveryCandidateDetails(44, request as typeof fetch),
    ).resolves.toEqual(detail);
  });

  it("rejects an available Show Notes response without its content", async () => {
    clearDiscoveryCandidateDetailsCache();
    const request = vi.fn().mockResolvedValue(
      successResponse({
        episode_id: 45,
        podcast_id: 1,
        episode_title: "正文缺失",
        podcast_title: "节目",
        show_notes_status: "available",
        pre_reads: [],
      }),
    );

    await expect(
      fetchDiscoveryCandidateDetails(45, request as typeof fetch),
    ).rejects.toThrow("response is invalid");
  });
});

describe("discovery candidates session cache", () => {
  function createStorage() {
    const values = new Map<string, string>();
    return {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => values.set(key, value),
      removeItem: (key: string) => values.delete(key),
    };
  }

  const cachedCandidates = [
    {
      episode_id: 1,
      podcast_id: 1,
      podcast_title: "缓存节目",
      podcast_author: "作者",
      podcast_cover_url: "",
      episode_title: "缓存单集",
      episode_no: "E1",
      duration: 1800,
      candidate_time: "2026-07-29T08:00:00+08:00",
      time_basis: "fetched_at" as const,
      source: "最近更新" as const,
      show_notes: "",
      show_notes_status: "available" as const,
      original_url: "",
      image_url: "",
      decision_state: "pending" as const,
      pre_reads: [],
    },
  ];

  it("restores non-empty content within the freshness window", () => {
    const storage = createStorage();
    writeDiscoveryCandidatesCache(storage, cachedCandidates, 1000);

    expect(readDiscoveryCandidatesCache(storage, 2000)).toEqual(
      cachedCandidates,
    );
  });

  it("does not expose expired or confirmed-empty content", () => {
    const storage = createStorage();
    writeDiscoveryCandidatesCache(storage, cachedCandidates, 1000);

    expect(
      readDiscoveryCandidatesCache(
        storage,
        1000 + DISCOVERY_CANDIDATES_CACHE_TTL_MS + 1,
      ),
    ).toBeUndefined();

    writeDiscoveryCandidatesCache(storage, [], 2000);
    expect(readDiscoveryCandidatesCache(storage, 2001)).toBeUndefined();
  });
});
