"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { EpisodeShowNotesStore } from "@/lib/episodeShowNotesStore";
import type { ShowNotesDocument } from "@/types/showNotes";

const DESKTOP_SHOW_NOTES_QUERY = "(min-width: 768px)";

type LoadState = {
  episodeId: number;
  status: "idle" | "loading" | "success" | "error";
  document?: ShowNotesDocument;
};

function getDesktopMatch() {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia(DESKTOP_SHOW_NOTES_QUERY).matches
  );
}

export function useEpisodeShowNotes(
  episodeId: number,
  hasShowNotes: boolean,
  store: EpisodeShowNotesStore,
) {
  const episodeIdRef = useRef(episodeId);
  episodeIdRef.current = episodeId;
  const requestSequence = useRef(0);
  const [isDesktop, setIsDesktop] = useState(getDesktopMatch);
  const [isHovered, setIsHovered] = useState(false);
  const [isFocusWithin, setIsFocusWithin] = useState(false);
  const [state, setState] = useState<LoadState>(() => {
    const cached = store.get(episodeId);
    return cached
      ? { episodeId, status: "success", document: cached }
      : { episodeId, status: "idle" };
  });

  useEffect(() => {
    const media = window.matchMedia(DESKTOP_SHOW_NOTES_QUERY);
    const update = () => setIsDesktop(media.matches);
    update();
    if (media.addEventListener) {
      media.addEventListener("change", update);
      return () => media.removeEventListener("change", update);
    }
    media.addListener(update);
    return () => media.removeListener(update);
  }, []);

  useEffect(() => {
    requestSequence.current += 1;
    setIsHovered(false);
    setIsFocusWithin(false);
    const cached = store.get(episodeId);
    setState(
      cached
        ? { episodeId, status: "success", document: cached }
        : { episodeId, status: "idle" },
    );
  }, [episodeId, store]);

  const load = useCallback(async () => {
    if (!hasShowNotes || !isDesktop) return;
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
  }, [episodeId, hasShowNotes, isDesktop, store]);

  const isExpanded = isDesktop && (isHovered || isFocusWithin);
  const currentState = state.episodeId === episodeId ? state : undefined;

  useEffect(() => {
    if (isExpanded && (currentState?.status ?? "idle") === "idle") {
      void load();
    }
  }, [currentState?.status, isExpanded, load]);

  return {
    isExpanded,
    status: currentState?.status ?? "idle",
    document: currentState?.document,
    enterHover: () => {
      if (isDesktop && hasShowNotes) setIsHovered(true);
    },
    leaveHover: () => setIsHovered(false),
    enterFocus: () => {
      if (isDesktop && hasShowNotes) setIsFocusWithin(true);
    },
    leaveFocus: () => setIsFocusWithin(false),
    retry: load,
  };
}
