import { describe, expect, it, vi } from "vitest";
import type { Podcast } from "@/types";
import {
  getPodcastCardCoverUrl,
  getPodcastCardDescription,
  getPodcastCardEpisodeCountText,
  getPodcastCardTagLimit,
  isPodcastRecentlyUpdated,
} from "../podcastCardDisplay";

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
  });
});
