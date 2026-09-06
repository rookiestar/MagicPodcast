import { act, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import TranscriptAudioPlayer, {
  DEFAULT_TRANSCRIPT_PLAYBACK_RATE,
  type TranscriptPlaybackRate,
} from "../TranscriptAudioPlayer";
import type { MinutesChapter, TranscriptSegment } from "@/types/processing";

const segments: TranscriptSegment[] = [
  { order: 1, speaker: "主持人", start_ms: 0, text: "开场内容" },
  { order: 2, speaker: "嘉宾", start_ms: 30_000, text: "中段内容" },
  { order: 3, speaker: "主持人", start_ms: 60_000, text: "尾段内容" },
];

interface ControlledAudio {
  play: ReturnType<typeof vi.fn>;
  pause: ReturnType<typeof vi.fn>;
  load: ReturnType<typeof vi.fn>;
  completePlay: () => void;
}

// Installs controllable media mocks. Play stays pending until completePlay(),
// which mirrors a browser that only starts playback once data is ready.
function controlAudio(
  audio: HTMLAudioElement,
  duration = 120,
): ControlledAudio {
  let paused = true;
  const playResolvers: Array<() => void> = [];
  const play = vi.fn(
    () =>
      new Promise<void>((resolve) => {
        playResolvers.push(() => {
          paused = false;
          fireEvent.play(audio);
          fireEvent.playing(audio);
          resolve();
        });
      }),
  );
  const pause = vi.fn(() => {
    paused = true;
    fireEvent.pause(audio);
  });
  const load = vi.fn();
  // happy-dom fires synthetic `canplay` the moment `src` is assigned; a real
  // browser only reports it after media data arrives, so media readiness is
  // driven by explicit events in these tests.
  Object.defineProperty(audio, "src", {
    configurable: true,
    get: () => audio.getAttribute("src") ?? "",
    set: (value: string) => {
      if (value === "") {
        audio.removeAttribute("src");
      } else {
        audio.setAttribute("src", value);
      }
    },
  });
  Object.defineProperties(audio, {
    duration: { configurable: true, value: duration },
    currentTime: { configurable: true, writable: true, value: 0 },
    defaultPlaybackRate: {
      configurable: true,
      writable: true,
      value: DEFAULT_TRANSCRIPT_PLAYBACK_RATE,
    },
    playbackRate: {
      configurable: true,
      writable: true,
      value: DEFAULT_TRANSCRIPT_PLAYBACK_RATE,
    },
    paused: { configurable: true, get: () => paused },
    play: { configurable: true, value: play },
    pause: { configurable: true, value: pause },
    load: { configurable: true, value: load },
  });
  return {
    play,
    pause,
    load,
    completePlay: () => playResolvers.shift()?.(),
  };
}

interface TestPlayerProps {
  artifactSetId?: number;
  segments?: TranscriptSegment[];
  mediaAvailable?: boolean;
  audioDurationSeconds?: number;
  chapters?: MinutesChapter[];
}

function StatefulTranscriptAudioPlayer({
  artifactSetId = 82,
  segments: playerSegments = segments,
  mediaAvailable = true,
  audioDurationSeconds,
  chapters,
}: TestPlayerProps) {
  const [playbackRate, setPlaybackRate] = useState<TranscriptPlaybackRate>(
    DEFAULT_TRANSCRIPT_PLAYBACK_RATE,
  );
  return (
    <TranscriptAudioPlayer
      artifactSetId={artifactSetId}
      segments={playerSegments}
      mediaAvailable={mediaAvailable}
      audioDurationSeconds={audioDurationSeconds}
      playbackRate={playbackRate}
      onPlaybackRateChange={setPlaybackRate}
      chapters={chapters}
    />
  );
}

function renderPlayer(props: TestPlayerProps = {}) {
  return render(<StatefulTranscriptAudioPlayer {...props} />);
}

function queryMediaStatus() {
  return document.querySelector(`[role="status"]`);
}

function rateButton() {
  return screen.getByRole("button", { name: /^播放倍速，当前 / });
}

function openRateMenu() {
  fireEvent.click(rateButton());
  return screen.getByRole("menu", { name: "选择播放倍速" });
}

function chooseRate(label: string) {
  fireEvent.click(
    within(openRateMenu()).getByRole("menuitemradio", { name: label }),
  );
}

afterEach(() => {
  vi.useRealTimers();
});

describe("TranscriptAudioPlayer", () => {
  it("keeps the first visit readable and requests audio only after play", () => {
    const { container } = renderPlayer({ audioDurationSeconds: 120 });
    const audio = container.querySelector("audio");
    expect(audio).not.toBeNull();
    expect(audio).not.toHaveAttribute("src");
    expect(screen.getByText("中段内容")).toBeVisible();
    expect(screen.getByText("00:00 / 02:00")).toBeVisible();
    expect(screen.queryByText("正在加载音频…")).not.toBeInTheDocument();
    const mediaStatus = screen.getByRole("status");
    expect(mediaStatus).toBeEmptyDOMElement();

    const playButton = screen.getByRole("button", { name: "播放音频" });
    expect(playButton).toBeEnabled();
    expect(screen.getByRole("slider", { name: "音频进度" })).toBeEnabled();
    expect(rateButton()).toBeEnabled();
    expect(rateButton()).toHaveTextContent("1×");

    const media = controlAudio(audio!);
    fireEvent.click(playButton);
    expect(screen.getByText("正在准备播放")).toBeVisible();
    expect(screen.getByRole("status")).toBe(mediaStatus);
    expect(audio).toHaveAttribute("src", "/api/v1/artifact-sets/82/audio");
    expect(media.load).toHaveBeenCalledTimes(1);
    expect(media.play).toHaveBeenCalledTimes(1);

    fireEvent.loadedMetadata(audio);
    fireEvent.canPlay(audio);
    media.completePlay();
    expect(
      screen.queryByText("正在准备播放"),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "暂停音频" })).toBeVisible();
    expect(media.load).toHaveBeenCalledTimes(1);
    expect(
      screen.getByRole("button", { name: "00:00 主持人：开场内容" }),
    ).toHaveAttribute("aria-current", "true");
  });

  it("syncs public media events, slider keys, and segment clicks after playback starts", () => {
    const { container } = renderPlayer({ audioDurationSeconds: 120 });
    const audio = container.querySelector("audio")!;
    const media = controlAudio(audio);
    expect(rateButton()).toHaveTextContent("1×");

    fireEvent.click(screen.getByRole("button", { name: "播放音频" }));
    fireEvent.loadedMetadata(audio);
    fireEvent.canPlay(audio);
    media.completePlay();
    expect(audio.playbackRate).toBe(1);

    const first = screen.getByRole("button", {
      name: "00:00 主持人：开场内容",
    });
    const second = screen.getByRole("button", {
      name: "00:30 嘉宾：中段内容",
    });
    const third = screen.getByRole("button", {
      name: "01:00 主持人：尾段内容",
    });
    expect(first).toHaveAttribute("aria-current", "true");
    expect(screen.getByText("正在播放")).toBeVisible();

    audio.currentTime = 31;
    fireEvent.timeUpdate(audio);
    expect(second).toHaveAttribute("aria-current", "true");
    expect(screen.getByText("正在播放")).toBeVisible();

    // Switching rate mid-playback applies immediately without interrupting
    // playback or moving the position.
    chooseRate("1.5×");
    expect(
      screen.queryByRole("menu", { name: "选择播放倍速" }),
    ).not.toBeInTheDocument();
    expect(rateButton()).toHaveTextContent("1.5×");
    expect(audio.playbackRate).toBe(1.5);
    expect(audio.currentTime).toBe(31);
    expect(second).toHaveAttribute("aria-current", "true");

    const slider = screen.getByRole("slider", { name: "音频进度" });
    fireEvent.change(slider, { target: { value: "61" } });
    expect(audio.currentTime).toBe(61);
    expect(third).toHaveAttribute("aria-current", "true");

    slider.focus();
    fireEvent.keyDown(slider, { key: "Home" });
    expect(slider).toHaveFocus();
    expect(audio.currentTime).toBe(0);
    expect(first).toHaveAttribute("aria-current", "true");
    fireEvent.keyDown(slider, { key: "ArrowRight" });
    expect(audio.currentTime).toBe(5);
    fireEvent.keyDown(slider, { key: "End" });
    expect(audio.currentTime).toBe(120);
    expect(third).toHaveAttribute("aria-current", "true");

    second.focus();
    fireEvent.click(second);
    expect(second).toHaveFocus();
    expect(audio.currentTime).toBe(30);
    expect(second).toHaveAttribute("aria-current", "true");

    fireEvent.click(screen.getByRole("button", { name: "暂停音频" }));
    expect(media.pause).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("button", { name: "播放音频" }));
    media.completePlay();
    expect(audio.playbackRate).toBe(1.5);
    expect(screen.getByRole("button", { name: "暂停音频" })).toBeVisible();
    expect(media.load).toHaveBeenCalledTimes(1);
  });

  it("keeps follow paused through buffering and resumes it on play or seek", () => {
    const { container } = renderPlayer({ audioDurationSeconds: 120 });
    const audio = container.querySelector("audio")!;
    const media = controlAudio(audio);
    fireEvent.click(screen.getByRole("button", { name: "播放音频" }));
    fireEvent.loadedMetadata(audio);
    fireEvent.canPlay(audio);
    media.completePlay();
    const transcript = screen.getByRole("region", { name: "同步逐字稿" });
    const third = screen.getByRole("button", {
      name: "01:00 主持人：尾段内容",
    });
    Object.defineProperty(transcript, "getBoundingClientRect", {
      configurable: true,
      value: () => ({ top: 0, bottom: 100 }),
    });
    Object.defineProperty(third, "getBoundingClientRect", {
      configurable: true,
      value: () => ({ top: 130, bottom: 170 }),
    });
    const scrollIntoView = vi.fn();
    Object.defineProperty(third, "scrollIntoView", {
      configurable: true,
      value: scrollIntoView,
    });

    fireEvent.scroll(transcript);
    expect(screen.getByText("自动跟随已暂停")).toBeVisible();
    audio.currentTime = 61;
    fireEvent.timeUpdate(audio);
    expect(third).toHaveAttribute("aria-current", "true");
    expect(scrollIntoView).not.toHaveBeenCalled();

    fireEvent.waiting(audio);
    fireEvent.playing(audio);
    expect(screen.getByText("自动跟随已暂停")).toBeVisible();
    expect(scrollIntoView).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "暂停音频" }));
    fireEvent.click(screen.getByRole("button", { name: "播放音频" }));
    expect(scrollIntoView).toHaveBeenCalledWith({
      block: "nearest",
      behavior: "auto",
    });
    media.completePlay();

    fireEvent.scroll(transcript);
    scrollIntoView.mockClear();
    fireEvent.change(screen.getByRole("slider", { name: "音频进度" }), {
      target: { value: "62" },
    });
    expect(scrollIntoView).toHaveBeenCalledTimes(1);
  });

  it("keeps the playback position and transcript stable while buffering", () => {
    const { container } = renderPlayer({ audioDurationSeconds: 120 });
    const audio = container.querySelector("audio")!;
    const media = controlAudio(audio);
    fireEvent.click(screen.getByRole("button", { name: "播放音频" }));
    fireEvent.loadedMetadata(audio);
    fireEvent.canPlay(audio);
    media.completePlay();

    audio.currentTime = 31;
    fireEvent.timeUpdate(audio);
    const currentSegment = screen.getByRole("button", {
      name: "00:30 嘉宾：中段内容",
    });
    expect(currentSegment).toHaveAttribute("aria-current", "true");

    fireEvent.waiting(audio);
    expect(screen.getByText("音频缓冲中…")).toBeVisible();
    expect(screen.getByText("00:31 / 02:00")).toBeVisible();
    expect(currentSegment).toHaveAttribute("aria-current", "true");

    fireEvent.playing(audio);
    expect(screen.queryByText("音频缓冲中…")).not.toBeInTheDocument();
    expect(screen.getByText("00:31 / 02:00")).toBeVisible();
    expect(currentSegment).toHaveAttribute("aria-current", "true");
  });

  it("keeps position, chapters, and navigation available while the first preparation is slow", () => {
    const { container } = renderPlayer({
      audioDurationSeconds: 120,
      chapters: [
        { order: 1, start_ms: 0, title: "开场章节", summary: "介绍" },
        { order: 2, start_ms: 30_000, title: "中段章节", summary: "讨论" },
      ],
    });
    const audio = container.querySelector("audio")!;
    const media = controlAudio(audio);
    const second = screen.getByRole("button", {
      name: "00:30 嘉宾：中段内容",
    });
    const third = screen.getByRole("button", {
      name: "01:00 主持人：尾段内容",
    });
    // Pre-play navigation only records the pending position.
    fireEvent.click(second);
    expect(media.load).not.toHaveBeenCalled();
    expect(screen.getByText("00:30 / 02:00")).toBeVisible();
    expect(second).toHaveAttribute("aria-current", "true");

    const transcript = screen.getByRole("region", { name: "同步逐字稿" });
    Object.defineProperty(transcript, "getBoundingClientRect", {
      configurable: true,
      value: () => ({ top: 0, bottom: 100 }),
    });
    Object.defineProperty(second, "getBoundingClientRect", {
      configurable: true,
      value: () => ({ top: 130, bottom: 170 }),
    });
    const scrollIntoView = vi.fn();
    Object.defineProperty(second, "scrollIntoView", {
      configurable: true,
      value: scrollIntoView,
    });
    fireEvent.scroll(transcript);
    fireEvent.click(screen.getByRole("button", { name: "播放音频" }));
    expect(screen.getByText("正在准备播放")).toBeVisible();
    expect(scrollIntoView).not.toHaveBeenCalled();
    fireEvent.loadedMetadata(audio);
    expect(audio.currentTime).toBe(30);
    expect(screen.getByText("中段内容")).toBeVisible();

    // Repeated intents merge into the single in-flight request, but the
    // pending position and the chapter navigation stay live.
    fireEvent.click(screen.getByRole("button", { name: "播放音频" }));
    fireEvent.click(third);
    expect(media.load).toHaveBeenCalledTimes(1);
    expect(audio.currentTime).toBe(60);
    expect(third).toHaveAttribute("aria-current", "true");
    fireEvent.scroll(transcript);

    chooseRate("1.5×");
    fireEvent.canPlay(audio);
    media.completePlay();
    expect(screen.queryByText("正在准备播放")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "暂停音频" })).toBeVisible();
    expect(audio.currentTime).toBe(60);
    expect(audio.playbackRate).toBe(1.5);
    expect(media.load).toHaveBeenCalledTimes(1);
    expect(screen.getByText("自动跟随已暂停")).toBeVisible();
  });

  it("treats the server duration as a display hint until media metadata arrives", () => {
    const laterSegments: TranscriptSegment[] = [
      segments[0],
      { ...segments[1], start_ms: 90_000 },
    ];
    const { container } = renderPlayer({
      segments: laterSegments,
      audioDurationSeconds: 30,
    });
    const audio = container.querySelector("audio")!;
    const media = controlAudio(audio, 120);
    const laterSegment = screen.getByRole("button", {
      name: "01:30 嘉宾：中段内容",
    });

    fireEvent.click(laterSegment);
    expect(audio.currentTime).toBe(0);
    expect(media.load).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "播放音频" }));
    fireEvent.loadedMetadata(audio);
    expect(audio.currentTime).toBe(90);
    fireEvent.canPlay(audio);
    media.completePlay();
    expect(screen.getByRole("button", { name: "暂停音频" })).toBeVisible();
  });

  it("uses a corrected media duration for later seeks", () => {
    const correctedSegments: TranscriptSegment[] = [
      segments[0],
      { ...segments[1], start_ms: 150_000 },
    ];
    const { container } = renderPlayer({
      segments: correctedSegments,
      audioDurationSeconds: 30,
    });
    const audio = container.querySelector("audio")!;
    controlAudio(audio, 120);
    fireEvent.click(screen.getByRole("button", { name: "播放音频" }));
    fireEvent.loadedMetadata(audio);

    Object.defineProperty(audio, "duration", {
      configurable: true,
      value: 180,
    });
    fireEvent.durationChange(audio);
    fireEvent.click(
      screen.getByRole("button", { name: "02:30 嘉宾：中段内容" }),
    );

    expect(audio.currentTime).toBe(150);
    expect(screen.getByText("02:30 / 03:00")).toBeVisible();
  });

  it("stops the first preparation after 30 seconds and retries with one new request", () => {
    vi.useFakeTimers();
    const { container } = renderPlayer({ audioDurationSeconds: 120 });
    const audio = container.querySelector("audio")!;
    const media = controlAudio(audio);

    fireEvent.click(screen.getByRole("button", { name: "播放音频" }));
    expect(screen.getByText("正在准备播放")).toBeVisible();
    expect(media.play).toHaveBeenCalledTimes(1);

    act(() => {
      vi.advanceTimersByTime(15_000);
    });
    expect(screen.getByText("正在准备播放")).toBeVisible();
    expect(audio).toHaveAttribute("src", "/api/v1/artifact-sets/82/audio");
    expect(media.pause).not.toHaveBeenCalled();

    act(() => {
      vi.advanceTimersByTime(15_000);
    });
    expect(screen.getByText("音频准备超时，请重试。")).toBeVisible();
    expect(screen.getByText("中段内容")).toBeVisible();
    expect(audio).not.toHaveAttribute("src");
    expect(media.pause).toHaveBeenCalled();
    expect(media.load).toHaveBeenCalledTimes(2);
    expect(media.play).toHaveBeenCalledTimes(1);

    // The stopped request must not resume or retry on its own.
    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    expect(media.load).toHaveBeenCalledTimes(2);
    expect(media.play).toHaveBeenCalledTimes(1);
    expect(queryMediaStatus()?.textContent).toBe("音频准备超时，请重试。");

    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    expect(screen.getByText("正在准备播放")).toBeVisible();
    expect(
      screen.queryByText("音频准备超时，请重试。"),
    ).not.toBeInTheDocument();
    expect(audio).toHaveAttribute("src", "/api/v1/artifact-sets/82/audio");
    expect(media.load).toHaveBeenCalledTimes(3);

    fireEvent.loadedMetadata(audio);
    fireEvent.canPlay(audio);
    media.completePlay();
    expect(screen.getByRole("button", { name: "暂停音频" })).toBeVisible();
    expect(media.load).toHaveBeenCalledTimes(3);
  });

  it("reports media failure without replacing the transcript and retries with the pending position", () => {
    const { container } = renderPlayer({ audioDurationSeconds: 120 });
    const audio = container.querySelector("audio")!;
    const media = controlAudio(audio);

    fireEvent.click(screen.getByRole("button", { name: "播放音频" }));
    fireEvent.loadedMetadata(audio);
    fireEvent.canPlay(audio);
    media.completePlay();
    audio.currentTime = 31;
    fireEvent.timeUpdate(audio);
    expect(
      screen.getByRole("button", { name: "00:30 嘉宾：中段内容" }),
    ).toHaveAttribute("aria-current", "true");

    fireEvent.error(audio);
    expect(screen.getByText("音频加载失败，逐字稿仍可阅读。")).toBeVisible();
    expect(screen.getByText("中段内容")).toBeVisible();
    expect(screen.getByRole("button", { name: "播放音频" })).toBeDisabled();
    expect(rateButton()).toBeDisabled();

    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    expect(screen.getByText("正在准备播放")).toBeVisible();
    expect(media.load).toHaveBeenCalledTimes(2);
    expect(media.play).toHaveBeenCalledTimes(2);
    expect(screen.getByText("尾段内容")).toBeVisible();

    fireEvent.loadedMetadata(audio);
    expect(audio.currentTime).toBe(31);
    fireEvent.canPlay(audio);
    media.completePlay();
    expect(screen.getByRole("button", { name: "暂停音频" })).toBeVisible();
    expect(media.load).toHaveBeenCalledTimes(2);
  });

  it("starts from a chosen chapter with lazy loading and keeps chapters seekable while playing", () => {
    const { container } = renderPlayer({
      audioDurationSeconds: 120,
      chapters: [
        { order: 1, start_ms: 0, title: "开场章节", summary: "介绍" },
        { order: 2, start_ms: 30_000, title: "中段章节", summary: "讨论" },
      ],
    });
    const chapterNav = screen.getByText("智能章节 · 2");
    const chapterDetails = chapterNav.closest("details");
    expect(chapterDetails).not.toHaveAttribute("open");
    fireEvent.click(chapterNav);
    expect(chapterDetails).toHaveAttribute("open");
    expect(screen.getByText("中段章节")).toBeVisible();

    const audio = container.querySelector("audio")!;
    const media = controlAudio(audio);
    fireEvent.click(screen.getByRole("button", { name: /00:30\s+中段章节/ }));
    expect(media.load).toHaveBeenCalledTimes(1);
    expect(screen.getByText("正在准备播放")).toBeVisible();
    fireEvent.loadedMetadata(audio);
    expect(audio.currentTime).toBe(30);
    fireEvent.canPlay(audio);
    media.completePlay();
    expect(
      screen.getByRole("button", { name: "00:30 嘉宾：中段内容" }),
    ).toHaveAttribute("aria-current", "true");
    expect(screen.getByRole("button", { name: "暂停音频" })).toBeVisible();

    fireEvent.click(screen.getByRole("button", { name: "暂停音频" }));
    fireEvent.click(screen.getByRole("button", { name: /00:30\s+中段章节/ }));
    expect(audio.currentTime).toBe(30);
    media.completePlay();
    expect(screen.getByRole("button", { name: "暂停音频" })).toBeVisible();
    expect(media.load).toHaveBeenCalledTimes(1);
  });

  it("chooses the last segment whose start is not later than playback", () => {
    const equalStartSegments: TranscriptSegment[] = [
      segments[0],
      { ...segments[1], order: 2 },
      { ...segments[2], order: 3, start_ms: 30_000 },
    ];
    const { container } = renderPlayer({
      segments: equalStartSegments,
      audioDurationSeconds: 120,
    });
    const audio = container.querySelector("audio")!;
    const media = controlAudio(audio);
    fireEvent.click(screen.getByRole("button", { name: "播放音频" }));
    fireEvent.loadedMetadata(audio);
    fireEvent.canPlay(audio);
    media.completePlay();
    audio.currentTime = 30;
    fireEvent.timeUpdate(audio);
    expect(
      screen.getByRole("button", { name: "00:30 主持人：尾段内容" }),
    ).toHaveAttribute("aria-current", "true");
  });

  it("scrolls to the matching transcript text when audio is unavailable", () => {
    renderPlayer({
      mediaAvailable: false,
      chapters: [
        { order: 1, start_ms: 0, title: "开场章节", summary: "介绍" },
        { order: 2, start_ms: 30_000, title: "中段章节", summary: "讨论" },
      ],
    });
    const transcript = screen.getByRole("region", { name: "同步逐字稿" });
    const target = screen.getByText("中段内容").closest("article")!;
    Object.defineProperty(transcript, "getBoundingClientRect", {
      configurable: true,
      value: () => ({ top: 0, bottom: 100 }),
    });
    Object.defineProperty(target, "getBoundingClientRect", {
      configurable: true,
      value: () => ({ top: 130, bottom: 170 }),
    });
    const scrollIntoView = vi.fn();
    Object.defineProperty(target, "scrollIntoView", {
      configurable: true,
      value: scrollIntoView,
    });

    fireEvent.click(screen.getByText("智能章节 · 2"));
    fireEvent.click(screen.getByRole("button", { name: /00:30\s+中段章节/ }));

    expect(target).toHaveAttribute("aria-current", "true");
    expect(scrollIntoView).toHaveBeenCalledWith({
      block: "nearest",
      behavior: "auto",
    });
  });

  it("cancels the previous audio node on keyed replacement and unmount", async () => {
    vi.useFakeTimers();
    const view = renderPlayer({ audioDurationSeconds: 120 });
    const audio = view.container.querySelector("audio")!;
    const firstMedia = controlAudio(audio);
    fireEvent.click(screen.getByRole("button", { name: "播放音频" }));
    expect(screen.getByText("正在准备播放")).toBeVisible();

    // A newly completed artifact replaces the keyed audio node without
    // unmounting the player component.
    await act(async () => {
      view.rerender(
        <StatefulTranscriptAudioPlayer
          artifactSetId={83}
          audioDurationSeconds={90}
        />,
      );
      await Promise.resolve();
    });
    const nextAudio = view.container.querySelector("audio")!;
    expect(nextAudio).not.toBe(audio);
    expect(audio).not.toHaveAttribute("src");
    expect(firstMedia.pause).toHaveBeenCalled();
    expect(firstMedia.load).toHaveBeenCalledTimes(2);
    expect(nextAudio).not.toHaveAttribute("src");
    expect(screen.getByText("00:00 / 01:30")).toBeVisible();

    act(() => {
      vi.advanceTimersByTime(30_000);
    });
    expect(screen.queryByText("音频准备超时，请重试。")).not.toBeInTheDocument();

    const nextMedia = controlAudio(nextAudio, 90);
    fireEvent.click(screen.getByRole("button", { name: "播放音频" }));
    expect(nextAudio).toHaveAttribute(
      "src",
      "/api/v1/artifact-sets/83/audio",
    );
    view.unmount();
    expect(nextAudio).not.toHaveAttribute("src");
    expect(nextMedia.pause).toHaveBeenCalled();
    expect(nextMedia.load).toHaveBeenCalledTimes(2);
  });

  it("keeps the bounded preparation until media is playable even when play fires early", () => {
    const { container } = renderPlayer({ audioDurationSeconds: 120 });
    const audio = container.querySelector("audio")!;
    const media = controlAudio(audio);

    fireEvent.click(screen.getByRole("button", { name: "播放音频" }));
    expect(screen.getByText("正在准备播放")).toBeVisible();

    // Real browsers raise `play` as soon as playback is intended, before any
    // media data arrives; buffering reports stay inside the preparation.
    fireEvent.play(audio);
    expect(screen.getByRole("button", { name: "播放音频" })).toBeDisabled();
    expect(screen.getByText("正在准备播放")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "播放音频" }));
    expect(media.load).toHaveBeenCalledTimes(1);
    expect(media.play).toHaveBeenCalledTimes(1);
    fireEvent.waiting(audio);
    fireEvent.stalled(audio);
    expect(screen.queryByText("音频缓冲中…")).not.toBeInTheDocument();

    fireEvent.loadedMetadata(audio);
    fireEvent.canPlay(audio);
    expect(screen.queryByText("正在准备播放")).not.toBeInTheDocument();
    media.completePlay();
    expect(screen.getByRole("button", { name: "暂停音频" })).toBeVisible();
    expect(media.load).toHaveBeenCalledTimes(1);
  });

  it("keeps the player usable when the response predates audio durations", () => {
    const { container } = renderPlayer();
    const audio = container.querySelector("audio")!;
    expect(screen.getByText("00:00 / --:--")).toBeVisible();
    expect(screen.getByRole("slider", { name: "音频进度" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "播放音频" })).toBeEnabled();

    const media = controlAudio(audio);
    fireEvent.click(screen.getByRole("button", { name: "播放音频" }));
    expect(screen.getByText("正在准备播放")).toBeVisible();
    fireEvent.loadedMetadata(audio);
    expect(screen.getByText("00:00 / 02:00")).toBeVisible();
    expect(screen.getByRole("slider", { name: "音频进度" })).toBeEnabled();
    media.completePlay();
    expect(screen.getByRole("button", { name: "暂停音频" })).toBeVisible();
  });

  it("exposes the five presets as a compact checked menu with full keyboard support", async () => {
    const user = userEvent.setup();
    renderPlayer({ audioDurationSeconds: 120 });
    const trigger = rateButton();
    expect(trigger).toHaveTextContent("1×");
    expect(trigger).toHaveAccessibleName("播放倍速，当前 1×");
    expect(trigger).toHaveAttribute("aria-haspopup", "menu");
    expect(trigger).toHaveAttribute("aria-expanded", "false");

    trigger.focus();
    await user.keyboard("{Enter}");
    const menu = screen.getByRole("menu", { name: "选择播放倍速" });
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    const options = within(menu).getAllByRole("menuitemradio");
    expect(options.map((option) => option.textContent)).toEqual([
      "0.75×",
      "1×",
      "1.25×",
      "1.5×",
      "2×",
    ]);
    // The checked preset is marked for assistive tech and highlighted in the
    // menu, not just colored.
    expect(options[1]).toHaveAttribute("aria-checked", "true");
    expect(options[1]).toHaveFocus();

    fireEvent.keyDown(menu, { key: "ArrowDown" });
    expect(options[2]).toHaveFocus();
    fireEvent.keyDown(menu, { key: "End" });
    expect(options[4]).toHaveFocus();
    fireEvent.keyDown(menu, { key: "Home" });
    expect(options[0]).toHaveFocus();
    fireEvent.keyDown(options[0], { key: "ArrowUp" });
    expect(options[4]).toHaveFocus();

    // Escape closes and returns focus to the rate button.
    fireEvent.keyDown(options[4], { key: "Escape" });
    expect(
      screen.queryByRole("menu", { name: "选择播放倍速" }),
    ).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();

    // Space opens and selects through the native button contract.
    await user.keyboard(" ");
    const spaceMenu = screen.getByRole("menu", { name: "选择播放倍速" });
    fireEvent.keyDown(spaceMenu, { key: "ArrowDown" });
    await user.keyboard(" ");
    expect(rateButton()).toHaveTextContent("1.25×");
    expect(
      screen.queryByRole("menu", { name: "选择播放倍速" }),
    ).not.toBeInTheDocument();

    // Enter also activates a focused preset.
    await user.keyboard("{Enter}");
    const enterMenu = screen.getByRole("menu", { name: "选择播放倍速" });
    fireEvent.keyDown(enterMenu, { key: "ArrowDown" });
    await user.keyboard("{Enter}");
    expect(rateButton()).toHaveTextContent("1.5×");

    // A press outside the menu also closes it.
    fireEvent.click(trigger);
    expect(
      screen.getByRole("menu", { name: "选择播放倍速" }),
    ).toBeInTheDocument();
    fireEvent.pointerDown(document.body);
    expect(
      screen.queryByRole("menu", { name: "选择播放倍速" }),
    ).not.toBeInTheDocument();

    // The shared menu closes after Tab moves focus beyond its trigger/menu.
    trigger.focus();
    await user.keyboard("{Enter}");
    const tabMenu = screen.getByRole("menu", { name: "选择播放倍速" });
    const tabOptions = within(tabMenu).getAllByRole("menuitemradio");
    expect(tabOptions[3]).toHaveFocus();
    await user.tab();
    expect(tabOptions[4]).toHaveFocus();
    expect(tabMenu).toBeInTheDocument();
    await user.tab();
    expect(
      screen.queryByRole("menu", { name: "选择播放倍速" }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "同步逐字稿" })).toHaveFocus();
  });
});
