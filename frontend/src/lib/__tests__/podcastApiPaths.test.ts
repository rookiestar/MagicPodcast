import { describe, expect, it } from "vitest";
import {
  buildPodcastBatchPath,
  buildPodcastCustomCoverPath,
  buildPodcastDetailPath,
  buildPodcastEpisodesCollectionPath,
  buildPodcastEpisodesPath,
  buildPodcastListPath,
  buildPodcastNotesPath,
  buildPodcastTagPath,
  buildPodcastTagsPath,
} from "../podcastApiPaths";

describe("podcastApiPaths", () => {
  it("builds podcast list paths with stable query ordering", () => {
    expect(buildPodcastListPath()).toBe("/api/v1/podcasts");
    expect(
      buildPodcastListPath({
        page: 2,
        page_size: 20,
        sort_by: "recent_update",
        search: "tech",
        tag_id: [1, 52],
      }),
    ).toBe(
      "/api/v1/podcasts?page=2&page_size=20&sort_by=recent_update&search=tech&tag_id=1&tag_id=52",
    );
  });

  it("builds podcast detail paths", () => {
    expect(buildPodcastDetailPath(7)).toBe("/api/v1/podcasts/7");
    expect(buildPodcastBatchPath()).toBe("/api/v1/podcasts/batch");
    expect(buildPodcastNotesPath(7)).toBe("/api/v1/podcasts/7/notes");
    expect(buildPodcastTagsPath(7)).toBe("/api/v1/podcasts/7/tags");
    expect(buildPodcastTagPath(7, 3)).toBe("/api/v1/podcasts/7/tags/3");
    expect(buildPodcastCustomCoverPath(7)).toBe(
      "/api/v1/podcasts/7/custom-cover",
    );
  });

  it("builds podcast episode paths", () => {
    expect(buildPodcastEpisodesPath(7, 2, 20)).toBe(
      "/api/v1/podcasts/7/episodes?page=2&page_size=20",
    );
    expect(buildPodcastEpisodesCollectionPath(7)).toBe(
      "/api/v1/podcasts/7/episodes",
    );
  });
});
