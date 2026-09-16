import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import NewPodcastsSection from "../NewPodcastsSection";
import type { ImportNewPodcast } from "@/lib/api/importTasks";
import * as importTasksModule from "@/lib/api/importTasks";
import * as workflowModule from "@/lib/api/workflow";
import { toast } from "@/lib/toast";

vi.mock("@/lib/api/importTasks", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/importTasks")>();
  return {
    ...actual,
    importTasksApi: {
      ...actual.importTasksApi,
      fetchTaskNewPodcasts: vi.fn(),
    },
  };
});

vi.mock("@/lib/api/workflow", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/workflow")>();
  return {
    ...actual,
    workflowApi: {
      ...actual.workflowApi,
      list: vi.fn(),
      appendPodcasts: vi.fn(),
    },
  };
});

const fetchTaskNewPodcasts = vi.mocked(importTasksModule.importTasksApi.fetchTaskNewPodcasts);
const listWorkflows = vi.mocked(workflowModule.workflowApi.list);
const appendPodcasts = vi.mocked(workflowModule.workflowApi.appendPodcasts);
const toastSuccess = vi.spyOn(toast, "success").mockImplementation(() => {});

// 计数文案内含 <strong> 等子元素，按节点整体文本匹配最内层容器。
function expectCounterText(text: string) {
  expect(
    screen.getByText(
      (_, node) => node?.textContent === text && ["SPAN", "P"].includes(node?.tagName ?? ""),
    ),
  ).toBeDefined();
}

function podcast(overrides: Partial<ImportNewPodcast> & { id: number }): ImportNewPodcast {
  return {
    title: `节目${overrides.id}`,
    feed_url: `http://f/${overrides.id}.xml`,
    ready: true,
    is_subscribed: true,
    workflows: [],
    ...overrides,
  };
}

const workflowListResponse = {
  workflows: [
    {
      id: 11,
      name: "科技周报",
      description: "",
      schedule: "0 6 * * 1",
      scope_type: "specific_podcasts" as const,
      scope_config: { podcast_ids: [2] },
      rules_config: {},
      is_enabled: true,
      created_at: "",
      updated_at: "",
    },
  ],
  pagination: { page: 1, page_size: 100, total: 1, total_pages: 1 },
};

async function renderSection() {
  const view = render(<NewPodcastsSection taskId={7} />);
  await waitFor(() => {
    expect(fetchTaskNewPodcasts).toHaveBeenCalledWith(7);
  });
  return view;
}

describe("NewPodcastsSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    fetchTaskNewPodcasts.mockResolvedValue({
      success: true,
      task_id: 7,
      total: 0,
      podcasts: [],
    });
    listWorkflows.mockResolvedValue(workflowListResponse);
    appendPodcasts.mockResolvedValue({
      success: true,
      workflow_id: 11,
      workflow_name: "科技周报",
      added: 1,
      already_member: 1,
      podcast_count: 3,
    });
  });

  it("lists batch podcasts with readiness and workflow attribution", async () => {
    fetchTaskNewPodcasts.mockResolvedValue({
      success: true,
      task_id: 7,
      total: 2,
      podcasts: [
        podcast({ id: 1, workflows: [{ id: 11, name: "科技周报", scope_type: "specific_podcasts", is_enabled: true }] }),
        podcast({ id: 2, ready: false }),
      ],
    });
    await renderSection();

    expect(screen.getByText("本批新增节目（2）")).toBeDefined();
    expect(screen.getByText("节目1")).toBeDefined();
    expect(screen.getByText("科技周报")).toBeDefined();
    expect(screen.getByText("待同步")).toBeDefined();
    expect(screen.getByText("节目2")).toBeDefined();
  });

  it("keeps rendering when a legacy response uses null for workflow coverage", async () => {
    const item = podcast({ id: 1 });
    fetchTaskNewPodcasts.mockResolvedValue({
      success: true,
      task_id: 7,
      total: 1,
      podcasts: [{ ...item, workflows: null as unknown as ImportNewPodcast["workflows"] }],
    });

    await renderSection();

    expect(screen.getByText("节目1")).toBeDefined();
    expect(screen.getByText("—")).toBeDefined();
  });

  it("shows a short empty result when the batch created nothing", async () => {
    await renderSection();
    expect(screen.getByText("本批没有新增节目")).toBeDefined();
  });

  it("filters by search and pending-only and keeps selection across pages", async () => {
    const batch = Array.from({ length: 45 }, (_, index) =>
      podcast({ id: index + 1, ready: index % 10 !== 0 }),
    );
    batch[0] = podcast({ id: 1, ready: false, title: "特殊节目" });
    fetchTaskNewPodcasts.mockResolvedValue({
      success: true,
      task_id: 7,
      total: batch.length,
      podcasts: batch,
    });
    await renderSection();

    // 第一页选择节目3，翻到第二页再选节目21，返回第一页后选择保留。
    fireEvent.click(screen.getByLabelText("选择「节目3」"));
    fireEvent.click(screen.getByRole("button", { name: "下一页" }));
    expect(screen.getByLabelText("选择「节目21」")).toBeDefined();
    fireEvent.click(screen.getByLabelText("选择「节目21」"));
    fireEvent.click(screen.getByRole("button", { name: "上一页" }));
    expectCounterText("已选 2 档");
    expect((screen.getByLabelText("选择「节目3」") as HTMLInputElement).checked).toBe(true);

    // 搜索过滤候选，不影响已选数量。
    fireEvent.change(screen.getByLabelText("搜索本批新增节目"), { target: { value: "特殊" } });
    expect(screen.getByText("特殊节目")).toBeDefined();
    expectCounterText("已选 2 档");
    fireEvent.change(screen.getByLabelText("搜索本批新增节目"), { target: { value: "" } });

    // 仅看待同步只留 ready=false 的 5 档。
    fireEvent.click(screen.getByLabelText("仅看待同步", { exact: false }));
    expect(screen.getByText("本批新增节目（5）")).toBeDefined();
    expect(screen.queryByText("节目3")).toBeNull();
    fireEvent.click(screen.getByLabelText("仅看待同步", { exact: false }));
  });

  it("select-all covers every filtered item, not only the visible page", async () => {
    const batch = Array.from({ length: 45 }, (_, index) => podcast({ id: index + 1 }));
    fetchTaskNewPodcasts.mockResolvedValue({
      success: true,
      task_id: 7,
      total: batch.length,
      podcasts: batch,
    });
    await renderSection();

    fireEvent.click(screen.getByLabelText("选择全部匹配节目"));
    expectCounterText("已选 45 档");
    fireEvent.click(screen.getByLabelText("选择全部匹配节目"));
    expectCounterText("已选 0 档");
  });

  it("opens the dialog, excludes duplicates and pending by default, then saves", async () => {
    fetchTaskNewPodcasts.mockResolvedValue({
      success: true,
      task_id: 7,
      total: 3,
      podcasts: [
        podcast({ id: 1 }),
        podcast({ id: 2 }),
        podcast({ id: 3, ready: false }),
      ],
    });
    await renderSection();

    fireEvent.click(screen.getByLabelText("选择「节目1」"));
    fireEvent.click(screen.getByLabelText("选择「节目2」"));
    fireEvent.click(screen.getByLabelText("选择「节目3」"));
    fireEvent.click(screen.getByRole("button", { name: "添加到工作流" }));

    // 目标 11 已包含节目 2：默认新增 1 档（待同步的节目 3 不纳入）。
    await waitFor(() => expectCounterText("新增 1 档 · 已包含 1 档"));
    expect(screen.getByLabelText(/包含待同步节目/)).toBeDefined();
    expect((screen.getByLabelText(/包含待同步节目/) as HTMLInputElement).checked).toBe(false);

    const saveButton = screen.getByRole("button", { name: "添加 1 档" });
    fireEvent.click(saveButton);
    await waitFor(() => expect(appendPodcasts).toHaveBeenCalledWith(11, [1]));
    expect(toastSuccess).toHaveBeenCalledWith("已添加 1 档节目到工作流");

    await waitFor(() => expect(fetchTaskNewPodcasts).toHaveBeenCalledTimes(2), {
      timeout: 2000,
    });
  });

  it("includes pending items only after the explicit checkbox", async () => {
    fetchTaskNewPodcasts.mockResolvedValue({
      success: true,
      task_id: 7,
      total: 2,
      podcasts: [podcast({ id: 1 }), podcast({ id: 3, ready: false })],
    });
    await renderSection();

    fireEvent.click(screen.getByLabelText("选择「节目1」"));
    fireEvent.click(screen.getByLabelText("选择「节目3」"));
    fireEvent.click(screen.getByRole("button", { name: "添加到工作流" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "添加 1 档" })).toBeDefined());

    fireEvent.click(screen.getByLabelText(/包含待同步节目/));
    fireEvent.click(screen.getByRole("button", { name: "添加 2 档" }));
    await waitFor(() => expect(appendPodcasts).toHaveBeenCalledWith(11, [1, 3]));
  });

  it("keeps the dialog and selection when the save fails", async () => {
    fetchTaskNewPodcasts.mockResolvedValue({
      success: true,
      task_id: 7,
      total: 1,
      podcasts: [podcast({ id: 1 })],
    });
    appendPodcasts.mockRejectedValue(new Error("network down"));
    await renderSection();

    fireEvent.click(screen.getByLabelText("选择「节目1」"));
    fireEvent.click(screen.getByRole("button", { name: "添加到工作流" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "添加 1 档" })).toBeDefined());
    fireEvent.click(screen.getByRole("button", { name: "添加 1 档" }));

    expect(await screen.findByText("添加失败，请重试")).toBeDefined();
    expect((screen.getByLabelText("选择「节目1」") as HTMLInputElement).checked).toBe(true);
    expect(screen.getByRole("button", { name: "取消" })).toBeDefined();

    fireEvent.click(screen.getByRole("button", { name: "取消" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(appendPodcasts).toHaveBeenCalledTimes(1);
  });

  it("cancelling the dialog never calls the append API", async () => {
    fetchTaskNewPodcasts.mockResolvedValue({
      success: true,
      task_id: 7,
      total: 1,
      podcasts: [podcast({ id: 1 })],
    });
    await renderSection();

    fireEvent.click(screen.getByLabelText("选择「节目1」"));
    fireEvent.click(screen.getByRole("button", { name: "添加到工作流" }));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeDefined());
    fireEvent.click(screen.getByRole("button", { name: "取消" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(appendPodcasts).not.toHaveBeenCalled();
  });
  it("ignores an older batch response after switching tasks", async () => {
    let resolveOld!: (value: Awaited<ReturnType<typeof importTasksModule.importTasksApi.fetchTaskNewPodcasts>>) => void;
    fetchTaskNewPodcasts.mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve; }));
    fetchTaskNewPodcasts.mockResolvedValueOnce({success:true,task_id:8,total:1,podcasts:[podcast({id:8,title:"New batch"})]});
    const view=render(<NewPodcastsSection taskId={7}/>);
    view.rerender(<NewPodcastsSection taskId={8}/>);
    await screen.findByText("New batch");
    await act(async () => resolveOld({success:true,task_id:7,total:1,podcasts:[podcast({id:1,title:"Old batch"})]}));
    expect(screen.queryByText("Old batch")).toBeNull();
    expect(screen.getByText("New batch")).toBeDefined();
  });

  it("includes targets beyond the first workflow page and traps keyboard focus", async () => {
    fetchTaskNewPodcasts.mockResolvedValue({success:true,task_id:7,total:1,podcasts:[podcast({id:1})]});
    listWorkflows.mockResolvedValueOnce({...workflowListResponse,pagination:{page:1,page_size:100,total:101,total_pages:2}});
    listWorkflows.mockResolvedValueOnce({...workflowListResponse,workflows:[{...workflowListResponse.workflows[0],id:99,name:"第二页目标"}],pagination:{page:2,page_size:100,total:101,total_pages:2}});
    await renderSection();
    fireEvent.click(await screen.findByLabelText("选择「节目1」"));
    const trigger=screen.getByRole("button",{name:"添加到工作流"}); trigger.focus();fireEvent.click(trigger);
    await screen.findByLabelText("选择工作流 第二页目标");
    expect(listWorkflows).toHaveBeenCalledWith({page:2,page_size:100});
    const save=screen.getByRole("button",{name:"添加 1 档"});save.focus();fireEvent.keyDown(save,{key:"Tab"});
    expect(document.activeElement).toBe(screen.getByLabelText("选择工作流 科技周报"));
    fireEvent.click(screen.getByRole("button",{name:"取消"}));expect(document.activeElement).toBe(trigger);
  });

});
