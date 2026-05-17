"use client";

import PodcastCover from "@/components/podcasts/PodcastCover";
import { SearchHighlightedText } from "@/components/search/SearchHighlightedText";
import {
  buildPodcastSearchResultHref,
  getPodcastSearchSnippet,
  getSearchResultImagePriority,
} from "@/lib/searchResultDisplay";
import type { PodcastSearchResult } from "@/types";

interface SearchPodcastResultCardProps {
  podcast: PodcastSearchResult;
  index: number;
  query: string;
}

export function SearchPodcastResultCard({
  podcast,
  index,
  query,
}: SearchPodcastResultCardProps) {
  const snippetToShow = getPodcastSearchSnippet(podcast);

  return (
    <a
      href={buildPodcastSearchResultHref(podcast.id)}
      target="_blank"
      rel="noopener noreferrer"
      className="block p-4 bg-white dark:bg-slate-800 rounded-lg hover:shadow-md transition-all border border-slate-200 dark:border-slate-700"
    >
      <div className="flex gap-4">
        <div className="w-20 h-20 flex-shrink-0">
          <PodcastCover
            coverUrl={podcast.cover_url}
            title={podcast.title}
            index={index}
            priority={getSearchResultImagePriority(index)}
          />
        </div>
        <div className="flex-1 min-w-0">
          <h3 className="font-semibold text-slate-900 dark:text-slate-50 mb-1 line-clamp-1">
            <SearchHighlightedText text={podcast.title} keyword={query} />
          </h3>
          <p className="text-sm text-slate-600 dark:text-slate-400 mb-2">
            {podcast.author} · {podcast.episode_count} 集
          </p>
          {snippetToShow ? (
            <p className="text-sm text-slate-500 dark:text-slate-500 line-clamp-2">
              <SearchHighlightedText text={snippetToShow} keyword={query} />
            </p>
          ) : null}
        </div>
      </div>
    </a>
  );
}
