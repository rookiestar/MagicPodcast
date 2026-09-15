import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import WorkflowFormModal from "../WorkflowFormModal";
import * as podcastApiModule from "@/lib/api/podcasts";
import type { Podcast } from "@/types";

vi.mock("@/lib/api/podcasts", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/podcasts")>();
  const all = [podcastRecord(1, "未覆盖节目"), podcastRecord(2, "已覆盖节目")];
  return {
    ...actual,
    podcastApi: {
      ...actual.podcastApi,
      list: vi.fn(),
      batchGet: vi.fn().mockImplementation(async (ids: number[]) =>
        all.filter((podcast) => ids.includes(podcast.id)),
      ),
    },
  };
});

const listPodcasts = vi.mocked(podcastApiModule.podcastApi.list);

function podcast(id: number, title: string): Podcast {
  return {
    id,
    xyz_id: `pod-${id}`,
    title,
    description: "",
    author: "作者",
    cover_url: "",
    feed_url: `http://f/${id}.xml`,
    is_subscribed: true,
    is_dead: false,
    episode_count: 1,
    newest_episode_date: "",
    created_at: "",
    updated_at: "",
    tags: [],
  } as Podcast;
}

function podcastRecord(id: number, title: string): Podcast {
  return podcast(id, title);
}

function pageResponse(items: Podcast[]) {
  return {
    data: items,
    pagination: { page: 1, page_size: 100, total: items.length, total_pages: 1 },
  } as never;
}

async function goToStep2WithScope() {
  fireEvent.change(screen.getByPlaceholderText("例如: 每日科技播客抓取"), {
    target: { value: "覆盖筛选工作流" },
  });
  fireEvent.click(screen.getAllByRole("button", { name: "下一步" })[0]);
  await waitFor(() => {
    expect(screen.getByText("指定节目")).toBeDefined();
  });
  fireEvent.click(screen.getByText("指定节目"));
}

describe("WorkflowFormModal coverage filter (#419)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    listPodcasts.mockResolvedValue(
      pageResponse([podcast(1, "未覆盖节目"), podcast(2, "已覆盖节目")]),
    );
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("shows the filter unchecked by default in create mode and only requests it when enabled", async () => {
    render(<WorkflowFormModal isOpen onClose={() => {}} onSuccess={() => {}} />);
    await goToStep2WithScope();

    const checkbox = await screen.findByLabelText("隐藏已加入其他工作流的节目");
    expect((checkbox as HTMLInputElement).checked).toBe(false);
    await waitFor(() => expect(listPodcasts).toHaveBeenCalled());
    const plainCall = listPodcasts.mock.calls[0][0];
    expect(plainCall?.exclude_covered).not.toBe("1");

    // 首批候选来自未筛选响应；开启筛选后重新请求并仅显示服务端返回的候选。
    expect(await screen.findByText("未覆盖节目")).toBeDefined();
    listPodcasts.mockResolvedValue(pageResponse([podcast(1, "未覆盖节目")]));
    fireEvent.click(checkbox);
    await waitFor(() => {
      expect(
        listPodcasts.mock.calls.some((call) => call[0]?.exclude_covered === "1"),
      ).toBe(true);
    });
    await waitFor(() => expect(screen.queryByText("已覆盖节目")).toBeNull());
    // 关闭筛选恢复原候选。
    listPodcasts.mockResolvedValue(
      pageResponse([podcast(1, "未覆盖节目"), podcast(2, "已覆盖节目")]),
    );
    fireEvent.click(screen.getByLabelText("隐藏已加入其他工作流的节目"));
    await waitFor(() => expect(screen.getByText("已覆盖节目")).toBeDefined());
  });

  it("keeps a covered selection in the selected panel after enabling the filter", async () => {
    render(<WorkflowFormModal isOpen onClose={() => {}} onSuccess={() => {}} />);
    await goToStep2WithScope();

    // 先选中将被隐藏的已覆盖节目。
    const coveredRow = await screen.findByText("已覆盖节目");
    fireEvent.click(coveredRow);
    await waitFor(() => expect(screen.getByText("已选 1")).toBeDefined());

    // 开启筛选：它从候选隐藏，仅在桌面+移动两个已选区出现（AC9）。
    listPodcasts.mockResolvedValue(pageResponse([podcast(1, "未覆盖节目")]));
    fireEvent.click(screen.getByLabelText("隐藏已加入其他工作流的节目"));
    await waitFor(() => expect(screen.getAllByText("已覆盖节目").length).toBe(2));
    expect(screen.getByText("已选 1")).toBeDefined();

    // 关闭筛选后候选重新可见（候选一行 + 两个已选区）。
    listPodcasts.mockResolvedValue(
      pageResponse([podcast(1, "未覆盖节目"), podcast(2, "已覆盖节目")]),
    );
    fireEvent.click(screen.getByLabelText("隐藏已加入其他工作流的节目"));
    await waitFor(() => expect(screen.getAllByText("已覆盖节目").length).toBe(3));
  });

  it("does not reselect a hidden covered podcast with add displayed", async () => {
    render(<WorkflowFormModal isOpen onClose={() => {}} onSuccess={() => {}} />);
    await goToStep2WithScope();
    fireEvent.click(await screen.findByText("已覆盖节目"));
    listPodcasts.mockResolvedValue(pageResponse([podcast(1, "未覆盖节目")]));
    fireEvent.click(screen.getByLabelText("隐藏已加入其他工作流的节目"));
    await waitFor(() => expect(screen.getAllByText("已覆盖节目")).toHaveLength(2));
    // Removing a selection must not put a covered item back into bulk selection.
    fireEvent.click(screen.getAllByRole("button", { name: "移除节目：已覆盖节目" })[0]);
    await waitFor(() => expect(screen.queryByText("已选 1")).toBeNull());
    fireEvent.click(screen.getAllByRole("button", { name: "添加当前显示的 1 个搜索结果" })[0]);
    await waitFor(() => expect(screen.getByText("已选 1")).toBeDefined());
    expect(screen.queryByText("已覆盖节目")).toBeNull();
  });

  it("restores a backfilled selection when the filtered next page returns it", async () => {
    let intersect: (() => void) | undefined;
    vi.stubGlobal("IntersectionObserver", class {
      constructor(callback: IntersectionObserverCallback) {
        intersect = () => callback([{ isIntersecting: true } as IntersectionObserverEntry], this as unknown as IntersectionObserver);
      }
      observe() {} disconnect() {} unobserve() {}
    });
    try {
      render(<WorkflowFormModal isOpen onClose={() => {}} onSuccess={() => {}} />);
      await goToStep2WithScope();
      fireEvent.click(await screen.findByText("已覆盖节目"));
      listPodcasts.mockResolvedValueOnce({ data: [podcast(1, "未覆盖节目")], pagination: { page: 1, page_size: 1, total: 2, total_pages: 2 } });
      fireEvent.click(screen.getByLabelText("隐藏已加入其他工作流的节目"));
      await waitFor(() => expect(screen.getAllByText("已覆盖节目")).toHaveLength(2));
      listPodcasts.mockResolvedValueOnce({ data: [podcast(2, "已覆盖节目")], pagination: { page: 2, page_size: 1, total: 2, total_pages: 2 } });
      await waitFor(() => expect(intersect).toBeDefined());
      await act(async () => { intersect?.(); });
      await waitFor(() => expect(screen.getAllByText("已覆盖节目")).toHaveLength(3));
    } finally { vi.unstubAllGlobals(); }
  });

  it("sends search to the server together with coverage before pagination", async () => {
    render(<WorkflowFormModal isOpen onClose={() => {}} onSuccess={() => {}} />);
    await goToStep2WithScope();
    await screen.findByText("未覆盖节目");
    fireEvent.click(screen.getByLabelText("隐藏已加入其他工作流的节目"));
    await waitFor(() => expect(listPodcasts.mock.lastCall?.[0]?.exclude_covered).toBe("1"));
    const search = screen.getByPlaceholderText(/搜索节目/);
    fireEvent.change(search, { target: { value: "跨页节目" } });
    await waitFor(() => expect(listPodcasts).toHaveBeenLastCalledWith(expect.objectContaining({ search: "跨页节目", exclude_covered: "1", page: 1 })));
  });

  it("chunks selected-podcast backfill at the API limit", async () => {
    listPodcasts.mockResolvedValue(pageResponse([]));
    const workflow = {
      id: 9, name: "大工作流", schedule: "0 0 8 * * *", scope_type: "specific_podcasts",
      scope_config: { podcast_ids: Array.from({ length: 151 }, (_, i) => i + 1) },
      rules_config: {}, is_enabled: true,
    } as import("@/types").Workflow;
    render(<WorkflowFormModal isOpen workflow={workflow} onClose={() => {}} onSuccess={() => {}} />);
    await goToStep2WithScope();
    await waitFor(() => expect(podcastApiModule.podcastApi.batchGet).toHaveBeenCalledTimes(2));
    expect(vi.mocked(podcastApiModule.podcastApi.batchGet).mock.calls.map(([ids]) => ids.length)).toEqual([100, 51]);
  });

  it("keeps content on a failed filter request and retries", async () => {
    render(<WorkflowFormModal isOpen onClose={() => {}} onSuccess={() => {}} />);
    await goToStep2WithScope();
    await screen.findByText("未覆盖节目");
    listPodcasts.mockRejectedValueOnce(new Error("offline"));
    fireEvent.click(screen.getByLabelText("隐藏已加入其他工作流的节目"));
    const retry = await screen.findByRole("button", { name: "重试" });
    expect(screen.getByText("未覆盖节目")).toBeDefined();
    listPodcasts.mockResolvedValue(pageResponse([podcast(1, "未覆盖节目")]));
    fireEvent.click(retry);
    await waitFor(() => expect(screen.queryByText("已覆盖节目")).toBeNull());
    expect(screen.queryByRole("button", { name: "重试" })).toBeNull();
  });

  it("hides the filter in edit mode", async () => {
    const workflow = {
      id: 7,
      name: "编辑工作流",
      description: "",
      schedule: "0 0 8 * * *",
      scope_type: "specific_podcasts",
      scope_config: { podcast_ids: [1] },
      rules_config: {},
      is_enabled: true,
    } as import("@/types").Workflow;
    render(<WorkflowFormModal isOpen workflow={workflow} onClose={() => {}} onSuccess={() => {}} />);
    await goToStep2WithScope();
    expect(screen.queryByLabelText("隐藏已加入其他工作流的节目")).toBeNull();
  });
});
