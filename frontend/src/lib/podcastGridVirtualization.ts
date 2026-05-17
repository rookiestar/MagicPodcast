export const PODCAST_GRID_LOAD_MORE_ROW_BUFFER = 3;

export function getPodcastGridEstimateRowHeight(isMobile: boolean) {
  return isMobile ? 124 : 482;
}

export function getPodcastGridRowGap(isMobile: boolean) {
  return isMobile ? 12 : 24;
}

interface PodcastVirtualRow {
  index: number;
  start: number;
}

export function getLastVisiblePodcastRowIndex(
  rows: PodcastVirtualRow[],
  viewportBottom: number,
) {
  return rows.reduce<number | null>((lastVisibleIndex, row) => {
    if (row.start < viewportBottom) {
      return row.index;
    }

    return lastVisibleIndex;
  }, null);
}

interface ShouldLoadMorePodcastRowsParams {
  lastVisibleRowIndex: number | null | undefined;
  rowCount: number;
  hasMore: boolean;
  isLoading: boolean;
  bufferRows?: number;
}

export function shouldLoadMorePodcastRows({
  lastVisibleRowIndex,
  rowCount,
  hasMore,
  isLoading,
  bufferRows = PODCAST_GRID_LOAD_MORE_ROW_BUFFER,
}: ShouldLoadMorePodcastRowsParams) {
  if (
    lastVisibleRowIndex == null ||
    rowCount <= 0 ||
    !hasMore ||
    isLoading
  ) {
    return false;
  }

  return lastVisibleRowIndex >= Math.max(0, rowCount - bufferRows);
}
