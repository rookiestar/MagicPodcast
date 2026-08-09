"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTags } from "@/hooks/useTagSWR";
import { usePodcastListInfinite } from "@/hooks/usePodcastSWR";
import { useUrlState } from "@/hooks/useUrlState";
import PageLayout from "@/components/layout/PageLayout";
import PodcastListResults from "@/components/podcasts/PodcastListResults";
import PodcastListSortControls from "@/components/podcasts/PodcastListSortControls";
import { MobilePodcastListSummary } from "@/components/podcasts/PodcastListStates";
import PodcastTagFilter from "@/components/podcasts/PodcastTagFilter";
import { useSearch } from "@/contexts/SearchContext";
import {
  getPageSize,
  getPageSizeForViewportWidth,
  useBreakpoint,
} from "@/hooks/useBreakpoint";
import {
  getDefaultPodcastTagCount,
  getPodcastListDescription,
  getPodcastListErrorMessage,
  getPodcastTagsWithPodcasts,
  getValidPodcastTagIds,
  getVisiblePodcastTags,
  hasMorePodcastTags,
  normalizePodcastTagIds,
  PODCAST_SORT_OPTIONS,
  type PodcastSortBy,
} from "@/lib/podcastListState";
import {
  clearPodcastListScrollSnapshot,
  getPodcastListScrollRestoreAction,
  getPodcastListStateKey,
  readPodcastListScrollSnapshot,
  restorePodcastListScroll,
  type PodcastListScrollSnapshot,
} from "@/lib/podcastListScrollState";

export default function PodcastsContent() {
  const [showAllTags, setShowAllTags] = useState(false);
  const pendingScrollRestoreRef = useRef<PodcastListScrollSnapshot | null>(null);
  const lastRestoreLoadRequestCountRef = useRef<number | null>(null);
  const { openSearch } = useSearch();
  const { isMobile, columns, isReady: isPageSizeReady } = useBreakpoint();
  const pageSize = isPageSizeReady
    ? getPageSize(columns)
    : getPageSizeForViewportWidth(
        typeof window === "undefined" ? undefined : window.innerWidth,
      );

  const [sortBy, setSortBy] = useUrlState<PodcastSortBy>(
    "sort_by",
    "recent_update",
  );
  const [selectedTagIdValues, setSelectedTagIdValues] = useUrlState<
    Array<number | string>
  >("tag_id", [], { isArray: true });
  const selectedTagIds = useMemo(
    () => normalizePodcastTagIds(selectedTagIdValues),
    [selectedTagIdValues],
  );
  const listStateKey = useMemo(
    () => getPodcastListStateKey({ sortBy, selectedTagIds }),
    [sortBy, selectedTagIds],
  );

  const { tags: allTags } = useTags();
  const tags = getPodcastTagsWithPodcasts(allTags);

  const {
    podcasts,
    totalCount,
    hasMore,
    isLoading,
    isLoadingMore,
    isError,
    error,
    loadMore,
    retryLastPage,
  } = usePodcastListInfinite({
    enabled: true,
    page_size: pageSize,
    sort_by: sortBy,
    tag_id: selectedTagIds.length > 0 ? selectedTagIds : undefined,
  });

  useEffect(() => {
    pendingScrollRestoreRef.current =
      readPodcastListScrollSnapshot(listStateKey);
    lastRestoreLoadRequestCountRef.current = null;
  }, [listStateKey]);

  useEffect(() => {
    const snapshot = pendingScrollRestoreRef.current;
    const action = getPodcastListScrollRestoreAction({
      snapshot,
      loadedCount: podcasts.length,
      hasMore,
      isLoadingMore,
    });

    if (action === "load-more") {
      if (lastRestoreLoadRequestCountRef.current !== podcasts.length) {
        lastRestoreLoadRequestCountRef.current = podcasts.length;
        loadMore();
      }
      return;
    }

    if (action === "restore" && snapshot) {
      restorePodcastListScroll(snapshot);
      clearPodcastListScrollSnapshot(snapshot.stateKey);
      pendingScrollRestoreRef.current = null;
    }
  }, [podcasts.length, hasMore, isLoadingMore, loadMore]);

  useEffect(() => {
    if (tags.length === 0 || selectedTagIds.length === 0) {
      return;
    }

    const validTagIds = getValidPodcastTagIds(selectedTagIds, tags);
    if (validTagIds.length !== selectedTagIds.length) {
      setSelectedTagIdValues(validTagIds);
    }
  }, [tags, selectedTagIds, setSelectedTagIdValues]);

  const handleTagToggle = useCallback(
    (tagId: number | null) => {
      let newSelected: number[];

      if (tagId === null) {
        newSelected = [];
      } else if (selectedTagIds.includes(tagId)) {
        newSelected = selectedTagIds.filter((id) => id !== tagId);
      } else {
        newSelected = [...selectedTagIds, tagId];
      }

      setSelectedTagIdValues(newSelected);
    },
    [selectedTagIds, setSelectedTagIdValues],
  );

  const handleSortChange = useCallback(
    (newSortBy: PodcastSortBy) => {
      setSortBy(newSortBy);
    },
    [setSortBy],
  );

  const defaultTagCount = getDefaultPodcastTagCount(isMobile);
  const displayTags = getVisiblePodcastTags(tags, showAllTags, defaultTagCount);
  const hasMoreTags = hasMorePodcastTags(tags, defaultTagCount);
  const listDescription = getPodcastListDescription(
    totalCount,
    selectedTagIds.length,
  );
  const errorMessage = getPodcastListErrorMessage(error);

  return (
    <PageLayout
      rootClassName="editorial-page-shell"
      className="podcast-library-page"
      onSearchClick={openSearch}
      toolbar={{
        title: "我的订阅",
        description: listDescription,
        rightContent: (
          <PodcastListSortControls
            sortBy={sortBy}
            options={PODCAST_SORT_OPTIONS}
            onSortChange={handleSortChange}
          />
        ),
        className: "editorial-page-toolbar",
      }}
    >
      <MobilePodcastListSummary
        totalCount={totalCount}
        selectedTagCount={selectedTagIds.length}
      />

      <PodcastTagFilter
        displayTags={displayTags}
        selectedTagIds={selectedTagIds}
        hasMoreTags={hasMoreTags}
        showAllTags={showAllTags}
        onTagToggle={handleTagToggle}
        onShowAllTagsChange={setShowAllTags}
      />

      <PodcastListResults
        podcasts={podcasts}
        columns={columns}
        isMobile={isMobile}
        listStateKey={listStateKey}
        sortBy={sortBy}
        selectedTagIds={selectedTagIds}
        hasMore={hasMore}
        isLoading={isLoading}
        isLoadingMore={isLoadingMore}
        isError={isError}
        errorMessage={errorMessage}
        onLoadMore={loadMore}
        onRetry={retryLastPage}
        onClearFilters={() => handleTagToggle(null)}
      />
    </PageLayout>
  );
}
