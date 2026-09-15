import { fireEvent, render, screen, waitFor } from "@testing-library/react";
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
