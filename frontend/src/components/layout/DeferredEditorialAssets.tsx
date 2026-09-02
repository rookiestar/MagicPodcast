"use client";

import { useEffect } from "react";
import paperTexture from "@/assets/warm-paper-grid-texture-v1.jpg";

export const EDITORIAL_ASSETS_READY_CLASS = "editorial-assets-ready";
export const EDITORIAL_TYPOGRAPHY_READY_CLASS =
  "editorial-typography-ready";

const CRITICAL_COVER_SELECTOR =
  '[data-podcast-cover-critical="true"] img';
const EDITORIAL_DISPLAY_SELECTOR =
  "h1, h2, h3, h4, .editorial-rich-text h5, .editorial-rich-text h6, " +
  '[data-editorial-display-text="true"]';
const ASSET_IDLE_TIMEOUT_MS = 1_500;
const CRITICAL_COVER_MAX_WAIT_MS = 15_000;
const EDITORIAL_FONT_MAX_WAIT_MS = 5_000;
const CJK_DISPLAY_FONT = "LXGW WenKai Screen";
const LATIN_DISPLAY_FONT = "Newsreader Variable";
const CJK_DISPLAY_CHARACTER_PATTERN =
  /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Hangul}\p{Script=Bopomofo}\u3000-\u303f\uff01-\uff60\uffe0-\uffe6]/gu;
const paperTextureUrl =
  typeof paperTexture === "string" ? paperTexture : paperTexture.src;
type TypographyStatus = "idle" | "pending" | "ready" | "failed";

function scheduleIdle(callback: () => void) {
  if (typeof window.requestIdleCallback === "function") {
    const idleId = window.requestIdleCallback(callback, {
      timeout: ASSET_IDLE_TIMEOUT_MS,
    });
    return () => window.cancelIdleCallback?.(idleId);
  }

  const timerId = window.setTimeout(callback, 0);
  return () => window.clearTimeout(timerId);
}

function readEditorialDisplayText() {
  const elements = Array.from(
    document.querySelectorAll<HTMLElement>(EDITORIAL_DISPLAY_SELECTOR),
  );
  const readText = (selectedElements: HTMLElement[]) =>
    selectedElements
      .map((element) => element.textContent?.trim() ?? "")
      .filter(Boolean)
      .join("\n");

  const displayElements = elements.filter(
    (element) =>
      !element.closest(".podcast-library-shell") ||
      element.dataset.editorialDisplayText === "true",
  );
  const displayText = readText(displayElements);
  const cjkSourceText = readText(
    displayElements.filter(
      (element) => !element.closest(".podcast-library-shell"),
    ),
  );

  return {
    cjkText:
      cjkSourceText.match(CJK_DISPLAY_CHARACTER_PATTERN)?.join("") ?? "",
    latinText: displayText
      .replace(CJK_DISPLAY_CHARACTER_PATTERN, "")
      .trim(),
  };
}

function waitForEditorialFontsWithTimeout(
  fontLoad: Promise<boolean>,
  timeoutMs: number,
) {
  let settled = false;
  let timerId: number | undefined;
  let resolveResult: ((ready: boolean) => void) | undefined;
  const promise = new Promise<boolean>((resolve) => {
    resolveResult = resolve;
    timerId = window.setTimeout(() => {
      if (settled) return;
      settled = true;
      resolve(false);
    }, timeoutMs);
  });

  void fontLoad.then((ready) => {
    if (settled) return;
    settled = true;
    if (timerId !== undefined) window.clearTimeout(timerId);
    resolveResult?.(ready);
  });

  return {
    promise,
    cancel: () => {
      if (settled) return;
      settled = true;
      if (timerId !== undefined) window.clearTimeout(timerId);
      resolveResult = undefined;
    },
  };
}

export default function DeferredEditorialAssets() {
  useEffect(() => {
    const root = document.documentElement;

    let disposed = false;
    let scheduled = false;
    let assetsActivated = root.classList.contains(
      EDITORIAL_ASSETS_READY_CLASS,
    );
    let cancelIdle: (() => void) | undefined;
    let maxWaitTimer: number | undefined;
    let cancelFontWait: (() => void) | undefined;
    let typographyRequestID = 0;
    let typographyStatus: TypographyStatus = "idle";
    let typographyDeadlineAt: number | undefined;
    let lastTypographyRequestKey: string | undefined;
    const pendingImages = new Set<HTMLImageElement>();
    const inFlightFontLoads = new Map<string, Promise<FontFace[]>>();

    const loadFontOnce = (font: string, text: string) => {
      const loadKey = JSON.stringify([font, text]);
      const existingLoad = inFlightFontLoads.get(loadKey);
      if (existingLoad) return existingLoad;

      const fontLoad = document.fonts.load(font, text);
      inFlightFontLoads.set(loadKey, fontLoad);
      const clearLoad = () => {
        if (inFlightFontLoads.get(loadKey) === fontLoad) {
          inFlightFontLoads.delete(loadKey);
        }
      };
      void fontLoad.then(clearLoad, clearLoad);
      return fontLoad;
    };

    const loadEditorialFontsOnce = (cjkText: string, latinText: string) => {
      const fontSet = document.fonts;
      if (!fontSet || typeof fontSet.load !== "function") {
        return Promise.resolve(false);
      }

      try {
        const fontLoads: Promise<FontFace[]>[] = [];
        if (cjkText) {
          fontLoads.push(
            loadFontOnce(`400 16px "${CJK_DISPLAY_FONT}"`, cjkText),
          );
        }
        if (latinText) {
          fontLoads.push(
            loadFontOnce(`650 16px "${LATIN_DISPLAY_FONT}"`, latinText),
          );
        }
        return Promise.all(fontLoads).then(
          () => true,
          () => false,
        );
      } catch {
        return Promise.resolve(false);
      }
    };

    const prepareTypography = () => {
      if (
        disposed ||
        !assetsActivated ||
        typographyStatus === "failed"
      ) {
        return;
      }

      const { cjkText, latinText } = readEditorialDisplayText();
      const requestKey = JSON.stringify([cjkText, latinText]);
      if (
        requestKey === lastTypographyRequestKey &&
        typographyStatus !== "idle"
      ) {
        return;
      }

      const previousStatus = typographyStatus;
      lastTypographyRequestKey = requestKey;
      typographyRequestID += 1;
      typographyStatus = cjkText || latinText ? "pending" : "idle";
      cancelFontWait?.();
      cancelFontWait = undefined;
      root.classList.remove(EDITORIAL_TYPOGRAPHY_READY_CLASS);

      if (!cjkText && !latinText) return;

      if (
        typographyDeadlineAt === undefined ||
        previousStatus === "ready"
      ) {
        typographyDeadlineAt = Date.now() + EDITORIAL_FONT_MAX_WAIT_MS;
      }

      const requestID = typographyRequestID;
      const fontWait = waitForEditorialFontsWithTimeout(
        loadEditorialFontsOnce(cjkText, latinText),
        Math.max(0, typographyDeadlineAt - Date.now()),
      );
      cancelFontWait = fontWait.cancel;
      void fontWait.promise.then((ready) => {
        if (disposed || requestID !== typographyRequestID) return;
        typographyStatus = ready ? "ready" : "failed";
        cancelFontWait = undefined;
        if (ready) {
          typographyDeadlineAt = undefined;
          root.classList.add(EDITORIAL_TYPOGRAPHY_READY_CLASS);
        }
      });
    };

    const activate = () => {
      if (!disposed) {
        root.style.setProperty(
          "--editorial-paper-texture",
          `url("${paperTextureUrl}")`,
        );
        root.classList.add(EDITORIAL_ASSETS_READY_CLASS);
        assetsActivated = true;
        prepareTypography();
      }
    };

    const removeImageListeners = () => {
      for (const image of pendingImages) {
        image.removeEventListener("load", handleSettled);
        image.removeEventListener("error", handleSettled);
      }
      pendingImages.clear();
    };

    const scheduleAssets = () => {
      if (scheduled || disposed) {
        return;
      }

      scheduled = true;
      if (maxWaitTimer !== undefined) {
        window.clearTimeout(maxWaitTimer);
        maxWaitTimer = undefined;
      }
      removeImageListeners();
      cancelIdle = scheduleIdle(activate);
    };

    const handleSettled = (event: Event) => {
      pendingImages.delete(event.currentTarget as HTMLImageElement);
      if (pendingImages.size === 0) {
        scheduleAssets();
      }
    };

    const criticalImages = Array.from(
      document.querySelectorAll<HTMLImageElement>(CRITICAL_COVER_SELECTOR),
    );
    for (const image of criticalImages) {
      if (!image.complete) {
        pendingImages.add(image);
        image.addEventListener("load", handleSettled, { once: true });
        image.addEventListener("error", handleSettled, { once: true });
      }
    }

    if (pendingImages.size === 0) {
      scheduleAssets();
    } else {
      maxWaitTimer = window.setTimeout(
        scheduleAssets,
        CRITICAL_COVER_MAX_WAIT_MS,
      );
    }

    const observer =
      typeof MutationObserver === "function"
        ? new MutationObserver(() => {
            prepareTypography();
          })
        : undefined;
    observer?.observe(document.body, {
      childList: true,
      characterData: true,
      subtree: true,
    });

    if (assetsActivated) {
      prepareTypography();
    }

    return () => {
      disposed = true;
      typographyRequestID += 1;
      typographyStatus = "idle";
      cancelFontWait?.();
      observer?.disconnect();
      if (maxWaitTimer !== undefined) {
        window.clearTimeout(maxWaitTimer);
      }
      cancelIdle?.();
      removeImageListeners();
    };
  }, []);

  return null;
}
