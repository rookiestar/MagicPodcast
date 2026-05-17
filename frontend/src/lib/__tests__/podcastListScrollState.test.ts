import { afterEach, describe, expect, it, vi } from "vitest";
import {
  clearPodcastListScrollSnapshot,
  getPodcastListScrollRestoreAction,
  getPodcastListStateKey,
  readPodcastListScrollSnapshot,
  restorePodcastListScroll,
  savePodcastListScrollSnapshot,
} from "../podcastListScrollState";

describe("podcastListScrollState", () => {
  afterEach(() => {
    window.sessionStorage.clear();
    vi.restoreAllMocks();
  });

  it("builds a stable list key from sort and tags", () => {
    expect(
      getPodcastListStateKey({
        sortBy: "recent_update",
        selectedTagIds: [52, 1, 52],
      }),
    ).toBe("/podcasts?sort_by=recent_update&tag_id=1&tag_id=52");
  });

  it("stores and reads a matching scroll snapshot", () => {
    const stateKey = "/podcasts?sort_by=recent_update";

    savePodcastListScrollSnapshot({
      stateKey,
      scrollY: 640,
      podcastIndex: 20,
      savedAt: 1000,
    });

    expect(readPodcastListScrollSnapshot(stateKey, { now: 1200 })).toEqual({
      stateKey,
      scrollY: 640,
      podcastIndex: 20,
      savedAt: 1000,
    });
  });

  it("ignores stale or mismatched snapshots", () => {
    savePodcastListScrollSnapshot({
      stateKey: "/podcasts?sort_by=recent_update",
      scrollY: 640,
      podcastIndex: 20,
      savedAt: 1000,
    });

    expect(readPodcastListScrollSnapshot("/podcasts?sort_by=title")).toBeNull();
    expect(
      readPodcastListScrollSnapshot("/podcasts?sort_by=recent_update", {
        now: 1000 + 31 * 60 * 1000,
      }),
    ).toBeNull();
  });

  it("decides when to load more before restoring", () => {
    const snapshot = {
      stateKey: "/podcasts",
      scrollY: 900,
      podcastIndex: 35,
      savedAt: 1000,
    };

    expect(
      getPodcastListScrollRestoreAction({
        snapshot,
        loadedCount: 20,
        hasMore: true,
        isLoadingMore: false,
      }),
    ).toBe("load-more");
    expect(
      getPodcastListScrollRestoreAction({
        snapshot,
        loadedCount: 20,
        hasMore: true,
        isLoadingMore: true,
      }),
    ).toBeNull();
    expect(
      getPodcastListScrollRestoreAction({
        snapshot,
        loadedCount: 40,
        hasMore: true,
        isLoadingMore: false,
      }),
    ).toBe("restore");
  });

  it("clears only matching snapshots when a key is provided", () => {
    const stateKey = "/podcasts";
    savePodcastListScrollSnapshot({
      stateKey,
      scrollY: 20,
      podcastIndex: 1,
      savedAt: 1000,
    });

    clearPodcastListScrollSnapshot("/podcasts?sort_by=title");
    expect(readPodcastListScrollSnapshot(stateKey, { now: 1100 })).not.toBeNull();

    clearPodcastListScrollSnapshot(stateKey);
    expect(readPodcastListScrollSnapshot(stateKey, { now: 1100 })).toBeNull();
  });

  it("restores the window scroll position on the next frame", () => {
    const scrollTo = vi.fn();
    const requestAnimationFrame = vi.fn((callback: FrameRequestCallback) => {
      callback(0);
      return 1;
    });
    Object.defineProperty(window, "scrollTo", {
      configurable: true,
      value: scrollTo,
    });
    Object.defineProperty(window, "requestAnimationFrame", {
      configurable: true,
      value: requestAnimationFrame,
    });

    restorePodcastListScroll({
      stateKey: "/podcasts",
      scrollY: 320,
      podcastIndex: 8,
      savedAt: 1000,
    });

    expect(scrollTo).toHaveBeenCalledWith(0, 320);
  });
});
