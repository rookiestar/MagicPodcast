import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import EpisodeProcessingPanel from "../EpisodeProcessingPanel";
import type { ConsumptionItem } from "@/types/consumption";
import type {
  ArtifactContent,
  EpisodeArtifactSet,
  ProcessingRun,
} from "@/types/processing";

const apiMocks = vi.hoisted(() => ({
  listEpisodeRuns: vi.fn(),
  getLatestAudio: vi.fn(),
  getScheduleStatus: vi.fn(),
  getRun: vi.fn(),
  start: vi.fn(),
  cancel: vi.fn(),
  retry: vi.fn(),
  recoverAudio: vi.fn(),
  getArtifactContent: vi.fn(),
}));

vi.mock("@/lib/api/processing", () => ({
  processingApi: apiMocks,
  getProcessingErrorDetails: vi.fn((error: unknown) => ({
    message: error instanceof Error ? error.message : "加工请求失败",
  })),
}));

const item: ConsumptionItem = {
  episode_id: 263,
  podcast_id: 26,
  podcast_title: "测试节目",
  podcast_author: "测试作者",
  podcast_cover_url: "",
  episode_title: "视觉归属测试",
  episode_no: "263",
  duration: 1800,
  published_date: "2026-09-05T08:00:00Z",
  show_notes: "",
  original_url: "https://example.com/episode/263",
  image_url: "",
  notes: "",
  tags: [],
  queue_state: "focus",
};

const run: ProcessingRun = {
  id: 263,
  episode_id: item.episode_id,
  pipeline_version: "focus-processing-v2",
  trigger_source: "manual",
  status: "completed",
  current_step: "",
  attempt_count: 1,
  max_attempts: 3,
  error_retryable: false,
  created_at: "2026-09-05T08:00:00Z",
  updated_at: "2026-09-05T08:05:00Z",
};

const artifact: EpisodeArtifactSet = {
  id: 263,
  run_id: run.id,
  episode_id: item.episode_id,
  pipeline_version: run.pipeline_version,
  manifest_path: "manifest.json",
  manifest_sha256: "a".repeat(64),
  minutes_summary_sha256: "b".repeat(64),
  transcript_sha256: "c".repeat(64),
  notes_sha256: "",
  capabilities: {
    minutes_summary: true,
    transcript: false,
    structured_timeline: false,
    matching_audio: false,
    legacy_episode_notes: false,
  },
  is_current: true,
  created_at: "2026-09-05T08:05:00Z",
};

const minutesContent: ArtifactContent = {
  kind: "minutes_summary",
  content: "# 纪要\n\n正文锚点\n\n后续正文",
  sha256: artifact.minutes_summary_sha256 ?? "",
  media_available: false,
  whiteboard: {
    media_id: "whiteboard",
    media_type: "image/png",
    width: 1280,
    height: 720,
    sha256: "d".repeat(64),
    alt: "总结画板",
  },
  visual_items: [
    {
      type: "whiteboard",
      media_id: "whiteboard",
      media_type: "image/png",
      width: 1280,
      height: 720,
      sha256: "d".repeat(64),
      alt: "总结画板",
    },
    {
      type: "image",
      media_id: "image-1",
      media_type: "image/png",
      width: 960,
      height: 540,
      sha256: "e".repeat(64),
      alt: "正文插图",
    },
  ],
  inline_images: [
    {
      media_id: "image-1",
      section: "summary",
      anchor_text: "正文锚点",
      anchor_occurrence: 1,
    },
  ],
};

function expectBefore(first: Element, second: Element) {
  expect(
    first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).not.toBe(0);
}

describe("EpisodeProcessingPanel visual tab user flow", () => {
  beforeEach(() => {
    Object.values(apiMocks).forEach((mock) => mock.mockReset());
    apiMocks.listEpisodeRuns.mockResolvedValue([run]);
    apiMocks.getRun.mockResolvedValue({
      run,
      current_artifact: artifact,
      deliveries: [],
    });
    apiMocks.getScheduleStatus.mockResolvedValue({
      enabled: false,
      cron: "",
      timezone: "",
      batch_size: 0,
    });
    apiMocks.getArtifactContent.mockResolvedValue(minutesContent);
  });

  it.each([390, 1280])(
    "keeps the whiteboard and body image in their own tabs at %ipx",
    async (viewportWidth) => {
      const previousWidth = window.innerWidth;
      Object.defineProperty(window, "innerWidth", {
        configurable: true,
        value: viewportWidth,
      });
      window.dispatchEvent(new Event("resize"));
      const view = render(<EpisodeProcessingPanel item={item} />);

      try {
        const summaryTab = await screen.findByRole("tab", { name: "总结" });
        expect(summaryTab).toHaveAttribute("aria-selected", "true");
        expect(
          await screen.findByRole("img", { name: "总结画板" }),
        ).toBeVisible();
        expect(
          screen.queryByRole("img", { name: "正文插图" }),
        ).not.toBeInTheDocument();

        fireEvent.click(screen.getByRole("tab", { name: "纪要" }));
        const bodyImage = await screen.findByRole("img", { name: "正文插图" });
        expect(
          screen.queryByRole("img", { name: "总结画板" }),
        ).not.toBeInTheDocument();
        expectBefore(screen.getByText("正文锚点"), bodyImage);
        expectBefore(bodyImage, screen.getByText("后续正文"));

        fireEvent.click(summaryTab);
        expect(
          await screen.findByRole("img", { name: "总结画板" }),
        ).toBeVisible();
        expect(
          screen.queryByRole("img", { name: "正文插图" }),
        ).not.toBeInTheDocument();
      } finally {
        view.unmount();
        Object.defineProperty(window, "innerWidth", {
          configurable: true,
          value: previousWidth,
        });
        window.dispatchEvent(new Event("resize"));
      }
    },
  );
});
