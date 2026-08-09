import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { podcastCoverLoadQueue } from "../podcastCoverLoadQueue";

describe("podcastCoverLoadQueue", () => {
  beforeEach(() => {
    podcastCoverLoadQueue.clear();
  });

  afterEach(() => {
    podcastCoverLoadQueue.clear();
  });

  it("starts visible covers before queued offscreen preloads", () => {
    const starts: string[] = [];

    podcastCoverLoadQueue.request({
      src: "offscreen-1",
      priority: "low",
      onStart: () => starts.push("offscreen-1"),
    });
    podcastCoverLoadQueue.request({
      src: "offscreen-2",
      priority: "low",
      onStart: () => starts.push("offscreen-2"),
    });

    expect(starts).toEqual(["offscreen-1"]);

    podcastCoverLoadQueue.request({
      src: "visible",
      priority: "medium",
      onStart: () => starts.push("visible"),
    });

    expect(starts).toEqual(["offscreen-1", "visible"]);

    podcastCoverLoadQueue.complete("offscreen-1");
    expect(starts).toEqual(["offscreen-1", "visible", "offscreen-2"]);
  });

  it("promotes a queued cover when it enters the viewport", () => {
    const starts: string[] = [];

    podcastCoverLoadQueue.request({
      src: "offscreen-1",
      priority: "low",
      onStart: () => starts.push("offscreen-1"),
    });
    const offscreenHandle = podcastCoverLoadQueue.request({
      src: "offscreen-2",
      priority: "low",
      onStart: () => starts.push("offscreen-2"),
    });

    offscreenHandle.updatePriority("high");
    podcastCoverLoadQueue.complete("offscreen-1");

    expect(starts).toEqual(["offscreen-1", "offscreen-2"]);
  });

  it("does not start a second request after a successful remount", () => {
    const starts = vi.fn();

    const firstMount = podcastCoverLoadQueue.request({
      src: "cached-cover",
      priority: "high",
      onStart: starts,
    });
    podcastCoverLoadQueue.complete("cached-cover");
    firstMount.release();

    const secondMount = podcastCoverLoadQueue.request({
      src: "cached-cover",
      priority: "medium",
      onStart: starts,
    });

    expect(starts).toHaveBeenCalledTimes(2);
    expect(podcastCoverLoadQueue.getStatus()).toMatchObject({
      active: 0,
      queued: 0,
      loaded: 1,
    });

    secondMount.release();
  });
});
