"use client";

import { SearchHighlightedText } from "@/components/search/SearchHighlightedText";
import {
  buildEpisodeSearchResultHref,
  getEpisodeSearchMetadata,
  getEpisodeSearchSnippet,
} from "@/lib/searchResultDisplay";
import type { EpisodeSearchResult } from "@/types";

interface SearchEpisodeResultCardProps {
  episode: EpisodeSearchResult;
  query: string;
}

export function SearchEpisodeResultCard({
  episode,
  query,
}: SearchEpisodeResultCardProps) {
  const snippetToShow = getEpisodeSearchSnippet(episode);

  return (
    <a
      href={buildEpisodeSearchResultHref(episode.podcast_id, episode.id)}
      target="_blank"
      rel="noopener noreferrer"
      className="search-episode-result"
    >
      <h3
        className="search-result-title"
        data-editorial-display-text="true"
      >
        <SearchHighlightedText text={episode.title} keyword={query} />
      </h3>
      <p className="search-result-meta">
        {getEpisodeSearchMetadata(episode)}
      </p>
      {snippetToShow ? (
        <p className="search-result-snippet line-clamp-2">
          <SearchHighlightedText text={snippetToShow} keyword={query} />
        </p>
      ) : null}
    </a>
  );
}
