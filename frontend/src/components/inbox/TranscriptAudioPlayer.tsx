"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
  type KeyboardEvent,
} from "react";
import {
  IconPlayerPause,
  IconPlayerPlay,
  IconRefresh,
} from "@tabler/icons-react";
import type { MinutesChapter, TranscriptSegment } from "@/types/processing";
import styles from "./InboxPage.module.css";

interface TranscriptAudioPlayerProps {
  artifactSetId: number;
  segments: TranscriptSegment[];
  mediaAvailable: boolean;
  audioDurationSeconds?: number;
  playbackRate: TranscriptPlaybackRate;
  onPlaybackRateChange: (rate: TranscriptPlaybackRate) => void;
  initialSeekMs?: number | null;
  chapters?: MinutesChapter[];
}

// The transcript is the primary content, so no audio body is requested until
// the first play intent. `idle` keeps an armed-free player that shows the
// server-provided duration; `preparing` covers the bounded first-load window.
type MediaState =
  | "idle"
  | "preparing"
  | "ready"
  | "waiting"
  | "error"
  | "unavailable";

type PreparationPhase = "inactive" | "active" | "stopped";

const PREPARE_TIMEOUT_MS = 15_000;

export const TRANSCRIPT_PLAYBACK_RATES = [0.75, 1, 1.25, 1.5, 2] as const;
export type TranscriptPlaybackRate = (typeof TRANSCRIPT_PLAYBACK_RATES)[number];
export const DEFAULT_TRANSCRIPT_PLAYBACK_RATE: TranscriptPlaybackRate = 1;

const transcriptScrollKeys = new Set([
  "ArrowDown",
  "ArrowUp",
  "End",
  "Home",
  "PageDown",
  "PageUp",
  " ",
]);

function parseTranscriptPlaybackRate(value: string): TranscriptPlaybackRate {
  const candidate = Number(value);
  for (const rate of TRANSCRIPT_PLAYBACK_RATES) {
    if (rate === candidate) return rate;
  }
  return DEFAULT_TRANSCRIPT_PLAYBACK_RATE;
}

function applyPlaybackRate(
  audio: HTMLAudioElement,
  rate: TranscriptPlaybackRate,
) {
  audio.defaultPlaybackRate = rate;
  audio.playbackRate = rate;
}

function formatPlaybackTime(seconds: number, unknown = false) {
  if (!Number.isFinite(seconds) || seconds < 0 || unknown) return "--:--";
  const wholeSeconds = Math.floor(seconds);
  const hours = Math.floor(wholeSeconds / 3600);
  const minutes = Math.floor((wholeSeconds % 3600) / 60);
  const remainder = wholeSeconds % 60;
  if (hours > 0) {
    return `${hours}:${String(minutes).padStart(2, "0")}:${String(
      remainder,
    ).padStart(2, "0")}`;
  }
  return `${String(minutes).padStart(2, "0")}:${String(remainder).padStart(
    2,
    "0",
  )}`;
}

function formatChapterTime(startMs: number) {
  return formatPlaybackTime(Math.max(startMs, 0) / 1000);
}

function currentSegmentAt(
  segments: TranscriptSegment[],
  currentTimeSeconds: number,
) {
  const currentTimeMS = currentTimeSeconds * 1000;
  let low = 0;
  let high = segments.length - 1;
  let match = -1;
  while (low <= high) {
    const middle = Math.floor((low + high) / 2);
    if (segments[middle].start_ms <= currentTimeMS) {
      match = middle;
      low = middle + 1;
    } else {
      high = middle - 1;
    }
  }
  return match;
}

function isOutsideViewport(container: HTMLElement, target: HTMLElement) {
  const containerRect = container.getBoundingClientRect();
  const targetRect = target.getBoundingClientRect();
  return (
    targetRect.top < containerRect.top ||
    targetRect.bottom > containerRect.bottom
  );
}

function normalizedAudioDuration(value: number | undefined) {
  return typeof value === "number" && Number.isFinite(value) && value > 0
    ? value
    : 0;
}

export default function TranscriptAudioPlayer({
  artifactSetId,
  segments,
  mediaAvailable,
  audioDurationSeconds,
  playbackRate,
  onPlaybackRateChange,
  initialSeekMs,
  chapters = [],
}: TranscriptAudioPlayerProps) {
  const audioRef = useRef<HTMLAudioElement>(null);
  // React detaches `audioRef` before unmount effects run; keep the last
  // mounted element so the unmount cleanup can still detach an in-flight
  // audio source.
  const lastAudioRef = useRef<HTMLAudioElement | null>(null);
  const transcriptRef = useRef<HTMLDivElement>(null);
  const segmentRefs = useRef(new Map<number, HTMLElement>());
  const followEnabledRef = useRef(true);
  const programmaticScrollRef = useRef(false);
  const programmaticScrollFrame = useRef<number | null>(null);
  const currentTimeRef = useRef(0);
  // The pending position to apply once the armed audio source exposes
  // metadata, so a pre-play seek or chapter choice survives lazy loading.
  const pendingSeekRef = useRef<number | null>(null);
  const preparePhaseRef = useRef<PreparationPhase>("inactive");
  const prepareTimeoutRef = useRef<number | null>(null);
  const [mediaState, setMediaState] = useState<MediaState>(
    mediaAvailable ? "idle" : "unavailable",
  );
  const [prepareTimedOut, setPrepareTimedOut] = useState(false);
  const [isPlaying, setIsPlaying] = useState(false);
  const [duration, setDuration] = useState(() =>
    normalizedAudioDuration(audioDurationSeconds),
  );
  const [currentTime, setCurrentTime] = useState(0);
  const [currentSegmentIndex, setCurrentSegmentIndex] = useState(-1);
  const [followEnabled, setFollowEnabledState] = useState(true);

  const setFollowEnabled = useCallback((enabled: boolean) => {
    followEnabledRef.current = enabled;
    setFollowEnabledState(enabled);
  }, []);

  const clearPrepareTimeout = useCallback(() => {
    if (prepareTimeoutRef.current !== null) {
      window.clearTimeout(prepareTimeoutRef.current);
      prepareTimeoutRef.current = null;
    }
  }, []);

  const finishPreparation = useCallback(() => {
    clearPrepareTimeout();
    preparePhaseRef.current = "inactive";
  }, [clearPrepareTimeout]);

  // Detaches the audio source so the browser aborts an in-flight media
  // request; `load()` is required for the removal to take effect.
  const detachAudioSource = useCallback(() => {
    const audio = audioRef.current;
    if (!audio || !audio.src) return;
    audio.pause();
    audio.removeAttribute("src");
    audio.load();
  }, []);

  const revealSegmentIfNeeded = useCallback(
    (segmentIndex: number) => {
      if (!followEnabledRef.current || segmentIndex < 0) return;
      const container = transcriptRef.current;
      const target = segmentRefs.current.get(segments[segmentIndex].order);
      if (
        !container ||
        !target ||
        typeof target.scrollIntoView !== "function" ||
        !isOutsideViewport(container, target)
      ) {
        return;
      }
      programmaticScrollRef.current = true;
      target.scrollIntoView({ block: "nearest", behavior: "auto" });
      if (programmaticScrollFrame.current !== null) {
        window.cancelAnimationFrame(programmaticScrollFrame.current);
      }
      programmaticScrollFrame.current = window.requestAnimationFrame(() => {
        programmaticScrollRef.current = false;
        programmaticScrollFrame.current = null;
      });
    },
    [segments],
  );

  const updatePosition = useCallback(
    (seconds: number, resumeFollow = false) => {
      const nextTime = Number.isFinite(seconds) ? Math.max(0, seconds) : 0;
      currentTimeRef.current = nextTime;
      if (resumeFollow) {
        setFollowEnabled(true);
      }
      setCurrentTime(nextTime);
      const segmentIndex = currentSegmentAt(segments, nextTime);
      setCurrentSegmentIndex(segmentIndex);
      if (resumeFollow || followEnabledRef.current) {
        revealSegmentIfNeeded(segmentIndex);
      }
    },
    [revealSegmentIfNeeded, segments, setFollowEnabled],
  );

  // Position-only navigation: updates the pending playback position without
  // arming the audio source or triggering any download.
  const seekTo = useCallback(
    (seconds: number) => {
      const bounded =
        duration > 0
          ? Math.min(Math.max(seconds, 0), duration)
          : Math.max(seconds, 0);
      const audio = audioRef.current;
      if (audio) {
        audio.currentTime = bounded;
      }
      if (preparePhaseRef.current === "active") {
        pendingSeekRef.current = bounded;
      }
      updatePosition(bounded, true);
    },
    [duration, updatePosition],
  );

  const handlePrepareTimeout = useCallback(() => {
    prepareTimeoutRef.current = null;
    if (preparePhaseRef.current !== "active") return;
    preparePhaseRef.current = "stopped";
    detachAudioSource();
    pendingSeekRef.current = null;
    setIsPlaying(false);
    setPrepareTimedOut(true);
    setMediaState("error");
  }, [detachAudioSource]);

  // First play intent (or an explicit retry): arms the managed audio source
  // once and starts playback from the pending position. Duplicate intents are
  // merged by the `preparing` guard in startPlayback.
  const prepareAndPlay = useCallback(
    (fromSeconds: number) => {
      const audio = audioRef.current;
      if (!audio || !mediaAvailable) return;
      const bounded =
        duration > 0
          ? Math.min(Math.max(fromSeconds, 0), duration)
          : Math.max(fromSeconds, 0);
      clearPrepareTimeout();
      preparePhaseRef.current = "active";
      pendingSeekRef.current = bounded;
      setPrepareTimedOut(false);
      setIsPlaying(false);
      setMediaState("preparing");
      audio.src = `/api/v1/artifact-sets/${artifactSetId}/audio`;
      audio.load();
      prepareTimeoutRef.current = window.setTimeout(
        handlePrepareTimeout,
        PREPARE_TIMEOUT_MS,
      );
      audio.play().catch(() => {
        // A late rejection after the timeout abort must not replace the
        // timeout state; only an active preparation reports a failure.
        if (preparePhaseRef.current !== "active") return;
        finishPreparation();
        setIsPlaying(false);
        setPrepareTimedOut(false);
        setMediaState("error");
      });
    },
    [
      artifactSetId,
      clearPrepareTimeout,
      duration,
      finishPreparation,
      handlePrepareTimeout,
      mediaAvailable,
    ],
  );

  const startPlayback = useCallback(
    (fromSeconds: number | null) => {
      const audio = audioRef.current;
      if (
        !audio ||
        !mediaAvailable ||
        mediaState === "error" ||
        mediaState === "unavailable"
      ) {
        // Without usable media the navigation contract stays position-only.
        if (fromSeconds !== null) {
          seekTo(fromSeconds);
        }
        return;
      }
      // An in-flight first preparation already owns the single media request;
      // repeated intents merge into it while navigation stays live.
      if (mediaState === "preparing") {
        seekTo(fromSeconds ?? currentTimeRef.current);
        return;
      }
      const target = fromSeconds ?? currentTimeRef.current;
      setFollowEnabled(true);
      if (mediaState === "idle") {
        updatePosition(target, true);
        prepareAndPlay(target);
        return;
      }
      seekTo(target);
      audio.play().catch(() => {
        setIsPlaying(false);
        setMediaState("error");
      });
    },
    [
      mediaAvailable,
      mediaState,
      prepareAndPlay,
      seekTo,
      setFollowEnabled,
      updatePosition,
    ],
  );

  const visibleChapters = chapters.filter(
    (chapter) => chapter.title.trim() || chapter.summary?.trim(),
  );

  const handleChapterSelect = useCallback(
    (startMs: number) => {
      startPlayback(Math.max(startMs, 0) / 1000);
    },
    [startPlayback],
  );

  // Switching episodes or availability (for example audio recovery) discards
  // any in-flight preparation so an old request can never autoplay.
  useEffect(() => {
    clearPrepareTimeout();
    preparePhaseRef.current = "inactive";
    pendingSeekRef.current = null;
    detachAudioSource();
    setMediaState(mediaAvailable ? "idle" : "unavailable");
    setPrepareTimedOut(false);
    setIsPlaying(false);
    setDuration(normalizedAudioDuration(audioDurationSeconds));
  }, [
    artifactSetId,
    audioDurationSeconds,
    clearPrepareTimeout,
    detachAudioSource,
    mediaAvailable,
  ]);

  useEffect(() => {
    const initialSeconds =
      typeof initialSeekMs === "number" && Number.isFinite(initialSeekMs)
        ? Math.max(initialSeekMs, 0) / 1000
        : 0;
    currentTimeRef.current = initialSeconds;
    setCurrentTime(initialSeconds);
    setCurrentSegmentIndex(currentSegmentAt(segments, initialSeconds));
    setFollowEnabled(true);
  }, [
    artifactSetId,
    initialSeekMs,
    mediaAvailable,
    segments,
    setFollowEnabled,
  ]);

  useEffect(() => {
    if (typeof initialSeekMs !== "number" || !Number.isFinite(initialSeekMs)) {
      return;
    }
    seekTo(Math.max(initialSeekMs, 0) / 1000);
  }, [artifactSetId, initialSeekMs, seekTo]);

  useEffect(() => {
    const audio = audioRef.current;
    if (!audio) return;
    applyPlaybackRate(audio, playbackRate);
  }, [artifactSetId, mediaAvailable, playbackRate]);

  useEffect(
    () => () => {
      if (programmaticScrollFrame.current !== null) {
        window.cancelAnimationFrame(programmaticScrollFrame.current);
      }
      if (prepareTimeoutRef.current !== null) {
        window.clearTimeout(prepareTimeoutRef.current);
        prepareTimeoutRef.current = null;
      }
      preparePhaseRef.current = "stopped";
      const audio = lastAudioRef.current;
      if (audio && audio.src) {
        audio.pause();
        audio.removeAttribute("src");
      }
    },
    [],
  );

  // Ends a bounded preparation by user choice: the in-flight request is
  // aborted and the player returns to the idle, play-enabled state without a
  // failure message.
  const cancelPreparation = useCallback(() => {
    if (preparePhaseRef.current !== "active") return;
    preparePhaseRef.current = "stopped";
    clearPrepareTimeout();
    detachAudioSource();
    pendingSeekRef.current = null;
    setIsPlaying(false);
    setMediaState("idle");
  }, [clearPrepareTimeout, detachAudioSource]);

  const handlePlayPause = useCallback(() => {
    const audio = audioRef.current;
    if (!audio) return;
    if (mediaState === "preparing") {
      // Before the browser confirms playback, a repeat click is a duplicate
      // play intent and merges into the in-flight request. Once the pause
      // control is showing, the click cancels the bounded wait instead.
      if (isPlaying) {
        cancelPreparation();
      }
      return;
    }
    if (!audio.paused) {
      audio.pause();
      return;
    }
    startPlayback(null);
  }, [cancelPreparation, isPlaying, mediaState, startPlayback]);

  const handleRetry = useCallback(() => {
    // One new request that keeps the transcript, chosen rate, and pending
    // playback position.
    prepareAndPlay(currentTimeRef.current);
  }, [prepareAndPlay]);

  const handlePlaybackRateChange = useCallback(
    (event: ChangeEvent<HTMLSelectElement>) => {
      const nextRate = parseTranscriptPlaybackRate(event.currentTarget.value);
      const audio = audioRef.current;
      if (audio) applyPlaybackRate(audio, nextRate);
      onPlaybackRateChange(nextRate);
    },
    [onPlaybackRateChange],
  );

  const handleSliderKeyDown = useCallback(
    (event: KeyboardEvent<HTMLInputElement>) => {
      let target: number | null = null;
      switch (event.key) {
        case "ArrowLeft":
        case "ArrowDown":
          target = currentTime - 5;
          break;
        case "ArrowRight":
        case "ArrowUp":
          target = currentTime + 5;
          break;
        case "PageDown":
          target = currentTime - 30;
          break;
        case "PageUp":
          target = currentTime + 30;
          break;
        case "Home":
          target = 0;
          break;
        case "End":
          target = duration;
          break;
      }
      if (target === null) return;
      event.preventDefault();
      seekTo(target);
    },
    [currentTime, duration, seekTo],
  );

  const pauseFollowing = useCallback(() => {
    if (!programmaticScrollRef.current) {
      setFollowEnabled(false);
    }
  }, [setFollowEnabled]);

  const mediaStatus =
    mediaState === "preparing"
      ? "正在准备播放"
      : mediaState === "waiting"
        ? "音频缓冲中…"
        : mediaState === "error"
          ? prepareTimedOut
            ? "音频准备超时，请重试。"
            : "音频加载失败，逐字稿仍可阅读。"
          : mediaState === "unavailable"
            ? "音频不可用，逐字稿仍可阅读。"
            : !followEnabled
              ? "自动跟随已暂停"
              : "";

  return (
    <div className={styles.transcriptExperience}>
      <div className={styles.transcriptPlayer} aria-label="逐字稿音频播放器">
        {mediaAvailable && (
          // No `src` here: the managed audio body is requested only when the
          // first play intent arms the source in prepareAndPlay.
          <audio
            key={artifactSetId}
            ref={(node) => {
              audioRef.current = node;
              if (node) {
                lastAudioRef.current = node;
              }
            }}
            className={styles.audioElement}
            aria-hidden="true"
            onLoadedMetadata={(event) => {
              applyPlaybackRate(event.currentTarget, playbackRate);
              const nextDuration = event.currentTarget.duration;
              if (Number.isFinite(nextDuration) && nextDuration > 0) {
                setDuration(nextDuration);
              }
              const pending = pendingSeekRef.current;
              if (pending !== null) {
                pendingSeekRef.current = null;
                event.currentTarget.currentTime = pending;
                updatePosition(pending);
              } else {
                updatePosition(event.currentTarget.currentTime);
              }
            }}
            onDurationChange={(event) => {
              if (
                Number.isFinite(event.currentTarget.duration) &&
                event.currentTarget.duration > 0
              ) {
                setDuration(event.currentTarget.duration);
              }
            }}
            onCanPlay={() => {
              finishPreparation();
              setMediaState("ready");
            }}
            onWaiting={() => {
              if (preparePhaseRef.current === "active") return;
              setMediaState("waiting");
            }}
            onStalled={() => {
              if (preparePhaseRef.current === "active") return;
              setMediaState("waiting");
            }}
            onError={() => {
              if (preparePhaseRef.current === "stopped") return;
              finishPreparation();
              setIsPlaying(false);
              setPrepareTimedOut(false);
              setMediaState("error");
            }}
            onPlay={(event) => {
              if (preparePhaseRef.current === "active") {
                // Browsers raise `play` as soon as playback is intended, even
                // before media data arrives. The bounded preparation window
                // stays in charge until the media is actually playable.
                setIsPlaying(true);
                return;
              }
              finishPreparation();
              setIsPlaying(true);
              setMediaState("ready");
              setFollowEnabled(true);
              updatePosition(event.currentTarget.currentTime, true);
            }}
            onPause={() => setIsPlaying(false)}
            onEnded={(event) => {
              setIsPlaying(false);
              updatePosition(event.currentTarget.currentTime);
            }}
            onTimeUpdate={(event) => {
              if (preparePhaseRef.current === "active") return;
              updatePosition(event.currentTarget.currentTime);
            }}
          />
        )}

        <button
          type="button"
          className={styles.transcriptPlayButton}
          aria-label={isPlaying ? "暂停音频" : "播放音频"}
          disabled={!mediaAvailable || mediaState === "error"}
          onClick={handlePlayPause}
        >
          {isPlaying ? (
            <IconPlayerPause size={18} stroke={1.8} aria-hidden="true" />
          ) : (
            <IconPlayerPlay size={18} stroke={1.8} aria-hidden="true" />
          )}
        </button>

        <label className={styles.transcriptProgress}>
          <span className={styles.srOnly}>音频进度</span>
          <input
            type="range"
            min="0"
            max={duration > 0 ? duration : 0}
            step="0.1"
            value={duration > 0 ? Math.min(currentTime, duration) : 0}
            disabled={
              !mediaAvailable || duration <= 0 || mediaState === "error"
            }
            aria-label="音频进度"
            aria-valuetext={`${formatPlaybackTime(
              currentTime,
            )} / ${formatPlaybackTime(duration, duration <= 0)}`}
            onChange={(event) => seekTo(Number(event.currentTarget.value))}
            onKeyDown={handleSliderKeyDown}
          />
        </label>

        <span className={styles.transcriptTime} aria-live="off">
          {formatPlaybackTime(currentTime)} /{" "}
          {formatPlaybackTime(duration, duration <= 0)}
        </span>

        <label className={styles.transcriptPlaybackRate}>
          <span className={styles.srOnly}>播放倍速</span>
          <select
            value={playbackRate}
            aria-label="播放倍速"
            disabled={!mediaAvailable || mediaState === "error"}
            onChange={handlePlaybackRateChange}
          >
            {TRANSCRIPT_PLAYBACK_RATES.map((rate) => (
              <option key={rate} value={rate}>
                {rate}×
              </option>
            ))}
          </select>
        </label>

        {mediaState === "error" && (
          <button
            type="button"
            className={styles.transcriptRetryButton}
            onClick={handleRetry}
          >
            <IconRefresh size={15} stroke={1.8} aria-hidden="true" />
            重试
          </button>
        )}

        {mediaStatus && (
          <span className={styles.transcriptMediaStatus} role="status">
            {mediaStatus}
          </span>
        )}
      </div>

      {visibleChapters.length > 0 && (
        <details className={styles.transcriptChapters}>
          <summary>智能章节 · {visibleChapters.length}</summary>
          <ol className={styles.minutesChapterList}>
            {visibleChapters.map((chapter) => (
              <li key={`${chapter.order}-${chapter.start_ms}`}>
                <button
                  type="button"
                  className={styles.minutesChapter}
                  onClick={() => handleChapterSelect(chapter.start_ms)}
                >
                  <span className={styles.minutesChapterTime}>
                    {formatChapterTime(chapter.start_ms)}
                  </span>
                  <span className={styles.minutesChapterBody}>
                    <strong>{chapter.title || "未命名章节"}</strong>
                    {chapter.summary?.trim() ? (
                      <span>{chapter.summary}</span>
                    ) : null}
                  </span>
                </button>
              </li>
            ))}
          </ol>
        </details>
      )}

      <div
        ref={transcriptRef}
        className={styles.transcriptSegments}
        role="region"
        aria-label="同步逐字稿"
        tabIndex={0}
        onWheel={pauseFollowing}
        onTouchMove={pauseFollowing}
        onScroll={pauseFollowing}
        onKeyDown={(event) => {
          if (
            event.target === event.currentTarget &&
            transcriptScrollKeys.has(event.key)
          ) {
            pauseFollowing();
          }
        }}
      >
        <ol>
          {segments.map((segment, index) => {
            const isCurrent = currentSegmentIndex === index;
            const timestamp = formatPlaybackTime(segment.start_ms / 1000);
            const content = (
              <>
                <span className={styles.transcriptSegmentHeader}>
                  <span>{segment.speaker}</span>
                  <time dateTime={`PT${segment.start_ms / 1000}S`}>
                    {timestamp}
                  </time>
                  {isCurrent && (
                    <span className={styles.transcriptCurrentMarker}>
                      {isPlaying ? "正在播放" : "当前段落"}
                    </span>
                  )}
                </span>
                <span className={styles.transcriptSegmentText}>
                  {segment.text}
                </span>
              </>
            );
            return (
              <li key={`${segment.order}-${segment.start_ms}`}>
                {mediaAvailable ? (
                  <button
                    ref={(node) => {
                      if (node) {
                        segmentRefs.current.set(segment.order, node);
                      } else {
                        segmentRefs.current.delete(segment.order);
                      }
                    }}
                    type="button"
                    className={styles.transcriptSegment}
                    aria-label={`${timestamp} ${segment.speaker}：${segment.text}`}
                    aria-current={isCurrent ? "true" : undefined}
                    onClick={() => seekTo(segment.start_ms / 1000)}
                  >
                    {content}
                  </button>
                ) : (
                  <article
                    ref={(node) => {
                      if (node) {
                        segmentRefs.current.set(segment.order, node);
                      } else {
                        segmentRefs.current.delete(segment.order);
                      }
                    }}
                    className={styles.transcriptSegment}
                    aria-current={isCurrent ? "true" : undefined}
                  >
                    {content}
                  </article>
                )}
              </li>
            );
          })}
        </ol>
      </div>
    </div>
  );
}
