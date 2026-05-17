import { describe, expect, it } from "vitest";
import type { EpisodeSearchResult, PodcastSearchResult } from "@/types";
import {
  buildEpisodeSearchResultHref,
  buildPodcastSearchResultHref,
  getEpisodeSearchSnippet,
  getPodcastSearchSnippet,
  getSearchExpandButtonLabel,
  getSearchResultsCount,
  getSearchTextHighlightParts,
  getVisibleSearchResults,
  shouldShowEpisodeSearchResults,
  shouldShowPodcastSearchResults,
  shouldShowSearchExpandButton,
  shouldShowSearchSectionHeading,
} from "../searchResultDisplay";

function makePodcast(overrides: Partial<PodcastSearchResult> = {}): PodcastSearchResult {
  return {
    id: 1,
    title: "Podcast",
    author: "Author",
    description: "Description",
    cover_url: "",
    episode_count: 3,
    newest_episode_date: "2026-01-01T00:00:00Z",
    relevance_score: 10,
    matched_fields: [],
    ...overrides,
  };
}

function makeEpisode(overrides: Partial<EpisodeSearchResult> = {}): EpisodeSearchResult {
  return {
    id: 7,
    podcast_id: 1,
    podcast_title: "Podcast",
    podcast_cover_url: "",
    title: "Episode",
    show_notes: "Show notes",
    published_date: null,
    duration: 0,
    relevance_score: 8,
    matched_fields: [],
    ...overrides,
  };
}

describe("searchResultDisplay", () => {
  it("splits text into highlighted search parts", () => {
    expect(getSearchTextHighlightParts("Tech podcast tech", "tech")).toEqual([
      { text: "Tech", highlighted: true },
      { text: " podcast ", highlighted: false },
      { text: "tech", highlighted: true },
    ]);
  });

  it("escapes search keywords before highlighting", () => {
    expect(getSearchTextHighlightParts("C++ podcast", "C++")).toEqual([
      { text: "C++", highlighted: true },
      { text: " podcast", highlighted: false },
    ]);
  });

  it("keeps plain text when highlight keyword is empty", () => {
    expect(getSearchTextHighlightParts("Podcast", " ")).toEqual([
      { text: "Podcast", highlighted: false },
    ]);
  });

  it("counts and gates visible result sections", () => {
    expect(getSearchResultsCount({ podcastCount: 2, episodeCount: 3 })).toBe(5);
    expect(shouldShowPodcastSearchResults("all", 1)).toBe(true);
    expect(shouldShowPodcastSearchResults("episodes", 1)).toBe(false);
    expect(shouldShowEpisodeSearchResults("all", 1)).toBe(true);
    expect(shouldShowEpisodeSearchResults("podcasts", 1)).toBe(false);
    expect(shouldShowSearchSectionHeading("all", 1)).toBe(true);
    expect(shouldShowSearchSectionHeading("podcasts", 1)).toBe(false);
  });

  it("limits visible results until expanded", () => {
    expect(getVisibleSearchResults([1, 2, 3], false, 2)).toEqual([1, 2]);
    expect(getVisibleSearchResults([1, 2, 3], true, 2)).toEqual([1, 2, 3]);
    expect(shouldShowSearchExpandButton(3, 2)).toBe(true);
    expect(shouldShowSearchExpandButton(2, 2)).toBe(false);
    expect(getSearchExpandButtonLabel(false, 3, "节目")).toBe(
      "展开全部 3 个节目",
    );
    expect(getSearchExpandButtonLabel(true, 3, "节目")).toBe("收起");
  });

  it("selects podcast snippets by matched field priority", () => {
    expect(
      getPodcastSearchSnippet(
        makePodcast({
          matched_fields: [
            { field: "title", score: 1, snippet: "title match" },
            { field: "description", score: 3, snippet: "description match" },
          ],
        }),
      ),
    ).toBe("description match");

    expect(getPodcastSearchSnippet(makePodcast())).toBe("Description");
  });

  it("selects episode snippets by matched field priority", () => {
    expect(
      getEpisodeSearchSnippet(
        makeEpisode({
          matched_fields: [
            { field: "title", score: 1, snippet: "title match" },
            { field: "show_notes", score: 3, snippet: "notes match" },
          ],
        }),
      ),
    ).toBe("notes match");

    expect(getEpisodeSearchSnippet(makeEpisode())).toBe("Show notes");
  });

  it("builds search result links", () => {
    expect(buildPodcastSearchResultHref(1)).toBe("/podcasts/1");
    expect(buildEpisodeSearchResultHref(1, 7)).toBe(
      "/podcasts/1?episode_id=7",
    );
  });
});
