import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { createRef, type RefObject } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import EpisodeProcessingPanel, {
  type EpisodeProcessingPanelHandle,
} from "../EpisodeProcessingPanel";
import type { ConsumptionItem } from "@/types/consumption";
import type {
  ArtifactContent,
  AudioRecoverySummary,
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
  recoverAudio: vi.fn(),
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

function transcriptContent(
  audioRecovery: AudioRecoverySummary | undefined,
  mediaAvailable = false,
): ArtifactContent {
  return {
    kind: "transcript",
    content: "# 可读逐字稿\n\n音频状态不应遮挡这段正文。",
    sha256: "b".repeat(64),
    media_available: mediaAvailable,
    audio_recovery: audioRecovery,
    segments: [
      { order: 1, speaker: "主持人", start_ms: 0, text: "正文段落" },
    ],
  };
}

function prepareTranscriptAudio(audio: HTMLAudioElement, duration = 120) {
  Object.defineProperties(audio, {
    duration: { configurable: true, value: duration },
    currentTime: { configurable: true, writable: true, value: 0 },
    defaultPlaybackRate: { configurable: true, writable: true, value: 1 },
    playbackRate: { configurable: true, writable: true, value: 1 },
  });
  fireEvent.loadedMetadata(audio);
}

async function openProcessingDiagnostics() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
  });
  const summary = screen.getByText("运行详情").closest("summary");
  if (!summary) throw new Error("processing diagnostics summary not found");
  if (!summary.parentElement?.hasAttribute("open")) {
    fireEvent.click(summary);
  }
}

function renderWithPrimaryAction() {
  const panelRef = createRef<EpisodeProcessingPanelHandle>();
  const result = render(<EpisodeProcessingPanel ref={panelRef} item={item} />);
  return { ...result, panelRef };
}

function activatePrimary(
  panelRef: RefObject<EpisodeProcessingPanelHandle | null>,
) {
  act(() => panelRef.current?.activatePrimary());
}

describe("EpisodeProcessingPanel", () => {
  beforeEach(() => {
    Object.values(apiMocks).forEach((mock) => mock.mockReset());
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

    const onHeaderStateChange = vi.fn();
    render(
      <EpisodeProcessingPanel
        item={item}
        onHeaderStateChange={onHeaderStateChange}
      />,
    );

    expect(screen.queryByText("自动加工")).not.toBeInTheDocument();
    expect(screen.getByText("正在读取转写内容")).toBeVisible();
    expect(screen.queryByRole("tab", { name: "纪要" })).not.toBeInTheDocument();
    expect(screen.queryByText("运行详情")).not.toBeInTheDocument();

    await act(async () => {
      resolveRuns([]);
    });
    await waitFor(() => expect(screen.getByText("暂无转写记录")).toBeVisible());
    expect(
      screen.queryByRole("button", { name: "开始转写" }),
    ).not.toBeInTheDocument();
    expect(onHeaderStateChange).toHaveBeenLastCalledWith(
      expect.objectContaining({
        action: "start",
        primaryLabel: "开始转写",
        showTranscriptTab: false,
      }),
    );
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
    expect(await screen.findByText("上一成功版本")).toBeVisible();

    rerender(
      <EpisodeProcessingPanel
        item={{ ...item, episode_id: 202, episode_title: "第二集" }}
      />,
    );

    expect(screen.queryByText("上一成功版本")).not.toBeInTheDocument();
    expect(screen.getByText("正在读取转写内容")).toBeVisible();
    await act(async () => {
      resolveSecond([]);
    });
    await waitFor(() => expect(screen.getByText("暂无转写记录")).toBeVisible());
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
    expect(screen.getByText("暂无转写记录")).toBeVisible();
  });

  it("reports status failures without hiding the panel controls", async () => {
    apiMocks.listEpisodeRuns
      .mockRejectedValueOnce(new Error("网络超时"))
      .mockResolvedValueOnce([]);

    render(<EpisodeProcessingPanel item={item} />);

    expect(
      await screen.findByText("加工状态读取失败，单集内容不受影响：网络超时"),
    ).toBeVisible();
    expect(screen.getByText("转写信息暂时不可用")).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "开始转写" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "重试读取加工状态" }),
    ).toBeEnabled();

    fireEvent.click(screen.getByRole("button", { name: "重试读取加工状态" }));
    expect(await screen.findByText("暂无转写记录")).toBeVisible();
    expect(
      screen.queryByText("加工状态读取失败，单集内容不受影响：网络超时"),
    ).not.toBeInTheDocument();
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
    await openProcessingDiagnostics();

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
    await openProcessingDiagnostics();

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
      await openProcessingDiagnostics();
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
      await openProcessingDiagnostics();
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
      await openProcessingDiagnostics();
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

    const onHeaderStateChange = vi.fn();
    render(
      <EpisodeProcessingPanel
        item={item}
        onHeaderStateChange={onHeaderStateChange}
      />,
    );

    await waitFor(() =>
      expect(onHeaderStateChange).toHaveBeenLastCalledWith(
        expect.objectContaining({ action: "start", primaryDisabled: false }),
      ),
    );
    expect(screen.queryByText("运行详情")).not.toBeInTheDocument();

    rejectSchedule(new Error("定时网络超时"));
    await openProcessingDiagnostics();
    expect(
      await screen.findByText("定时计划暂时无法读取：定时网络超时"),
    ).toBeVisible();
    expect(screen.getByText("定时状态暂时不可用")).toBeVisible();
    expect(screen.queryByText("未启用")).not.toBeInTheDocument();
    expect(onHeaderStateChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ action: "start", primaryDisabled: false }),
    );
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
    await openProcessingDiagnostics();

    expect(screen.getAllByText("等待飞书转写").length).toBeGreaterThan(0);
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
    const { container, panelRef } = renderWithPrimaryAction();

    expect(await screen.findByText("上一成功版本")).toBeVisible();
    expect(
      screen.getByText("这是旧版纪要；重新转写后可获得妙记纪要和同步逐字稿。"),
    ).toBeVisible();
    fireEvent.click(screen.getByRole("tab", { name: "逐字稿" }));
    expect(await screen.findByText("# 规范逐字稿")).toBeVisible();
    expect(
      container.querySelector('[data-copilot-source="transcript"]'),
    ).toHaveAttribute("data-copilot-episode-id", "201");

    expect(
      screen.queryByRole("button", { name: "重试转写" }),
    ).not.toBeInTheDocument();
    activatePrimary(panelRef);
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

    const { panelRef } = renderWithPrimaryAction();

    expect(await screen.findByText("# 旧版纪要")).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "重新转写" }),
    ).not.toBeInTheDocument();

    activatePrimary(panelRef);
    await waitFor(() => expect(apiMocks.start).toHaveBeenCalledTimes(1));
    expect(screen.getByText("# 旧版纪要")).toBeVisible();
    expect(apiMocks.start).toHaveBeenCalledWith(item.episode_id);
    expect(apiMocks.retry).not.toHaveBeenCalled();

    rejectStart(new Error("新转写启动失败"));
    expect(await screen.findByText("新转写启动失败")).toBeVisible();
    expect(screen.getByText("# 旧版纪要")).toBeVisible();
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

    const { panelRef } = renderWithPrimaryAction();

    expect(await screen.findByText("转写失败")).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "重新转写" }),
    ).not.toBeInTheDocument();
    activatePrimary(panelRef);
    await waitFor(() =>
      expect(apiMocks.start).toHaveBeenCalledWith(item.episode_id),
    );
    expect(apiMocks.retry).not.toHaveBeenCalled();
  });

  it("defaults native Minutes artifacts to visual summary and preserves the selected subtab", async () => {
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
      whiteboard: {
        media_id: "whiteboard",
        media_type: "image/png",
        width: 320,
        height: 180,
        sha256: "5".repeat(64),
        alt: "飞书智能纪要画板",
      },
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
    expect(screen.queryByRole("tab", { name: "总结" })).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "纪要" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.queryByText("# 妙记纪要")).not.toBeInTheDocument();
    await act(async () => {
      resolveSummary(summaryContent);
    });
    expect(
      await screen.findByRole("img", { name: "飞书智能纪要画板" }),
    ).toBeVisible();
    const summaryTab = screen.getByRole("tab", { name: "总结" });
    const minutesTab = screen.getByRole("tab", { name: "纪要" });
    const transcriptTab = screen.getByRole("tab", { name: "逐字稿" });
    expect(summaryTab).toHaveAttribute("aria-selected", "true");
    expect(summaryTab).toHaveAttribute(
      "aria-controls",
      "processing-artifact-panel-summary",
    );
    expect(screen.getByRole("tabpanel", { name: "总结" })).toBeVisible();
    expect(apiMocks.getArtifactContent).toHaveBeenCalledWith(
      nativeArtifact.id,
      "minutes_summary",
    );

    summaryTab.focus();
    fireEvent.keyDown(summaryTab, { key: "ArrowRight" });
    expect(minutesTab).toHaveFocus();
    expect(minutesTab).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("# 妙记纪要")).toBeVisible();

    minutesTab.focus();
    fireEvent.keyDown(minutesTab, { key: "ArrowRight" });
    expect(transcriptTab).toHaveFocus();
    expect(transcriptTab).toHaveAttribute("aria-selected", "true");
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
    expect(screen.getByRole("tabpanel", { name: "逐字稿" })).toBeVisible();

    const transcriptAudio = document.querySelector("audio");
    expect(transcriptAudio).not.toBeNull();
    prepareTranscriptAudio(transcriptAudio!);
    const playbackRate = screen.getByRole("combobox", { name: "播放倍速" });
    expect(playbackRate).toHaveValue("1");
    fireEvent.change(playbackRate, { target: { value: "1.5" } });
    expect(playbackRate).toHaveValue("1.5");
    expect(transcriptAudio!.playbackRate).toBe(1.5);

    fireEvent.keyDown(transcriptTab, { key: "Home" });
    expect(summaryTab).toHaveFocus();
    expect(summaryTab).toHaveAttribute("aria-selected", "true");
    expect(
      await screen.findByRole("img", { name: "飞书智能纪要画板" }),
    ).toBeVisible();

    fireEvent.click(screen.getByRole("tab", { name: "逐字稿" }));
    expect(screen.getByText("正文")).toBeVisible();
    expect(apiMocks.getArtifactContent).toHaveBeenCalledTimes(2);
    expect(screen.getByRole("combobox", { name: "播放倍速" })).toHaveValue("1.5");
    expect(document.querySelector("audio")?.playbackRate).toBe(1.5);

    rerender(<EpisodeProcessingPanel item={item} />);
    expect(screen.getByRole("tab", { name: "逐字稿" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.queryByText("来自同一条飞书妙记")).not.toBeInTheDocument();

    const nextItem = {
      ...item,
      episode_id: 202,
      episode_title: "下一集",
    };
    const nextRun: ProcessingRun = {
      ...completedRun,
      id: 63,
      episode_id: nextItem.episode_id,
    };
    const nextArtifact: EpisodeArtifactSet = {
      ...nativeArtifact,
      id: 42,
      run_id: nextRun.id,
      episode_id: nextItem.episode_id,
    };
    const nextSummaryContent: ArtifactContent = {
      ...summaryContent,
      content: "# 新集纪要",
      sha256: nextArtifact.minutes_summary_sha256 ?? "",
    };
    const nextTranscriptContent: ArtifactContent = {
      ...transcriptContent,
      content: "# 新集逐字稿",
      sha256: nextArtifact.transcript_sha256,
      timeline_sha256: nextArtifact.transcript_timeline_sha256,
      segments: [
        {
          order: 1,
          speaker: "说话人",
          start_ms: 195,
          text: "新集正文",
        },
      ],
    };
    apiMocks.listEpisodeRuns.mockImplementation((episodeID: number) =>
      Promise.resolve(episodeID === nextItem.episode_id ? [nextRun] : [completedRun]),
    );
    apiMocks.getRun.mockImplementation((runID: number) =>
      Promise.resolve(
        runID === nextRun.id
          ? {
              run: nextRun,
              current_artifact: nextArtifact,
              deliveries: [],
            }
          : {
              run: completedRun,
              current_artifact: nativeArtifact,
              deliveries: [],
            },
      ),
    );
    apiMocks.getArtifactContent.mockReset();
    apiMocks.getArtifactContent.mockImplementation(
      (_artifactSetId: number, kind: string) =>
        Promise.resolve(
          kind === "transcript" ? nextTranscriptContent : nextSummaryContent,
        ),
    );

    rerender(<EpisodeProcessingPanel item={nextItem} />);
    expect(
      await screen.findByRole("img", { name: "飞书智能纪要画板" }),
    ).toBeVisible();
    fireEvent.click(screen.getByRole("tab", { name: "纪要" }));
    expect(await screen.findByText("# 新集纪要")).toBeVisible();
    fireEvent.click(screen.getByRole("tab", { name: "逐字稿" }));
    expect(await screen.findByText("新集正文")).toBeVisible();
    expect(screen.getByRole("combobox", { name: "播放倍速" })).toHaveValue("1");
  });

  it("keeps the previous artifact when a replacement read is slow or fails", async () => {
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
      let rejectReplacement: (reason?: unknown) => void = () => undefined;
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
          new Promise<ArtifactContent>((_, reject) => {
            rejectReplacement = reject;
          }),
        )
        .mockResolvedValueOnce({
          kind: "minutes_summary",
          content: "# 重试后的新纪要",
          sha256: replacementArtifact.minutes_summary_sha256 ?? "",
          media_available: false,
        });

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
      expect(screen.getByText("# 上一成功纪要")).toBeVisible();
      expect(
        screen.getByText("正在读取纪要，暂时显示上一成功内容…"),
      ).toBeVisible();

      await act(async () => {
        rejectReplacement(new Error("网络超时"));
      });
      expect(screen.getByText("产物读取失败：网络超时")).toBeVisible();
      expect(screen.getByText("# 上一成功纪要")).toBeVisible();

      await act(async () => {
        fireEvent.click(screen.getByRole("button", { name: "重试读取纪要" }));
        await Promise.resolve();
        await Promise.resolve();
        await Promise.resolve();
      });
      expect(screen.getByText("# 重试后的新纪要")).toBeVisible();
      expect(
        screen.queryByText("产物读取失败：网络超时"),
      ).not.toBeInTheDocument();
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

    expect(await screen.findByText("# 旧版纪要")).toBeVisible();
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

    const { panelRef } = renderWithPrimaryAction();

    expect(await screen.findByText("# 旧版纪要")).toBeVisible();
    activatePrimary(panelRef);
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
    await openProcessingDiagnostics();

    expect((await screen.findAllByText("已完成")).length).toBeGreaterThan(0);
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
    await openProcessingDiagnostics();

    expect((await screen.findAllByText("已取消")).length).toBeGreaterThan(0);
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

    const { panelRef } = renderWithPrimaryAction();

    expect(await screen.findByText("# 旧版纪要")).toBeVisible();
    activatePrimary(panelRef);
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

    const { panelRef } = renderWithPrimaryAction();
    expect(await screen.findByText("暂无转写记录")).toBeVisible();

    activatePrimary(panelRef);
    await openProcessingDiagnostics();

    expect(screen.getAllByText("等待准备音频").length).toBeGreaterThan(0);
    expect(screen.getByText("准备与校验音频")).toBeVisible();
    expect(screen.getByText("音频就绪后会自动提交飞书妙记。")).toBeVisible();
    expect(screen.getByRole("button", { name: "取消" })).toBeEnabled();
    expect(apiMocks.start).toHaveBeenCalledWith(item.episode_id);
    expect(apiMocks.getRun).toHaveBeenCalledWith(pendingRun.id);
  });

  it("queues recoverable missing audio once and keeps the transcript readable", async () => {
    const initialRecovery: AudioRecoverySummary = {
      recoverable: true,
      can_retry: false,
    };
    const queuedRecovery: AudioRecoverySummary = {
      recoverable: true,
      status: "queued",
      can_retry: false,
      updated_at: "2026-09-01T05:00:00Z",
    };
    let transcriptReads = 0;
    apiMocks.listEpisodeRuns.mockResolvedValue([failedRun]);
    apiMocks.getRun.mockResolvedValue(detail());
    apiMocks.getArtifactContent.mockImplementation(
      (_artifactSetId: number, kind: string) =>
        Promise.resolve(
          kind === "transcript"
            ? transcriptContent(
                transcriptReads++ === 0 ? initialRecovery : queuedRecovery,
              )
            : {
                kind,
                content: "# 旧版纪要",
                sha256: "c".repeat(64),
                media_available: false,
              },
        ),
    );
    apiMocks.recoverAudio.mockResolvedValue({
      artifact_set_id: artifact.id,
      audio_recovery: queuedRecovery,
      queued: true,
      reused: false,
      already_available: false,
    });

    render(<EpisodeProcessingPanel item={item} />);
    expect(await screen.findByText("# 旧版纪要")).toBeVisible();
    fireEvent.click(screen.getByRole("tab", { name: "逐字稿" }));

    const recoverButton = await screen.findByRole("button", {
      name: "恢复音频",
    });
    expect(screen.getByText("正文段落")).toBeVisible();
    fireEvent.click(recoverButton);

    await waitFor(() =>
      expect(apiMocks.recoverAudio).toHaveBeenCalledWith(artifact.id),
    );
    expect(
      await screen.findByRole("button", { name: "已排队" }),
    ).toBeDisabled();
    expect(screen.getByText("恢复已排队")).toBeVisible();
    expect(screen.getByText("正文段落")).toBeVisible();

    fireEvent.click(screen.getByRole("button", { name: "已排队" }));
    expect(apiMocks.recoverAudio).toHaveBeenCalledTimes(1);
  });

  it("shows downloading and terminal failure without replacing transcript text", async () => {
    const cases: Array<{
      name: string;
      recovery: AudioRecoverySummary;
      button?: string;
      label: string;
    }> = [
      {
        name: "downloading",
        recovery: {
          recoverable: true,
          status: "downloading",
          can_retry: false,
        },
        button: "恢复中",
        label: "正在恢复音频",
      },
      {
        name: "terminal failure",
        recovery: {
          recoverable: true,
          status: "failed",
          error_code: "AUDIO_RECOVERY_DIGEST_MISMATCH",
          error_message: "远端音频与目标产物不一致，无法恢复。",
          can_retry: false,
        },
        label: "音频恢复失败",
      },
      {
        name: "unavailable",
        recovery: {
          recoverable: false,
          error_code: "AUDIO_RECOVERY_CHECKPOINT_MISSING",
          error_message: "没有可用的受保护恢复来源。",
          can_retry: false,
        },
        label: "音频暂不可恢复",
      },
	    ];

	    for (const testCase of cases) {
	      apiMocks.listEpisodeRuns.mockReset();
	      apiMocks.getRun.mockReset();
	      apiMocks.getArtifactContent.mockReset();
	      apiMocks.listEpisodeRuns.mockResolvedValue([failedRun]);
	      apiMocks.getRun.mockResolvedValue(detail());
      apiMocks.getArtifactContent.mockImplementation(
        (_artifactSetId: number, kind: string) =>
          Promise.resolve(
            kind === "transcript"
              ? transcriptContent(testCase.recovery)
              : {
                  kind,
                  content: "# 旧版纪要",
                  sha256: "c".repeat(64),
                  media_available: false,
	              },
	          ),
	      );

	      const { unmount } = render(<EpisodeProcessingPanel item={item} />);
      expect(await screen.findByText("# 旧版纪要")).toBeVisible();
      fireEvent.click(screen.getByRole("tab", { name: "逐字稿" }));
      expect(await screen.findByText(testCase.label)).toBeVisible();
      expect(screen.getByText("正文段落")).toBeVisible();
      if (testCase.button) {
        expect(
          screen.getByRole("button", { name: testCase.button }),
        ).toBeDisabled();
      } else {
        expect(
          screen.queryByRole("button", { name: "重试恢复" }),
        ).not.toBeInTheDocument();
      }
      unmount();
    }
  });

  it("offers an explicit retry and adopts media only after the transcript read confirms it", async () => {
    const failedRecovery: AudioRecoverySummary = {
      recoverable: true,
      status: "failed",
      error_code: "AUDIO_RECOVERY_DOWNLOAD_FAILED",
      error_message: "下载暂时失败，请稍后重试。",
      can_retry: true,
    };
    const queuedRecovery: AudioRecoverySummary = {
      recoverable: true,
      status: "queued",
      can_retry: false,
    };
    let transcriptReads = 0;
    apiMocks.listEpisodeRuns.mockResolvedValue([failedRun]);
    apiMocks.getRun.mockResolvedValue(detail());
    apiMocks.getArtifactContent.mockImplementation(
      (_artifactSetId: number, kind: string) =>
        Promise.resolve(
          kind === "transcript"
            ? transcriptContent(
                transcriptReads++ === 0 ? failedRecovery : queuedRecovery,
              )
            : {
                kind,
                content: "# 旧版纪要",
                sha256: "c".repeat(64),
                media_available: false,
              },
        ),
    );
    apiMocks.recoverAudio.mockResolvedValue({
      artifact_set_id: artifact.id,
      audio_recovery: queuedRecovery,
      queued: true,
      reused: false,
      already_available: false,
    });

    render(<EpisodeProcessingPanel item={item} />);
    expect(await screen.findByText("# 旧版纪要")).toBeVisible();
    fireEvent.click(screen.getByRole("tab", { name: "逐字稿" }));
    expect(await screen.findByText("音频恢复失败")).toBeVisible();
    expect(screen.getByText("下载暂时失败，请稍后重试。")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "重试恢复" }));
    await waitFor(() =>
      expect(apiMocks.recoverAudio).toHaveBeenCalledTimes(1),
    );
    expect(
      await screen.findByRole("button", { name: "已排队" }),
    ).toBeDisabled();
    expect(screen.getByText("正文段落")).toBeVisible();
  });
});
