import type { EpisodePersonCandidate } from "@/types/episodeCopilot";

export interface MentionDraft {
  start: number;
  query: string;
}

export function mentionDraft(
  value: string,
  cursor: number,
): MentionDraft | null {
  const prefix = value.slice(0, cursor);
  const at = prefix.lastIndexOf("@");
  if (at < 0) return null;
  if (at > 0 && /\S/.test(prefix.charAt(at - 1))) return null;
  const query = prefix.slice(at + 1);
  if (query.includes(" ") || query.includes("\n")) return null;
  return { start: at, query };
}

export function filterPeople(
  people: EpisodePersonCandidate[] | undefined,
  query: string,
): EpisodePersonCandidate[] {
  const normalized = query.trim().toLowerCase();
  const list = people ?? [];
  if (!normalized) return list;
  return list.filter((person) => {
    if (person.display_name.toLowerCase().includes(normalized)) return true;
    return person.aliases.some((alias) =>
      alias.toLowerCase().includes(normalized),
    );
  });
}

export function replaceMention(
  value: string,
  draft: MentionDraft,
  cursor: number,
): string {
  return `${value.slice(0, draft.start)}${value.slice(cursor)}`.replace(
    /^\s+/,
    "",
  );
}

export function roleLabel(role: string) {
  if (role === "host") return "主播";
  if (role === "guest") return "嘉宾";
  return "未知角色";
}

export function parseLibrarySources(answer: string) {
  const matches = [
    ...answer.matchAll(
      /\[库内 S(\d+)\] 单集 (\d+) · ([^·]+) · 片段 (\d+)(?: · 版本 ([^\s]+))?(?: · 标题 ([^\n]+))?/g,
    ),
  ];
  return matches.map((match) => ({
    index: Number(match[1]),
    episodeId: Number(match[2]),
    date: match[3].trim(),
    fragmentOrder: Number(match[4]),
    ...(match[5] ? { sourceVersion: match[5] } : {}),
    ...(match[6] ? { title: match[6].trim() } : {}),
  }));
}

export async function jumpToLibrarySource(
  source: { episodeId: number; fragmentOrder: number },
  options: {
    host?: Document;
    openEpisode?: (episodeId: number) => void | Promise<void>;
  } = {},
) {
  const host = options.host ?? document;
  const alreadyOpen = Boolean(
    host.querySelector(
      `[data-copilot-source="transcript"][data-copilot-episode-id="${source.episodeId}"]`,
    ),
  );
  if (!alreadyOpen && options.openEpisode) {
    await options.openEpisode(source.episodeId);
  }
  host.getElementById("detail-tab-transcript")?.click();
  host.getElementById("processing-artifact-tab-transcript")?.click();
  const locate = () => host.querySelector<HTMLElement>(
    `[data-copilot-source="transcript"][data-copilot-episode-id="${source.episodeId}"] [data-fragment-order="${source.fragmentOrder}"]`,
  );
  for (let attempt = 0; attempt < 40; attempt += 1) {
    const target = locate();
    if (target) { target.scrollIntoView({ block: "center" }); return; }
    await new Promise((resolve) => window.setTimeout(resolve, 100));
    host.getElementById("detail-tab-transcript")?.click();
    host.getElementById("processing-artifact-tab-transcript")?.click();
  }
  throw new Error("source fragment unavailable");
}
