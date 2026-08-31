import { episodeApi } from "@/lib/api/episode";
import type {
  EpisodeShowNotesPayload,
  ShowNotesDocument,
} from "@/types/showNotes";

export type EpisodeShowNotesLoader = (
  episodeId: number,
) => Promise<EpisodeShowNotesPayload>;

export interface EpisodeShowNotesStore {
  get: (episodeId: number) => ShowNotesDocument | undefined;
  load: (episodeId: number) => Promise<ShowNotesDocument>;
}

const DEFAULT_MAX_DOCUMENTS = 12;

function isShowNotesDocument(value: unknown): value is ShowNotesDocument {
  if (!value || typeof value !== "object") return false;
  const document = value as Partial<ShowNotesDocument>;
  return (
    typeof document.content === "string" &&
    (document.format === "html" || document.format === "markdown")
  );
}

export function createEpisodeShowNotesStore(
  loader: EpisodeShowNotesLoader = episodeApi.getShowNotes,
  maxDocuments = DEFAULT_MAX_DOCUMENTS,
): EpisodeShowNotesStore {
  const documents = new Map<number, ShowNotesDocument>();
  const inFlight = new Map<number, Promise<ShowNotesDocument>>();
  const capacity = Math.max(1, Math.floor(maxDocuments));

  const get = (episodeId: number) => {
    const document = documents.get(episodeId);
    if (!document) return undefined;
    documents.delete(episodeId);
    documents.set(episodeId, document);
    return document;
  };

  const remember = (episodeId: number, document: ShowNotesDocument) => {
    documents.delete(episodeId);
    documents.set(episodeId, document);
    while (documents.size > capacity) {
      const oldestEpisodeId = documents.keys().next().value as
        | number
        | undefined;
      if (oldestEpisodeId === undefined) break;
      documents.delete(oldestEpisodeId);
    }
  };

  const load = (episodeId: number): Promise<ShowNotesDocument> => {
    const cached = get(episodeId);
    if (cached) return Promise.resolve(cached);

    const pending = inFlight.get(episodeId);
    if (pending) return pending;

    let request: Promise<ShowNotesDocument>;
    request = loader(episodeId)
      .then((payload) => {
        if (
          payload.episode_id !== episodeId ||
          !isShowNotesDocument(payload.show_notes_document)
        ) {
          throw new Error("Episode Show Notes response identity mismatch");
        }
        remember(episodeId, payload.show_notes_document);
        return payload.show_notes_document;
      })
      .finally(() => {
        if (inFlight.get(episodeId) === request) {
          inFlight.delete(episodeId);
        }
      });
    inFlight.set(episodeId, request);
    return request;
  };

  return { get, load };
}
