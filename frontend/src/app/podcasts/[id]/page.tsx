"use client";

import { useParams, useSearchParams } from "next/navigation";
import {
  buildPodcastListBackUrl,
  useTargetEpisodeNavigation,
} from "@/hooks/usePodcastDetailNavigation";
import {
  usePodcast,
  usePodcastNotes,
  usePodcastTags,
} from "@/hooks/usePodcastSWR";
import { usePodcastEpisodes } from "@/hooks/usePodcastEpisodes";
import { usePodcastMetadataEditing } from "@/hooks/usePodcastMetadataEditing";
import { useInfiniteScrollTrigger } from "@/hooks/usePagination";
import {
  canAutoLoadMorePodcastEpisodes,
  getPodcastDetailCoverUrl,
  getPodcastDetailDescription,
  getPodcastDetailErrorMessage,
  getPodcastDetailTitle,
  parsePodcastDetailId,
} from "@/lib/podcastDetailState";
import PageLayout from "@/components/layout/PageLayout";
import PodcastDetailContent from "@/components/podcasts/PodcastDetailContent";

const PAGE_SIZE = 20;
const EPISODE_SCROLL_OPTIONS: IntersectionObserverInit = {
  rootMargin: "300px",
};

export default function PodcastDetailPage() {
  const params = useParams();
  const searchParams = useSearchParams();
  const podcastId = parsePodcastDetailId(params.id);
  const targetEpisodeId = searchParams.get("episode_id");
  const sortBy = searchParams.get("sort_by") || "";
  const tagIds = searchParams.get("tag_ids") || searchParams.getAll("tag_id");
  const backUrl = buildPodcastListBackUrl({ sortBy, tagIds });

  const {
    podcast,
    isLoading: podcastLoading,
    isError: podcastError,
  } = usePodcast(podcastId);
  const { tags, mutate: mutateTags } = usePodcastTags(podcastId);
  const { notes: swrNotes, mutate: mutateNotes } = usePodcastNotes(podcastId);

  const {
    episodes,
    episodesLoading,
    isLoadingMore,
    hasMoreEpisodes,
    totalEpisodes,
    episodesError,
    loadMoreEpisodes,
    retryEpisodes,
  } = usePodcastEpisodes({
    podcastId: podcastId ?? 0,
    enabled: Boolean(podcastId && !podcastLoading),
    pageSize: PAGE_SIZE,
  });

  const {
    notes,
    setNotes,
    isEditingNotes,
    setIsEditingNotes,
    isSavingNotes,
    isUpdatingTags,
    handleTagsChange,
    handleNotesSave,
    cancelNotesEdit,
  } = usePodcastMetadataEditing({
    podcastId: podcastId ?? 0,
    tags,
    swrNotes,
    mutateTags,
    mutateNotes,
  });

  const error = getPodcastDetailErrorMessage(podcastError);
  const canAutoLoadMoreEpisodes = canAutoLoadMorePodcastEpisodes({
    episodeCount: episodes.length,
    episodesLoading,
    isLoadingMore,
    hasMoreEpisodes,
    episodesError,
  });

  const { ref: loadMoreRef } = useInfiniteScrollTrigger(
    loadMoreEpisodes,
    {
      ...EPISODE_SCROLL_OPTIONS,
      enabled: canAutoLoadMoreEpisodes,
    },
  );

  useTargetEpisodeNavigation({
    targetEpisodeId,
    episodes,
    episodesLoading,
    totalEpisodes,
    hasMoreEpisodes,
    isLoadingMore,
    loadMoreEpisodes,
  });

  return (
    <PageLayout
      rootClassName="editorial-page-shell"
      className="podcast-detail-page"
      toolbar={{
        breadcrumbs: [{ label: "返回列表", href: backUrl }],
        title: getPodcastDetailTitle(podcast),
        description: getPodcastDetailDescription(podcast, episodes.length),
        className: "editorial-page-toolbar",
      }}
    >
      <div className="podcast-detail-content py-6">
        <PodcastDetailContent
          error={error}
          podcast={podcast}
          tags={tags}
          notes={notes}
          isEditingNotes={isEditingNotes}
          isSavingNotes={isSavingNotes}
          isUpdatingTags={isUpdatingTags}
          episodes={episodes}
          episodesLoading={episodesLoading}
          isLoadingMore={isLoadingMore}
          hasMoreEpisodes={hasMoreEpisodes}
          totalEpisodes={totalEpisodes}
          episodesError={episodesError}
          podcastCover={getPodcastDetailCoverUrl(podcast)}
          loadMoreRef={loadMoreRef}
          onNotesChange={setNotes}
          onEditNotes={() => setIsEditingNotes(true)}
          onSaveNotes={handleNotesSave}
          onCancelNotesEdit={cancelNotesEdit}
          onTagsChange={handleTagsChange}
          onRetryEpisodes={retryEpisodes}
        />
      </div>
    </PageLayout>
  );
}
