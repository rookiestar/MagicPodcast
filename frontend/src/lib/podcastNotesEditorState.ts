export function shouldShowPodcastNotesEditButton(isEditingNotes: boolean) {
  return !isEditingNotes;
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
