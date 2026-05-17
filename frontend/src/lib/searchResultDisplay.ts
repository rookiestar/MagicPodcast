import type {
  EpisodeSearchResult,
  MatchedField,
  PodcastSearchResult,
} from "@/types";
import type { SearchType } from "@/lib/searchSidebarState";

export function getSearchResultsCount({
  podcastCount,
  episodeCount,
}: {
  podcastCount: number;
  episodeCount: number;
}) {
  return podcastCount + episodeCount;
}

function escapeSearchKeyword(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

export function getSearchTextHighlightParts(text: string, keyword: string) {
  const normalizedKeyword = keyword.trim();
  if (!normalizedKeyword) {
    return [{ text, highlighted: false }];
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
  return (
    findMatchedSnippet(episode.matched_fields, ["show_notes", "title"]) ||
    episode.show_notes
  );
}

export function buildPodcastSearchResultHref(podcastId: number) {
  return `/podcasts/${podcastId}`;
}

export function buildEpisodeSearchResultHref(podcastId: number, episodeId: number) {
  return `/podcasts/${podcastId}?episode_id=${episodeId}`;
}
