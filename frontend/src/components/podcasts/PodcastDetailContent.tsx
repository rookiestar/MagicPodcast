"use client";

import type { Episode, Podcast, Tag } from "@/types";
import EpisodeListSection from "@/components/episodes/EpisodeListSection";
import {
  DesktopPodcastDetailInfo,
  MobilePodcastDetailInfo,
} from "@/components/podcasts/PodcastDetailInfo";

interface PodcastDetailContentProps {
  error: string | null;
  podcast?: Podcast | null;
  tags: Tag[];
  notes: string;
  isEditingNotes: boolean;
  isSavingNotes?: boolean;
  isUpdatingTags?: boolean;
  episodes: Episode[];
  episodesLoading: boolean;
  isLoadingMore: boolean;
  hasMoreEpisodes: boolean;
  totalEpisodes: number;
  episodesError?: string | null;
  podcastCover?: string;
  loadMoreRef: (element: HTMLDivElement | null) => void;
  onNotesChange: (notes: string) => void;
  onEditNotes: () => void;
  onSaveNotes: () => void;
  onCancelNotesEdit: () => void;
  onTagsChange: (tags: Tag[]) => void;
  onRetryEpisodes: () => void;
}

function PodcastDetailError({ message }: { message: string }) {
  return (
    <div
      role="alert"
      className="rounded-lg border border-red-200 bg-red-50 p-6"
    >
      <h3 className="mb-2 font-semibold text-red-800">加载失败</h3>
      <p className="text-red-600">{message}</p>
    </div>
  );
}

export default function PodcastDetailContent({
  error,
  podcast,
  tags,
  notes,
  isEditingNotes,
  isSavingNotes,
  isUpdatingTags,
  episodes,
  episodesLoading,
  isLoadingMore,
  hasMoreEpisodes,
  totalEpisodes,
  episodesError,
  podcastCover,
  loadMoreRef,
  onNotesChange,
  onEditNotes,
  onSaveNotes,
  onCancelNotesEdit,
  onTagsChange,
  onRetryEpisodes,
}: PodcastDetailContentProps) {
  if (error) {
    return <PodcastDetailError message={error} />;
  }

  if (!podcast) {
    return null;
  }

  return (
    <>
      <MobilePodcastDetailInfo
        podcast={podcast}
        tags={tags}
        notes={notes}
        isEditingNotes={isEditingNotes}
        isSavingNotes={isSavingNotes}
        isUpdatingTags={isUpdatingTags}
        onNotesChange={onNotesChange}
        onEditNotes={onEditNotes}
        onSaveNotes={onSaveNotes}
        onCancelNotesEdit={onCancelNotesEdit}
        onTagsChange={onTagsChange}
      />
      <DesktopPodcastDetailInfo
        podcast={podcast}
        tags={tags}
        notes={notes}
        isEditingNotes={isEditingNotes}
        isSavingNotes={isSavingNotes}
        isUpdatingTags={isUpdatingTags}
        onNotesChange={onNotesChange}
        onEditNotes={onEditNotes}
        onSaveNotes={onSaveNotes}
        onCancelNotesEdit={onCancelNotesEdit}
        onTagsChange={onTagsChange}
      />
      <EpisodeListSection
        episodes={episodes}
        episodesLoading={episodesLoading}
        isLoadingMore={isLoadingMore}
        hasMoreEpisodes={hasMoreEpisodes}
        totalEpisodes={totalEpisodes}
        episodesError={episodesError}
        podcastCover={podcastCover}
        loadMoreRef={loadMoreRef}
        onRetry={onRetryEpisodes}
      />
    </>
  );
}
