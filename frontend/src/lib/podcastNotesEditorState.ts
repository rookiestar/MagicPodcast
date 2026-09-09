export function shouldShowPodcastNotesEditButton(
  isEditingNotes: boolean,
  hasNotes = true,
) {
  return !isEditingNotes && hasNotes;
}

export function shouldShowPodcastNotesAddButton(
  isEditingNotes: boolean,
  hasNotes: boolean,
) {
  return !isEditingNotes && !hasNotes;
}

export function arePodcastNotesControlsDisabled(isSavingNotes: boolean) {
  return isSavingNotes;
}

export function getPodcastNotesSaveButtonLabel(isSavingNotes: boolean) {
  return isSavingNotes ? "保存中..." : "保存";
}

export function getPodcastNotesReadOnlyText(notes: string) {
  return notes || "暂无备注";
}

export function hasPodcastNotes(notes: string) {
  return notes.length > 0;
}
