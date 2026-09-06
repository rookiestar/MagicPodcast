import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import EpisodeCopilotPanel from "../EpisodeCopilotPanel";
import { episodeCopilotApi } from "@/lib/api/episodeCopilot";
import type { ConsumptionItem } from "@/types/consumption";
import type { EpisodeCopilotStreamEvent } from "@/types/episodeCopilot";

const item: ConsumptionItem = {
  episode_id: 302,
  podcast_id: 30,
  podcast_title: "测试节目",
  podcast_author: "测试作者",
  podcast_cover_url: "",
  episode_title: "执行活动卡测试",
  episode_no: "302",
  duration: 1200,
  published_date: "2026-09-06T08:00:00Z",
  show_notes: "The activity card must reflect real runtime stages.",
  original_url: "https://example.com/episode/302",
  image_url: "",
  notes: "",
  tags: [],
  queue_state: "focus",
};

function activityEvent(activity: Record<string, unknown>): string {
  return (
    'data: {"type":"status","transcript_used":false,' +
    `"private_note_included":false,"activity":${JSON.stringify(activity)}}\n\n`
  );
}

function stageActivity(
  stage: string,
  state: string,
  text: string,
): Record<string, unknown> {
  return {
    id: `stage:${stage}`,
    ordinal: 1,
    stage,
    category: "stage",
    state,
    text,
    observed_at: "2026-09-06T12:00:00Z",
  };
}

function runtimeActivity(
  id: string,
  ordinal: number,
  stage: string,
  category: string,
  state: string,
  text: string,
  metadata?: Record<string, string>,
): Record<string, unknown> {
  return {
    id,
    ordinal,
    stage,
    category,
    state,
    text,
    observed_at: "2026-09-06T12:00:00Z",
    ...(metadata ? { metadata } : {}),
  };
}

function deltaEvent(text: string): string {
  return (
    'data: {"type":"answer_delta","message":' +
    `${JSON.stringify(text)},"transcript_used":false,` +
    '"private_note_included":false}\n\n'
  );
}

function completeEvent(
  firstContentMS: number,
  totalMS: number,
): string {
  return (
    'data: {"type":"complete","message":"回答完成",'+
    '"transcript_used":false,"private_note_included":false,' +
    `"profile_id":"balanced","first_content_ms":${firstContentMS},` +
    `"total_ms":${totalMS},"stage_timings":{` +
    '"research_runtime_ready_ms":800,"public_research_ms":1500,' +
    '"source_validation_ms":40,"answer_runtime_ready_ms":900,' +
    '"citation_validation_ms":60}}\n\n'
  );
}

function sseStream(
  chunks: string[],
  options: { holdOpen?: boolean } = {},
): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(encoder.encode(chunk));
      }
      if (!options.holdOpen) controller.close();
    },
  });
}

async function askQuestion() {
  const group = await screen.findByTestId("copilot-profiles");
  fireEvent.change(screen.getByRole("textbox", { name: "向单集助手提问" }), {
    target: { value: "这次执行经历了什么？" },
  });
  fireEvent.click(screen.getByRole("button", { name: "提问" }));
  return group;
}

describe("EpisodeCopilotActivityCard (driven through the panel SSE flow)", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  async function renderPanel() {
    vi.spyOn(episodeCopilotApi, "getContext").mockResolvedValue({
      episode_id: 302,
      show_notes_available: true,
      transcript_available: false,
      private_note_available: false,
      profiles: [
        {
          id: "balanced",
          model: "gpt-5.6-luna",
          effort: "max",
          service_tier: "priority",
          service_tier_name: "Fast",
          is_default: true,
        },
      ],
      default_profile_id: "balanced",
    });
    return render(<EpisodeCopilotPanel item={item} />);
  }

  it("creates the card immediately on submit and stays honest with no activity", async () => {
    vi.useFakeTimers();
    await renderPanel();
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(sseStream([], { holdOpen: true }), { status: 200 }),
      ),
    );
    await act(async () => {});
    expect(screen.getByTestId("copilot-profiles")).toBeInTheDocument();
    fireEvent.change(screen.getByRole("textbox", { name: "向单集助手提问" }), {
      target: { value: "第一次访问" },
    });
    fireEvent.click(screen.getByRole("button", { name: "提问" }));

    const card = screen.getByRole("region", { name: "助手执行活动" });
    expect(within(card).getByText("读取单集上下文")).toBeInTheDocument();
    expect(within(card).getAllByText("待执行").length).toBeGreaterThan(0);
    expect(within(card).getByText(/尚未收到新活动/)).toBeInTheDocument();
    expect(within(card).getByText(/已等待/)).toBeInTheDocument();
    // No fabricated model actions, progress, or ETA while silent.
    expect(card.textContent).not.toContain("正在思考");
    expect(card.textContent).not.toContain("%");
    expect(card.textContent).not.toContain("剩余");

    await act(async () => {
      vi.advanceTimersByTime(3200);
    });
    expect(
      within(card).getByText("响应较慢；单集仍可阅读，可随时取消。"),
    ).toBeInTheDocument();
    // The slow hint supplements the stage instead of replacing it.
    expect(within(card).getByText("读取单集上下文")).toBeInTheDocument();
    expect(
      within(card).getAllByText(/尚未收到新活动/).length,
    ).toBeGreaterThan(0);
  });

  it("streams the full two-phase run with in-place activity updates and a collapsed summary", async () => {
    await renderPanel();
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        sseStream([
          'data: {"type":"context","stage":"read_context","message":"将使用当前单集的 Show Notes；未使用逐字稿","transcript_used":false,"private_note_included":false}\n\n',
          activityEvent(
            stageActivity(
              "research_runtime",
              "started",
              "正在启动公开资料检索 Runtime…",
            ),
          ),
          activityEvent(
            stageActivity(
              "research_runtime",
              "completed",
              "公开资料检索 Runtime 已就绪",
            ),
          ),
          activityEvent(
            runtimeActivity(
              "research:a1",
              3,
              "public_research",
              "web_search",
              "started",
              "runtime contract 主题",
            ),
          ),
          activityEvent(
            runtimeActivity(
              "research:a1",
              3,
              "public_research",
              "web_search",
              "completed",
              "runtime contract 主题",
              {
                candidate_domains: "authority.example.com",
                candidate_count: "2",
              },
            ),
          ),
          activityEvent(
            stageActivity("source_validation", "started", "正在校验公开来源…"),
          ),
          activityEvent(
            stageActivity(
              "source_validation",
              "completed",
              "已验证 2 个公开来源。",
            ),
          ),
          activityEvent(
            stageActivity("answer_runtime", "started", "正在启动回答 Runtime…"),
          ),
          activityEvent(
            stageActivity(
              "answer_runtime",
              "completed",
              "回答 Runtime 已就绪",
            ),
          ),
          deltaEvent("依据现有内容。"),
          activityEvent(
            stageActivity("citation_validation", "started", "正在核验引用…"),
          ),
          activityEvent(
            stageActivity(
              "citation_validation",
              "completed",
              "引用核验完成",
            ),
          ),
          completeEvent(900, 4200),
        ]),
        { status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await askQuestion();

    // In-place updates: the same activity ID stays one row and shows the
    // final state.
    const summary = await screen.findByTestId("copilot-run-summary");
    expect(summary.textContent).toContain("档位 均衡");
    expect(summary.textContent).toContain("首字 900ms");
    expect(summary.textContent).toContain("总耗时 4200ms");
    expect(summary.textContent).toContain("已验证公开来源 2 个");
    // The completed card collapses to the summary and can be re-opened.
    const card = screen.getByRole("region", { name: "助手执行活动" });
    expect(within(card).queryByText("runtime contract 主题")).not.toBeInTheDocument();
    const toggle = within(card).getByRole("button", {
      name: "展开执行详情",
    });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    const list = within(card).getByTestId("copilot-activities");
    const sameActivity = within(list).getAllByText("runtime contract 主题");
    expect(sameActivity.length).toBe(1);
    expect(within(list).getByText("候选 2")).toBeInTheDocument();
    expect(within(list).getByText("authority.example.com")).toBeInTheDocument();
    // Every stage completed with words, not only color.
    expect(within(card).getAllByText("完成").length).toBe(7);
  });

  it("keeps the current stage visible while the answer streams", async () => {
    await renderPanel();
    const fetchCalls: string[] = [];
    const streamResponse = () =>
      new Response(
        sseStream(
          [
            'data: {"type":"context","stage":"read_context","message":"将使用当前单集的 Show Notes","transcript_used":false,"private_note_included":false}\n\n',
            activityEvent(
              stageActivity("research_runtime", "started", "正在启动公开资料检索 Runtime…"),
            ),
            activityEvent(
              stageActivity("research_runtime", "completed", "公开资料检索 Runtime 已就绪"),
            ),
            activityEvent(
              stageActivity("source_validation", "completed", "已验证 1 个公开来源。"),
            ),
            activityEvent(
              stageActivity("answer_runtime", "started", "正在启动回答 Runtime…"),
            ),
            activityEvent(
              stageActivity("answer_runtime", "completed", "回答 Runtime 已就绪"),
            ),
            deltaEvent("第一个可读答案"),
            deltaEvent("，后续继续生成。"),
          ],
          { holdOpen: true },
        ),
        { status: 200 },
      );
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation((url: string | URL) => {
        fetchCalls.push(String(url));
        console.log("FETCH_CALL", String(url));
        return Promise.resolve(streamResponse());
      }),
    );

    await askQuestion();
    expect(await screen.findByText(/第一个可读答案/)).toBeInTheDocument();
    console.log("CALLS_AT_ANSWER", JSON.stringify(fetchCalls));
    const card = screen.getByRole("region", { name: "助手执行活动" });
    expect(within(card).getByText(/当前阶段：/)).toHaveTextContent(
      /组织回答/,
    );
    expect(within(card).getByText("组织回答")).toBeInTheDocument();
  });

  it("announces degraded research and no-source outcomes without fake verification", async () => {
    await renderPanel();
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation((_url: string, init?: RequestInit) => {
        const events: EpisodeCopilotStreamEvent[] = [];
        void init;
        void events;
        return Promise.resolve(
          new Response(
            sseStream([
              'data: {"type":"context","stage":"read_context","message":"将使用当前单集的 Show Notes","transcript_used":false,"private_note_included":false}\n\n',
              activityEvent(
                stageActivity(
                  "source_validation",
                  "failed",
                  "公开资料检索失败，将仅依据单集内部内容回答。",
                ),
              ),
              activityEvent(
                stageActivity("answer_runtime", "started", "正在启动回答 Runtime…"),
              ),
              activityEvent(
                stageActivity("answer_runtime", "completed", "回答 Runtime 已就绪"),
              ),
              deltaEvent("降级后的回答"),
              activityEvent(
                stageActivity("citation_validation", "completed", "引用核验完成"),
              ),
              completeEvent(700, 3000),
            ]),
            { status: 200 },
          ),
        );
      }),
    );

    await askQuestion();
    expect(
      await screen.findByText(/公开检索降级，仅依据单集内部内容/),
    ).toBeInTheDocument();
    const summary = screen.getByTestId("copilot-run-summary");
    expect(summary.textContent).not.toContain("已验证公开来源 0");
    // Re-open the collapsed details to confirm the degradation activity.
    fireEvent.click(
      screen.getByRole("button", { name: "展开执行详情" }),
    );
    expect(screen.getAllByText(/公开资料检索失败/).length).toBeGreaterThan(0);
  });

  it("shows zero verified sources and conflicts as explicit text", async () => {
    await renderPanel();
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          sseStream([
            'data: {"type":"context","stage":"read_context","message":"将使用当前单集的 Show Notes","transcript_used":false,"private_note_included":false}\n\n',
            activityEvent(
              stageActivity(
                "source_validation",
                "completed",
                "已验证 1 个公开来源；已发现来源冲突，正在整理答案。",
              ),
            ),
            deltaEvent("冲突场景回答"),
            activityEvent(
              stageActivity("citation_validation", "completed", "引用核验完成"),
            ),
            completeEvent(500, 2500),
          ]),
          { status: 200 },
        ),
      ),
    );

    await askQuestion();
    const summary = await screen.findByTestId("copilot-run-summary");
    expect(summary.textContent).toContain("已验证公开来源 1 个");
    fireEvent.click(
      screen.getByRole("button", { name: "展开执行详情" }),
    );
    expect(screen.getAllByText(/已发现来源冲突/).length).toBeGreaterThan(0);
  });

  it("keeps activities and partial answers on failure and offers retry", async () => {
    await renderPanel();
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          sseStream([
            'data: {"type":"context","stage":"read_context","message":"将使用当前单集的 Show Notes","transcript_used":false,"private_note_included":false}\n\n',
            activityEvent(
              stageActivity("research_runtime", "started", "正在启动公开资料检索 Runtime…"),
            ),
            activityEvent(
              stageActivity("research_runtime", "completed", "公开资料检索 Runtime 已就绪"),
            ),
            deltaEvent("已经生成的部分答案"),
            'data: {"type":"error","message":"本地 Codex Runtime 暂时无法完成回答","code":"execution_failed","retryable":true,"transcript_used":false,"private_note_included":false}\n\n',
          ]),
          { status: 200 },
        ),
      ),
    );

    await askQuestion();
    const card = await screen.findByRole("region", { name: "助手执行活动" });
    await waitFor(() => {
      expect(card.getAttribute("data-outcome")).toBe("failed");
    });
    expect(within(card).getAllByText("失败").length).toBeGreaterThan(0);
    fireEvent.click(
      within(card).getByRole("button", { name: "展开执行详情" }),
    );
    expect(within(card).getByText("公开资料检索 Runtime 已就绪")).toBeInTheDocument();
    expect(screen.getByText(/已经生成的部分答案/)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "重试" }),
    ).toBeEnabled();
  });

  it("preserves question, activities, and answer when cancelling mid-run", async () => {
    await renderPanel();
    // Mirror real fetch semantics: an aborted signal cancels the body
    // stream, so the pending SSE read rejects.
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation((_url: string, init?: RequestInit) => {
        const signal = init?.signal as AbortSignal | undefined;
        const encoder = new TextEncoder();
        let controllerRef: ReadableStreamDefaultController<Uint8Array> | null =
          null;
        const stream = new ReadableStream<Uint8Array>({
          start(controller) {
            controllerRef = controller;
            for (const chunk of [
              'data: {"type":"context","stage":"read_context","message":"将使用当前单集的 Show Notes","transcript_used":false,"private_note_included":false}\n\n',
              activityEvent(
                stageActivity("research_runtime", "started", "正在启动公开资料检索 Runtime…"),
              ),
              activityEvent(
                runtimeActivity(
                  "research:a1",
                  2,
                  "public_research",
                  "web_search",
                  "started",
                  "取消前的活动",
                ),
              ),
            ]) {
              controller.enqueue(encoder.encode(chunk));
            }
          },
        });
        signal?.addEventListener(
          "abort",
          () => {
            controllerRef?.error(
              new DOMException("The operation was aborted.", "AbortError"),
            );
          },
          { once: true },
        );
        return Promise.resolve(new Response(stream, { status: 200 }));
      }),
    );

    await askQuestion();
    await screen.findByText("取消前的活动");
    fireEvent.click(screen.getByRole("button", { name: "取消" }));

    const card = await screen.findByRole("region", { name: "助手执行活动" });
    await waitFor(() => {
      expect(card.getAttribute("data-outcome")).toBe("cancelled");
    });
    expect(within(card).getAllByText("已取消").length).toBeGreaterThan(0);
    fireEvent.click(
      within(card).getByRole("button", { name: "展开执行详情" }),
    );
    expect(within(card).getByText("取消前的活动")).toBeInTheDocument();
    expect(
      screen.getByRole("textbox", { name: "向单集助手提问" }),
    ).toHaveValue("这次执行经历了什么？");
    expect(screen.getByRole("button", { name: "重试" })).toBeEnabled();
  });

  it("retries with the original question, selection, and profile", async () => {
    await renderPanel();
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        sseStream([
          'data: {"type":"context","stage":"read_context","message":"将使用当前单集的 Show Notes","transcript_used":false,"private_note_included":false}\n\n',
          'data: {"type":"error","message":"本地 Codex Runtime 暂时无法完成回答","code":"execution_failed","retryable":true,"transcript_used":false,"private_note_included":false}\n\n',
        ]),
        { status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await askQuestion();
    await screen.findByRole("button", { name: "重试" });
    fireEvent.click(screen.getByRole("button", { name: "重试" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    const firstBody = JSON.parse(
      String((fetchMock.mock.calls[0]?.[1] as RequestInit).body),
    );
    const secondBody = JSON.parse(
      String((fetchMock.mock.calls[1]?.[1] as RequestInit).body),
    );
    expect(secondBody).toEqual(firstBody);
    expect(secondBody.profile_id).toBe("balanced");
  });

  it("clears the previous question's card when switching episodes", async () => {
    const renderResult = await renderPanel();
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          sseStream(
            [
              'data: {"type":"context","stage":"read_context","message":"将使用当前单集的 Show Notes","transcript_used":false,"private_note_included":false}\n\n',
            ],
            { holdOpen: true },
          ),
          { status: 200 },
        ),
      ),
    );

    await askQuestion();
    expect(
      screen.getByRole("region", { name: "助手执行活动" }),
    ).toBeInTheDocument();

    const nextItem = {
      ...item,
      episode_id: 303,
      episode_title: "切换单集",
    };
    await act(async () => {
      renderResult.unmount();
      render(<EpisodeCopilotPanel item={nextItem} />);
    });
    await screen.findByTestId("copilot-profiles");
    expect(
      screen.queryByRole("region", { name: "助手执行活动" }),
    ).not.toBeInTheDocument();
  });
});
