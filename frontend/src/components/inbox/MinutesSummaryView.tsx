"use client";

import {
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
  type RefObject,
} from "react";
import { createPortal } from "react-dom";
import MarkdownViewer from "@/components/workflows/MarkdownViewer";
import {
  getOptimizedImageUrl,
  RICH_TEXT_IMAGE_WIDTH,
} from "@/lib/imageOptimization";
import { acquireDocumentScrollLock } from "@/lib/documentScrollLock";
import type {
  MinutesInlineImage,
  MinutesLink,
  MinutesQuote,
  MinutesVisualItem,
  MinutesWhiteboard,
} from "@/types/processing";
import styles from "./InboxPage.module.css";

const MIN_LIGHTBOX_ZOOM = 1;
const MAX_LIGHTBOX_ZOOM = 4;
const LIGHTBOX_ZOOM_STEP = 0.25;

interface LightboxPan {
  x: number;
  y: number;
}

interface LightboxPointer {
  x: number;
  y: number;
}

interface LightboxSize {
  width: number;
  height: number;
}

type LightboxGesture =
  | {
      kind: "pan";
      pointerId: number;
      startX: number;
      startY: number;
      origin: LightboxPan;
    }
  | {
      kind: "pinch";
      startDistance: number;
      startZoom: number;
    };

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
  inlineImages?: MinutesInlineImage[];
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
  inlineImages = [],
}: MinutesSummaryViewProps) {
  const visibleKeywords = keywords.map((item) => item.trim()).filter(Boolean);
  const visibleDecisions = decisions.map((item) => item.trim()).filter(Boolean);
  const visibleQuotes = quotes.filter((item) => item.quote.trim());
  const visibleLinks = links.filter((item) => isSafeMinutesLink(item.url));
  const managedVisualItems = normalizeMinutesVisualItems(
    visualItems,
    whiteboard,
  );
  const isVisualMode = mode === "visual";
  const visibleVisualItems = managedVisualItems.filter(
    (item) => item.type === "whiteboard",
  );
  const visibleInlineImages = normalizeMinutesInlineImages(
    inlineImages,
    managedVisualItems,
  );
  const [openMediaId, setOpenMediaId] = useState<string | null>(null);
  const [failedMediaIds, setFailedMediaIds] = useState<Set<string>>(
    () => new Set(),
  );
  const previewButtonRefs = useRef<Record<string, HTMLButtonElement | null>>(
    {},
  );
  const lightboxRef = useRef<HTMLDivElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const lightboxTriggerId = useRef<string | null>(null);
  const titleId = useId();
  const lightboxId = `${titleId}-lightbox`;
  const summaryContent = stripRedundantSummaryHeading(content);
  const summaryInlineImages = visibleInlineImages.filter(
    (image) =>
      !image.appendAtEnd &&
      (image.section === undefined || image.section === "summary"),
  );
  const decisionImagePlacement = placeMinutesInlineImages(
    visibleDecisions,
    visibleInlineImages.filter((image) => image.section === "decisions"),
  );
  const quoteImagePlacement = placeMinutesInlineImages(
    visibleQuotes.map((item) =>
      [item.quote, item.explanation?.trim()].filter(Boolean).join(" "),
    ),
    visibleInlineImages.filter((image) => image.section === "quotes"),
  );
  const linkImagePlacement = placeMinutesInlineImages(
    visibleLinks.map((item) => `${item.title} ${item.url}`),
    visibleInlineImages.filter((image) => image.section === "links"),
  );
  const trailingInlineImages = visibleInlineImages.filter(
    (image) =>
      image.appendAtEnd ||
      image.section === "body" ||
      image.section === "chapters",
  );
  const minutesContent = injectMinutesInlineImages(
    summaryContent,
    summaryInlineImages,
    artifactSetId,
  );
  const visualIdentity = visibleVisualItems
    .map((item) => item.media_id)
    .concat(visibleInlineImages.map((image) => image.item.media_id))
    .join(",");
  const isOpenMediaVisible = openMediaId
    ? managedVisualItems.some(
        (item) =>
          item.media_id === openMediaId &&
          (isVisualMode ? item.type === "whiteboard" : item.type === "image"),
      )
    : false;

  useEffect(() => {
    setFailedMediaIds(new Set());
  }, [artifactSetId, visualIdentity]);

  useEffect(() => {
    if (openMediaId && !isOpenMediaVisible) {
      setOpenMediaId(null);
    }
  }, [isOpenMediaVisible, openMediaId]);

  useEffect(() => {
    if (!openMediaId || !isOpenMediaVisible) {
      if (lightboxTriggerId.current) {
        previewButtonRefs.current[lightboxTriggerId.current]?.focus();
        lightboxTriggerId.current = null;
      }
      return undefined;
    }

    lightboxTriggerId.current = openMediaId;
    closeButtonRef.current?.focus();
    const releaseScrollLock = acquireDocumentScrollLock({
      lockDocumentElement: true,
      disableTouchAction: true,
    });

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        setOpenMediaId(null);
        return;
      }
      if (event.key !== "Tab") return;
      const dialog = lightboxRef.current;
      if (!dialog || !dialog.contains(event.target as Node)) return;
      const focusable = Array.from(
        dialog.querySelectorAll<HTMLElement>(
          'button:not(:disabled), [href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])',
        ),
      ).filter((element) => element.getAttribute("aria-hidden") !== "true");
      if (focusable.length === 0) return;
      event.preventDefault();
      event.stopPropagation();
      const currentIndex = focusable.indexOf(
        document.activeElement as HTMLElement,
      );
      const direction = event.shiftKey ? -1 : 1;
      const nextIndex =
        currentIndex < 0
          ? 0
          : (currentIndex + direction + focusable.length) % focusable.length;
      focusable[nextIndex].focus();
    };
    window.addEventListener("keydown", onKeyDown, true);
    return () => {
      window.removeEventListener("keydown", onKeyDown, true);
      releaseScrollLock();
    };
  }, [artifactSetId, isOpenMediaVisible, openMediaId]);
  const handleVisualError = (mediaId: string) => {
    setFailedMediaIds((current) => {
      if (current.has(mediaId)) return current;
      const next = new Set(current);
      next.add(mediaId);
      return next;
    });
  };

  const inlineVisualsByMediaID = new Map(
    visibleInlineImages.map((image) => [image.item.media_id, image.item]),
  );
  const renderInlineImageItem = (item: MinutesVisualItem, alt = "") => {
    const src = managedMediaURL(artifactSetId, item.media_id);
    if (failedMediaIds.has(item.media_id)) {
      return (
        <span
          className={styles.minutesInlineImage}
          key={item.media_id}
          role="status"
        >
          图片暂时无法加载
        </span>
      );
    }
    return (
      <span className={styles.minutesInlineImage} key={item.media_id}>
        <button
          ref={(element) => {
            previewButtonRefs.current[item.media_id] = element;
          }}
          type="button"
          className={styles.minutesInlineImageButton}
          onClick={() => setOpenMediaId(item.media_id)}
          aria-haspopup="dialog"
          aria-expanded={openMediaId === item.media_id}
          aria-controls={openMediaId === item.media_id ? lightboxId : undefined}
          aria-label={`放大查看图片：${item.alt || alt || "飞书智能纪要图片"}`}
        >
          {/* Managed local artifact media; not a remote Next image. */}
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={getOptimizedImageUrl(src, RICH_TEXT_IMAGE_WIDTH)}
            alt={item.alt || alt || "飞书智能纪要图片"}
            width={item.width || undefined}
            height={item.height || undefined}
            loading="lazy"
            decoding="async"
            onError={() => handleVisualError(item.media_id)}
          />
          <span>放大查看图片</span>
        </button>
        {item.summary ? (
          <span className={styles.minutesInlineImageCaption}>
            {item.summary}
          </span>
        ) : null}
      </span>
    );
  };
  const renderInlineImage = ({ src, alt }: { src: string; alt: string }) => {
    const mediaID = managedMediaIDFromURL(src);
    const item = mediaID ? inlineVisualsByMediaID.get(mediaID) : undefined;
    return item ? renderInlineImageItem(item, alt) : undefined;
  };
  const renderInlineImages = (images: VisibleMinutesInlineImage[]) =>
    images.map((image) => renderInlineImageItem(image.item));

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
          aria-controls={openMediaId === item.media_id ? lightboxId : undefined}
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

  const openMedia = openMediaId && isOpenMediaVisible
    ? managedVisualItems.find((item) => item.media_id === openMediaId)
    : undefined;

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
            <MarkdownViewer
              content={minutesContent}
              renderImage={isVisualMode ? undefined : renderInlineImage}
            />
          </section>
          {(visibleDecisions.length > 0 ||
            hasMinutesImagePlacement(decisionImagePlacement)) && (
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
              {renderInlineImages(decisionImagePlacement.before)}
              {visibleDecisions.length > 0 ? (
                <ol className={styles.minutesDecisionList}>
                  {visibleDecisions.map((decision, index) => (
                    <li key={decision}>
                      {decision}
                      {renderInlineImages(
                        decisionImagePlacement.after.get(index) ?? [],
                      )}
                    </li>
                  ))}
                </ol>
              ) : null}
              {renderInlineImages(decisionImagePlacement.trailing)}
            </section>
          )}
          {(visibleQuotes.length > 0 ||
            hasMinutesImagePlacement(quoteImagePlacement)) && (
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
              {renderInlineImages(quoteImagePlacement.before)}
              {visibleQuotes.length > 0 ? (
                <ul className={styles.minutesQuoteList}>
                  {visibleQuotes.map((item, index) => (
                    <li key={item.quote}>
                      <blockquote>
                        <p>{item.quote}</p>
                      </blockquote>
                      {item.explanation?.trim() ? (
                        <p className={styles.minutesQuoteExplanation}>
                          {item.explanation}
                        </p>
                      ) : null}
                      {renderInlineImages(
                        quoteImagePlacement.after.get(index) ?? [],
                      )}
                    </li>
                  ))}
                </ul>
              ) : null}
              {renderInlineImages(quoteImagePlacement.trailing)}
            </section>
          )}
          {(visibleLinks.length > 0 ||
            hasMinutesImagePlacement(linkImagePlacement)) && (
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
              {renderInlineImages(linkImagePlacement.before)}
              {visibleLinks.length > 0 ? (
                <ul className={styles.minutesLinkList}>
                  {visibleLinks.map((link, index) => (
                    <li key={link.url}>
                      <a
                        href={link.url}
                        rel="noreferrer noopener"
                        target="_blank"
                      >
                        {safeMinutesLinkTitle(link.title, link.url)}
                      </a>
                      {renderInlineImages(
                        linkImagePlacement.after.get(index) ?? [],
                      )}
                    </li>
                  ))}
                </ul>
              ) : null}
              {renderInlineImages(linkImagePlacement.trailing)}
            </footer>
          )}
          {trailingInlineImages.length > 0 ? (
            <section
              className={styles.minutesSection}
              aria-label="纪要正文图片"
            >
              {renderInlineImages(trailingInlineImages)}
            </section>
          ) : null}
        </>
      )}
      {openMedia
        ? (() => {
            const lightbox = (
              <MinutesLightbox
                key={managedMediaURL(artifactSetId, openMedia.media_id)}
                alt={
                  openMedia.alt ||
                  (openMedia.type === "whiteboard"
                    ? "飞书智能纪要画板"
                    : "飞书智能纪要图片")
                }
                closeButtonRef={closeButtonRef}
                dialogRef={lightboxRef}
                id={lightboxId}
                isWhiteboard={openMedia.type === "whiteboard"}
                onClose={() => setOpenMediaId(null)}
                onError={() => {
                  handleVisualError(openMedia.media_id);
                  setOpenMediaId(null);
                }}
                src={managedMediaURL(artifactSetId, openMedia.media_id)}
              />
            );
            return typeof document === "undefined"
              ? lightbox
              : createPortal(lightbox, document.body);
          })()
        : null}
    </div>
  );
}

interface MinutesLightboxProps {
  alt: string;
  closeButtonRef: RefObject<HTMLButtonElement>;
  dialogRef: RefObject<HTMLDivElement>;
  id: string;
  isWhiteboard: boolean;
  onClose: () => void;
  onError: () => void;
  src: string;
}

function MinutesLightbox({
  alt,
  closeButtonRef,
  dialogRef,
  id,
  isWhiteboard,
  onClose,
  onError,
  src,
}: MinutesLightboxProps) {
  const viewportRef = useRef<HTMLDivElement>(null);
  const imageRef = useRef<HTMLImageElement>(null);
  const [zoom, setZoom] = useState(MIN_LIGHTBOX_ZOOM);
  const [pan, setPan] = useState<LightboxPan>({ x: 0, y: 0 });
  const zoomRef = useRef(zoom);
  const panRef = useRef(pan);
  const pointersRef = useRef(new Map<number, LightboxPointer>());
  const gestureRef = useRef<LightboxGesture | null>(null);
  const wheelHandlerRef = useRef<(event: WheelEvent) => void>(() => undefined);

  useEffect(() => {
    zoomRef.current = zoom;
  }, [zoom]);

  useEffect(() => {
    panRef.current = pan;
  }, [pan]);

  const updatePan = (next: LightboxPan) => {
    panRef.current = next;
    setPan(next);
  };

  const constrainPan = (next: LightboxPan, nextZoom: number) => {
    if (nextZoom <= MIN_LIGHTBOX_ZOOM) return { x: 0, y: 0 };
    const viewport = viewportRef.current;
    const image = imageRef.current;
    if (!viewport || !image) return next;
    const containedImage = getContainedImageSize(image, viewport);
    const maxX = Math.max(
      0,
      (containedImage.width * nextZoom - viewport.clientWidth) / 2,
    );
    const maxY = Math.max(
      0,
      (containedImage.height * nextZoom - viewport.clientHeight) / 2,
    );
    return {
      x: Math.min(maxX, Math.max(-maxX, next.x)),
      y: Math.min(maxY, Math.max(-maxY, next.y)),
    };
  };

  const updateZoom = (next: number, anchor?: LightboxPan) => {
    const current = zoomRef.current;
    const clamped = Math.min(
      MAX_LIGHTBOX_ZOOM,
      Math.max(MIN_LIGHTBOX_ZOOM, Number(next.toFixed(2))),
    );
    if (clamped === current && !anchor) return;
    if (clamped <= MIN_LIGHTBOX_ZOOM) {
      zoomRef.current = MIN_LIGHTBOX_ZOOM;
      panRef.current = { x: 0, y: 0 };
      setZoom(MIN_LIGHTBOX_ZOOM);
      setPan({ x: 0, y: 0 });
      return;
    }

    let nextPan = panRef.current;
    if (anchor) {
      const ratio = clamped / current;
      nextPan = {
        x: anchor.x - (anchor.x - nextPan.x) * ratio,
        y: anchor.y - (anchor.y - nextPan.y) * ratio,
      };
    }
    nextPan = constrainPan(nextPan, clamped);
    zoomRef.current = clamped;
    panRef.current = nextPan;
    setZoom(clamped);
    setPan(nextPan);
  };

  const resetZoom = () => updateZoom(MIN_LIGHTBOX_ZOOM);

  const handleWheel = (event: WheelEvent) => {
    event.preventDefault();
    const viewport = viewportRef.current;
    if (!viewport) return;
    const rect = viewport.getBoundingClientRect();
    const clientX = Number.isFinite(event.clientX)
      ? event.clientX
      : rect.left + rect.width / 2;
    const clientY = Number.isFinite(event.clientY)
      ? event.clientY
      : rect.top + rect.height / 2;
    const anchor = {
      x: clientX - (rect.left + rect.width / 2),
      y: clientY - (rect.top + rect.height / 2),
    };
    updateZoom(
      zoomRef.current +
        (event.deltaY < 0 ? LIGHTBOX_ZOOM_STEP : -LIGHTBOX_ZOOM_STEP),
      anchor,
    );
  };

  useEffect(() => {
    wheelHandlerRef.current = handleWheel;
  });

  useEffect(() => {
    const viewport = viewportRef.current;
    if (!viewport) return undefined;
    const onWheel = (event: WheelEvent) => wheelHandlerRef.current(event);
    viewport.addEventListener("wheel", onWheel, { passive: false });
    return () => viewport.removeEventListener("wheel", onWheel);
  }, []);

  useEffect(() => {
    const onResize = () =>
      updatePan(constrainPan(panRef.current, zoomRef.current));
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, []);

  const handleDialogKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    if (event.key === "+" || event.key === "=") {
      event.preventDefault();
      updateZoom(zoomRef.current + LIGHTBOX_ZOOM_STEP);
    } else if (event.key === "-") {
      event.preventDefault();
      updateZoom(zoomRef.current - LIGHTBOX_ZOOM_STEP);
    } else if (event.key === "0") {
      event.preventDefault();
      resetZoom();
    }
  };

  const handlePointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.pointerType === "mouse" && event.button !== 0) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    pointersRef.current.set(event.pointerId, {
      x: event.clientX,
      y: event.clientY,
    });
    if (pointersRef.current.size === 1) {
      gestureRef.current = {
        kind: "pan",
        pointerId: event.pointerId,
        startX: event.clientX,
        startY: event.clientY,
        origin: panRef.current,
      };
      return;
    }
    if (pointersRef.current.size === 2) {
      const [first, second] = Array.from(pointersRef.current.values());
      gestureRef.current = {
        kind: "pinch",
        startDistance: distanceBetween(first, second),
        startZoom: zoomRef.current,
      };
    }
  };

  const handlePointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!pointersRef.current.has(event.pointerId)) return;
    pointersRef.current.set(event.pointerId, {
      x: event.clientX,
      y: event.clientY,
    });
    const pointers = Array.from(pointersRef.current.values());
    const gesture = gestureRef.current;
    if (pointers.length >= 2) {
      if (gesture?.kind !== "pinch") return;
      const nextDistance = distanceBetween(pointers[0], pointers[1]);
      if (gesture.startDistance === 0) return;
      updateZoom(gesture.startZoom * (nextDistance / gesture.startDistance));
      return;
    }
    if (
      gesture?.kind !== "pan" ||
      gesture.pointerId !== event.pointerId ||
      zoomRef.current <= MIN_LIGHTBOX_ZOOM
    ) {
      return;
    }
    updatePan(
      constrainPan(
        {
          x: gesture.origin.x + event.clientX - gesture.startX,
          y: gesture.origin.y + event.clientY - gesture.startY,
        },
        zoomRef.current,
      ),
    );
  };

  const handlePointerUp = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
    pointersRef.current.delete(event.pointerId);
    if (pointersRef.current.size === 0) {
      gestureRef.current = null;
      return;
    }
    if (pointersRef.current.size === 1) {
      const [pointerId, point] = Array.from(pointersRef.current.entries())[0];
      gestureRef.current = {
        kind: "pan",
        pointerId,
        startX: point.x,
        startY: point.y,
        origin: panRef.current,
      };
    }
  };

  const zoomLabel = `${Math.round(zoom * 100)}%`;

  return (
    <div
      ref={dialogRef}
      id={id}
      className={styles.minutesLightbox}
      role="dialog"
      aria-modal="true"
      aria-label={isWhiteboard ? "画板预览" : "图片预览"}
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
      onKeyDown={handleDialogKeyDown}
    >
      <div className={styles.minutesLightboxCard}>
        <div className={styles.minutesLightboxToolbar}>
          <div className={styles.minutesLightboxControls}>
            <button
              type="button"
              className={styles.minutesLightboxControl}
              onClick={() => updateZoom(zoomRef.current - LIGHTBOX_ZOOM_STEP)}
              disabled={zoom <= MIN_LIGHTBOX_ZOOM}
              aria-label="缩小"
              title="缩小"
            >
              −
            </button>
            <span
              className={styles.minutesLightboxZoom}
              role="status"
              aria-live="polite"
              aria-label={`当前缩放 ${zoomLabel}`}
            >
              {zoomLabel}
            </span>
            <button
              type="button"
              className={styles.minutesLightboxControl}
              onClick={() => updateZoom(zoomRef.current + LIGHTBOX_ZOOM_STEP)}
              disabled={zoom >= MAX_LIGHTBOX_ZOOM}
              aria-label="放大"
              title="放大"
            >
              +
            </button>
            <button
              type="button"
              className={styles.minutesLightboxFit}
              onClick={resetZoom}
              disabled={zoom <= MIN_LIGHTBOX_ZOOM && pan.x === 0 && pan.y === 0}
            >
              适配全屏
            </button>
          </div>
          <button
            ref={closeButtonRef}
            type="button"
            className={styles.minutesLightboxClose}
            onClick={onClose}
          >
            关闭
          </button>
        </div>
        <div
          ref={viewportRef}
          className={styles.minutesLightboxViewport}
          data-zoomed={zoom > MIN_LIGHTBOX_ZOOM ? "true" : "false"}
          onPointerDown={handlePointerDown}
          onPointerMove={handlePointerMove}
          onPointerUp={handlePointerUp}
          onPointerCancel={handlePointerUp}
        >
          <div
            className={styles.minutesLightboxImageFrame}
            style={{ transform: `translate3d(${pan.x}px, ${pan.y}px, 0)` }}
          >
            {/* Managed local artifact media; not a remote Next image. */}
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              ref={imageRef}
              src={src}
              alt={alt}
              draggable={false}
              className={styles.minutesLightboxImage}
              style={{ transform: `scale(${zoom})` }}
              onLoad={() =>
                updatePan(constrainPan(panRef.current, zoomRef.current))
              }
              onError={onError}
            />
          </div>
        </div>
      </div>
    </div>
  );
}

function distanceBetween(first: LightboxPointer, second: LightboxPointer) {
  return Math.hypot(second.x - first.x, second.y - first.y);
}

function getContainedImageSize(
  image: HTMLImageElement,
  viewport: HTMLDivElement,
): LightboxSize {
  const boxWidth = image.offsetWidth || viewport.clientWidth;
  const boxHeight = image.offsetHeight || viewport.clientHeight;
  if (
    boxWidth <= 0 ||
    boxHeight <= 0 ||
    image.naturalWidth <= 0 ||
    image.naturalHeight <= 0
  ) {
    return { width: boxWidth, height: boxHeight };
  }
  const scale = Math.min(
    boxWidth / image.naturalWidth,
    boxHeight / image.naturalHeight,
  );
  return {
    width: image.naturalWidth * scale,
    height: image.naturalHeight * scale,
  };
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

interface VisibleMinutesInlineImage {
  item: MinutesVisualItem;
  section?: NonNullable<MinutesInlineImage["section"]>;
  sectionStart: boolean;
  anchorText: string;
  anchorOccurrence: number;
  appendAtEnd: boolean;
}

function normalizeMinutesInlineImages(
  images: MinutesInlineImage[],
  visualItems: MinutesVisualItem[],
): VisibleMinutesInlineImage[] {
  const visualByMediaID = new Map(
    visualItems
      .filter((item) => item.type === "image")
      .map((item) => [item.media_id, item]),
  );
  if (images.length === 0) {
    return visualItems
      .filter((item) => item.type === "image")
      .map((item) => ({
        item,
        sectionStart: false,
        anchorText: "",
        anchorOccurrence: 0,
        appendAtEnd: true,
      }));
  }
  const normalized: VisibleMinutesInlineImage[] = [];
  const seen = new Set<string>();
  for (const image of images) {
    const mediaID = image.media_id;
    if (!isSafeManagedMediaId(mediaID) || seen.has(mediaID)) {
      continue;
    }
    const item = visualByMediaID.get(mediaID);
    if (!item) continue;
    const section = isMinutesInlineSection(image.section)
      ? image.section
      : undefined;
    const sectionStart = image.section_start === true && section !== undefined;
    seen.add(mediaID);
    normalized.push({
      item,
      section,
      sectionStart,
      anchorText: sectionStart ? "" : (image.anchor_text?.trim() ?? ""),
      anchorOccurrence:
        !sectionStart &&
        Number.isInteger(image.anchor_occurrence) &&
        (image.anchor_occurrence ?? 0) > 0
          ? image.anchor_occurrence!
          : 0,
      appendAtEnd: false,
    });
  }
  return normalized;
}

interface MinutesInlineImagePlacement {
  before: VisibleMinutesInlineImage[];
  after: Map<number, VisibleMinutesInlineImage[]>;
  trailing: VisibleMinutesInlineImage[];
}

function placeMinutesInlineImages(
  texts: string[],
  images: VisibleMinutesInlineImage[],
): MinutesInlineImagePlacement {
  const placement: MinutesInlineImagePlacement = {
    before: [],
    after: new Map(),
    trailing: [],
  };
  const normalizedTexts = texts.map(normalizeMarkdownText);
  let searchFrom = 0;
  let previousAnchor = "";
  let previousIndex = -1;
  for (const image of images) {
    if (image.sectionStart) {
      placement.before.push(image);
      continue;
    }
    if (image.appendAtEnd) {
      placement.trailing.push(image);
      continue;
    }
    const anchor = normalizeMarkdownText(image.anchorText);
    if (!anchor) {
      placement.trailing.push(image);
      continue;
    }
    let index = findMinutesAnchorIndex(
      normalizedTexts,
      anchor,
      image.anchorOccurrence,
      searchFrom,
    );
    if (
      index < 0 &&
      image.anchorOccurrence === 0 &&
      anchor === previousAnchor
    ) {
      index = previousIndex;
    }
    if (index < 0) {
      placement.trailing.push(image);
      continue;
    }
    const atIndex = placement.after.get(index) ?? [];
    atIndex.push(image);
    placement.after.set(index, atIndex);
    searchFrom = Math.max(searchFrom, index + 1);
    previousAnchor = anchor;
    previousIndex = index;
  }
  return placement;
}

function hasMinutesImagePlacement(placement: MinutesInlineImagePlacement) {
  return (
    placement.before.length > 0 ||
    placement.after.size > 0 ||
    placement.trailing.length > 0
  );
}

function injectMinutesInlineImages(
  content: string,
  images: VisibleMinutesInlineImage[],
  artifactSetId: number,
) {
  if (images.length === 0) return content;
  const lines = content.split("\n");
  const placement = placeMinutesInlineImages(lines, images);
  const output = placement.before.map((image) =>
    minutesInlineImageMarkdown(image, artifactSetId),
  );
  for (let index = 0; index < lines.length; index += 1) {
    output.push(lines[index]);
    const indent = lines[index]?.match(/^\s*/)?.[0] ?? "";
    for (const image of placement.after.get(index) ?? []) {
      output.push(
        `${indent}${minutesInlineImageMarkdown(image, artifactSetId)}`,
      );
    }
  }
  for (const image of placement.trailing) {
    output.push(minutesInlineImageMarkdown(image, artifactSetId));
  }
  return output.join("\n");
}

function minutesInlineImageMarkdown(
  image: VisibleMinutesInlineImage,
  artifactSetId: number,
) {
  return `![${escapeMarkdownImageAlt(
    image.item.alt || "飞书智能纪要图片",
  )}](${managedMediaURL(artifactSetId, image.item.media_id)})`;
}

function findMinutesAnchorIndex(
  texts: string[],
  anchor: string,
  occurrence: number,
  start: number,
) {
  let seen = 0;
  for (let index = 0; index < texts.length; index += 1) {
    if (!texts[index].includes(anchor)) continue;
    seen += countTextOccurrences(texts[index], anchor);
    if (occurrence > 0 ? seen >= occurrence : index >= Math.max(0, start)) {
      return index;
    }
  }
  return -1;
}

function countTextOccurrences(value: string, search: string) {
  if (!value || !search) return 0;
  return value.split(search).length - 1;
}

function isMinutesInlineSection(
  value: MinutesInlineImage["section"],
): value is NonNullable<MinutesInlineImage["section"]> {
  return (
    value === "body" ||
    value === "summary" ||
    value === "chapters" ||
    value === "decisions" ||
    value === "quotes" ||
    value === "links"
  );
}

function normalizeMarkdownText(value: string) {
  return value
    .replace(/!\[[^\]]*\]\([^)]*\)/g, " ")
    .replace(/\[([^\]]+)\]\([^)]*\)/g, "$1")
    .replace(/[`*_~>#]/g, "")
    .replace(/^\s*(?:[-*+]|\d+\.)\s+/, "")
    .replace(/\s+/g, " ")
    .trim();
}

function escapeMarkdownImageAlt(value: string) {
  return value.replace(/[\\\[\]]/g, "\\$&").replace(/\n/g, " ");
}

function isSafeManagedMediaId(mediaId: string) {
  return /^[a-z][a-z0-9_-]{1,63}$/.test(mediaId);
}

function managedMediaURL(artifactSetId: number, mediaId: string) {
  if (!isSafeManagedMediaId(mediaId)) return "";
  return `/api/v1/artifact-sets/${artifactSetId}/media/${encodeURIComponent(mediaId)}`;
}

function managedMediaIDFromURL(src: string) {
  return src.match(/\/media\/([a-z][a-z0-9_-]{1,63})$/)?.[1] ?? "";
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
