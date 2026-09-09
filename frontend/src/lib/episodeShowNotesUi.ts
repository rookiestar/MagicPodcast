export function getEpisodeShowNotesToggleLabel(isExpanded: boolean) {
  return isExpanded ? "收起" : "展开简介";
}

export function shouldKeepEpisodeShowNotesPreview(
  isExpanded: boolean,
  status: "idle" | "loading" | "success" | "error",
) {
  return !isExpanded || status === "idle" || status === "loading" || status === "error";
}

export function measureEpisodeShowNotesHeight(node: HTMLElement | null) {
  if (!node) {
    return 0;
  }

  const previousHeight = node.style.height;
  node.style.height = "auto";
  const nextHeight = node.getBoundingClientRect().height;
  node.style.height = previousHeight;
  return nextHeight;
}

export function shouldAnimateEpisodeShowNotesHeight(
  previousHeight: number,
  nextHeight: number,
  reduceMotion: boolean,
) {
  return !reduceMotion && previousHeight > 0 && nextHeight > 0 && previousHeight !== nextHeight;
}

export function syncEpisodeShowNotesBodyHeight(
  node: HTMLElement | null,
  storedHeight: number,
  reduceMotion: boolean,
) {
  if (!node) {
    return storedHeight;
  }

  const nextHeight = measureEpisodeShowNotesHeight(node);

  if (
    !shouldAnimateEpisodeShowNotesHeight(storedHeight, nextHeight, reduceMotion)
  ) {
    node.style.height = "";
    return nextHeight > 0 ? nextHeight : storedHeight;
  }

  node.style.height = `${storedHeight}px`;
  node.getBoundingClientRect();
  node.style.height = `${nextHeight}px`;
  return nextHeight;
}
