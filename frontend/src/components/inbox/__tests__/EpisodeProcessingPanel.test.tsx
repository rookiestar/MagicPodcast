import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import EpisodeProcessingPanel from "../EpisodeProcessingPanel";
import type { ConsumptionItem } from "@/types/consumption";
import type {
  EpisodeArtifactSet,
  ProcessingRun,
  ProcessingRunDetail,
} from "@/types/processing";

const apiMocks = vi.hoisted(() => ({
  listEpisodeRuns: vi.fn(),
  getLatestAudio: vi.fn(),
  getRun: vi.fn(),
  start: vi.fn(),
  cancel: vi.fn(),
  retry: vi.fn(),
  getArtifactContent: vi.fn(),
}));

vi.mock("@/lib/api/processing", () => ({
  processingApi: apiMocks,
  getProcessingErrorDetails: vi.fn((error: unknown) => {
    const candidate = error as {
      message?: string;
      response?: {
        status?: number;
        data?: { error?: { code?: string; message?: string } };
      };
    };
    return {
      code: candidate?.response?.data?.error?.code,
      message:
        candidate?.response?.data?.error?.message ||
        candidate?.message ||
        "加工请求失败",
      status: candidate?.response?.status,
    };
  }),
}));

vi.mock("@/components/workflows/MarkdownViewer", () => ({
  default: ({ content }: { content: string }) => <pre>{content}</pre>,
}));

const item: ConsumptionItem = {
  episode_id: 201,
  podcast_id: 20,
  podcast_title: "测试节目",
  podcast_author: "测试作者",
  podcast_cover_url: "",
  episode_title: "加工测试",
  episode_no: "201",
  duration: 2400,
  published_date: "2026-08-10T08:00:00Z",
  show_notes: "<p>正文</p>",
  original_url: "https://example.com/episode/201",
  image_url: "",
  notes: "",
  tags: [],
  queue_state: "focus",
  queue_updated_at: "2026-08-10T08:00:00Z",
};

const failedRun: ProcessingRun = {
  id: 31,
  episode_id: item.episode_id,
  pipeline_version: "focus-processing-v1",
  trigger_source: "manual",
  status: "failed",
  current_step: "episode_notes",
  attempt_count: 1,
  max_attempts: 3,
  error_code: "RUNTIME_UNAVAILABLE",
  error_message: "Codex Runtime 暂不可用",
  error_retryable: true,
  created_at: "2026-08-24T08:00:00Z",
  updated_at: "2026-08-24T08:05:00Z",
};

const artifact: EpisodeArtifactSet = {
  id: 41,
  run_id: 30,
  episode_id: item.episode_id,
  pipeline_version: "focus-processing-v1",
  manifest_path: "manifest.json",
  manifest_sha256: "a".repeat(64),
  transcript_sha256: "b".repeat(64),
  notes_sha256: "c".repeat(64),
  is_current: true,
  created_at: "2026-08-24T07:00:00Z",
};

function detail(run: ProcessingRun = failedRun): ProcessingRunDetail {
  return {
    run,
    current_artifact: artifact,
    deliveries: [],
    action_suggestion: "恢复 Runtime 后从检查点重试。",
  };
}

describe("EpisodeProcessingPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    apiMocks.listEpisodeRuns.mockResolvedValue([]);
    apiMocks.getLatestAudio.mockRejectedValue({
      response: { status: 404 },
    });
  });

  it("keeps a stable first-visit state while processing status is slow", async () => {
    let resolveRuns: (runs: ProcessingRun[]) => void = () => undefined;
    apiMocks.listEpisodeRuns.mockReturnValue(
      new Promise<ProcessingRun[]>((resolve) => {
        resolveRuns = resolve;
      }),
    );

    render(<EpisodeProcessingPanel item={item} />);

    expect(screen.getByRole("heading", { name: "自动加工" })).toBeVisible();
    expect(screen.getByRole("status", { name: "" })).toHaveTextContent(
      "正在读取…",
    );
    expect(screen.getByText("尚未加工")).toBeVisible();

    resolveRuns([]);
    await waitFor(() =>
      expect(screen.queryByText("正在读取…")).not.toBeInTheDocument(),
    );
    expect(
      screen.getByRole("button", { name: "开始加工" }),
    ).toBeEnabled();
  });

  it("clears the previous episode state before a slow identity switch", async () => {
    let resolveSecond: (runs: ProcessingRun[]) => void = () => undefined;
    apiMocks.listEpisodeRuns
      .mockResolvedValueOnce([failedRun])
      .mockReturnValueOnce(
        new Promise<ProcessingRun[]>((resolve) => {
          resolveSecond = resolve;
        }),
      );
    apiMocks.getRun.mockResolvedValue(detail());
    const { rerender } = render(<EpisodeProcessingPanel item={item} />);
    expect(await screen.findByText("加工失败")).toBeVisible();
    expect(screen.getByText("上一成功版本")).toBeVisible();

    rerender(
      <EpisodeProcessingPanel
        item={{ ...item, episode_id: 202, episode_title: "第二集" }}
      />,
    );

    expect(screen.queryByText("加工失败")).not.toBeInTheDocument();
    expect(screen.queryByText("上一成功版本")).not.toBeInTheDocument();
    expect(screen.getByText("尚未加工")).toBeVisible();
    resolveSecond([]);
    await waitFor(() =>
      expect(screen.queryByText("正在读取…")).not.toBeInTheDocument(),
    );
  });

  it("reports status failures without hiding the panel controls", async () => {
    apiMocks.listEpisodeRuns.mockRejectedValue(new Error("网络超时"));

    render(<EpisodeProcessingPanel item={item} />);

    expect(
      await screen.findByText(
        "加工状态读取失败，单集内容不受影响：网络超时",
      ),
    ).toBeVisible();
    expect(screen.getByText("尚未加工")).toBeVisible();
    expect(
      screen.getByRole("button", { name: "开始加工" }),
    ).toBeEnabled();
    expect(
      screen.getByRole("button", { name: "重试读取加工状态" }),
    ).toBeEnabled();
  });

  it("does not disguise managed-audio read failures as an empty first visit", async () => {
    apiMocks.getLatestAudio.mockRejectedValue({
      response: {
        status: 503,
        data: {
          error: {
            code: "AUDIO_ASSET_READ_FAILED",
            message: "音频状态暂不可用",
          },
        },
      },
    });

    render(<EpisodeProcessingPanel item={item} />);

    expect(
      await screen.findByText(
        "加工状态读取失败，单集内容不受影响：音频状态暂不可用",
      ),
    ).toBeVisible();
  });

  it("shows the previous successful artifact and retries from a safe checkpoint", async () => {
    apiMocks.listEpisodeRuns.mockResolvedValue([failedRun]);
    apiMocks.getRun.mockResolvedValue(detail());
    apiMocks.retry.mockResolvedValue({
      run: { ...failedRun, id: 32, status: "queued", error_message: undefined },
      reused_active: false,
      reused_successful: false,
      preparing_audio: false,
    });
    apiMocks.getArtifactContent.mockResolvedValue({
      kind: "transcript",
      content: "# 规范逐字稿",
      sha256: artifact.transcript_sha256,
    });

    const { container } = render(<EpisodeProcessingPanel item={item} />);

    expect(await screen.findByText("加工失败")).toBeVisible();
    expect(screen.getByText("上一成功版本")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "逐字稿" }));
    expect(await screen.findByText("# 规范逐字稿")).toBeVisible();
    expect(
      container.querySelector('[data-copilot-source="transcript"]'),
    ).toHaveAttribute("data-copilot-episode-id", "201");

    fireEvent.click(screen.getByRole("button", { name: "从检查点重试" }));
    await waitFor(() => expect(apiMocks.retry).toHaveBeenCalledWith(31));
    await waitFor(() => expect(apiMocks.getRun).toHaveBeenCalledWith(32));
  });

  it("does not offer an unsafe retry when an external write result is unknown", async () => {
    const unknownRun: ProcessingRun = {
      ...failedRun,
      error_code: "LARK_MINUTES_RESULT_UNKNOWN",
      error_message: "飞书妙记创建结果未知",
      error_retryable: false,
    };
    apiMocks.listEpisodeRuns.mockResolvedValue([unknownRun]);
    apiMocks.getRun.mockResolvedValue(detail(unknownRun));

    render(<EpisodeProcessingPanel item={item} />);

    expect(await screen.findByText("加工失败")).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "从检查点重试" }),
    ).not.toBeInTheDocument();
  });

  it("tracks managed audio preparation after a manual start", async () => {
    const pendingRun: ProcessingRun = {
      ...failedRun,
      id: 51,
      status: "queued",
      current_step: "audio_prepare",
      error_code: undefined,
      error_message: undefined,
      error_retryable: false,
    };
    apiMocks.getRun.mockResolvedValue({
      run: pendingRun,
      deliveries: [],
    });
    apiMocks.start.mockResolvedValue({
      run: pendingRun,
      reused_active: false,
      reused_successful: false,
      preparing_audio: true,
      audio_asset: {
        id: 51,
        episode_id: item.episode_id,
        status: "queued",
        size_bytes: 0,
        duration_seconds: 0,
        queued_at: "2026-08-24T09:00:00Z",
        created_at: "2026-08-24T09:00:00Z",
        updated_at: "2026-08-24T09:00:00Z",
      },
    });

    render(<EpisodeProcessingPanel item={item} />);
    expect(
      await screen.findByRole("button", { name: "开始加工" }),
    ).toBeEnabled();

    fireEvent.click(screen.getByRole("button", { name: "开始加工" }));

    expect(await screen.findByText("等待准备音频")).toBeVisible();
    expect(screen.getByText("准备与校验音频")).toBeVisible();
    expect(screen.getByText("正在下载并校验音频…")).toBeVisible();
    expect(screen.getByRole("button", { name: "取消" })).toBeEnabled();
    expect(apiMocks.start).toHaveBeenCalledWith(item.episode_id);
    expect(apiMocks.getRun).toHaveBeenCalledWith(pendingRun.id);
  });
});
