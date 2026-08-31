export interface ShowNotesDocument {
  content: string;
  format: "html" | "markdown";
}

export interface EpisodeShowNotesPayload {
  episode_id: number;
  show_notes_document: ShowNotesDocument;
}
