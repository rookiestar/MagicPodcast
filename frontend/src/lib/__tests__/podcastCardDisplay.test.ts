import { describe, expect, it, vi } from "vitest";
import type { Podcast, PodcastHistorySyncSummary } from "@/types";
import {
  getPodcastCardDateStatusText,
  getPodcastCardCoverUrl,
  getPodcastCardDescription,
  getPodcastCardEpisodeCountText,
  getPodcastCardTagLimit,
  getPodcastSyncStateText,
  isPodcastRecentlyUpdated,
} from "../podcastCardDisplay";

function syncSummary(
  overrides: Partial<PodcastHistorySyncSummary>,
): PodcastHistorySyncSummary {
  return {
    task_id: 1,
    status: "pending",
    trigger: "workflow",
    processed_count: 0,
    total_known: null,
    created_count: 0,
    updated_count: 0,
    failed_count: 0,
    ...overrides,
  };
}

vi.mock("@/lib/imageProxy", () => ({
  getEffectiveCoverUrl: vi.fn((customCoverUrl?: string, coverUrl?: string) =>
    customCoverUrl || coverUrl || "",
  ),
}));

const podcast = {
  id: 1,
  xyz_id: "xyz",
  title: "Test Podcast",
  description: "",
  author: "Author",
  cover_url: "https://example.com/cover.jpg",
  custom_cover_url: "https://example.com/custom.jpg",
  episode_count: 42,
  newest_episode_date: "2026-05-15T00:00:00Z",
  created_at: "2026-01-01T00:00:00Z",
  is_subscribed: true,
  is_dead: false,
} satisfies Podcast;

describe("podcastCardDisplay", () => {
  it("strips html before showing descriptions", () => {
    expect(
      getPodcastCardDescription("<p>Hello <strong>world</strong></p>", false),
    ).toContain("Hello world");
  });

  it("uses denser tag limits on mobile", () => {
    expect(getPodcastCardTagLimit(true)).toBe(2);
    expect(getPodcastCardTagLimit(false)).toBe(3);
  });

  it("uses custom cover before regular cover", () => {
    expect(getPodcastCardCoverUrl(podcast)).toBe(
      "https://example.com/custom.jpg",
    );
  });

  it("formats episode count safely", () => {
    expect(getPodcastCardEpisodeCountText(podcast)).toBe("42 集");
    expect(
      getPodcastCardEpisodeCountText({ ...podcast, episode_count: 0 }),
    ).toBe("0 集");
  });

  it("detects recently updated podcasts", () => {
    const now = new Date("2026-05-17T00:00:00Z");

    expect(isPodcastRecentlyUpdated("2026-05-15T00:00:00Z", now)).toBe(true);
    expect(isPodcastRecentlyUpdated("2026-05-01T00:00:00Z", now)).toBe(false);
    expect(isPodcastRecentlyUpdated("bad", now)).toBe(false);
    expect(isPodcastRecentlyUpdated(undefined, now)).toBe(false);
    // 零值占位日期（Go time.Time 零值）不产生 New 标记（#463）。
    expect(isPodcastRecentlyUpdated("0001-01-01T00:00:00Z", now)).toBe(false);
    expect(isPodcastRecentlyUpdated(null, now)).toBe(false);
  });

  // #463/#462：缺失有效日期时卡片按任务状态展示，不再出现年代差文案。
  describe("getPodcastSyncStateText", () => {
    it("shows 待同步 when never synced", () => {
      expect(getPodcastSyncStateText({ history_sync: null, episode_count: 0 })).toBe(
        "待同步",
      );
      expect(getPodcastSyncStateText({ episode_count: 0 })).toBe("待同步");
    });

    it("distinguishes queued, running, partial and failed states", () => {
      const base = { episode_count: 0 };
      expect(
        getPodcastSyncStateText({
          ...base,
          history_sync: syncSummary({ status: "queued" }),
        }),
      ).toBe("排队中");
      expect(
        getPodcastSyncStateText({
          ...base,
          history_sync: syncSummary({ status: "running" }),
        }),
      ).toBe("同步中");
      expect(
        getPodcastSyncStateText({
          ...base,
          history_sync: syncSummary({ status: "partial" }),
        }),
      ).toBe("部分同步");
      expect(
        getPodcastSyncStateText({
          ...base,
          history_sync: syncSummary({ status: "failed" }),
        }),
      ).toBe("同步未完成");
    });

    it("distinguishes real empty source from unsynced after completion", () => {
      const base = { episode_count: 0 };
      expect(
        getPodcastSyncStateText({
          ...base,
          history_sync: syncSummary({ status: "completed" }),
        }),
      ).toBe("暂无可获取单集");
      expect(
        getPodcastSyncStateText({
          ...base,
          episode_count: 5,
          history_sync: syncSummary({ status: "completed" }),
        }),
      ).toBe("日期未知");
    });
  });

  describe("getPodcastCardDateStatusText", () => {
    it("keeps relative time for valid dates", () => {
      expect(
        getPodcastCardDateStatusText({
          ...podcast,
          newest_episode_date: new Date().toISOString(),
        }),
      ).toBe("刚刚");
    });

    it("never renders year-one dates as ages (#463)", () => {
      const text = getPodcastCardDateStatusText({
        ...podcast,
        newest_episode_date: "0001-01-01T00:00:00Z",
        history_sync: null,
      });
      expect(text).toBe("待同步");
      expect(text).not.toContain("年前");
    });
  });
});
