"use client";

import { useId, useLayoutEffect, useRef, type TransitionEvent } from "react";
import { ShowNotesDocumentView } from "@/components/common/ShowNotesDocumentView";
import {
  getEpisodeShowNotesToggleLabel,
  shouldKeepEpisodeShowNotesPreview,
  syncEpisodeShowNotesBodyHeight,
} from "@/lib/episodeShowNotesUi";
import type { ShowNotesDocument } from "@/types/showNotes";

interface EpisodeShowNotesProps {
  summary: string;
  link: string;
  isExpanded: boolean;
  status: "idle" | "loading" | "success" | "error";
  document?: ShowNotesDocument;
  onToggle: () => void;
  onRetry: () => void;
  onOriginalOpen?: () => void;
}

export function EpisodeShowNotes({
  summary,
  link,
  isExpanded,
  status,
  document,
  onToggle,
  onRetry,
  onOriginalOpen,
}: EpisodeShowNotesProps) {
  const bodyId = useId();
  const bodyRef = useRef<HTMLDivElement>(null);
  const storedHeightRef = useRef(0);
  const preview = shouldKeepEpisodeShowNotesPreview(isExpanded, status) ? (
    <p className="podcast-episode-show-notes-preview">{summary}</p>
  ) : null;

  useLayoutEffect(() => {
    const reduceMotion =
      typeof window !== "undefined" &&
      typeof window.matchMedia === "function" &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches;

    storedHeightRef.current = syncEpisodeShowNotesBodyHeight(
      bodyRef.current,
      storedHeightRef.current,
      reduceMotion,
    );
  }, [document, isExpanded, status, summary]);

  const handleHeightTransitionEnd = (
    event: TransitionEvent<HTMLDivElement>,
  ) => {
    if (event.propertyName !== "height") {
      return;
    }

    event.currentTarget.style.height = "";
  };

  return (
    <div className="podcast-episode-show-notes">
      <div
        id={bodyId}
        ref={bodyRef}
        className={`podcast-episode-show-notes-body${
          isExpanded ? " is-expanded" : ""
        }`}
        onTransitionEnd={handleHeightTransitionEnd}
      >
        {preview}

        {isExpanded && status === "loading" && (
          <div className="podcast-episode-show-notes-state" aria-live="polite">
            <p role="status">正在读取完整 Show Notes…</p>
          </div>
        )}

        {isExpanded && status === "error" && (
          <div className="podcast-episode-show-notes-state" role="alert">
            <p>完整 Show Notes 读取失败，预览仍可查看。</p>
            <button type="button" onClick={onRetry}>
              重试全文
            </button>
          </div>
        )}

        {isExpanded && status === "success" && document && (
          <div
            className="podcast-episode-show-notes-reader"
            role="region"
            aria-label="完整 Show Notes"
          >
            <ShowNotesDocumentView
              document={document}
              density="compact"
              className="podcast-episode-show-notes-content"
              emptyFallback={<p>该单集暂无完整 Show Notes。</p>}
            />
          </div>
        )}
      </div>

      <button
        type="button"
        className="podcast-episode-show-notes-toggle"
        aria-expanded={isExpanded}
        aria-controls={bodyId}
        onClick={onToggle}
      >
        {getEpisodeShowNotesToggleLabel(isExpanded)}
      </button>

      {link && (
        <a
          href={link}
          target="_blank"
          rel="noopener noreferrer"
          className="podcast-episode-show-notes-link md:hidden"
          onClick={onOriginalOpen}
        >
          查看详情 →
        </a>
      )}
    </div>
  );
}
