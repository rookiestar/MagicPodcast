/**
 * 执行历史可见等待验收（PRD #34）。
 *
 * 驱动真实工作流详情页：从点击「执行历史」到首条历史记录可见。
 * 使用受控假数据，不写真实数据库、不触发同步或付费能力。
 */
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SWRConfig } from "swr";
import { SearchProvider } from "@/contexts/SearchContext";
import { podcastApi, workflowApi } from "@/lib/api";
import { apiClient } from "@/lib/fetcher";
import { ToastProvider } from "@/lib/toast";
import { buildWorkflowJobsSummaryPath } from "@/lib/workflowJobsPaths";
import WorkflowDetailPage from "../page";

const WORKFLOW_ID = 42;
const JOBS_PAGE2_PATH = buildWorkflowJobsSummaryPath(WORKFLOW_ID, 2, 10);
const WORKFLOW_PATH = `/api/v1/workflows/${WORKFLOW_ID}`;

const routerReplace = vi.fn();
const routerPush = vi.fn();
let currentSearchParams = new URLSearchParams();

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: routerPush,
    replace: routerReplace,
    prefetch: vi.fn(),
    back: vi.fn(),
  }),
  useParams: () => ({ id: String(WORKFLOW_ID) }),
  useSearchParams: () => currentSearchParams,
  usePathname: () => `/workflows/${WORKFLOW_ID}`,
}));

vi.mock("next/dynamic", () => ({
  default: () => {
    function MockDynamic() {
      return null;
    }
    return MockDynamic;
  },
}));

vi.mock("@/components/podcasts/PodcastCover", () => ({
  default: ({ title }: { title: string }) =>
    createElement("div", { "data-testid": "podcast-cover" }, title),
}));

function makeJob(id: number, overrides: Record<string, unknown> = {}) {
  return {
    id,
    workflow_id: WORKFLOW_ID,
    status: "completed",
    podcasts_processed: 1,
    episodes_found: 2,
    episodes_created: 1,
    episodes_matched: 3,
    error_count: 0,
    triggered_by: "manual",
    created_at: `2026-05-0${(id % 9) + 1}T10:00:00Z`,
    duration: 1200,
    ...overrides,
  };
}

function successEnvelope<T>(data: T) {
  return { data: { success: true as const, data } };
}

function jobsPayload(
  jobs: ReturnType<typeof makeJob>[],
  page: number,
  totalPages: number,
) {
  return {
    jobs,
    pagination: {
      page,
      page_size: 10,
      total: totalPages * 10,
      total_pages: totalPages,
    },
  };
}

function workflowPayload(scopePodcastIds: number[] = []) {
  return {
    id: WORKFLOW_ID,
    name: "验收工作流",
    description: "执行历史验收",
    schedule: "0 8 * * *",
    scope_type:
      scopePodcastIds.length > 0 ? "specific_podcasts" : "all_subscribed",
    scope_config:
      scopePodcastIds.length > 0 ? { podcast_ids: scopePodcastIds } : {},
    rules_config: {},
    is_enabled: true,
    stats: {
      total_jobs: 2,
      total_episodes: 4,
      podcast_count: scopePodcastIds.length,
    },
  };
}

interface ApiController {
  workflowDelayMs: number;
  jobsDelayMs: number;
  jobsFail: boolean;
  jobsByPage: Map<number, ReturnType<typeof makeJob>[]>;
  totalPages: number;
  batchGetCalls: number;
  calls: string[];
  jobsGate?: Promise<void>;
}

function installApi(controller: ApiController) {
  return vi.spyOn(apiClient, "get").mockImplementation(async (url: string) => {
    const path = String(url);
    controller.calls.push(path);

    if (path === WORKFLOW_PATH || path.startsWith(`${WORKFLOW_PATH}?`)) {
      if (controller.workflowDelayMs > 0) {
        await new Promise((r) => setTimeout(r, controller.workflowDelayMs));
      }
      return successEnvelope(workflowPayload([101, 102]));
    }

    if (path.includes("/jobs?")) {
      if (controller.jobsGate) {
        await controller.jobsGate;
      }
      if (controller.jobsDelayMs > 0) {
        await new Promise((r) => setTimeout(r, controller.jobsDelayMs));
      }
      if (controller.jobsFail) {
        throw new Error("jobs summary failed");
      }
      const page = Number(
        new URL(path, "http://local").searchParams.get("page") ?? "1",
      );
      const jobs = controller.jobsByPage.get(page) ?? [];
      return successEnvelope(jobsPayload(jobs, page, controller.totalPages));
    }

    return successEnvelope({});
  });
}

function installBatchGet(controller: ApiController) {
  return vi.spyOn(podcastApi, "batchGet").mockImplementation(async () => {
    controller.batchGetCalls += 1;
    await new Promise((r) => setTimeout(r, 50));
    return [
      { id: 101, title: "节目 A", cover_url: "https://example.com/a.png" },
      { id: 102, title: "节目 B", cover_url: "https://example.com/b.png" },
    ];
  });
}

function Providers({ children }: { children: ReactNode }) {
  return createElement(
    SWRConfig,
    {
      value: {
        provider: () => new Map(),
        dedupingInterval: 0,
        revalidateOnFocus: false,
        shouldRetryOnError: false,
      },
    },
    createElement(
      SearchProvider,
      null,
      createElement(ToastProvider, null, children),
    ),
  );
}

function renderDetail() {
  return render(
    createElement(Providers, null, createElement(WorkflowDetailPage)),
  );
}

function jobsSummaryCalls(controller: ApiController) {
  return controller.calls.filter(
    (c) => c.includes("/jobs?") && c.includes("view=summary"),
  );
}

function firstPageJobsCalls(controller: ApiController) {
  return jobsSummaryCalls(controller).filter((c) => c.includes("page=1"));
}

async function waitForWorkflowReady() {
  await waitFor(() => {
    expect(
      screen.getByRole("tab", { name: /执行历史/ }),
    ).toBeInTheDocument();
  });
  await waitFor(() => {
    // 标题在桌面/移动工具栏可能各渲染一次
    expect(screen.getAllByText(/验收工作流/).length).toBeGreaterThan(0);
  });
}

function clickJobsTab() {
  fireEvent.click(screen.getByRole("tab", { name: /执行历史/ }));
}

function intentPrefetchJobsTab() {
  fireEvent.mouseEnter(screen.getByRole("tab", { name: /执行历史/ }));
}

beforeEach(() => {
  routerReplace.mockClear();
  routerPush.mockClear();
  currentSearchParams = new URLSearchParams();
  window.history.replaceState({}, "", `/workflows/${WORKFLOW_ID}`);
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("工作流执行历史可见等待验收 (#34)", () => {
  it("冷态：点击执行历史后首条记录可见，且首屏最多一次摘要请求、无 router 导航", async () => {
    const controller: ApiController = {
      workflowDelayMs: 0,
      jobsDelayMs: 20,
      jobsFail: false,
      jobsByPage: new Map([[1, [makeJob(1001), makeJob(1002)]]]),
      totalPages: 1,
      batchGetCalls: 0,
      calls: [],
    };
    installApi(controller);
    installBatchGet(controller);

    renderDetail();
    await waitForWorkflowReady();

    const beforeJobs = firstPageJobsCalls(controller).length;
    clickJobsTab();

    await waitFor(() => {
      expect(screen.getAllByText(/匹配数/).length).toBeGreaterThan(0);
    });

    const afterJobs = firstPageJobsCalls(controller).length - beforeJobs;
    expect(afterJobs).toBeLessThanOrEqual(1);
    expect(afterJobs).toBeGreaterThanOrEqual(1);

    expect(routerReplace).not.toHaveBeenCalled();
    expect(routerPush).not.toHaveBeenCalled();
    expect(window.location.search).toContain("tab=jobs");
  });

  it("AI 摘要失败时列表显示失败标记，状态仍为已完成且错误数为 0", async () => {
    const controller: ApiController = {
      workflowDelayMs: 0,
      jobsDelayMs: 0,
      jobsFail: false,
      jobsByPage: new Map([
        [
          1,
          [
            makeJob(1301, {
              status: "completed",
              error_count: 0,
              llm_error:
                "读取响应失败: context deadline exceeded (Client.Timeout or context cancellation while reading body)",
            }),
            makeJob(1302, {
              status: "completed",
              error_count: 0,
              llm_tokens_used: 8900,
              llm_model_used: "deepseek-v4-flash",
            }),
          ],
        ],
      ]),
      totalPages: 1,
      batchGetCalls: 0,
      calls: [],
    };
    installApi(controller);
    installBatchGet(controller);

    renderDetail();
    await waitForWorkflowReady();
    clickJobsTab();

    await waitFor(() => {
      expect(screen.getAllByText("AI摘要失败").length).toBeGreaterThan(0);
    });
    expect(screen.getAllByText("已完成").length).toBeGreaterThan(0);
    expect(screen.getAllByText("0").length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: "仅重试失败 Feed" })).not.toBeInTheDocument();
    expect(screen.getByText(/AI: 8.9K \(deepseek-v4-flash\)/)).toBeInTheDocument();
  });

  it("意图预取后进入：缓存命中时点击到首条可见 P95 ≤300ms，且无 router 导航", async () => {
    const controller: ApiController = {
      workflowDelayMs: 0,
      jobsDelayMs: 5,
      jobsFail: false,
      jobsByPage: new Map([[1, [makeJob(2001)]]]),
      totalPages: 1,
      batchGetCalls: 0,
      calls: [],
    };
    installApi(controller);
    installBatchGet(controller);

    renderDetail();
    await waitForWorkflowReady();

    intentPrefetchJobsTab();
    await waitFor(() => {
      expect(firstPageJobsCalls(controller).length).toBeGreaterThanOrEqual(1);
    });
    const prefetchCount = firstPageJobsCalls(controller).length;
    fireEvent.focus(screen.getByRole("tab", { name: /执行历史/ }));

    const samples: number[] = [];
    for (let i = 0; i < 5; i += 1) {
      fireEvent.click(screen.getByRole("tab", { name: /概览/ }));
      const t0 = performance.now();
      clickJobsTab();
      await waitFor(() => {
        expect(screen.getAllByText(/匹配数/).length).toBeGreaterThan(0);
        expect(
          screen.queryByText("正在加载执行历史..."),
        ).not.toBeInTheDocument();
      });
      samples.push(performance.now() - t0);
    }

    expect(firstPageJobsCalls(controller)).toHaveLength(prefetchCount);
    samples.sort((a, b) => a - b);
    const p95Index = Math.min(
      samples.length - 1,
      Math.ceil(samples.length * 0.95) - 1,
    );
    expect(samples[p95Index]).toBeLessThanOrEqual(300);
    expect(routerReplace).not.toHaveBeenCalled();
    expect(prefetchCount).toBeGreaterThanOrEqual(1);
  });

  it("可分享 URL：刷新进入 tab=jobs 时直接恢复执行历史", async () => {
    currentSearchParams = new URLSearchParams("tab=jobs");
    window.history.replaceState(
      {},
      "",
      `/workflows/${WORKFLOW_ID}?tab=jobs`,
    );
    const controller: ApiController = {
      workflowDelayMs: 0,
      jobsDelayMs: 0,
      jobsFail: false,
      jobsByPage: new Map([[1, [makeJob(2501)]]]),
      totalPages: 1,
      batchGetCalls: 0,
      calls: [],
    };
    installApi(controller);
    installBatchGet(controller);

    renderDetail();
    await waitForWorkflowReady();
    await waitFor(() => {
      expect(screen.getAllByText(/匹配数/).length).toBeGreaterThan(0);
    });

    expect(window.location.search).toBe("?tab=jobs");
    expect(firstPageJobsCalls(controller)).toHaveLength(1);
    expect(routerReplace).not.toHaveBeenCalled();
  });

  it("缓存复访：再次进入历史时先展示已有列表，不出现加载清空", async () => {
    const controller: ApiController = {
      workflowDelayMs: 0,
      jobsDelayMs: 30,
      jobsFail: false,
      jobsByPage: new Map([[1, [makeJob(3001)]]]),
      totalPages: 1,
      batchGetCalls: 0,
      calls: [],
    };
    installApi(controller);
    installBatchGet(controller);

    renderDetail();
    await waitForWorkflowReady();
    clickJobsTab();
    await waitFor(() =>
      expect(screen.getAllByText(/匹配数/).length).toBeGreaterThan(0),
    );

    fireEvent.click(screen.getByRole("tab", { name: /概览/ }));
    await waitFor(() =>
      expect(screen.getByText(/配置详情/)).toBeInTheDocument(),
    );

    clickJobsTab();
    expect(screen.queryByText("正在加载执行历史...")).not.toBeInTheDocument();
    expect(screen.getAllByText(/匹配数/).length).toBeGreaterThan(0);
  });

  it("运行中任务按既有节奏轮询刷新", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const controller: ApiController = {
      workflowDelayMs: 0,
      jobsDelayMs: 0,
      jobsFail: false,
      jobsByPage: new Map([[1, [makeJob(4001, { status: "running" })]]]),
      totalPages: 1,
      batchGetCalls: 0,
      calls: [],
    };
    installApi(controller);
    installBatchGet(controller);

    renderDetail();
    await waitForWorkflowReady();
    clickJobsTab();
    await waitFor(() =>
      expect(screen.getAllByText(/匹配数/).length).toBeGreaterThan(0),
    );

    const before = jobsSummaryCalls(controller).length;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3100);
    });
    await waitFor(() => {
      expect(jobsSummaryCalls(controller).length).toBeGreaterThan(before);
    });
  });

  it("补偿启动后立即刷新列表并进入活动任务轮询", async () => {
    const sourceJob = makeJob(4101, {
      status: "partial",
      can_compensate: true,
      error_count: 1,
    });
    const controller: ApiController = {
      workflowDelayMs: 0,
      jobsDelayMs: 0,
      jobsFail: false,
      jobsByPage: new Map([[1, [sourceJob]]]),
      totalPages: 1,
      batchGetCalls: 0,
      calls: [],
    };
    installApi(controller);
    installBatchGet(controller);
    const compensate = vi.spyOn(workflowApi, "compensateFailed").mockResolvedValue();
    vi.stubGlobal("prompt", vi.fn(() => "RETRY FAILED FEEDS JOB 4101"));
    vi.stubGlobal("alert", vi.fn());

    renderDetail();
    await waitForWorkflowReady();
    clickJobsTab();
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "仅重试失败 Feed" })).toBeInTheDocument(),
    );
    const before = jobsSummaryCalls(controller).length;
    controller.jobsByPage.set(1, [
      makeJob(4102, { status: "finalizing", triggered_by: "compensation" }),
      makeJob(4101, {
        status: "partial",
        error_count: 1,
        compensated_by_job_id: 4102,
        can_compensate: false,
      }),
    ]);

    fireEvent.click(screen.getByRole("button", { name: "仅重试失败 Feed" }));

    await waitFor(() => {
      expect(compensate).toHaveBeenCalledWith(4101, "RETRY FAILED FEEDS JOB 4101");
      expect(jobsSummaryCalls(controller).length).toBeGreaterThan(before);
      expect(screen.queryByRole("button", { name: "仅重试失败 Feed" })).not.toBeInTheDocument();
    });
  });

  it("分页：可切换到下一页并看到对应记录", async () => {
    const controller: ApiController = {
      workflowDelayMs: 0,
      jobsDelayMs: 0,
      jobsFail: false,
      jobsByPage: new Map([
        [1, [makeJob(5001, { episodes_matched: 11 })]],
        [2, [makeJob(5002, { episodes_matched: 22 })]],
      ]),
      totalPages: 2,
      batchGetCalls: 0,
      calls: [],
    };
    installApi(controller);
    installBatchGet(controller);

    renderDetail();
    await waitForWorkflowReady();
    clickJobsTab();
    await waitFor(() =>
      expect(screen.getAllByText("11").length).toBeGreaterThan(0),
    );

    fireEvent.click(screen.getByRole("button", { name: "下一页" }));
    await waitFor(() => {
      expect(controller.calls.some((c) => c.includes("page=2"))).toBe(true);
      expect(screen.getAllByText("22").length).toBeGreaterThan(0);
    });
    expect(JOBS_PAGE2_PATH).toContain("page=2");
  });

  it("请求失败：展示失败态并可重试", async () => {
    const controller: ApiController = {
      workflowDelayMs: 0,
      jobsDelayMs: 0,
      jobsFail: true,
      jobsByPage: new Map([[1, [makeJob(6001)]]]),
      totalPages: 1,
      batchGetCalls: 0,
      calls: [],
    };
    installApi(controller);
    installBatchGet(controller);

    renderDetail();
    await waitForWorkflowReady();
    clickJobsTab();

    await waitFor(() => {
      expect(screen.getByTestId("jobs-load-error")).toBeInTheDocument();
      expect(screen.getByText(/执行历史加载失败/)).toBeInTheDocument();
    });

    controller.jobsFail = false;
    fireEvent.click(
      within(screen.getByTestId("jobs-load-error")).getByRole("button", {
        name: /重试/,
      }),
    );
    await waitFor(() => {
      expect(screen.getAllByText(/匹配数/).length).toBeGreaterThan(0);
    });
  });

  it("离开概览后不再发起概览封面 batchGet 竞争", async () => {
    const controller: ApiController = {
      workflowDelayMs: 0,
      jobsDelayMs: 0,
      jobsFail: false,
      jobsByPage: new Map([[1, [makeJob(7001)]]]),
      totalPages: 1,
      batchGetCalls: 0,
      calls: [],
    };
    installApi(controller);
    const batchGet = installBatchGet(controller);

    renderDetail();
    await waitForWorkflowReady();
    await waitFor(() =>
      expect(controller.batchGetCalls).toBeGreaterThanOrEqual(1),
    );
    const batchGetOptions = (
      batchGet.mock.calls as unknown as Array<
        [number[], { signal?: AbortSignal }]
      >
    )[0]?.[1];
    expect(batchGetOptions?.signal).toBeInstanceOf(AbortSignal);
    const afterOverview = controller.batchGetCalls;

    clickJobsTab();
    await waitFor(() =>
      expect(screen.getAllByText(/匹配数/).length).toBeGreaterThan(0),
    );
    await act(async () => {
      await new Promise((r) => setTimeout(r, 80));
    });
    expect(batchGetOptions?.signal?.aborted).toBe(true);
    expect(controller.batchGetCalls).toBe(afterOverview);
  });

  it("冷态摘要响应到达后 ≤100ms 完成渲染", async () => {
    let resolveJobs!: () => void;
    const jobsGate = new Promise<void>((resolve) => {
      resolveJobs = resolve;
    });
    const controller: ApiController = {
      workflowDelayMs: 0,
      jobsDelayMs: 0,
      jobsFail: false,
      jobsByPage: new Map([[1, [makeJob(8001)]]]),
      totalPages: 1,
      batchGetCalls: 0,
      calls: [],
      jobsGate,
    };
    installApi(controller);
    installBatchGet(controller);

    renderDetail();
    await waitForWorkflowReady();
    clickJobsTab();
    await waitFor(() =>
      expect(screen.getByText("正在加载执行历史...")).toBeInTheDocument(),
    );

    const responseAt = performance.now();
    await act(async () => {
      resolveJobs();
    });
    await waitFor(() => {
      expect(screen.getAllByText(/匹配数/).length).toBeGreaterThan(0);
    });
    const renderedAt = performance.now();
    expect(renderedAt - responseAt).toBeLessThanOrEqual(100);
  });
});
