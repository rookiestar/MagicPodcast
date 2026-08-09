import { act, fireEvent, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import DeferredEditorialAssets, {
  EDITORIAL_ASSETS_READY_CLASS,
} from "../DeferredEditorialAssets";

describe("DeferredEditorialAssets", () => {
  let idleCallback: IdleRequestCallback | undefined;

  beforeEach(() => {
    idleCallback = undefined;
    document.documentElement.classList.remove(EDITORIAL_ASSETS_READY_CLASS);
    document.documentElement.style.removeProperty(
      "--editorial-paper-texture",
    );
    document.body.replaceChildren();
    vi.stubGlobal(
      "requestIdleCallback",
      vi.fn((callback: IdleRequestCallback) => {
        idleCallback = callback;
        return 1;
      }),
    );
    vi.stubGlobal("cancelIdleCallback", vi.fn());
  });

  afterEach(() => {
    document.documentElement.classList.remove(EDITORIAL_ASSETS_READY_CLASS);
    document.documentElement.style.removeProperty(
      "--editorial-paper-texture",
    );
    vi.unstubAllGlobals();
  });

  it("waits for server-rendered critical covers before scheduling assets", () => {
    const cover = document.createElement("div");
    cover.dataset.podcastCoverCritical = "true";
    const image = document.createElement("img");
    Object.defineProperty(image, "complete", {
      configurable: true,
      value: false,
    });
    cover.append(image);
    document.body.append(cover);

    render(<DeferredEditorialAssets />);

    expect(requestIdleCallback).not.toHaveBeenCalled();
    expect(
      document.documentElement.classList.contains(
        EDITORIAL_ASSETS_READY_CLASS,
      ),
    ).toBe(false);

    fireEvent.load(image);
    expect(requestIdleCallback).toHaveBeenCalledTimes(1);

    act(() => {
      idleCallback?.({ didTimeout: false, timeRemaining: () => 10 });
    });
    expect(
      document.documentElement.classList.contains(
        EDITORIAL_ASSETS_READY_CLASS,
      ),
    ).toBe(true);
    expect(
      document.documentElement.style.getPropertyValue(
        "--editorial-paper-texture",
      ),
    ).toContain("url(");
  });

  it("uses the idle path immediately when critical covers are already cached", () => {
    const cover = document.createElement("div");
    cover.dataset.podcastCoverCritical = "true";
    const image = document.createElement("img");
    Object.defineProperty(image, "complete", {
      configurable: true,
      value: true,
    });
    Object.defineProperty(image, "naturalWidth", {
      configurable: true,
      value: 256,
    });
    cover.append(image);
    document.body.append(cover);

    render(<DeferredEditorialAssets />);

    expect(requestIdleCallback).toHaveBeenCalledTimes(1);
  });

  it("treats a critical cover that failed before hydration as settled", () => {
    const cover = document.createElement("div");
    cover.dataset.podcastCoverCritical = "true";
    const image = document.createElement("img");
    Object.defineProperty(image, "complete", {
      configurable: true,
      value: true,
    });
    Object.defineProperty(image, "naturalWidth", {
      configurable: true,
      value: 0,
    });
    cover.append(image);
    document.body.append(cover);

    render(<DeferredEditorialAssets />);

    expect(requestIdleCallback).toHaveBeenCalledTimes(1);
  });

  it("releases deferred assets after an in-flight critical cover fails", () => {
    const cover = document.createElement("div");
    cover.dataset.podcastCoverCritical = "true";
    const image = document.createElement("img");
    Object.defineProperty(image, "complete", {
      configurable: true,
      value: false,
    });
    cover.append(image);
    document.body.append(cover);

    render(<DeferredEditorialAssets />);
    expect(requestIdleCallback).not.toHaveBeenCalled();

    fireEvent.error(image);

    expect(requestIdleCallback).toHaveBeenCalledTimes(1);
  });

  it("does not hold non-podcast editorial pages behind a missing cover", () => {
    render(<DeferredEditorialAssets />);

    expect(requestIdleCallback).toHaveBeenCalledTimes(1);
  });
});
