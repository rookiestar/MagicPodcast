import { act, fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import TranscriptAudioPlayer from "../TranscriptAudioPlayer";
import type { TranscriptSegment } from "@/types/processing";

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

describe("TranscriptAudioPlayer", () => {
  it("syncs public media events, slider keys, and segment clicks", async () => {
    const { container } = render(
      <TranscriptAudioPlayer
        artifactSetId={82}
        segments={segments}
        mediaAvailable
      />,
    );
    const audio = container.querySelector("audio");
    expect(audio).not.toBeNull();
    expect(audio).toHaveAttribute(
      "src",
      "/api/v1/artifact-sets/82/audio",
    );
    const media = prepareAudio(audio!);

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
  });

  it("pauses follow after manual scrolling and resumes it on play or seek", async () => {
    const { container } = render(
      <TranscriptAudioPlayer
        artifactSetId={82}
        segments={segments}
        mediaAvailable
      />,
    );
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
    const { container, rerender } = render(
      <TranscriptAudioPlayer
        artifactSetId={82}
        segments={segments}
        mediaAvailable
      />,
    );
    expect(screen.getByText("正在加载音频…")).toBeVisible();
    expect(screen.getByText("中段内容")).toBeVisible();

    const audio = container.querySelector("audio")!;
    const { load } = prepareAudio(audio);
    const source = audio.getAttribute("src");
    fireEvent.error(audio);
    expect(
      screen.getByText("音频加载失败，逐字稿仍可阅读。"),
    ).toBeVisible();
    expect(screen.getByText("尾段内容")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    expect(load).toHaveBeenCalledTimes(1);
    expect(audio).toHaveAttribute("src", source);
    expect(screen.getByText("正在加载音频…")).toBeVisible();

    rerender(
      <TranscriptAudioPlayer
        artifactSetId={83}
        segments={segments}
        mediaAvailable={false}
      />,
    );
    expect(screen.queryByRole("slider")).toBeDisabled();
    expect(screen.queryByRole("button", { name: "播放音频" })).toBeDisabled();
    expect(screen.queryByRole("button", { name: /主持人：开场内容/ })).toBeNull();
    expect(screen.getByText("开场内容")).toBeVisible();
    expect(screen.getByText("音频不可用，逐字稿仍可阅读。")).toBeVisible();
  });

  it("keeps controls and text present in a 390px viewport", () => {
    const previousWidth = window.innerWidth;
    Object.defineProperty(window, "innerWidth", {
      configurable: true,
      value: 390,
    });
    try {
      render(
        <TranscriptAudioPlayer
          artifactSetId={82}
          segments={segments}
          mediaAvailable
        />,
      );
      expect(
        screen.getByLabelText("逐字稿音频播放器"),
      ).toBeInTheDocument();
      expect(screen.getByRole("slider", { name: "音频进度" })).toBeVisible();
      expect(
        screen.getByRole("region", { name: "同步逐字稿" }),
      ).toBeVisible();
      expect(screen.getByText("尾段内容")).toBeVisible();
    } finally {
      Object.defineProperty(window, "innerWidth", {
        configurable: true,
        value: previousWidth,
      });
    }
  });

  it("chooses the last segment whose start is not later than playback", () => {
    const equalStartSegments: TranscriptSegment[] = [
      segments[0],
      { ...segments[1], order: 2 },
      { ...segments[2], order: 3, start_ms: 30_000 },
    ];
    const { container } = render(
      <TranscriptAudioPlayer
        artifactSetId={82}
        segments={equalStartSegments}
        mediaAvailable
      />,
    );
    const audio = container.querySelector("audio")!;
    prepareAudio(audio);
    audio.currentTime = 30;
    fireEvent.timeUpdate(audio);
    expect(
      screen.getByRole("button", { name: "00:30 主持人：尾段内容" }),
    ).toHaveAttribute("aria-current", "true");
  });
});
