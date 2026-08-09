"use client";

import { useEffect } from "react";
import paperTexture from "@/assets/warm-paper-grid-texture-v1.jpg";

export const EDITORIAL_ASSETS_READY_CLASS = "editorial-assets-ready";

const CRITICAL_COVER_SELECTOR =
  '[data-podcast-cover-critical="true"] img';
const ASSET_IDLE_TIMEOUT_MS = 1_500;
const CRITICAL_COVER_MAX_WAIT_MS = 15_000;
const paperTextureUrl =
  typeof paperTexture === "string" ? paperTexture : paperTexture.src;

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

export default function DeferredEditorialAssets() {
  useEffect(() => {
    const root = document.documentElement;
    if (root.classList.contains(EDITORIAL_ASSETS_READY_CLASS)) {
      return;
    }

    let disposed = false;
    let scheduled = false;
    let cancelIdle: (() => void) | undefined;
    let maxWaitTimer: number | undefined;
    const pendingImages = new Set<HTMLImageElement>();

    const activate = () => {
      if (!disposed) {
        root.style.setProperty(
          "--editorial-paper-texture",
          `url("${paperTextureUrl}")`,
        );
        root.classList.add(EDITORIAL_ASSETS_READY_CLASS);
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

    return () => {
      disposed = true;
      if (maxWaitTimer !== undefined) {
        window.clearTimeout(maxWaitTimer);
      }
      cancelIdle?.();
      removeImageListeners();
    };
  }, []);

  return null;
}
