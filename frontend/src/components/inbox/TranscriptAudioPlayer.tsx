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
import type { TranscriptSegment } from "@/types/processing";
import styles from "./InboxPage.module.css";

interface TranscriptAudioPlayerProps {
  artifactSetId: number;
  segments: TranscriptSegment[];
  mediaAvailable: boolean;
  playbackRate: TranscriptPlaybackRate;
  onPlaybackRateChange: (rate: TranscriptPlaybackRate) => void;
}

type MediaState = "loading" | "ready" | "waiting" | "error" | "unavailable";

export const TRANSCRIPT_PLAYBACK_RATES = [0.75, 1, 1.25, 1.5, 2] as const;
export type TranscriptPlaybackRate =
  (typeof TRANSCRIPT_PLAYBACK_RATES)[number];
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
    targetRect.top < containerRect.top || targetRect.bottom > containerRect.bottom
  );
}

export default function TranscriptAudioPlayer({
  artifactSetId,
  segments,
  mediaAvailable,
  playbackRate,
  onPlaybackRateChange,
}: TranscriptAudioPlayerProps) {
  const audioRef = useRef<HTMLAudioElement>(null);
  const transcriptRef = useRef<HTMLDivElement>(null);
  const segmentRefs = useRef(new Map<number, HTMLButtonElement>());
  const followEnabledRef = useRef(true);
  const programmaticScrollRef = useRef(false);
  const programmaticScrollFrame = useRef<number | null>(null);
  const [mediaState, setMediaState] = useState<MediaState>(
    mediaAvailable ? "loading" : "unavailable",
  );
  const [isPlaying, setIsPlaying] = useState(false);
  const [duration, setDuration] = useState(0);
  const [currentTime, setCurrentTime] = useState(0);
  const [currentSegmentIndex, setCurrentSegmentIndex] = useState(-1);
  const [followEnabled, setFollowEnabledState] = useState(true);

  const setFollowEnabled = useCallback((enabled: boolean) => {
    followEnabledRef.current = enabled;
    setFollowEnabledState(enabled);
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

  const seekTo = useCallback(
    (seconds: number) => {
      const audio = audioRef.current;
      if (!audio) return;
      const bounded =
        duration > 0 ? Math.min(Math.max(seconds, 0), duration) : Math.max(seconds, 0);
      audio.currentTime = bounded;
      updatePosition(bounded, true);
    },
    [duration, updatePosition],
  );

  useEffect(() => {
    setMediaState(mediaAvailable ? "loading" : "unavailable");
    setIsPlaying(false);
    setDuration(0);
    setCurrentTime(0);
    setCurrentSegmentIndex(currentSegmentAt(segments, 0));
    setFollowEnabled(true);
  }, [artifactSetId, mediaAvailable, segments, setFollowEnabled]);

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
    },
    [],
  );

  const handlePlayPause = useCallback(async () => {
    const audio = audioRef.current;
    if (!audio || mediaState === "error" || mediaState === "unavailable") return;
    if (!audio.paused) {
      audio.pause();
      return;
    }
    setFollowEnabled(true);
    updatePosition(audio.currentTime, true);
    try {
      await audio.play();
    } catch {
      setIsPlaying(false);
      setMediaState("error");
    }
  }, [mediaState, setFollowEnabled, updatePosition]);

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
    mediaState === "loading"
      ? "正在加载音频…"
      : mediaState === "waiting"
        ? "音频缓冲中…"
        : mediaState === "error"
          ? "音频加载失败，逐字稿仍可阅读。"
          : mediaState === "unavailable"
            ? "音频不可用，逐字稿仍可阅读。"
            : !followEnabled
              ? "自动跟随已暂停"
              : "";

  return (
    <div className={styles.transcriptExperience}>
      <div className={styles.transcriptPlayer} aria-label="逐字稿音频播放器">
        {mediaAvailable && (
          <audio
            key={artifactSetId}
            ref={audioRef}
            className={styles.audioElement}
            src={`/api/v1/artifact-sets/${artifactSetId}/audio`}
            preload="metadata"
            aria-hidden="true"
            onLoadedMetadata={(event) => {
              applyPlaybackRate(event.currentTarget, playbackRate);
              const nextDuration = Number.isFinite(event.currentTarget.duration)
                ? event.currentTarget.duration
                : 0;
              setDuration(nextDuration);
              setMediaState("ready");
              updatePosition(event.currentTarget.currentTime);
            }}
            onDurationChange={(event) => {
              if (Number.isFinite(event.currentTarget.duration)) {
                setDuration(event.currentTarget.duration);
              }
            }}
            onCanPlay={() => setMediaState("ready")}
            onWaiting={() => setMediaState("waiting")}
            onStalled={() => setMediaState("waiting")}
            onError={() => {
              setIsPlaying(false);
              setMediaState("error");
            }}
            onPlay={(event) => {
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
            onTimeUpdate={(event) =>
              updatePosition(event.currentTarget.currentTime)
            }
          />
        )}

        <button
          type="button"
          className={styles.transcriptPlayButton}
          aria-label={isPlaying ? "暂停音频" : "播放音频"}
          disabled={
            !mediaAvailable ||
            mediaState === "loading" ||
            mediaState === "error"
          }
          onClick={() => void handlePlayPause()}
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
            disabled={!mediaAvailable || duration <= 0 || mediaState === "error"}
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
            disabled={
              !mediaAvailable ||
              mediaState === "loading" ||
              mediaState === "error"
            }
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
            onClick={() => {
              const audio = audioRef.current;
              if (!audio) return;
              setMediaState("loading");
              setIsPlaying(false);
              audio.load();
            }}
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
