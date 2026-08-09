const PODCAST_GRID_LOAD_MORE_ROW_BUFFER = 3;
const MOBILE_FIRST_SCREEN_COVER_COUNT = 5;
const DESKTOP_OVERSCAN_ROWS = 2;
const MOBILE_OVERSCAN_ROWS = 4;

export function getPodcastGridCoverPriority(
  index: number,
  columns: number,
  isMobile: boolean,
): "high" | "low" {
  const eagerCount = isMobile
    ? MOBILE_FIRST_SCREEN_COVER_COUNT
    : Math.max(1, columns);
  return index < eagerCount ? "high" : "low";
}

export function getPodcastGridEstimateRowHeight(isMobile: boolean) {
  return isMobile ? 124 : 482;
}

export function getPodcastGridRowGap(isMobile: boolean) {
  return isMobile ? 12 : 24;
}

export function getPodcastGridOverscan(isMobile: boolean) {
  return isMobile ? MOBILE_OVERSCAN_ROWS : DESKTOP_OVERSCAN_ROWS;
}

interface PodcastVirtualRow {
  index: number;
  start: number;
  end?: number;
}

export function getLastVisiblePodcastRowIndex(
  rows: PodcastVirtualRow[],
  viewportBottom: number,
) {
  return rows.reduce<number | null>((lastVisibleIndex, row) => {
    const rowBottom = row.end ?? row.start;
    if (rowBottom <= viewportBottom) {
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
