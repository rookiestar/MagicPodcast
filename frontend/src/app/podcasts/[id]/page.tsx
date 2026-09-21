"use client";

import { positiveID, singleParam } from "@/lib/navigation";
import { useParams, useSearchParams } from "next/navigation";
import { useCallback } from "react";
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
import { usePodcastHistorySync } from "@/hooks/usePodcastHistorySync";
import { usePodcastMetadataEditing } from "@/hooks/usePodcastMetadataEditing";
import { useInfiniteScrollTrigger } from "@/hooks/usePagination";
import { getPodcastSyncControl } from "@/lib/podcastSyncControl";
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
  const targetEpisodeId = singleParam(searchParams, "episode_id");
  const sortBy = searchParams.get("sort_by") || "";
  const tagIds = searchParams.get("tag_ids") || searchParams.getAll("tag_id");
  const backUrl = buildPodcastListBackUrl({ sortBy, tagIds });

  const {
    podcast,
    isLoading: podcastLoading,
    isError: podcastError,
    mutate: mutatePodcast,
  } = usePodcast(podcastId);
  const { tags, mutate: mutateTags } = usePodcastTags(podcastId);
  const { notes: swrNotes, mutate: mutateNotes } = usePodcastNotes(podcastId);

  const {
    updateEpisodeQueue,
    episodes,
    episodesLoading,
    isLoadingMore,
    hasMoreEpisodes,
    totalEpisodes,
    episodesError,
    loadMoreEpisodes,
    retryEpisodes,
    refreshEpisodes,
  } = usePodcastEpisodes({
    podcastId: podcastId ?? 0,
    enabled: Boolean(podcastId && !podcastLoading),
    pageSize: PAGE_SIZE,
  });

  // 历史同步任务到达终态后刷新节目详情与单集列表，保持列表/详情与入库
  // 结果一致（#465）。
  const refreshAfterSyncSettled = useCallback(() => {
    mutatePodcast();
    refreshEpisodes();
  }, [mutatePodcast, refreshEpisodes]);

  const {
    task: historySyncTask,
    starting: historySyncStarting,
    actionError: historySyncActionError,
    start: startHistorySync,
  } = usePodcastHistorySync({
    podcastId: podcastId ?? 0,
    enabled: Boolean(podcastId && podcast),
    onSettled: refreshAfterSyncSettled,
  });

  const syncControl = podcast
    ? getPodcastSyncControl({
        podcast,
        task: historySyncTask,
        starting: historySyncStarting,
        startError: historySyncActionError,
        onStart: startHistorySync,
      })
    : null;

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

  const error = !podcastId ? "节目地址无效。" : getPodcastDetailErrorMessage(podcastError);
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
        {searchParams.has("episode_id") && !positiveID(targetEpisodeId) && <p role="alert">单集卡片地址无效，无法定位。</p>}
        <PodcastDetailContent
          onQueueChange={updateEpisodeQueue}
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
          syncControl={syncControl}
        />
      </div>
    </PageLayout>
  );
}
