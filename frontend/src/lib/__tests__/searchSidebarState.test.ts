import { describe, expect, it } from "vitest";
import {
  createEmptySearchData,
  filterSearchResults,
  getSearchSidebarPanelState,
  normalizeSearchData,
} from "../searchSidebarState";
import type { SearchData } from "@/types";

const emptyPagination = {
  page: 1,
  page_size: 50,
  total: 0,
  total_pages: 0,
};

function makeSearchData(): SearchData {
  return {
    podcasts: [
      {
        id: 1,
        title: "科技播客",
        author: "作者",
        description: "简介",
        cover_url: "",
        episode_count: 2,
        newest_episode_date: "2026-01-01T00:00:00Z",
        relevance_score: 10,
      },
    ],
    episodes: [
      {
        id: 10,
        podcast_id: 1,
        podcast_title: "科技播客",
        podcast_cover_url: "",
        title: "科技单集",
        show_notes: "内容",
        published_date: null,
        duration: 0,
        relevance_score: 8,
      },
    ],
    pagination: {
      podcasts: emptyPagination,
      episodes: emptyPagination,
    },
  };
}

describe("searchSidebarState", () => {
  it("creates empty search data", () => {
    expect(createEmptySearchData()).toEqual({
      podcasts: [],
      episodes: [],
      pagination: null,
    });
  });

  it("normalizes missing result arrays without crashing", () => {
    expect(
      normalizeSearchData({
        pagination: null,
      } as unknown as SearchData),
    ).toEqual({
      podcasts: [],
      episodes: [],
      pagination: null,
    });
  });

  it("filters cached results by selected type", () => {
    const data = {
      ...makeSearchData(),
      pagination: null,
    };

    expect(filterSearchResults(data, "podcasts")).toEqual({
      podcasts: data.podcasts,
      episodes: [],
    });
    expect(filterSearchResults(data, "episodes")).toEqual({
      podcasts: [],
      episodes: data.episodes,
    });
  });

  it("prioritizes search sidebar panel states", () => {
    expect(
      getSearchSidebarPanelState({
        loading: true,
        isQueryTooShort: true,
        showHistory: true,
        searchError: "错误",
        hasResults: true,
      }),
    ).toBe("loading");

    expect(
      getSearchSidebarPanelState({
        loading: false,
        isQueryTooShort: true,
        showHistory: true,
        searchError: null,
        hasResults: false,
      }),
    ).toBe("history");

    expect(
      getSearchSidebarPanelState({
        loading: false,
        isQueryTooShort: true,
        showHistory: false,
        searchError: null,
        hasResults: false,
      }),
    ).toBe("prompt");

    expect(
      getSearchSidebarPanelState({
        loading: false,
        isQueryTooShort: false,
        showHistory: false,
        searchError: "错误",
        hasResults: false,
      }),
    ).toBe("error");

    expect(
      getSearchSidebarPanelState({
        loading: false,
        isQueryTooShort: false,
        showHistory: false,
        searchError: null,
        hasResults: false,
      }),
    ).toBe("empty");

    expect(
      getSearchSidebarPanelState({
        loading: false,
        isQueryTooShort: false,
        showHistory: false,
        searchError: null,
        hasResults: true,
      }),
    ).toBe("results");
  });
});
