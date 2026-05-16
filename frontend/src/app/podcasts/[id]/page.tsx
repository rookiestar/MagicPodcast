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
import { getEffectiveCoverUrl } from "@/lib/imageProxy";
import PageLayout from "@/components/layout/PageLayout";
import PodcastDetailContent from "@/components/podcasts/PodcastDetailContent";

const PAGE_SIZE = 20;
const EPISODE_SCROLL_OPTIONS: IntersectionObserverInit = {
  rootMargin: "300px",
};

export default function PodcastDetailPage() {
  const params = useParams();
  const searchParams = useSearchParams();
  const id = parseInt(params.id as string);
  const targetEpisodeId = searchParams.get("episode_id");
  const sortBy = searchParams.get("sort_by") || "";
  const tagIds = searchParams.get("tag_ids");
  const backUrl = buildPodcastListBackUrl({ sortBy, tagIds });

  const {
    podcast,
    isLoading: podcastLoading,
    isError: podcastError,
  } = usePodcast(id);
  const { tags, mutate: mutateTags } = usePodcastTags(id);
  const { notes: swrNotes, mutate: mutateNotes } = usePodcastNotes(id);

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
    podcastId: id,
    enabled: Boolean(id && !podcastLoading),
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
    podcastId: id,
    tags,
    swrNotes,
    mutateTags,
    mutateNotes,
  });

  const error = podcastError ? "加载播客失败" : null;
  const canAutoLoadMoreEpisodes = Boolean(
    episodes.length > 0 &&
      !episodesLoading &&
      !isLoadingMore &&
      hasMoreEpisodes &&
      !episodesError,
  );

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
      toolbar={{
        breadcrumbs: [{ label: "返回列表", href: backUrl }],
        title: podcast?.title || "播客详情",
        description:
          podcast && (podcast.episode_count || episodes.length) > 0
            ? `共 ${podcast.episode_count || episodes.length} 个单集`
            : undefined,
      }}
    >
      <div className="py-6">
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
          podcastCover={
            podcast
              ? getEffectiveCoverUrl(
                  podcast.custom_cover_url,
                  podcast.cover_url,
                )
              : undefined
          }
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
