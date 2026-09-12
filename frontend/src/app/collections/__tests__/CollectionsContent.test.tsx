import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { SWRConfig } from "swr";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { fetchSummariesMock, pushMock } = vi.hoisted(() => ({
  fetchSummariesMock: vi.fn(),
  pushMock: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock, replace: vi.fn() }),
  usePathname: () => "/collections",
  useSearchParams: () => new URLSearchParams(),
  useParams: () => ({}),
  redirect: vi.fn(),
}));

vi.mock("@/contexts/SearchContext", () => ({
  useSearch: () => ({ openSearch: vi.fn(), closeSearch: vi.fn(), isSearchOpen: false }),
}));

vi.mock("@/lib/collections", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/collections")>();
  return {
    ...actual,
    fetchCollectionSummaries: fetchSummariesMock,
    COLLECTIONS_PATH: "/api/v1/collections",
  };
});

import CollectionsContent from "../CollectionsContent";
import type { CollectionSummary } from "@/types/collection";

function makeSummary(overrides: Partial<CollectionSummary> = {}): CollectionSummary {
  return {
    id: 1,
    title: "穿透半导体迷雾",
    description: "半导体产业盘点。",
    author: "小宇宙领航员",
    platform: "xiaoyuzhoufm",
    external_id: "6a20323b78a52c96d821a769",
    source_url: "https://www.xiaoyuzhoufm.com/collection/episode/6a20323b78a52c96d821a769",
    total_known: false,
    item_count: 8,
    adopted_count: 0,
    created_at: "2026-09-12T16:00:00Z",
    last_refreshed_at: null,
    ...overrides,
  };
}

// 每个用例使用独立 SWR 缓存，避免用例间串扰。
function renderCollections(ui: ReactElement) {
  return render(<SWRConfig value={{ provider: () => new Map() }}>{ui}</SWRConfig>);
}

describe("CollectionsContent", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    fetchSummariesMock.mockResolvedValue([makeSummary()]);
  });

  it("renders saved collections with fixed copy and real counts", async () => {
    renderCollections(<CollectionsContent />);

    expect(await screen.findAllByRole("link", { name: "穿透半导体迷雾" })).not.toHaveLength(0);
    expect(screen.getByText(/作者：小宇宙领航员/)).toBeInTheDocument();
    expect(screen.getByText(/已读取 8 集/)).toBeInTheDocument();
    expect(screen.getByText(/已收录 0 集/)).toBeInTheDocument();

    const openLinks = screen.getAllByRole("link", { name: "打开清单" });
    expect(openLinks.length).toBeGreaterThan(0);
    for (const link of openLinks) {
      expect(link).toHaveAttribute("href", "/collections/1");
    }
    const sourceLinks = screen.getAllByRole("link", { name: "小宇宙链接" });
    expect(sourceLinks.length).toBeGreaterThan(0);
    for (const link of sourceLinks) {
      expect(link).toHaveAttribute("href", expect.stringContaining("xiaoyuzhoufm.com"));
      expect(link).toHaveAttribute("target", "_blank");
    }
    // 列表不提供采纳入口，收录动作属于第 2 票。
    expect(screen.queryByRole("button", { name: "加入 Inbox" })).not.toBeInTheDocument();
  });

  it("searches by title and restores the full list after clearing", async () => {
    const user = userEvent.setup();
    renderCollections(<CollectionsContent />);
    await screen.findAllByRole("link", { name: "穿透半导体迷雾" });

    const input = screen.getByRole("searchbox", { name: "按清单标题搜索" });
    await user.type(input, "芯片");
    await waitFor(() => {
      expect(fetchSummariesMock).toHaveBeenLastCalledWith("芯片");
    });

    await user.click(screen.getByRole("button", { name: "清空搜索并恢复列表" }));
    await waitFor(() => {
      expect(input).toHaveValue("");
    });
    // 清空后恢复完整列表（命中缓存或重新读取都应可见）。
    await screen.findAllByRole("link", { name: "穿透半导体迷雾" });
  });

  it("shows the import entry when no collections exist", async () => {
    fetchSummariesMock.mockResolvedValue([]);
    renderCollections(<CollectionsContent />);

    expect(
      await screen.findByText("还没有导入任何清单"),
    ).toBeInTheDocument();
    const emptyImport = screen.getAllByRole("button", { name: "导入清单" });
    expect(emptyImport.length).toBeGreaterThan(0);
  });

  it("opens the import modal with the scope notice", async () => {
    const user = userEvent.setup();
    renderCollections(<CollectionsContent />);
    await screen.findAllByRole("link", { name: "穿透半导体迷雾" });

    await user.click(screen.getAllByRole("button", { name: "导入清单" })[0]);
    expect(await screen.findByRole("dialog", { name: "导入清单" })).toBeInTheDocument();
    expect(
      screen.getByText(/不会批量入库，也不会关注节目或自动开始加工/),
    ).toBeInTheDocument();
    // 链接为空时主预览按钮不可用。
    expect(screen.getByRole("button", { name: "预览" })).toBeDisabled();
  });
});
