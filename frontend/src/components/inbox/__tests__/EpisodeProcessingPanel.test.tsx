import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import EpisodeProcessingPanel from "../EpisodeProcessingPanel";
import type { ConsumptionItem } from "@/types/consumption";
import type {
  ArtifactContent,
  EpisodeArtifactSet,
  ProcessingRun,
  ProcessingRunDetail,
} from "@/types/processing";

const apiMocks = vi.hoisted(() => ({
  listEpisodeRuns: vi.fn(),
  getLatestAudio: vi.fn(),
  getScheduleStatus: vi.fn(),
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
  capabilities: {
    minutes_summary: false,
    transcript: true,
    structured_timeline: false,
    matching_audio: false,
    legacy_episode_notes: true,
  },
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
    apiMocks.getScheduleStatus.mockResolvedValue({
      enabled: false,
      cron: "",
      timezone: "",
      batch_size: 0,
    });
    apiMocks.getArtifactContent.mockImplementation(
      (_artifactSetId: number, kind: string) =>
        Promise.resolve({
          kind,
          content: kind === "transcript" ? "# 规范逐字稿" : "# 旧版纪要",
          sha256: "d".repeat(64),
          media_available: false,
        }),
    );
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
    expect(screen.getByRole("button", { name: "开始转写" })).toBeEnabled();
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

  it("ignores a stale processing response after switching episodes", async () => {
    let resolveFirst: (runs: ProcessingRun[]) => void = () => undefined;
    apiMocks.listEpisodeRuns
      .mockReturnValueOnce(
        new Promise<ProcessingRun[]>((resolve) => {
          resolveFirst = resolve;
        }),
      )
      .mockResolvedValueOnce([]);
    apiMocks.getRun.mockResolvedValue(detail());
    const { rerender } = render(<EpisodeProcessingPanel item={item} />);

    rerender(
      <EpisodeProcessingPanel
        item={{ ...item, episode_id: 202, episode_title: "第二集" }}
      />,
    );
    await waitFor(() =>
      expect(apiMocks.listEpisodeRuns).toHaveBeenCalledWith(202),
    );
    resolveFirst([failedRun]);
    await act(async () => {
      await Promise.resolve();
    });

    expect(apiMocks.getRun).not.toHaveBeenCalled();
    expect(screen.queryByText("加工失败")).not.toBeInTheDocument();
    expect(screen.getByText("尚未加工")).toBeVisible();
  });

  it("reports status failures without hiding the panel controls", async () => {
    apiMocks.listEpisodeRuns.mockRejectedValue(new Error("网络超时"));

    render(<EpisodeProcessingPanel item={item} />);

    expect(
      await screen.findByText("加工状态读取失败，单集内容不受影响：网络超时"),
    ).toBeVisible();
    expect(screen.getByText("尚未加工")).toBeVisible();
    expect(screen.getByRole("button", { name: "开始转写" })).toBeEnabled();
    expect(
      screen.getByRole("button", { name: "重试读取加工状态" }),
    ).toBeEnabled();
  });

  it("shows scheduled state and the selected episode's skip reason", async () => {
    apiMocks.getScheduleStatus.mockResolvedValue({
      enabled: true,
      cron: "0 0 9 * * *",
      timezone: "Asia/Shanghai",
      batch_size: 1,
      next_run_at: "2026-08-25T09:00:00Z",
      latest_run: {
        run: {
          id: 71,
          scheduled_for: "2026-08-25T08:00:00Z",
          cron_expression: "0 0 9 * * *",
          timezone: "Asia/Shanghai",
          batch_size: 1,
          status: "completed",
          candidate_count: 2,
          started_count: 1,
          skipped_count: 1,
          created_at: "2026-08-25T08:00:00Z",
          updated_at: "2026-08-25T08:00:01Z",
        },
        items: [
          {
            id: 72,
            schedule_run_id: 71,
            episode_id: item.episode_id,
            queue_position: 0,
            outcome: "skipped",
            reason: "batch_limit",
            created_at: "2026-08-25T08:00:01Z",
            updated_at: "2026-08-25T08:00:01Z",
          },
        ],
      },
    });

    render(<EpisodeProcessingPanel item={item} />);

    expect(await screen.findByText("已启用 · 每批 1 集")).toBeVisible();
    expect(
      screen.getByText("cron：0 0 9 * * * · 时区：Asia/Shanghai · 每批 1 集"),
    ).toBeVisible();
    expect(screen.getByText("修改主机配置并重启服务后生效。")).toBeVisible();
    expect(screen.getByText("最近定时：已完成")).toBeVisible();
    expect(screen.getByText("此集跳过：本批已达上限")).toBeVisible();
  });

  it("does not present a pending scheduled candidate as skipped", async () => {
    apiMocks.getScheduleStatus.mockResolvedValue({
      enabled: true,
      cron: "0 0 9 * * *",
      timezone: "Asia/Shanghai",
      batch_size: 1,
      latest_run: {
        run: {
          id: 73,
          scheduled_for: "2026-08-25T08:00:00Z",
          cron_expression: "0 0 9 * * *",
          timezone: "Asia/Shanghai",
          batch_size: 1,
          status: "running",
          candidate_count: 1,
          started_count: 0,
          skipped_count: 0,
          created_at: "2026-08-25T08:00:00Z",
          updated_at: "2026-08-25T08:00:01Z",
        },
        items: [
          {
            id: 74,
            schedule_run_id: 73,
            episode_id: item.episode_id,
            queue_position: 0,
            outcome: "skipped",
            reason: "selection_pending",
            created_at: "2026-08-25T08:00:01Z",
            updated_at: "2026-08-25T08:00:01Z",
          },
        ],
      },
    });

    render(<EpisodeProcessingPanel item={item} />);

    expect(await screen.findByText("最近定时：正在选择候选")).toBeVisible();
    expect(screen.getByText("此集正在确认加工资格")).toBeVisible();
    expect(screen.queryByText(/^此集跳过：/)).not.toBeInTheDocument();
  });

  it("keeps the last successful schedule visible while a processing poll refreshes slowly", async () => {
    vi.useFakeTimers();
    try {
      const activeRun: ProcessingRun = {
        ...failedRun,
        status: "waiting_external",
        current_step: "transcription",
        error_code: undefined,
        error_message: undefined,
        error_retryable: false,
      };
      const schedule = {
        enabled: true,
        cron: "0 0 9 * * *",
        timezone: "Asia/Shanghai",
        batch_size: 1,
        next_run_at: "2026-08-25T09:00:00Z",
      };
      let resolveRefresh: (value: typeof schedule) => void = () => undefined;
      apiMocks.listEpisodeRuns.mockResolvedValue([activeRun]);
      apiMocks.getRun.mockResolvedValue(detail(activeRun));
      apiMocks.getScheduleStatus
        .mockResolvedValueOnce(schedule)
        .mockReturnValueOnce(
          new Promise<typeof schedule>((resolve) => {
            resolveRefresh = resolve;
          }),
        );

      render(<EpisodeProcessingPanel item={item} />);
      await act(async () => {
        await Promise.resolve();
        await Promise.resolve();
        await Promise.resolve();
      });
      expect(screen.getByText("已启用 · 每批 1 集")).toBeVisible();

      await act(async () => {
        await vi.advanceTimersByTimeAsync(4000);
      });
      expect(apiMocks.getScheduleStatus).toHaveBeenCalledTimes(2);
      expect(screen.getByText("已启用 · 每批 1 集")).toBeVisible();
      expect(screen.queryByText("正在读取…")).not.toBeInTheDocument();

      resolveRefresh(schedule);
      await act(async () => {
        await Promise.resolve();
      });
    } finally {
      vi.useRealTimers();
    }
  });

  it("does not overlap slow schedule status polls", async () => {
    vi.useFakeTimers();
    try {
      const activeRun: ProcessingRun = {
        ...failedRun,
        status: "waiting_external",
        current_step: "transcription",
        error_code: undefined,
        error_message: undefined,
        error_retryable: false,
      };
      const schedule = {
        enabled: true,
        cron: "0 0 9 * * *",
        timezone: "Asia/Shanghai",
        batch_size: 1,
        next_run_at: "2026-08-25T09:00:00Z",
      };
      let resolvePoll: (value: typeof schedule) => void = () => undefined;
      apiMocks.listEpisodeRuns.mockResolvedValue([activeRun]);
      apiMocks.getRun.mockResolvedValue(detail(activeRun));
      apiMocks.getScheduleStatus
        .mockResolvedValueOnce(schedule)
        .mockReturnValueOnce(
          new Promise<typeof schedule>((resolve) => {
            resolvePoll = resolve;
          }),
        );

      render(<EpisodeProcessingPanel item={item} />);
      await act(async () => {
        await Promise.resolve();
        await Promise.resolve();
        await Promise.resolve();
      });

      await act(async () => {
        await vi.advanceTimersByTimeAsync(4000);
      });
      expect(apiMocks.getScheduleStatus).toHaveBeenCalledTimes(2);

      await act(async () => {
        await vi.advanceTimersByTimeAsync(12000);
      });
      expect(apiMocks.getScheduleStatus).toHaveBeenCalledTimes(2);

      resolvePoll(schedule);
      await act(async () => {
        await Promise.resolve();
      });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(4000);
      });
      expect(apiMocks.getScheduleStatus).toHaveBeenCalledTimes(3);
    } finally {
      vi.useRealTimers();
    }
  });

  it("keeps the last successful schedule visible when a processing poll refresh fails", async () => {
    vi.useFakeTimers();
    try {
      const activeRun: ProcessingRun = {
        ...failedRun,
        status: "waiting_external",
        current_step: "transcription",
        error_code: undefined,
        error_message: undefined,
        error_retryable: false,
      };
      const schedule = {
        enabled: true,
        cron: "0 0 9 * * *",
        timezone: "Asia/Shanghai",
        batch_size: 1,
        next_run_at: "2026-08-25T09:00:00Z",
      };
      apiMocks.listEpisodeRuns.mockResolvedValue([activeRun]);
      apiMocks.getRun.mockResolvedValue(detail(activeRun));
      apiMocks.getScheduleStatus
        .mockResolvedValueOnce(schedule)
        .mockRejectedValueOnce(new Error("定时网络超时"));

      render(<EpisodeProcessingPanel item={item} />);
      await act(async () => {
        await Promise.resolve();
        await Promise.resolve();
        await Promise.resolve();
      });
      expect(screen.getByText("已启用 · 每批 1 集")).toBeVisible();

      await act(async () => {
        await vi.advanceTimersByTimeAsync(4000);
      });
      expect(
        screen.getByText("定时计划暂时无法读取：定时网络超时"),
      ).toBeVisible();
      expect(screen.getByText("已启用 · 每批 1 集")).toBeVisible();
      expect(screen.queryByText("正在读取…")).not.toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });

  it("keeps manual processing available when scheduled status is slow or fails", async () => {
    let rejectSchedule: (reason?: unknown) => void = () => undefined;
    apiMocks.getScheduleStatus.mockReturnValue(
      new Promise((_, reject) => {
        rejectSchedule = reject;
      }),
    );

    render(<EpisodeProcessingPanel item={item} />);

    expect(screen.getAllByText("正在读取…").length).toBeGreaterThan(0);
    expect(
      await screen.findByRole("button", { name: "开始转写" }),
    ).toBeEnabled();

    rejectSchedule(new Error("定时网络超时"));
    expect(
      await screen.findByText("定时计划暂时无法读取：定时网络超时"),
    ).toBeVisible();
    expect(screen.getByText("定时状态暂时不可用")).toBeVisible();
    expect(screen.queryByText("未启用")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "开始转写" })).toBeEnabled();
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

  it("shows a pending automatic retry without hiding the active run", async () => {
    const retryingRun: ProcessingRun = {
      ...failedRun,
      status: "waiting_external",
      current_step: "transcription",
      next_attempt_at: "2026-08-25T09:15:00Z",
      attempt_count: 2,
      max_attempts: 3,
      error_code: undefined,
      error_message: undefined,
      error_retryable: true,
    };
    apiMocks.listEpisodeRuns.mockResolvedValue([retryingRun]);
    apiMocks.getRun.mockResolvedValue(detail(retryingRun));

    render(<EpisodeProcessingPanel item={item} />);

    expect(await screen.findByText("等待飞书转写")).toBeVisible();
    expect(screen.getByText(/自动重试：.*已尝试 2\/3 次/)).toBeVisible();
    expect(screen.getByRole("button", { name: "取消" })).toBeEnabled();
  });

  it("shows the previous successful artifact and retries from a safe checkpoint", async () => {
    const nativeFailedRun: ProcessingRun = {
      ...failedRun,
      pipeline_version: "focus-processing-v2",
    };
    apiMocks.listEpisodeRuns.mockResolvedValue([nativeFailedRun]);
    apiMocks.getRun.mockResolvedValue(detail(nativeFailedRun));
    apiMocks.retry.mockResolvedValue({
      run: {
        ...nativeFailedRun,
        id: 32,
        status: "queued",
        error_message: undefined,
      },
      reused_active: false,
      reused_successful: false,
      preparing_audio: false,
    });
    const { container } = render(<EpisodeProcessingPanel item={item} />);

    expect(await screen.findByText("加工失败")).toBeVisible();
    expect(screen.getByText("上一成功版本")).toBeVisible();
    expect(
      screen.getByText("这是旧版纪要；重新转写后可获得妙记纪要和同步逐字稿。"),
    ).toBeVisible();
    fireEvent.click(screen.getByRole("tab", { name: "逐字稿" }));
    expect(await screen.findByText("# 规范逐字稿")).toBeVisible();
    expect(
      container.querySelector('[data-copilot-source="transcript"]'),
    ).toHaveAttribute("data-copilot-episode-id", "201");

    fireEvent.click(screen.getByRole("button", { name: "重试转写" }));
    await waitFor(() => expect(apiMocks.retry).toHaveBeenCalledWith(31));
    await waitFor(() => expect(apiMocks.getRun).toHaveBeenCalledWith(32));
  });

  it("restarts a failed legacy run while preserving old content on slow failure", async () => {
    let rejectStart: (reason: Error) => void = () => undefined;
    apiMocks.listEpisodeRuns.mockResolvedValue([failedRun]);
    apiMocks.getRun.mockResolvedValue(detail());
    apiMocks.start.mockReturnValue(
      new Promise((_, reject) => {
        rejectStart = reject;
      }),
    );

    render(<EpisodeProcessingPanel item={item} />);

    expect(await screen.findByText("加工失败")).toBeVisible();
    expect(await screen.findByText("# 旧版纪要")).toBeVisible();
    const reprocess = screen.getByRole("button", { name: "重新转写" });
    expect(
      screen.queryByRole("button", { name: "重试转写" }),
    ).not.toBeInTheDocument();

    fireEvent.click(reprocess);
    expect(reprocess).toBeDisabled();
    expect(screen.getByText("正在提交…")).toBeVisible();
    expect(screen.getByText("# 旧版纪要")).toBeVisible();
    fireEvent.click(reprocess);
    expect(apiMocks.start).toHaveBeenCalledTimes(1);
    expect(apiMocks.start).toHaveBeenCalledWith(item.episode_id);
    expect(apiMocks.retry).not.toHaveBeenCalled();

    rejectStart(new Error("新转写启动失败"));
    expect(await screen.findByText("新转写启动失败")).toBeVisible();
    expect(screen.getByText("加工失败")).toBeVisible();
    expect(screen.getByText("# 旧版纪要")).toBeVisible();
    expect(reprocess).toBeEnabled();
  });

  it("starts v2 for a terminal legacy run without an artifact", async () => {
    const pendingRun: ProcessingRun = {
      ...failedRun,
      id: 33,
      pipeline_version: "focus-processing-v2",
      status: "queued",
      current_step: "transcription",
      error_code: undefined,
      error_message: undefined,
      error_retryable: false,
    };
    apiMocks.listEpisodeRuns.mockResolvedValue([failedRun]);
    apiMocks.getRun.mockResolvedValue({
      run: failedRun,
      deliveries: [],
    });
    apiMocks.start.mockResolvedValue({
      run: pendingRun,
      reused_active: false,
      reused_successful: false,
      preparing_audio: false,
    });

    render(<EpisodeProcessingPanel item={item} />);

    expect(await screen.findByText("加工失败")).toBeVisible();
    const reprocess = screen.getByRole("button", { name: "重新转写" });
    expect(
      screen.queryByRole("button", { name: "重试转写" }),
    ).not.toBeInTheDocument();
    fireEvent.click(reprocess);
    await waitFor(() =>
      expect(apiMocks.start).toHaveBeenCalledWith(item.episode_id),
    );
    expect(apiMocks.retry).not.toHaveBeenCalled();
  });

  it("defaults native Minutes artifacts to summary and preserves the selected subtab", async () => {
    const completedRun: ProcessingRun = {
      ...failedRun,
      id: 62,
      pipeline_version: "focus-processing-v2",
      status: "completed",
      current_step: "",
      error_code: undefined,
      error_message: undefined,
      error_retryable: false,
    };
    const nativeArtifact: EpisodeArtifactSet = {
      ...artifact,
      run_id: completedRun.id,
      pipeline_version: completedRun.pipeline_version,
      minutes_summary_sha256: "e".repeat(64),
      notes_sha256: "",
      transcript_timeline_sha256: "f".repeat(64),
      capabilities: {
        minutes_summary: true,
        transcript: true,
        structured_timeline: true,
        matching_audio: true,
        legacy_episode_notes: false,
      },
    };
    apiMocks.listEpisodeRuns.mockResolvedValue([completedRun]);
    apiMocks.getRun.mockResolvedValue({
      run: completedRun,
      current_artifact: nativeArtifact,
      deliveries: [],
    });
    const summaryContent: ArtifactContent = {
      kind: "minutes_summary",
      content: "# 妙记纪要",
      sha256: nativeArtifact.minutes_summary_sha256 ?? "",
      media_available: false,
    };
    const transcriptContent: ArtifactContent = {
      kind: "transcript",
      content: "# 妙记逐字稿",
      sha256: nativeArtifact.transcript_sha256,
      timeline_sha256: nativeArtifact.transcript_timeline_sha256,
      segments: [
        {
          order: 1,
          speaker: "说话人",
          start_ms: 195,
          text: "正文",
        },
      ],
      media_available: true,
    };
    let resolveSummary: (content: ArtifactContent) => void = () => undefined;
    let resolveTranscript: (content: ArtifactContent) => void = () => undefined;
    apiMocks.getArtifactContent
      .mockReturnValueOnce(
        new Promise<ArtifactContent>((resolve) => {
          resolveSummary = resolve;
        }),
      )
      .mockReturnValueOnce(
        new Promise<ArtifactContent>((resolve) => {
          resolveTranscript = resolve;
        }),
      )
      .mockResolvedValueOnce(summaryContent);

    const { rerender } = render(<EpisodeProcessingPanel item={item} />);

    expect(await screen.findByText("正在读取纪要…")).toBeVisible();
    expect(screen.queryByText("# 妙记纪要")).not.toBeInTheDocument();
    await act(async () => {
      resolveSummary(summaryContent);
    });
    expect(await screen.findByText("# 妙记纪要")).toBeVisible();
    expect(screen.getByRole("tab", { name: "纪要" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(apiMocks.getArtifactContent).toHaveBeenCalledWith(
      nativeArtifact.id,
      "minutes_summary",
    );

    fireEvent.click(screen.getByRole("tab", { name: "逐字稿" }));
    expect(screen.queryByText("# 妙记纪要")).not.toBeInTheDocument();
    expect(screen.getByText("正在读取逐字稿…")).toBeVisible();
    await act(async () => {
      resolveTranscript(transcriptContent);
    });
    expect(await screen.findByText("正文")).toBeVisible();
    expect(screen.getByLabelText("逐字稿音频播放器")).toBeVisible();
    expect(screen.getByRole("region", { name: "同步逐字稿" })).toBeVisible();
    expect(screen.getByText("逐字稿 · 1 段")).toBeVisible();
    expect(screen.getByText("音频可用")).toBeVisible();

    fireEvent.click(screen.getByRole("tab", { name: "纪要" }));
    expect(await screen.findByText("# 妙记纪要")).toBeVisible();

    let rejectTranscript: (reason?: unknown) => void = () => undefined;
    apiMocks.getArtifactContent.mockReturnValueOnce(
      new Promise<ArtifactContent>((_, reject) => {
        rejectTranscript = reject;
      }),
    );
    fireEvent.click(screen.getByRole("tab", { name: "逐字稿" }));
    expect(screen.queryByText("# 妙记纪要")).not.toBeInTheDocument();
    expect(screen.getByText("正在读取逐字稿…")).toBeVisible();
    await act(async () => {
      rejectTranscript(new Error("网络超时"));
    });
    expect(await screen.findByText("产物读取失败：网络超时")).toBeVisible();
    expect(screen.queryByText("# 妙记纪要")).not.toBeInTheDocument();
    expect(screen.queryByText("正文")).not.toBeInTheDocument();

    rerender(<EpisodeProcessingPanel item={item} />);
    expect(screen.getByRole("tab", { name: "逐字稿" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.queryByText("来自同一条飞书妙记")).not.toBeInTheDocument();
  });

  it("hides the previous artifact while a processing poll loads its replacement", async () => {
    vi.useFakeTimers();
    try {
      const activeRun: ProcessingRun = {
        ...failedRun,
        status: "waiting_external",
        current_step: "transcription",
        error_code: undefined,
        error_message: undefined,
        error_retryable: false,
      };
      const completedRun: ProcessingRun = {
        ...activeRun,
        status: "completed",
        current_step: "",
      };
      const replacementArtifact: EpisodeArtifactSet = {
        ...artifact,
        id: 42,
        run_id: completedRun.id,
        pipeline_version: "focus-processing-v2",
        minutes_summary_sha256: "e".repeat(64),
        notes_sha256: "",
        transcript_timeline_sha256: "f".repeat(64),
        capabilities: {
          minutes_summary: true,
          transcript: true,
          structured_timeline: true,
          matching_audio: true,
          legacy_episode_notes: false,
        },
      };
      let resolveReplacement: (content: ArtifactContent) => void = () =>
        undefined;
      apiMocks.listEpisodeRuns.mockResolvedValue([activeRun]);
      apiMocks.getRun
        .mockResolvedValueOnce(detail(activeRun))
        .mockResolvedValueOnce({
          run: completedRun,
          current_artifact: replacementArtifact,
          deliveries: [],
        });
      apiMocks.getArtifactContent
        .mockResolvedValueOnce({
          kind: "episode_notes",
          content: "# 上一成功纪要",
          sha256: artifact.notes_sha256,
          media_available: false,
        } satisfies ArtifactContent)
        .mockReturnValueOnce(
          new Promise<ArtifactContent>((resolve) => {
            resolveReplacement = resolve;
          }),
        );

      render(<EpisodeProcessingPanel item={item} />);
      await act(async () => {
        await Promise.resolve();
        await Promise.resolve();
        await Promise.resolve();
      });
      expect(screen.getByText("# 上一成功纪要")).toBeVisible();

      await act(async () => {
        await vi.advanceTimersByTimeAsync(4000);
      });
      expect(apiMocks.getArtifactContent).toHaveBeenCalledWith(
        replacementArtifact.id,
        "minutes_summary",
      );
      expect(screen.queryByText("# 上一成功纪要")).not.toBeInTheDocument();
      expect(screen.getByText("正在读取纪要…")).toBeVisible();

      await act(async () => {
        resolveReplacement({
          kind: "minutes_summary",
          content: "# 新妙记纪要",
          sha256: replacementArtifact.minutes_summary_sha256 ?? "",
          media_available: false,
        });
      });
      expect(screen.getByText("# 新妙记纪要")).toBeVisible();
    } finally {
      vi.useRealTimers();
    }
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
      screen.queryByRole("button", { name: "重试转写" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "重新转写" }),
    ).not.toBeInTheDocument();
    expect(apiMocks.start).not.toHaveBeenCalled();
  });

  it("allows v2 restart after a legacy local Runtime result is unknown", async () => {
    const runtimeUnknownRun: ProcessingRun = {
      ...failedRun,
      error_code: "RUNTIME_RESULT_UNKNOWN",
      error_message: "本地 Codex Runtime 结果未知",
      error_retryable: false,
    };
    const pendingRun: ProcessingRun = {
      ...runtimeUnknownRun,
      id: 64,
      pipeline_version: "focus-processing-v2",
      status: "queued",
      current_step: "transcription",
      error_code: undefined,
      error_message: undefined,
    };
    apiMocks.listEpisodeRuns.mockResolvedValue([runtimeUnknownRun]);
    apiMocks.getRun.mockResolvedValue(detail(runtimeUnknownRun));
    apiMocks.start.mockResolvedValue({
      run: pendingRun,
      reused_active: false,
      reused_successful: false,
      preparing_audio: false,
    });

    render(<EpisodeProcessingPanel item={item} />);

    expect(await screen.findByText("加工失败")).toBeVisible();
    const restart = screen.getByRole("button", { name: "重新转写" });
    fireEvent.click(restart);
    await waitFor(() =>
      expect(apiMocks.start).toHaveBeenCalledWith(item.episode_id),
    );
    expect(apiMocks.retry).not.toHaveBeenCalled();
  });

  it("shows pending manual knowledge delivery separately from local completion", async () => {
    const completedRun: ProcessingRun = {
      ...failedRun,
      id: 61,
      status: "completed",
      current_step: "",
      error_code: undefined,
      error_message: undefined,
      error_retryable: false,
    };
    apiMocks.listEpisodeRuns.mockResolvedValue([completedRun]);
    apiMocks.getRun.mockResolvedValue({
      run: completedRun,
      current_artifact: { ...artifact, run_id: completedRun.id },
      deliveries: [
        {
          id: 71,
          artifact_set_id: artifact.id,
          target: "ima",
          destination: "manual-import",
          adapter_version: "ima-manual-import-v2",
          status: "pending",
          attempt_count: 1,
          error_retryable: false,
          created_at: "2026-08-24T08:06:00Z",
          updated_at: "2026-08-24T08:06:00Z",
        },
      ],
    });

    render(<EpisodeProcessingPanel item={item} />);

    expect(await screen.findByText("已完成")).toBeVisible();
    expect(screen.getByText("知识交付")).toBeVisible();
    expect(
      screen.getByText("ima · manual-import · 包已生成 / 待人工导入"),
    ).toBeVisible();
    expect(screen.getByText("本地包已保存，可按说明人工导入。")).toBeVisible();
  });

  it("shows a cancellation warning and blocks retry while external work may continue", async () => {
    const cancelledRun: ProcessingRun = {
      ...failedRun,
      status: "cancelled",
      current_step: "",
      error_code: "cancelled_external_result_unknown",
      error_message:
        "已取消本机加工；飞书端任务可能继续，已创建的远端资源会保留。",
      error_retryable: false,
    };
    apiMocks.listEpisodeRuns.mockResolvedValue([cancelledRun]);
    apiMocks.getRun.mockResolvedValue(detail(cancelledRun));

    render(<EpisodeProcessingPanel item={item} />);

    expect(await screen.findByText("已取消")).toBeVisible();
    expect(
      screen.getByText("飞书端任务可能继续", { exact: false }),
    ).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "重试转写" }),
    ).not.toBeInTheDocument();
  });

  it("allows a legacy restart when the backend confirms the transcript was stored", async () => {
    const cancelledRun: ProcessingRun = {
      ...failedRun,
      status: "cancelled",
      current_step: "",
      error_code: "cancelled_external_result_unknown",
      error_message: "旧版取消提示",
      error_retryable: false,
    };
    const pendingRun: ProcessingRun = {
      ...cancelledRun,
      id: 65,
      pipeline_version: "focus-processing-v2",
      status: "queued",
      current_step: "transcription",
      error_code: undefined,
      error_message: undefined,
    };
    apiMocks.listEpisodeRuns.mockResolvedValue([cancelledRun]);
    apiMocks.getRun.mockResolvedValue({
      ...detail(cancelledRun),
      external_result_unresolved: false,
      action_suggestion: "飞书逐字稿已完整保存，可重新转写。",
    });
    apiMocks.start.mockResolvedValue({
      run: pendingRun,
      reused_active: false,
      reused_successful: false,
      preparing_audio: false,
    });

    render(<EpisodeProcessingPanel item={item} />);

    expect(await screen.findByText("已取消")).toBeVisible();
    const restart = screen.getByRole("button", { name: "重新转写" });
    fireEvent.click(restart);
    await waitFor(() =>
      expect(apiMocks.start).toHaveBeenCalledWith(item.episode_id),
    );
    expect(apiMocks.retry).not.toHaveBeenCalled();
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
      await screen.findByRole("button", { name: "开始转写" }),
    ).toBeEnabled();

    fireEvent.click(screen.getByRole("button", { name: "开始转写" }));

    expect(await screen.findByText("等待准备音频")).toBeVisible();
    expect(screen.getByText("准备与校验音频")).toBeVisible();
    expect(screen.getByText("正在下载并校验音频…")).toBeVisible();
    expect(screen.getByRole("button", { name: "取消" })).toBeEnabled();
    expect(apiMocks.start).toHaveBeenCalledWith(item.episode_id);
    expect(apiMocks.getRun).toHaveBeenCalledWith(pendingRun.id);
  });
});
