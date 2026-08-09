import { describe, expect, it, vi } from "vitest";
import {
  DISCOVERY_CANDIDATES_CACHE_TTL_MS,
  DISCOVERY_CANDIDATES_PATH,
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
      time_basis: "published_date" as const,
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
