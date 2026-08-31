"use client";

import { ShowNotesDocumentView } from "@/components/common/ShowNotesDocumentView";
import type { ShowNotesDocument } from "@/types/showNotes";

interface EpisodeShowNotesProps {
  summary: string;
  link: string;
  isExpanded: boolean;
  status: "idle" | "loading" | "success" | "error";
  document?: ShowNotesDocument;
  onRetry: () => void;
  onOriginalOpen?: () => void;
}

export function EpisodeShowNotes({
  summary,
  link,
  isExpanded,
  status,
  document,
  onRetry,
  onOriginalOpen,
}: EpisodeShowNotesProps) {
  const preview = <p className="podcast-episode-show-notes-preview">{summary}</p>;

  return (
    <div className="podcast-episode-show-notes">
      {!isExpanded || status === "idle" ? preview : null}

      {isExpanded && status === "loading" && (
        <div className="podcast-episode-show-notes-state" aria-live="polite">
          {preview}
          <p role="status">正在读取完整 Show Notes…</p>
        </div>
      )}

      {isExpanded && status === "error" && (
        <div className="podcast-episode-show-notes-state" role="alert">
          {preview}
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
          tabIndex={0}
        >
          <ShowNotesDocumentView
            document={document}
            density="compact"
            className="podcast-episode-show-notes-content"
            emptyFallback={<p>该单集暂无完整 Show Notes。</p>}
          />
        </div>
      )}

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
