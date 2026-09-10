"use client";

import { useEffect, useId, useRef, useState } from "react";
import { IconX } from "@tabler/icons-react";
import { ShowNotesDocumentView } from "@/components/common/ShowNotesDocumentView";
import type { ShowNotesDocument } from "@/types/showNotes";

interface EpisodeShowNotesProps {
  title?: string;
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
  title = "单集简介", summary, link, isExpanded, status,
  document: notesDocument, onToggle, onRetry, onOriginalOpen,
}: EpisodeShowNotesProps) {
  const titleId = useId();
  const dialogRef = useRef<HTMLDialogElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [closing, setClosing] = useState(false);

  useEffect(() => {
    if (!isExpanded) return;
    const dialog = dialogRef.current;
    if (!dialog) return;
    const oldOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    dialog.showModal();
    closeRef.current?.focus({ preventScroll: true });
    const trigger = triggerRef.current;
    return () => {
      if (closeTimer.current) clearTimeout(closeTimer.current);
      closeTimer.current = null;
      dialog.close();
      document.body.style.overflow = oldOverflow;
      requestAnimationFrame(() => {
        if (trigger?.isConnected) trigger.focus({ preventScroll: true });
      });
    };
  }, [isExpanded]);

  const close = () => {
    if (closeTimer.current) return;
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      onToggle();
      return;
    }
    setClosing(true);
    closeTimer.current = setTimeout(() => {
      closeTimer.current = null;
      onToggle();
    }, 200);
  };

  return (
    <div className="podcast-episode-show-notes">
      <p className="podcast-episode-show-notes-preview">{summary}</p>
      <button ref={triggerRef} type="button" className="podcast-episode-show-notes-toggle"
        aria-haspopup="dialog" onClick={() => { setClosing(false); onToggle(); }}>
        阅读简介
      </button>
      {isExpanded && (
        <dialog ref={dialogRef} className={`podcast-notes-dialog${closing ? " is-closing" : ""}`}
          aria-labelledby={titleId}
          onKeyDown={(event) => {
            if (event.key !== "Tab") return;
            const targets = Array.from(event.currentTarget.querySelectorAll<HTMLElement>(
              'button:not([disabled]), a[href], [tabindex="0"]',
            ));
            const first = targets[0];
            const last = targets[targets.length - 1];
            if (event.shiftKey && document.activeElement === first) {
              event.preventDefault();
              last?.focus();
            } else if (!event.shiftKey && document.activeElement === last) {
              event.preventDefault();
              first?.focus();
            }
          }}
          onCancel={(event) => { event.preventDefault(); close(); }}
          onClick={(event) => {
            if (event.target !== event.currentTarget) return;
            const rect = event.currentTarget.getBoundingClientRect();
            if (event.clientX < rect.left || event.clientX > rect.right ||
                event.clientY < rect.top || event.clientY > rect.bottom) close();
          }}>
          <header className="podcast-notes-dialog-heading">
            <div><p>单集简介</p><h2 id={titleId}>{title}</h2></div>
            <button ref={closeRef} type="button" aria-label="关闭简介" onClick={close}>
              <IconX size={20} aria-hidden="true" />
            </button>
          </header>
          <div className="podcast-notes-dialog-content" tabIndex={0} role="region" aria-label="完整 Show Notes">
            {status !== "success" && <p className="podcast-notes-dialog-summary">{summary}</p>}
            {status === "loading" && <p role="status">正在读取完整 Show Notes…</p>}
            {status === "error" && (
              <div className="podcast-episode-show-notes-state" role="alert">
                <p>完整 Show Notes 读取失败，预览仍可查看。</p>
                <button type="button" onClick={onRetry}>重试全文</button>
              </div>
            )}
            {status === "success" && notesDocument && (
              <ShowNotesDocumentView document={notesDocument} density="compact"
                className="podcast-episode-show-notes-content"
                emptyFallback={<p>该单集暂无完整 Show Notes。</p>} />
            )}
          </div>
        </dialog>
      )}
      {link && (
        <a href={link} target="_blank" rel="noopener noreferrer"
          className="podcast-episode-show-notes-link md:hidden" onClick={onOriginalOpen}>
          查看详情 →
        </a>
      )}
    </div>
  );
}
