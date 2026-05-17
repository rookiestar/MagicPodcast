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
      className="block p-4 bg-slate-50 dark:bg-slate-900 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors"
    >
      <h3 className="font-semibold text-slate-900 dark:text-slate-50 mb-1">
        <SearchHighlightedText text={episode.title} keyword={query} />
      </h3>
      <p className="text-sm text-slate-600 dark:text-slate-400 mb-2">
        {getEpisodeSearchMetadata(episode)}
      </p>
      {snippetToShow ? (
        <p className="text-sm text-slate-500 dark:text-slate-500 line-clamp-2">
          <SearchHighlightedText text={snippetToShow} keyword={query} />
        </p>
      ) : null}
    </a>
  );
}
