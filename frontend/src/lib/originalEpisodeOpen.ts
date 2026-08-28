import { sanitizeContentUrl } from "@/lib/imageSourcePolicy";

const XIAOYUZHOU_EPISODE_HOSTS = new Set([
  "www.xiaoyuzhoufm.com",
  "web.xiaoyuzhoufm.com",
]);

export type OriginalEpisodeOpenPlan =
  | {
      recovery: false;
      openUrl: string;
    }
  | {
      recovery: true;
      openUrl: string;
      retryUrl: string;
      appUrl: string;
      copyText: string;
    };

export type OriginalEpisodeRecoveryPlan = Extract<
  OriginalEpisodeOpenPlan,
  { recovery: true }
>;

export function getSafeOriginalUrl(value: string | undefined | null) {
  const safeUrl = sanitizeContentUrl(value);
  if (
    /^https?:\/\//i.test(safeUrl) ||
    (safeUrl.startsWith("/") && !safeUrl.startsWith("//"))
  ) {
    return safeUrl;
  }
  return "";
}

function planOriginalEpisodeOpen(openUrl: string): OriginalEpisodeOpenPlan {
  const parsed = parseHttpUrl(openUrl);
  if (!parsed) {
    return { recovery: false, openUrl };
  }

  const host = parsed.hostname.replace(/\.$/, "").toLowerCase();
  if (!XIAOYUZHOU_EPISODE_HOSTS.has(host)) {
    return { recovery: false, openUrl };
  }

  const segments = parsed.pathname.replace(/\/+$/, "").split("/").filter(Boolean);
  if (segments.length !== 2 || segments[0] !== "episode") {
    return { recovery: false, openUrl };
  }

  const episodeId = segments[1];
  const retryUrl = `https://www.xiaoyuzhoufm.com/episode/${episodeId}`;
  return {
    recovery: true,
    openUrl,
    retryUrl,
    appUrl: `cosmos://page.cos/episode/${episodeId}`,
    copyText: retryUrl,
  };
}

export function planSafeOriginalEpisodeOpen(
  value: string | undefined | null,
): OriginalEpisodeOpenPlan | null {
  const openUrl = getSafeOriginalUrl(value);
  if (!openUrl) {
    return null;
  }
  return planOriginalEpisodeOpen(openUrl);
}

export function openOriginalEpisodeTab(url: string) {
  window.open(url, "_blank", "noopener,noreferrer");
}

function parseHttpUrl(value: string) {
  try {
    const parsed = new URL(value);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}
