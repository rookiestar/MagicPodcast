"use client";

import type { Podcast } from "@/types";
import VirtualPodcastGrid from "@/components/common/VirtualPodcastGrid";
import type { PodcastSortBy } from "@/lib/podcastListState";
import {
  PodcastListEmptyFilterState,
  PodcastListEmptyLibraryState,
  PodcastListErrorState,
  PodcastListFooter,
  PodcastListLoadingGrid,
} from "./PodcastListStates";

interface PodcastListResultsProps {
  podcasts: Podcast[];
  columns: number;
  isMobile: boolean;
  listStateKey: string;
  sortBy: PodcastSortBy;
  selectedTagIds: number[];
  hasMore: boolean;
  isLoading: boolean;
  isLoadingMore: boolean;
  isError: boolean;
  errorMessage: string;
  onLoadMore: () => void;
  onRetry: () => void;
  onClearFilters: () => void;
}

export default function PodcastListResults({
  podcasts,
  columns,
  isMobile,
  listStateKey,
  sortBy,
  selectedTagIds,
  hasMore,
  isLoading,
  isLoadingMore,
  isError,
  errorMessage,
  onLoadMore,
  onRetry,
  onClearFilters,
}: PodcastListResultsProps) {
  if (isError) {
    return <PodcastListErrorState message={errorMessage} onRetry={onRetry} />;
  }

  if (isLoading) {
    return <PodcastListLoadingGrid isMobile={isMobile} />;
  }

  if (podcasts.length === 0 && selectedTagIds.length > 0) {
    return <PodcastListEmptyFilterState onClearFilters={onClearFilters} />;
  }

  if (podcasts.length === 0) {
    return <PodcastListEmptyLibraryState />;
  }

  return (
    <>
      <VirtualPodcastGrid
        podcasts={podcasts}
        columns={columns}
        isMobile={isMobile}
        listStateKey={listStateKey}
        sortBy={sortBy}
        selectedTagIds={selectedTagIds}
        onLoadMore={onLoadMore}
        hasMore={hasMore}
        isLoading={isLoadingMore}
      />

      <PodcastListFooter
        hasPodcasts={podcasts.length > 0}
        hasMore={hasMore}
        isLoadingMore={isLoadingMore}
      />
    </>
  );
}
