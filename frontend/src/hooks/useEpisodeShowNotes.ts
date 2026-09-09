"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { EpisodeShowNotesStore } from "@/lib/episodeShowNotesStore";
import type { ShowNotesDocument } from "@/types/showNotes";

type LoadState = {
  episodeId: number;
  status: "idle" | "loading" | "success" | "error";
  document?: ShowNotesDocument;
};

export function useEpisodeShowNotes(
  episodeId: number,
  hasShowNotes: boolean,
  store: EpisodeShowNotesStore,
) {
  const episodeIdRef = useRef(episodeId);
  episodeIdRef.current = episodeId;
  const requestSequence = useRef(0);
  const [isExpanded, setIsExpanded] = useState(false);
  const [state, setState] = useState<LoadState>(() => {
    const cached = store.get(episodeId);
    return cached
      ? { episodeId, status: "success", document: cached }
      : { episodeId, status: "idle" };
  });

  useEffect(() => {
    requestSequence.current += 1;
    setIsExpanded(false);
    const cached = store.get(episodeId);
    setState(
      cached
        ? { episodeId, status: "success", document: cached }
        : { episodeId, status: "idle" },
    );
  }, [episodeId, store]);

  const load = useCallback(async () => {
    if (!hasShowNotes) return;
    const cached = store.get(episodeId);
    if (cached) {
      setState({ episodeId, status: "success", document: cached });
      return;
    }

    const sequence = ++requestSequence.current;
    setState({ episodeId, status: "loading" });
    try {
      const document = await store.load(episodeId);
      if (
        episodeIdRef.current === episodeId &&
        requestSequence.current === sequence
      ) {
        setState({ episodeId, status: "success", document });
      }
    } catch {
      if (
        episodeIdRef.current === episodeId &&
        requestSequence.current === sequence
      ) {
        setState({ episodeId, status: "error" });
      }
    }
  }, [episodeId, hasShowNotes, store]);

  const currentState = state.episodeId === episodeId ? state : undefined;

  useEffect(() => {
    if (isExpanded && (currentState?.status ?? "idle") === "idle") {
      void load();
    }
  }, [currentState?.status, isExpanded, load]);

  const expand = useCallback(() => {
    if (!hasShowNotes) return;
    setIsExpanded(true);
  }, [hasShowNotes]);

  const collapse = useCallback(() => {
    setIsExpanded(false);
  }, []);

  const toggle = useCallback(() => {
    if (isExpanded) {
      collapse();
      return;
    }

    expand();
  }, [collapse, expand, isExpanded]);

  return {
    isExpanded,
    status: currentState?.status ?? "idle",
    document: currentState?.document,
    toggle,
    expand,
    collapse,
    retry: load,
  };
}
