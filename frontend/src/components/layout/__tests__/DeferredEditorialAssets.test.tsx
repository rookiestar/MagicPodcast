import { act, fireEvent, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import DeferredEditorialAssets, {
  EDITORIAL_ASSETS_READY_CLASS,
  EDITORIAL_TYPOGRAPHY_READY_CLASS,
} from "../DeferredEditorialAssets";

describe("DeferredEditorialAssets", () => {
  let idleCallback: IdleRequestCallback | undefined;
  let originalFonts: FontFaceSet | undefined;
  let hadFontsProperty = false;
  let fontLoad: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    idleCallback = undefined;
    hadFontsProperty = "fonts" in document;
    originalFonts = document.fonts;
    fontLoad = vi.fn().mockResolvedValue([]);
    Object.defineProperty(document, "fonts", {
      configurable: true,
      value: { load: fontLoad },
    });
    document.documentElement.classList.remove(EDITORIAL_ASSETS_READY_CLASS);
    document.documentElement.classList.remove(
      EDITORIAL_TYPOGRAPHY_READY_CLASS,
    );
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
    vi.useRealTimers();
    document.documentElement.classList.remove(EDITORIAL_ASSETS_READY_CLASS);
    document.documentElement.classList.remove(
      EDITORIAL_TYPOGRAPHY_READY_CLASS,
    );
    document.documentElement.style.removeProperty(
      "--editorial-paper-texture",
    );
    if (hadFontsProperty) {
      Object.defineProperty(document, "fonts", {
        configurable: true,
        value: originalFonts,
      });
    } else {
      Reflect.deleteProperty(document, "fonts");
    }
    vi.unstubAllGlobals();
  });

  function triggerIdle() {
    act(() => {
      idleCallback?.({ didTimeout: false, timeRemaining: () => 10 });
    });
  }

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

    triggerIdle();
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

  it("enables editorial typography only after all rendered heading fonts settle", async () => {
    const heading = document.createElement("h3");
    heading.textContent = "具身智能 Alpha 的金钱游戏：进展难测";
    document.body.append(heading);

    const fontResolvers: Array<() => void> = [];
    fontLoad.mockImplementation(
      () =>
        new Promise((resolve) => {
          fontResolvers.push(() => resolve([]));
        }),
    );

    render(<DeferredEditorialAssets />);
    triggerIdle();

    expect(fontLoad).toHaveBeenCalled();
    expect(
      document.documentElement.classList.contains(
        EDITORIAL_ASSETS_READY_CLASS,
      ),
    ).toBe(true);
    expect(
      document.documentElement.classList.contains(
        EDITORIAL_TYPOGRAPHY_READY_CLASS,
      ),
    ).toBe(false);

    await act(async () => {
      fontResolvers[0]?.();
    });
    expect(
      document.documentElement.classList.contains(
        EDITORIAL_TYPOGRAPHY_READY_CLASS,
      ),
    ).toBe(false);

    await act(async () => {
      fontResolvers[1]?.();
    });

    expect(
      document.documentElement.classList.contains(
        EDITORIAL_TYPOGRAPHY_READY_CLASS,
      ),
    ).toBe(true);
  });

  it("includes semantic editorial titles that are not heading elements", async () => {
    const title = document.createElement("strong");
    title.dataset.editorialDisplayText = "true";
    title.textContent = "候选 Podcast 单集标题";
    document.body.append(title);

    render(<DeferredEditorialAssets />);
    triggerIdle();
    await act(async () => {
      await Promise.resolve();
    });

    expect(fontLoad).toHaveBeenCalledTimes(2);
    expect(new Set(fontLoad.mock.calls.map(([, text]) => text))).toEqual(
      new Set(["候选单集标题", "Podcast"]),
    );
    expect(
      document.documentElement.classList.contains(
        EDITORIAL_TYPOGRAPHY_READY_CLASS,
      ),
    ).toBe(true);
  });

  it("keeps the system fallback when an editorial font fails", async () => {
    const heading = document.createElement("h3");
    heading.textContent = "具身智能 Alpha 的金钱游戏：进展难测";
    document.body.append(heading);
    fontLoad.mockRejectedValue(new Error("font request failed"));

    render(<DeferredEditorialAssets />);
    triggerIdle();
    await act(async () => {
      await Promise.resolve();
    });

    expect(
      document.documentElement.classList.contains(
        EDITORIAL_ASSETS_READY_CLASS,
      ),
    ).toBe(true);
    expect(
      document.documentElement.classList.contains(
        EDITORIAL_TYPOGRAPHY_READY_CLASS,
      ),
    ).toBe(false);

    const callsAfterFailure = fontLoad.mock.calls.length;
    heading.textContent = "失败后出现的新标题";
    await act(async () => {
      await Promise.resolve();
    });
    expect(fontLoad).toHaveBeenCalledTimes(callsAfterFailure);
  });

  it("shares one bounded font wait across rapid content changes", async () => {
    vi.useFakeTimers();
    const heading = document.createElement("h3");
    heading.textContent = "具身智能 Alpha 的金钱游戏：进展难测";
    document.body.append(heading);
    fontLoad.mockImplementation(() => new Promise(() => undefined));

    render(<DeferredEditorialAssets />);
    triggerIdle();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(4_000);
    });
    heading.textContent = "四秒后 Beta 出现的新标题";
    await act(async () => {
      await Promise.resolve();
      await vi.advanceTimersByTimeAsync(1_000);
    });

    expect(
      document.documentElement.classList.contains(
        EDITORIAL_TYPOGRAPHY_READY_CLASS,
      ),
    ).toBe(false);

    expect(fontLoad).toHaveBeenCalledTimes(4);
    const callsAfterTimeout = fontLoad.mock.calls.length;
    heading.textContent = "超时后出现的新标题";
    await act(async () => {
      await Promise.resolve();
    });
    expect(fontLoad).toHaveBeenCalledTimes(callsAfterTimeout);
  });

  it("does not retry a failed font request for unrelated DOM changes", async () => {
    const heading = document.createElement("h3");
    heading.textContent = "具身智能的金钱游戏：进展难测";
    document.body.append(heading);
    fontLoad.mockRejectedValue(new Error("font request failed"));

    render(<DeferredEditorialAssets />);
    triggerIdle();
    await act(async () => {
      await Promise.resolve();
    });
    const callsAfterFailure = fontLoad.mock.calls.length;

    document.body.append(document.createElement("div"));
    await act(async () => {
      await Promise.resolve();
    });

    expect(fontLoad.mock.calls.length).toBe(callsAfterFailure);
  });

  it("keeps podcast Chinese text off WenKai while restoring Newsreader", async () => {
    const library = document.createElement("section");
    library.className = "podcast-library-shell";
    const pageTitle = document.createElement("h1");
    pageTitle.textContent = "Podcasts 播客库";
    pageTitle.dataset.editorialDisplayText = "true";
    const cardTitle = document.createElement("h3");
    cardTitle.textContent = "Dynamic 动态节目标题";
    library.append(pageTitle, cardTitle);
    document.body.append(library);

    render(<DeferredEditorialAssets />);
    triggerIdle();
    await act(async () => {
      await Promise.resolve();
    });

    expect(fontLoad).toHaveBeenCalledTimes(1);
    expect(fontLoad.mock.calls[0]?.[0]).toContain("Newsreader Variable");
    expect(fontLoad.mock.calls[0]?.[0]).not.toContain("LXGW WenKai Screen");
    expect(fontLoad.mock.calls[0]?.[1]).toBe("Podcasts");
    expect(
      document.documentElement.classList.contains(
        EDITORIAL_TYPOGRAPHY_READY_CLASS,
      ),
    ).toBe(true);
  });

  it("prepares rendered hidden headings for later viewport changes", async () => {
    const hiddenSection = document.createElement("section");
    hiddenSection.style.display = "none";
    const heading = document.createElement("h3");
    heading.textContent = "折叠 Report 标题";
    hiddenSection.append(heading);
    document.body.append(hiddenSection);

    render(<DeferredEditorialAssets />);
    triggerIdle();
    await act(async () => {
      await Promise.resolve();
    });
    expect(fontLoad).toHaveBeenCalledTimes(2);
    expect(
      document.documentElement.classList.contains(
        EDITORIAL_TYPOGRAPHY_READY_CLASS,
      ),
    ).toBe(true);

    const callsBeforeReveal = fontLoad.mock.calls.length;
    hiddenSection.style.display = "block";
    await act(async () => {
      await Promise.resolve();
    });

    expect(fontLoad).toHaveBeenCalledTimes(callsBeforeReveal);
  });

  it("rechecks new rendered headings before restoring editorial typography", async () => {
    const firstHeading = document.createElement("h3");
    firstHeading.textContent = "第一份报告";
    document.body.append(firstHeading);

    render(<DeferredEditorialAssets />);
    triggerIdle();
    await act(async () => {
      await Promise.resolve();
    });
    expect(
      document.documentElement.classList.contains(
        EDITORIAL_TYPOGRAPHY_READY_CLASS,
      ),
    ).toBe(true);

    const callsBeforeNewHeading = fontLoad.mock.calls.length;
    const secondHeading = document.createElement("h3");
    secondHeading.textContent = "新增的报告字形";
    document.body.append(secondHeading);

    await act(async () => {
      await Promise.resolve();
    });

    expect(fontLoad.mock.calls.length).toBeGreaterThan(callsBeforeNewHeading);
    expect(
      document.documentElement.classList.contains(
        EDITORIAL_TYPOGRAPHY_READY_CLASS,
      ),
    ).toBe(true);
  });

  it("reuses each in-flight font text and ignores a stale generation", async () => {
    const heading = document.createElement("h3");
    heading.textContent = "第一份报告 Alpha";
    document.body.append(heading);

    const fontResolvers = new Map<string, Array<() => void>>();
    fontLoad.mockImplementation(
      (_font: string, text: string) =>
        new Promise((resolve) => {
          const resolvers = fontResolvers.get(text) ?? [];
          resolvers.push(() => resolve([]));
          fontResolvers.set(text, resolvers);
        }),
    );

    render(<DeferredEditorialAssets />);
    triggerIdle();

    heading.textContent = "第二份报告 Alpha";
    await act(async () => {
      await Promise.resolve();
    });
    heading.textContent = "第一份报告 Alpha";
    await act(async () => {
      await Promise.resolve();
    });

    expect(fontLoad).toHaveBeenCalledTimes(3);

    await act(async () => {
      fontResolvers.get("第二份报告")?.forEach((resolve) => resolve());
      fontResolvers.get("Alpha")?.forEach((resolve) => resolve());
    });
    expect(
      document.documentElement.classList.contains(
        EDITORIAL_TYPOGRAPHY_READY_CLASS,
      ),
    ).toBe(false);

    await act(async () => {
      fontResolvers.get("第一份报告")?.forEach((resolve) => resolve());
    });
    expect(
      document.documentElement.classList.contains(
        EDITORIAL_TYPOGRAPHY_READY_CLASS,
      ),
    ).toBe(true);
  });

  it("keeps the fallback when the Font Loading API is unavailable", async () => {
    const heading = document.createElement("h3");
    heading.textContent = "无字体 API";
    document.body.append(heading);
    Object.defineProperty(document, "fonts", {
      configurable: true,
      value: undefined,
    });

    render(<DeferredEditorialAssets />);
    triggerIdle();
    await act(async () => {
      await Promise.resolve();
    });

    expect(
      document.documentElement.classList.contains(
        EDITORIAL_TYPOGRAPHY_READY_CLASS,
      ),
    ).toBe(false);
  });

  it("ignores font results after unmount", async () => {
    const heading = document.createElement("h3");
    heading.textContent = "卸载中的报告";
    document.body.append(heading);
    const fontResolvers: Array<() => void> = [];
    fontLoad.mockImplementation(
      () =>
        new Promise((resolve) => {
          fontResolvers.push(() => resolve([]));
        }),
    );

    const { unmount } = render(<DeferredEditorialAssets />);
    triggerIdle();
    unmount();
    await act(async () => {
      fontResolvers.forEach((resolve) => resolve());
    });

    expect(
      document.documentElement.classList.contains(
        EDITORIAL_TYPOGRAPHY_READY_CLASS,
      ),
    ).toBe(false);
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
