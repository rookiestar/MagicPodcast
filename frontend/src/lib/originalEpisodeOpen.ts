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

function planOriginalEpisodeOpen(
  openUrl: string,
  parsed: URL,
): OriginalEpisodeOpenPlan {
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
  const parsed = parseAbsoluteHttpUrl(openUrl);
  if (!parsed) {
    return null;
  }
  return planOriginalEpisodeOpen(openUrl, parsed);
}

export const ORIGINAL_EPISODE_MISSING_TEXT = "原节目链接暂缺";
export const ORIGINAL_EPISODE_REJECTED_TEXT = "原节目链接不可安全打开";

export type OriginalEpisodeAccess =
  | { state: "openable"; openUrl: string; plan: OriginalEpisodeOpenPlan }
  | { state: "missing" }
  | { state: "rejected" };

export function originalEpisodeAccessText(access: OriginalEpisodeAccess) {
  if (access.state === "missing") {
    return ORIGINAL_EPISODE_MISSING_TEXT;
  }
  if (access.state === "rejected") {
    return ORIGINAL_EPISODE_REJECTED_TEXT;
  }
  return "";
}

/**
 * The single tri-state open planner shared by every user entry (Inbox,
 * podcast detail, Discovery, reports). Empty or blank values mean the source
 * data has no link (暂缺); a non-empty value that fails real URL parsing, is
 * not an absolute http(s) address with a host, or carries a dangerous scheme
 * is rejected (不可安全打开). Only the openable state may navigate.
 */
export function planOriginalEpisodeAccess(
  value: string | undefined | null,
): OriginalEpisodeAccess {
  const candidate = value?.trim() ?? "";
  if (!candidate) {
    return { state: "missing" };
  }
  const parsed = parseAbsoluteHttpUrl(candidate);
  if (!parsed) {
    return { state: "rejected" };
  }
  const plan = planOriginalEpisodeOpen(candidate, parsed);
  return { state: "openable", openUrl: plan.openUrl, plan };
}

function parseAbsoluteHttpUrl(value: string) {
  try {
    const parsed = new URL(value);
    if (
      (parsed.protocol === "http:" || parsed.protocol === "https:") &&
      parsed.hostname !== ""
    ) {
      return parsed;
    }
    return null;
  } catch {
    return null;
  }
}

export function openOriginalEpisodeTab(url: string) {
  window.open(url, "_blank", "noopener,noreferrer");
}
