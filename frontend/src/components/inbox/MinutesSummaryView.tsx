"use client";

import { useEffect, useId, useRef, useState } from "react";
import MarkdownViewer from "@/components/workflows/MarkdownViewer";
import type {
  MinutesChapter,
  MinutesLink,
  MinutesQuote,
  MinutesWhiteboard,
} from "@/types/processing";
import styles from "./InboxPage.module.css";

const COLLAPSED_CHAPTER_COUNT = 6;

interface MinutesSummaryViewProps {
  artifactSetId: number;
  content: string;
  chapters?: MinutesChapter[];
  keywords?: string[];
  decisions?: string[];
  quotes?: MinutesQuote[];
  links?: MinutesLink[];
  whiteboard?: MinutesWhiteboard;
  onChapterSelect?: (startMs: number) => void;
}

export default function MinutesSummaryView({
  artifactSetId,
  content,
  chapters = [],
  keywords = [],
  decisions = [],
  quotes = [],
  links = [],
  whiteboard,
  onChapterSelect,
}: MinutesSummaryViewProps) {
  const visibleChapters = chapters.filter(
    (chapter) => chapter.title.trim() || chapter.summary?.trim(),
  );
  const visibleKeywords = keywords.map((item) => item.trim()).filter(Boolean);
  const visibleDecisions = decisions.map((item) => item.trim()).filter(Boolean);
  const visibleQuotes = quotes.filter((item) => item.quote.trim());
  const visibleLinks = links.filter((item) => isSafeMinutesLink(item.url));
  const [chaptersExpanded, setChaptersExpanded] = useState(false);
  const [whiteboardOpen, setWhiteboardOpen] = useState(false);
  const previewButtonRef = useRef<HTMLButtonElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const lightboxWasOpen = useRef(false);
  const titleId = useId();
  const collapsed =
    !chaptersExpanded && visibleChapters.length > COLLAPSED_CHAPTER_COUNT;
  const shownChapters = collapsed
    ? visibleChapters.slice(0, COLLAPSED_CHAPTER_COUNT)
    : visibleChapters;

  useEffect(() => {
    if (whiteboardOpen) {
      lightboxWasOpen.current = true;
      closeButtonRef.current?.focus();
      const onKeyDown = (event: KeyboardEvent) => {
        if (event.key === "Escape") {
          event.preventDefault();
          setWhiteboardOpen(false);
        }
      };
      window.addEventListener("keydown", onKeyDown);
      return () => window.removeEventListener("keydown", onKeyDown);
    }
    if (lightboxWasOpen.current) {
      previewButtonRef.current?.focus();
      lightboxWasOpen.current = false;
    }
    return undefined;
  }, [whiteboardOpen]);

  const whiteboardSrc = whiteboard
    ? `/api/v1/artifact-sets/${artifactSetId}/media/${encodeURIComponent(
        whiteboard.media_id,
      )}`
    : "";

  return (
    <div className={styles.minutesSummary}>
      <p className={styles.minutesAiNotice} role="note">
        内容由飞书 AI 生成，可能不准确。
      </p>
      {whiteboard && (
        <figure className={styles.minutesWhiteboard}>
          <button
            ref={previewButtonRef}
            type="button"
            className={styles.minutesWhiteboardButton}
            onClick={() => setWhiteboardOpen(true)}
            aria-haspopup="dialog"
            aria-expanded={whiteboardOpen}
          >
            {/* Managed local artifact media; not a remote Next image. */}
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src={whiteboardSrc}
              alt={whiteboard.alt || "飞书智能纪要画板"}
              width={whiteboard.width || undefined}
              height={whiteboard.height || undefined}
            />
            <span>放大查看画板</span>
          </button>
        </figure>
      )}
      <MarkdownViewer content={content} />
      {shownChapters.length > 0 && (
        <section className={styles.minutesSection} aria-labelledby={`${titleId}-chapters`}>
          <h3 id={`${titleId}-chapters`}>智能章节</h3>
          <ol className={styles.minutesChapterList}>
            {shownChapters.map((chapter) => (
              <li key={`${chapter.order}-${chapter.start_ms}`}>
                <button
                  type="button"
                  className={styles.minutesChapter}
                  onClick={() => onChapterSelect?.(chapter.start_ms)}
                >
                  <span className={styles.minutesChapterTime}>
                    {formatChapterTime(chapter.start_ms)}
                  </span>
                  <span className={styles.minutesChapterBody}>
                    <strong>{chapter.title || "未命名章节"}</strong>
                    {chapter.summary?.trim() ? <span>{chapter.summary}</span> : null}
                  </span>
                </button>
              </li>
            ))}
          </ol>
          {visibleChapters.length > COLLAPSED_CHAPTER_COUNT && (
            <button
              type="button"
              className={styles.minutesExpand}
              aria-expanded={chaptersExpanded}
              onClick={() => setChaptersExpanded((current) => !current)}
            >
              {chaptersExpanded
                ? "收起章节"
                : `展开全部 ${visibleChapters.length} 个章节`}
            </button>
          )}
        </section>
      )}
      {visibleDecisions.length > 0 && (
        <section className={styles.minutesSection} aria-labelledby={`${titleId}-decisions`}>
          <h3 id={`${titleId}-decisions`}>关键决策</h3>
          <ul className={styles.minutesDecisionList}>
            {visibleDecisions.map((decision) => (
              <li key={decision}>{decision}</li>
            ))}
          </ul>
        </section>
      )}
      {visibleQuotes.length > 0 && (
        <section className={styles.minutesSection} aria-labelledby={`${titleId}-quotes`}>
          <h3 id={`${titleId}-quotes`}>金句</h3>
          <ul className={styles.minutesQuoteList}>
            {visibleQuotes.map((item) => (
              <li key={item.quote}>
                <blockquote>
                  <p>{item.quote}</p>
                </blockquote>
                {item.explanation?.trim() ? (
                  <p className={styles.minutesQuoteExplanation}>{item.explanation}</p>
                ) : null}
              </li>
            ))}
          </ul>
        </section>
      )}
      {visibleKeywords.length > 0 && (
        <section className={styles.minutesSection} aria-labelledby={`${titleId}-keywords`}>
          <h3 id={`${titleId}-keywords`}>关键词</h3>
          <ul className={styles.minutesKeywordList} aria-label="关键词">
            {visibleKeywords.map((keyword) => (
              <li key={keyword}>{keyword}</li>
            ))}
          </ul>
        </section>
      )}
      {visibleLinks.length > 0 && (
        <section className={styles.minutesSection} aria-labelledby={`${titleId}-links`}>
          <h3 id={`${titleId}-links`}>相关链接</h3>
          <ul className={styles.minutesLinkList}>
            {visibleLinks.map((link) => (
              <li key={link.url}>
                <a href={link.url} rel="noreferrer noopener" target="_blank">
                  {link.title.trim() || link.url}
                </a>
              </li>
            ))}
          </ul>
        </section>
      )}
      {whiteboardOpen && whiteboard && (
        <div
          className={styles.minutesLightbox}
          role="dialog"
          aria-modal="true"
          aria-label="画板预览"
        >
          <div className={styles.minutesLightboxCard}>
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src={whiteboardSrc}
              alt={whiteboard.alt || "飞书智能纪要画板"}
            />
            <button
              ref={closeButtonRef}
              type="button"
              className={styles.minutesLightboxClose}
              onClick={() => setWhiteboardOpen(false)}
            >
              关闭
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

export function formatChapterTime(startMs: number) {
  const totalSeconds = Math.max(0, Math.floor(startMs / 1000));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (hours > 0) {
    return `${hours}:${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
  }
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}

export function isSafeMinutesLink(raw: string) {
  try {
    const parsed = new URL(raw);
    if (parsed.protocol !== "https:" || parsed.username || parsed.password) {
      return false;
    }
    const host = parsed.hostname.toLowerCase();
    if (
      /(^|\.)(feishu\.cn|feishu\.com|larksuite\.com|larksuite\.cn|larkoffice\.com|larkoffice\.cn)$/.test(
        host,
      )
    ) {
      return false;
    }
    const haystack = `${parsed.pathname}?${parsed.search}#${parsed.hash}`.toLowerCase();
    if (
      /(minute_token|note_id|file_token|whiteboard_token|doc_token|token=)/.test(
        haystack,
      )
    ) {
      return false;
    }
    if (/\/(minutes|docx|wiki|drive|whiteboard|notes)\//.test(parsed.pathname)) {
      return false;
    }
    return true;
  } catch {
    return false;
  }
}
