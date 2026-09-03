import { act, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
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

function prepareAudio(audio: HTMLAudioElement, duration = 120) {
  let paused = true;
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
  });
  const play = vi.fn(async () => {
    paused = false;
    fireEvent.play(audio);
  });
  const pause = vi.fn(() => {
    paused = true;
    fireEvent.pause(audio);
  });
  const load = vi.fn();
  Object.defineProperties(audio, {
    play: { configurable: true, value: play },
    pause: { configurable: true, value: pause },
    load: { configurable: true, value: load },
  });
  fireEvent.loadedMetadata(audio);
  return { play, pause, load };
}

interface TestPlayerProps {
  artifactSetId?: number;
  segments?: TranscriptSegment[];
  mediaAvailable?: boolean;
  chapters?: MinutesChapter[];
}

function StatefulTranscriptAudioPlayer({
  artifactSetId = 82,
  segments: playerSegments = segments,
  mediaAvailable = true,
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
      playbackRate={playbackRate}
      onPlaybackRateChange={setPlaybackRate}
      chapters={chapters}
    />
  );
}

function renderPlayer(props: TestPlayerProps = {}) {
  return render(<StatefulTranscriptAudioPlayer {...props} />);
}

describe("TranscriptAudioPlayer", () => {
  it("syncs public media events, slider keys, and segment clicks", async () => {
    const { container } = renderPlayer();
    const audio = container.querySelector("audio");
    expect(audio).not.toBeNull();
    expect(audio).toHaveAttribute("src", "/api/v1/artifact-sets/82/audio");
    const media = prepareAudio(audio!);
    const playbackRate = screen.getByRole("combobox", { name: "播放倍速" });
    expect(playbackRate).toHaveValue("1");
    expect(
      Array.from(playbackRate.querySelectorAll("option")).map(
        (option) => option.value,
      ),
    ).toEqual(["0.75", "1", "1.25", "1.5", "2"]);
    expect(audio!.playbackRate).toBe(1);

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
    expect(screen.getByText("当前段落")).toBeVisible();

    fireEvent.click(screen.getByRole("button", { name: "播放音频" }));
    await act(async () => {
      await Promise.resolve();
    });
    expect(media.play).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "暂停音频" })).toBeVisible();

    audio!.currentTime = 31;
    fireEvent.timeUpdate(audio!);
    expect(second).toHaveAttribute("aria-current", "true");
    expect(screen.getByText("正在播放")).toBeVisible();

    fireEvent.change(playbackRate, { target: { value: "1.5" } });
    expect(playbackRate).toHaveValue("1.5");
    expect(audio!.playbackRate).toBe(1.5);
    expect(audio!.currentTime).toBe(31);
    expect(second).toHaveAttribute("aria-current", "true");

    const slider = screen.getByRole("slider", { name: "音频进度" });
    fireEvent.change(slider, { target: { value: "61" } });
    expect(audio!.currentTime).toBe(61);
    expect(third).toHaveAttribute("aria-current", "true");

    slider.focus();
    fireEvent.keyDown(slider, { key: "Home" });
    expect(slider).toHaveFocus();
    expect(audio!.currentTime).toBe(0);
    expect(first).toHaveAttribute("aria-current", "true");
    fireEvent.keyDown(slider, { key: "ArrowRight" });
    expect(audio!.currentTime).toBe(5);
    fireEvent.keyDown(slider, { key: "End" });
    expect(audio!.currentTime).toBe(120);
    expect(third).toHaveAttribute("aria-current", "true");

    second.focus();
    fireEvent.click(second);
    expect(second).toHaveFocus();
    expect(audio!.currentTime).toBe(30);
    expect(second).toHaveAttribute("aria-current", "true");

    fireEvent.click(screen.getByRole("button", { name: "暂停音频" }));
    expect(media.pause).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("button", { name: "播放音频" }));
    await act(async () => {
      await Promise.resolve();
    });
    expect(audio!.playbackRate).toBe(1.5);
    expect(screen.getByRole("button", { name: "暂停音频" })).toBeVisible();
  });

  it("pauses follow after manual scrolling and resumes it on play or seek", async () => {
    const { container } = renderPlayer();
    const audio = container.querySelector("audio")!;
    prepareAudio(audio);
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

    fireEvent.play(audio);
    expect(scrollIntoView).toHaveBeenCalledWith({
      block: "nearest",
      behavior: "auto",
    });

    fireEvent.scroll(transcript);
    scrollIntoView.mockClear();
    fireEvent.change(screen.getByRole("slider", { name: "音频进度" }), {
      target: { value: "62" },
    });
    expect(scrollIntoView).toHaveBeenCalledTimes(1);
  });

  it("keeps transcript text readable while media is slow, failed, or unavailable", () => {
    const { container, rerender } = renderPlayer();
    expect(screen.getByText("正在加载音频…")).toBeVisible();
    expect(screen.getByText("中段内容")).toBeVisible();
    expect(screen.getByRole("combobox", { name: "播放倍速" })).toBeDisabled();

    const audio = container.querySelector("audio")!;
    const { load } = prepareAudio(audio);
    const source = audio.getAttribute("src");
    fireEvent.error(audio);
    expect(screen.getByText("音频加载失败，逐字稿仍可阅读。")).toBeVisible();
    expect(screen.getByText("尾段内容")).toBeVisible();
    expect(screen.getByRole("combobox", { name: "播放倍速" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    expect(load).toHaveBeenCalledTimes(1);
    expect(audio).toHaveAttribute("src", source);
    expect(screen.getByText("正在加载音频…")).toBeVisible();
    expect(screen.getByRole("combobox", { name: "播放倍速" })).toBeDisabled();

    rerender(
      <StatefulTranscriptAudioPlayer
        artifactSetId={83}
        mediaAvailable={false}
      />,
    );
    expect(screen.queryByRole("slider")).toBeDisabled();
    expect(screen.queryByRole("button", { name: "播放音频" })).toBeDisabled();
    expect(screen.getByRole("combobox", { name: "播放倍速" })).toBeDisabled();
    expect(
      screen.queryByRole("button", { name: /主持人：开场内容/ }),
    ).toBeNull();
    expect(screen.getByText("开场内容")).toBeVisible();
    expect(screen.getByText("音频不可用，逐字稿仍可阅读。")).toBeVisible();
  });

  it("chooses the last segment whose start is not later than playback", () => {
    const equalStartSegments: TranscriptSegment[] = [
      segments[0],
      { ...segments[1], order: 2 },
      { ...segments[2], order: 3, start_ms: 30_000 },
    ];
    const { container } = renderPlayer({ segments: equalStartSegments });
    const audio = container.querySelector("audio")!;
    prepareAudio(audio);
    audio.currentTime = 30;
    fireEvent.timeUpdate(audio);
    expect(
      screen.getByRole("button", { name: "00:30 主持人：尾段内容" }),
    ).toHaveAttribute("aria-current", "true");
  });

  it("keeps chapters collapsed until opened and seeks plus plays on click", async () => {
    const { container } = renderPlayer({
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
    const media = prepareAudio(audio);
    fireEvent.click(screen.getByRole("button", { name: /00:30\s+中段章节/ }));
    expect(audio.currentTime).toBe(30);
    await act(async () => {
      await Promise.resolve();
    });
    expect(media.play).toHaveBeenCalled();
    expect(
      screen.getByRole("button", { name: "00:30 嘉宾：中段内容" }),
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
});
