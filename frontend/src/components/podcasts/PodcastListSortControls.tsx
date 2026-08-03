"use client";

import type {
  PodcastSortBy,
  PodcastSortOption,
} from "@/lib/podcastListState";
import EditorialSortControls from "@/components/layout/EditorialSortControls";

interface PodcastListSortControlsProps {
  sortBy: PodcastSortBy;
  options: PodcastSortOption[];
  onSortChange: (sortBy: PodcastSortBy) => void;
}

export default function PodcastListSortControls({
  sortBy,
  options,
  onSortChange,
}: PodcastListSortControlsProps) {
  return (
    <EditorialSortControls<PodcastSortBy>
      sortBy={sortBy}
      options={options}
      onSortChange={onSortChange}
    />
  );
}
