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
      className="search-podcast-result"
    >
      <div className="flex gap-4">
        <div className="search-result-cover">
          <PodcastCover
            coverUrl={podcast.cover_url}
            title={podcast.title}
            index={index}
            priority={getSearchResultImagePriority(index)}
          />
        </div>
        <div className="flex-1 min-w-0">
          <h3 className="search-result-title line-clamp-1">
            <SearchHighlightedText text={podcast.title} keyword={query} />
          </h3>
          <p className="search-result-meta">
            {podcast.author} · {podcast.episode_count} 集
          </p>
          {snippetToShow ? (
            <p className="search-result-snippet line-clamp-2">
              <SearchHighlightedText text={snippetToShow} keyword={query} />
            </p>
          ) : null}
        </div>
      </div>
    </a>
  );
}
