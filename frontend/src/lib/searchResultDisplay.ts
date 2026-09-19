import type {
  EpisodeSearchResult,
  MatchedField,
  PodcastSearchResult,
} from "@/types";
import { stripHtml } from "@/lib/textUtils";
import type { SearchType } from "@/lib/searchSidebarState";

export type SearchResultImagePriority = "high" | "medium" | "low";

const SEARCH_TYPE_OPTION_CONFIGS: Array<{
  type: SearchType;
  label: string;
}> = [
  { type: "all", label: "全部" },
  { type: "podcasts", label: "节目" },
  { type: "episodes", label: "单集" },
];

interface SearchResultCounts {
  podcastCount: number;
  episodeCount: number;
}

export function getSearchTypeOptionConfigs() {
  return SEARCH_TYPE_OPTION_CONFIGS;
}

export function getSearchResultsCount({
  podcastCount,
  episodeCount,
}: SearchResultCounts) {
  return podcastCount + episodeCount;
}

export function getSearchTypeOptionCount(
  searchType: SearchType,
  counts: SearchResultCounts,
) {
  if (searchType === "podcasts") return counts.podcastCount;
  if (searchType === "episodes") return counts.episodeCount;
  return getSearchResultsCount(counts);
}

export function getSearchTypeOptionLabel(
  searchType: SearchType,
  counts: SearchResultCounts,
) {
  const option = SEARCH_TYPE_OPTION_CONFIGS.find(
    (config) => config.type === searchType,
  );
  const label = option?.label ?? "";
  const count = getSearchTypeOptionCount(searchType, counts);

  return count > 0 ? `${label} (${count})` : label;
}

function escapeSearchKeyword(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// Keep source UTF-16 offsets so normalization never changes displayed text.
function normalizeSearchDisplay(text: string) {
  const chars = Array.from(text);
  const positions: number[] = [];
  let offset = 0;
  for (const char of chars) { positions.push(offset); offset += char.length; }
  let normalized = "";
  const starts: number[] = [];
  const ends: number[] = [];
  const isSpace = (char: string) => /[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]/u.test(char);
  const isHan = (char: string) => /\p{Script=Han}/u.test(char);
  const isASCII = (char: string) => /[A-Za-z0-9]/.test(char);
  for (let i = 0; i < chars.length;) {
    if (isSpace(chars[i])) {
      let end = i + 1;
      while (end < chars.length && isSpace(chars[end])) end++;
      if (i === 0 || end === chars.length ||
          (isHan(chars[i - 1]) && isASCII(chars[end])) ||
          (isASCII(chars[i - 1]) && isHan(chars[end]))) { i = end; continue; }
    }
    const lowered = chars[i].toLowerCase();
    normalized += lowered;
    for (let j = 0; j < lowered.length; j++) {
      starts.push(positions[i]); ends.push(positions[i] + chars[i].length);
    }
    i++;
  }
  return { normalized, starts, ends };
}

export function getSearchTextHighlightParts(text: string, keyword: string) {
  const normalizedKeyword = keyword.trim();
  if (!normalizedKeyword) {
    return [{ text, highlighted: false }];
  }

  if (/\p{Script=Han}/u.test(normalizedKeyword)) {
    const source = normalizeSearchDisplay(text);
    const query = normalizeSearchDisplay(normalizedKeyword).normalized;
    if (!query) return [{ text, highlighted: false }];
    const parts: Array<{ text: string; highlighted: boolean }> = [];
    let cursor = 0;
    let from = 0;
    for (;;) {
      const match = source.normalized.indexOf(query, from);
      if (match < 0) break;
      const start = source.starts[match];
      const end = source.ends[match + query.length - 1];
      if (start > cursor) parts.push({ text: text.slice(cursor, start), highlighted: false });
      parts.push({ text: text.slice(start, end), highlighted: true });
      cursor = end;
      from = match + query.length;
    }
    if (cursor < text.length) parts.push({ text: text.slice(cursor), highlighted: false });
    return parts.length ? parts : [{ text, highlighted: false }];
  }

  const keywordRegex = new RegExp(`(${escapeSearchKeyword(normalizedKeyword)})`, "gi");
  const keywordLower = normalizedKeyword.toLowerCase();

  return text
    .split(keywordRegex)
    .filter((part) => part.length > 0)
    .map((part) => ({
      text: part,
      highlighted: part.toLowerCase() === keywordLower,
    }));
}

export function shouldShowPodcastSearchResults(
  searchType: SearchType,
  podcastCount: number,
) {
  return (searchType === "all" || searchType === "podcasts") && podcastCount > 0;
}

export function shouldShowEpisodeSearchResults(
  searchType: SearchType,
  episodeCount: number,
) {
  return (searchType === "all" || searchType === "episodes") && episodeCount > 0;
}

export function shouldShowSearchSectionHeading(
  searchType: SearchType,
  peerResultCount: number,
) {
  return searchType === "all" && peerResultCount > 0;
}

export function getVisibleSearchResults<T>(
  results: T[],
  expanded: boolean,
  limit: number,
) {
  return expanded ? results : results.slice(0, limit);
}

export function shouldShowSearchExpandButton(resultCount: number, limit: number) {
  return resultCount > limit;
}

export function getSearchExpandButtonLabel(
  expanded: boolean,
  resultCount: number,
  unitLabel: string,
) {
  return expanded ? "收起" : `展开全部 ${resultCount} 个${unitLabel}`;
}

function findMatchedSnippet(
  matchedFields: MatchedField[] | undefined,
  fieldPriority: string[],
) {
  for (const fieldName of fieldPriority) {
    const matchedField = matchedFields?.find((field) => field.field === fieldName);
    if (matchedField?.snippet) {
      return matchedField.snippet;
    }
  }

  return "";
}

export function getPodcastSearchSnippet(podcast: PodcastSearchResult) {
  return (
    findMatchedSnippet(podcast.matched_fields, ["description", "author", "title"]) ||
    podcast.description
  );
}

export function getEpisodeSearchSnippet(episode: EpisodeSearchResult) {
  // Preserve the complete match and its significant whitespace while retaining
  // existing HTML cleanup/entity decoding. React renders the result as text.
  const matched = findMatchedSnippet(episode.matched_fields, ["show_notes", "title"]);
  return matched
    ? stripHtml(matched, Number.POSITIVE_INFINITY, true)
    : stripHtml(episode.show_notes, 180);
}

export function getSearchResultImagePriority(
  index: number,
): SearchResultImagePriority {
  if (index < 3) return "high";
  if (index < 10) return "medium";
  return "low";
}

export function getEpisodeSearchPublishedDateText(
  publishedDate: string | null | undefined,
) {
  return publishedDate ? new Date(publishedDate).toLocaleDateString() : "";
}

export function getEpisodeSearchMetadata(episode: EpisodeSearchResult) {
  const publishedDateText = getEpisodeSearchPublishedDateText(
    episode.published_date,
  );

  return publishedDateText
    ? `${episode.podcast_title} · ${publishedDateText}`
    : episode.podcast_title;
}

export function buildPodcastSearchResultHref(podcastId: number) {
  return `/podcasts/${podcastId}`;
}

export function buildEpisodeSearchResultHref(episodeId: number) {
  return `/episodes/${episodeId}?from=search`;
}
