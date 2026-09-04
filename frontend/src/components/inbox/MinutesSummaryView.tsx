"use client";

import { useEffect, useId, useRef, useState } from "react";
import MarkdownViewer from "@/components/workflows/MarkdownViewer";
import {
  getOptimizedImageUrl,
  RICH_TEXT_IMAGE_WIDTH,
} from "@/lib/imageOptimization";
import type {
  MinutesLink,
  MinutesQuote,
  MinutesVisualItem,
  MinutesWhiteboard,
} from "@/types/processing";
import styles from "./InboxPage.module.css";

interface MinutesSummaryViewProps {
  artifactSetId: number;
  content: string;
  mode?: "minutes" | "visual";
  keywords?: string[];
  decisions?: string[];
  quotes?: MinutesQuote[];
  links?: MinutesLink[];
  whiteboard?: MinutesWhiteboard;
  visualItems?: MinutesVisualItem[];
}

export default function MinutesSummaryView({
  artifactSetId,
  content,
  mode = "minutes",
  keywords = [],
  decisions = [],
  quotes = [],
  links = [],
  whiteboard,
  visualItems = [],
}: MinutesSummaryViewProps) {
  const visibleKeywords = keywords.map((item) => item.trim()).filter(Boolean);
  const visibleDecisions = decisions.map((item) => item.trim()).filter(Boolean);
  const visibleQuotes = quotes.filter((item) => item.quote.trim());
  const visibleLinks = links.filter((item) => isSafeMinutesLink(item.url));
  const visibleVisualItems = normalizeMinutesVisualItems(
    visualItems,
    whiteboard,
  );
  const [openMediaId, setOpenMediaId] = useState<string | null>(null);
  const [failedMediaIds, setFailedMediaIds] = useState<Set<string>>(
    () => new Set(),
  );
  const previewButtonRefs = useRef<Record<string, HTMLButtonElement | null>>(
    {},
  );
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const lightboxTriggerId = useRef<string | null>(null);
  const titleId = useId();
  const summaryContent = stripRedundantSummaryHeading(content);
  const isVisualMode = mode === "visual";
  const visualIdentity = visibleVisualItems
    .map((item) => item.media_id)
    .join(",");

  useEffect(() => {
    setFailedMediaIds(new Set());
  }, [artifactSetId, visualIdentity]);

  useEffect(() => {
    if (openMediaId) {
      lightboxTriggerId.current = openMediaId;
      closeButtonRef.current?.focus();
      const onKeyDown = (event: KeyboardEvent) => {
        if (event.key === "Escape") {
          event.preventDefault();
          event.stopPropagation();
          setOpenMediaId(null);
          return;
        }
        if (event.key === "Tab") {
          event.preventDefault();
          event.stopPropagation();
          closeButtonRef.current?.focus();
        }
      };
      window.addEventListener("keydown", onKeyDown, true);
      return () => window.removeEventListener("keydown", onKeyDown, true);
    }
    if (lightboxTriggerId.current) {
      previewButtonRefs.current[lightboxTriggerId.current]?.focus();
      lightboxTriggerId.current = null;
    }
    return undefined;
  }, [openMediaId]);

  useEffect(() => {
    if (!isVisualMode && openMediaId) {
      setOpenMediaId(null);
    }
  }, [isVisualMode, openMediaId]);

  const handleVisualError = (mediaId: string) => {
    setFailedMediaIds((current) => {
      if (current.has(mediaId)) return current;
      const next = new Set(current);
      next.add(mediaId);
      return next;
    });
  };

  const visualPreviews = visibleVisualItems.map((item) => {
    const src = managedMediaURL(artifactSetId, item.media_id);
    const previewSrc = getOptimizedImageUrl(src, RICH_TEXT_IMAGE_WIDTH);
    const isWhiteboard = item.type === "whiteboard";
    const alt =
      item.alt || (isWhiteboard ? "飞书智能纪要画板" : "飞书智能纪要图片");
    if (failedMediaIds.has(item.media_id)) {
      return (
        <figure className={styles.minutesWhiteboard} key={item.media_id}>
          <div className={styles.minutesVisualLoadError} role="status">
            {isWhiteboard ? "画板暂时无法加载" : "图片暂时无法加载"}
          </div>
        </figure>
      );
    }
    return (
      <figure className={styles.minutesWhiteboard} key={item.media_id}>
        <button
          ref={(element) => {
            previewButtonRefs.current[item.media_id] = element;
          }}
          type="button"
          className={styles.minutesWhiteboardButton}
          onClick={() => setOpenMediaId(item.media_id)}
          aria-haspopup="dialog"
          aria-expanded={openMediaId === item.media_id}
          aria-label={
            isWhiteboard ? `放大查看画板：${alt}` : `放大查看图片：${alt}`
          }
        >
          {/* Managed local artifact media; not a remote Next image. */}
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={previewSrc}
            alt={alt}
            width={item.width || undefined}
            height={item.height || undefined}
            onError={() => handleVisualError(item.media_id)}
          />
          <span>{isWhiteboard ? "放大查看画板" : "放大查看图片"}</span>
        </button>
        {item.summary ? <figcaption>{item.summary}</figcaption> : null}
      </figure>
    );
  });

  return (
    <div
      className={`${styles.minutesSummary} ${
        isVisualMode ? styles.minutesVisualSummary : ""
      }`}
    >
      {isVisualMode ? (
        <section
          className={styles.minutesSection}
          aria-labelledby={`${titleId}-visual-summary`}
        >
          <h2
            id={`${titleId}-visual-summary`}
            className={styles.minutesSummaryTitle}
          >
            总结
          </h2>
          {visualPreviews.length > 0 ? (
            <div className={styles.minutesVisualList}>{visualPreviews}</div>
          ) : null}
        </section>
      ) : (
        <>
          <header className={styles.minutesMasthead}>
            <p className={styles.minutesAiNotice} role="note">
              飞书妙记 · 内容由 AI 生成，可能不准确
            </p>
            {visibleKeywords.length > 0 && (
              <div className={styles.minutesKeywords}>
                <span aria-hidden="true">关键词</span>
                <ul className={styles.minutesKeywordList} aria-label="关键词">
                  {visibleKeywords.map((keyword) => (
                    <li key={keyword}>{keyword}</li>
                  ))}
                </ul>
              </div>
            )}
          </header>
          <section
            className={styles.minutesSection}
            aria-labelledby={`${titleId}-summary`}
          >
            <h2
              id={`${titleId}-summary`}
              className={styles.minutesSummaryTitle}
            >
              总结
            </h2>
            <MarkdownViewer content={summaryContent} />
          </section>
          {visibleDecisions.length > 0 && (
            <section
              className={styles.minutesSection}
              aria-labelledby={`${titleId}-decisions`}
            >
              <h2
                id={`${titleId}-decisions`}
                className={styles.minutesSummaryTitle}
              >
                关键决策
              </h2>
              <ol className={styles.minutesDecisionList}>
                {visibleDecisions.map((decision) => (
                  <li key={decision}>{decision}</li>
                ))}
              </ol>
            </section>
          )}
          {visibleQuotes.length > 0 && (
            <section
              className={styles.minutesSection}
              aria-labelledby={`${titleId}-quotes`}
            >
              <h2
                id={`${titleId}-quotes`}
                className={styles.minutesSummaryTitle}
              >
                金句时刻
              </h2>
              <ul className={styles.minutesQuoteList}>
                {visibleQuotes.map((item) => (
                  <li key={item.quote}>
                    <blockquote>
                      <p>{item.quote}</p>
                    </blockquote>
                    {item.explanation?.trim() ? (
                      <p className={styles.minutesQuoteExplanation}>
                        {item.explanation}
                      </p>
                    ) : null}
                  </li>
                ))}
              </ul>
            </section>
          )}
          {visibleLinks.length > 0 && (
            <footer
              className={styles.minutesSection}
              aria-labelledby={`${titleId}-links`}
            >
              <h2
                id={`${titleId}-links`}
                className={styles.minutesSummaryTitle}
              >
                相关链接
              </h2>
              <ul className={styles.minutesLinkList}>
                {visibleLinks.map((link) => (
                  <li key={link.url}>
                    <a
                      href={link.url}
                      rel="noreferrer noopener"
                      target="_blank"
                    >
                      {safeMinutesLinkTitle(link.title, link.url)}
                    </a>
                  </li>
                ))}
              </ul>
            </footer>
          )}
        </>
      )}
      {isVisualMode &&
        openMediaId &&
        visibleVisualItems.some((item) => item.media_id === openMediaId) && (
          <div
            className={styles.minutesLightbox}
            role="dialog"
            aria-modal="true"
            aria-label={
              visibleVisualItems.find((item) => item.media_id === openMediaId)
                ?.type === "whiteboard"
                ? "画板预览"
                : "图片预览"
            }
          >
            <div className={styles.minutesLightboxCard}>
              <div className={styles.minutesLightboxScroll}>
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src={managedMediaURL(artifactSetId, openMediaId)}
                  alt={
                    visibleVisualItems.find(
                      (item) => item.media_id === openMediaId,
                    )?.alt || "飞书智能纪要图片"
                  }
                  onError={() => {
                    handleVisualError(openMediaId);
                    setOpenMediaId(null);
                  }}
                />
              </div>
              <button
                ref={closeButtonRef}
                type="button"
                className={styles.minutesLightboxClose}
                onClick={() => setOpenMediaId(null)}
              >
                关闭
              </button>
            </div>
          </div>
        )}
    </div>
  );
}

function normalizeMinutesVisualItems(
  items: MinutesVisualItem[],
  whiteboard?: MinutesWhiteboard,
) {
  const normalized: MinutesVisualItem[] = [];
  const seen = new Set<string>();
  const add = (item: MinutesVisualItem) => {
    if (
      !item ||
      (item.type !== "image" && item.type !== "whiteboard") ||
      !isSafeManagedMediaId(item.media_id) ||
      seen.has(item.media_id)
    ) {
      return;
    }
    seen.add(item.media_id);
    normalized.push(item);
  };
  if (whiteboard) {
    add({ ...whiteboard, type: "whiteboard" });
  }
  items.forEach(add);
  const boardIndex = normalized.findIndex((item) => item.type === "whiteboard");
  if (boardIndex > 0) {
    const [board] = normalized.splice(boardIndex, 1);
    normalized.unshift(board);
  }
  return normalized;
}

function isSafeManagedMediaId(mediaId: string) {
  return /^[a-z][a-z0-9_-]{1,63}$/.test(mediaId);
}

function managedMediaURL(artifactSetId: number, mediaId: string) {
  if (!isSafeManagedMediaId(mediaId)) return "";
  return `/api/v1/artifact-sets/${artifactSetId}/media/${encodeURIComponent(mediaId)}`;
}

export function stripRedundantSummaryHeading(content: string) {
  return content.replace(
    /^\s*#\s*(纪要|总结|飞书智能纪要|妙记原生纪要)\s*\n+/,
    "",
  );
}

export function isSafeMinutesLink(raw: string) {
  try {
    const parsed = new URL(raw);
    if (parsed.protocol !== "https:" || parsed.username || parsed.password) {
      return false;
    }
    const host = parsed.hostname.toLowerCase();
    if (
      isLocalMinutesHost(host) ||
      /(^|\.)(feishu\.cn|feishu\.com|larksuite\.com|larksuite\.cn|larkoffice\.com|larkoffice\.cn)$/.test(
        host,
      )
    ) {
      return false;
    }
    const haystack =
      `${parsed.pathname}?${parsed.search}#${parsed.hash}`.toLowerCase();
    if (
      /(minute_token|note_id|file_token|whiteboard_token|doc_token|token=|(?:^|[^a-z0-9])(?:(?:obcn|wbcn|boxcn|doxcn)[a-z0-9_-]{4,}|docx_[a-z0-9_-]{4,}))/.test(
        haystack,
      )
    ) {
      return false;
    }
    let decoded = haystack;
    for (let pass = 0; pass < 8; pass += 1) {
      const next = decodeMinutesURLLayer(decoded);
      if (next === decoded) break;
      decoded = next.toLowerCase();
    }
    if (
      /(?:^|[^a-z0-9])(?:(?:obcn|wbcn|boxcn|doxcn)[a-z0-9_-]{4,}|docx_[a-z0-9_-]{4,})/.test(
        decoded,
      ) ||
      /(?:^|[^a-z])(?:javascript|data|file):/.test(decoded)
    ) {
      return false;
    }
    for (const nested of decoded.matchAll(/https?:\/\/[^\s<>"']+/g)) {
      try {
        const nestedURL = new URL(nested[0]);
        const nestedHost = nestedURL.hostname.toLowerCase();
        if (
          isLocalMinutesHost(nestedHost) ||
          /(^|\.)(feishu\.cn|feishu\.com|larksuite\.com|larksuite\.cn|larkoffice\.com|larkoffice\.cn)$/.test(
            nestedHost,
          )
        ) {
          return false;
        }
      } catch {
        return false;
      }
    }
    if (
      /\/(minutes|docx|wiki|drive|whiteboard|notes)\//.test(parsed.pathname)
    ) {
      return false;
    }
    return true;
  } catch {
    return false;
  }
}

export function safeMinutesLinkTitle(title: string, safeURL: string) {
  const candidate = title.trim();
  if (!candidate) return safeURL;
  let decoded = candidate;
  for (let pass = 0; pass < 8; pass += 1) {
    const next = decodeMinutesURLLayer(decoded);
    if (next === decoded) break;
    decoded = next;
  }
  if (
    /(minute_token|note_id|file_token|whiteboard_token|doc_token|(?:^|[^a-z0-9])(?:(?:obcn|wbcn|boxcn|doxcn)[a-z0-9_-]{4,}|docx_[a-z0-9_-]{4,})|\/(?:tmp|private\/var)\/|(?:^|[\s/:.])(localhost|127\.0\.0\.1|::1)(?:[\s/:.]|$)|(?:^|\.)((?:feishu|larksuite|larkoffice)\.(?:cn|com)))/i.test(
      decoded,
    )
  ) {
    return safeURL;
  }
  return candidate;
}

function decodeMinutesURLLayer(value: string) {
  return value.replace(/(?:%[0-9a-f]{2})+/gi, (encoded) => {
    try {
      return decodeURIComponent(encoded);
    } catch {
      return encoded;
    }
  });
}

function isLocalMinutesHost(hostname: string) {
  const host = hostname.toLowerCase().replace(/^\[|\]$/g, "");
  if (
    host === "localhost" ||
    host.endsWith(".localhost") ||
    host.endsWith(".local") ||
    host.endsWith(".internal") ||
    host.endsWith(".home.arpa") ||
    host === "::1" ||
    host === "::"
  ) {
    return true;
  }
  const octets = host.split(".").map(Number);
  if (octets.length !== 4 || octets.some((part) => !Number.isInteger(part))) {
    return /^(?:fc|fd|fe[89ab])/i.test(host);
  }
  return (
    octets[0] === 0 ||
    octets[0] === 10 ||
    octets[0] === 127 ||
    (octets[0] === 169 && octets[1] === 254) ||
    (octets[0] === 172 && octets[1] >= 16 && octets[1] <= 31) ||
    (octets[0] === 192 && octets[1] === 168)
  );
}
