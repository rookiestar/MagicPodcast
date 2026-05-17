import { describe, expect, it } from "vitest";
import {
  applyEpisodePage,
  canLoadMoreEpisodes,
  mergeUniqueEpisodes,
} from "../podcastEpisodePagination";
import type { Episode } from "@/types";

function makeEpisode(id: number): Episode {
  return {
    id,
    guid: `episode-${id}`,
    podcast_id: 1,
    episode_no: "",
    title: `Episode ${id}`,
    medium_url: "",
    show_notes: "",
    published_date: "2026-01-01T00:00:00Z",
    duration: 0,
    link: "",
    image_url: "",
    enclosure_type: "",
    enclosure_length: 0,
    my_rate: 0,
    notes: "",
  };
}

describe("podcastEpisodePagination", () => {
  it("deduplicates incoming episodes while preserving order", () => {
    const merged = mergeUniqueEpisodes(
      [makeEpisode(1), makeEpisode(2)],
      [makeEpisode(2), makeEpisode(3)],
    );

    expect(merged.map((episode) => episode.id)).toEqual([1, 2, 3]);
  });

  it("replaces episodes for a fresh page", () => {
    const next = applyEpisodePage({
      existingEpisodes: [makeEpisode(1)],
      incomingEpisodes: [makeEpisode(2)],
      append: false,
    });

    expect(next.map((episode) => episode.id)).toEqual([2]);
  });

  it("only allows loading more after an initial page is ready", () => {
    expect(
      canLoadMoreEpisodes({
        episodeCount: 1,
        episodesLoading: false,
        isLoadingMore: false,
        hasMoreEpisodes: true,
      }),
    ).toBe(true);

    expect(
      canLoadMoreEpisodes({
        episodeCount: 0,
        episodesLoading: false,
        isLoadingMore: false,
        hasMoreEpisodes: true,
      }),
    ).toBe(false);
  });
});
