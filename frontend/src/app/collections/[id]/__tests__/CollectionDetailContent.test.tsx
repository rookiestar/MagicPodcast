import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { SWRConfig } from "swr";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { detailMock, adoptMock } = vi.hoisted(() => ({
  detailMock: vi.fn(),
  adoptMock: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  usePathname: () => "/collections/1",
  useSearchParams: () => new URLSearchParams(),
  useParams: () => ({ id: "1" }),
  redirect: vi.fn(),
}));

vi.mock("@/contexts/SearchContext", () => ({
  useSearch: () => ({ openSearch: vi.fn(), closeSearch: vi.fn(), isSearchOpen: false }),
}));

vi.mock("@/lib/collections", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/collections")>();
  return {
    ...actual,
    fetchCollectionDetail: detailMock,
    adoptCollectionItem: adoptMock,
  };
});

import CollectionDetailContent from "../CollectionDetailContent";
import type {
  CollectionDetail,
  CollectionItemDetail,
} from "@/types/collection";

function makeItem(overrides: Partial<CollectionItemDetail> = {}): CollectionItemDetail {
  return {
    id: 1,
    position: 0,
    external_episode_id: "e1",
    external_podcast_id: "p1",
    podcast_title: "投资实战派",
    podcast_author: "wong永庆",
    podcast_cover_url: "",
    episode_title: "E185 芯片规律 × AI浪潮",
    recommendation: "存储芯片为何五年内持续短缺？",
    shownotes: "<p>时间轴：00:00 开场</p>",
    duration: 4692,
    published_at: "2026-06-01T00:00:00Z",
    image_url: "",
    episode_url: "https://www.xiaoyuzhoufm.com/episode/e1",
    pay_type: "FREE",
    is_private_media: false,
    adopted_episode_id: null,
    adopted_episode_title: "",
    adopted_episode_queue: null,
    ...overrides,
  };
}

function makeDetail(overrides: Partial<CollectionDetail> = {}): CollectionDetail {
  const items = overrides.items ?? [
    makeItem(),
    makeItem({
      id: 2,
      position: 1,
      external_episode_id: "e2",
      episode_title: "No.24 芯片江湖之中国半导体劫起",
      recommendation: "",
      shownotes: "",
      adopted_episode_id: 77,
      adopted_episode_title: "No.24 芯片江湖之中国半导体劫起",
      adopted_episode_queue: "inbox",
    }),
  ];
  return {
    id: 1,
    title: "穿透半导体迷雾",
    description: "半导体产业盘点。",
    author: "小宇宙领航员",
    platform: "xiaoyuzhoufm",
    external_id: "6a20323b78a52c96d821a769",
    source_url: "https://www.xiaoyuzhoufm.com/collection/episode/6a20323b78a52c96d821a769",
    total_known: false,
    revision: 1,
    item_count: 2,
    adopted_count: 1,
    created_at: "2026-09-12T16:00:00Z",
    last_refreshed_at: null,
    items,
    ...overrides,
  };
}

function renderDetail(ui: ReactElement) {
  return render(<SWRConfig value={{ provider: () => new Map() }}>{ui}</SWRConfig>);
}

describe("CollectionDetailContent", () => {
  let callCount = 0;
  beforeEach(() => {
    callCount = 0;
  });
  beforeEach(() => {
    vi.clearAllMocks();
    vi.resetAllMocks();
    detailMock.mockResolvedValue(makeDetail());
  });

  it("preserves original order, recommendations and real adopted states", async () => {
    renderDetail(<CollectionDetailContent collectionID={1} />);

    // 标题同时出现在工具栏与文档流中，等待任一出现即可。
    await screen.findAllByText("穿透半导体迷雾");
    const metas = await screen.findAllByText(/作者：小宇宙领航员/);
    expect(metas.length).toBeGreaterThan(0);

    const items = screen.getAllByTestId("collection-item");
    expect(items).toHaveLength(2);
    expect(items[0]).toHaveTextContent("E185 芯片规律 × AI浪潮");
    expect(items[1]).toHaveTextContent("No.24 芯片江湖之中国半导体劫起");

    // 推荐语分区展示；缺失推荐语不编造。
    expect(screen.getByText("存储芯片为何五年内持续短缺？")).toBeInTheDocument();
    expect(within2(items[1]).queryByText("清单推荐语")).not.toBeInTheDocument();

    // 收录状态来自真实队列：未收录 vs 已在 Inbox。
    expect(within2(items[0]).getByText("未收录")).toBeInTheDocument();
    expect(within2(items[1]).getByText("已在 Inbox")).toBeInTheDocument();
    expect(within2(items[1]).getByRole("link", { name: "查看收录单集" })).toHaveAttribute(
      "href",
      "/episodes/77",
    );
  });

  it("filters entries by adopted state and keeps original numbering", async () => {
    const user = userEvent.setup();
    renderDetail(<CollectionDetailContent collectionID={1} />);
    await screen.findByText("E185 芯片规律 × AI浪潮");

    await user.click(screen.getByRole("button", { name: /未收录/ }));
    const unadopted = screen.getAllByTestId("collection-item");
    expect(unadopted).toHaveLength(1);
    expect(unadopted[0]).toHaveTextContent("E185 芯片规律 × AI浪潮");
    // 过滤后保留原始序号。
    expect(within2(unadopted[0]).getByText("1")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /已收录/ }));
    const adopted = screen.getAllByTestId("collection-item");
    expect(adopted).toHaveLength(1);
    expect(adopted[0]).toHaveTextContent("No.24 芯片江湖之中国半导体劫起");

    await user.click(screen.getByRole("button", { name: /全部/ }));
    expect(screen.getAllByTestId("collection-item")).toHaveLength(2);
  });

  it("opens show notes and the source episode link", async () => {
    const user = userEvent.setup();
    renderDetail(<CollectionDetailContent collectionID={1} />);
    await screen.findByText("E185 芯片规律 × AI浪潮");

    await user.click(screen.getByText("Show Notes"));
    expect(await screen.findByText("时间轴：00:00 开场")).toBeInTheDocument();

    const sourceLinks = screen.getAllByRole("link", { name: "打开原单集" });
    expect(sourceLinks[0]).toHaveAttribute(
      "href",
      "https://www.xiaoyuzhoufm.com/episode/e1",
    );
    expect(sourceLinks[0]).toHaveAttribute("target", "_blank");
  });

  it("shows a distinct failure state with a way back", async () => {
    detailMock.mockRejectedValue(new Error("清单不存在"));
    renderDetail(<CollectionDetailContent collectionID={1} />);

    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(screen.getByText("清单暂时无法读取")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "返回播客清单" })).toHaveAttribute(
      "href",
      "/collections",
    );
  });

  it("adopts an un-collected entry into Inbox and refreshes real states", async () => {
    const user = userEvent.setup();
    adoptMock.mockResolvedValue({
      item_id: 1,
      episode_id: 501,
      queue_state: "inbox",
      dismissed_at: null,
      episode_created: true,
      podcast_created: true,
      podcast_id: 60,
      podcast_title: "投资实战派",
      podcast_subscribed: false,
      collection_only: true,
      audio_available: true,
      inbox_written: true,
    });
    // 收录后重新读取的详情：该条目已关联本地单集并进入 Inbox。
    detailMock.mockImplementation(() =>
      Promise.resolve(
        callCount++ === 0
          ? makeDetail()
          : makeDetail({
              adopted_count: 2,
              items: [
                makeItem({
                  adopted_episode_id: 501,
                  adopted_episode_title: "E185 芯片规律 × AI浪潮",
                  adopted_episode_queue: "inbox",
                }),
                makeItem({
                  id: 2,
                  position: 1,
                  external_episode_id: "e2",
                  episode_title: "No.24 芯片江湖之中国半导体劫起",
                  recommendation: "",
                  shownotes: "",
                  adopted_episode_id: 77,
                  adopted_episode_title: "No.24",
                  adopted_episode_queue: "inbox",
                }),
              ],
            }),
      ),
    );
    renderDetail(<CollectionDetailContent collectionID={1} />);
    await screen.findByText("E185 芯片规律 × AI浪潮");

    const adoptButton = screen.getByRole("button", { name: "加入 Inbox" });
    expect(
      screen.getAllByRole("button", { name: "加入 Inbox" }).length,
    ).toBe(1);
    await user.click(adoptButton);

    await waitFor(() => {
      expect(adoptMock).toHaveBeenCalledWith(1, 1);
      // 收录状态来自服务端回读：按钮消失，条目显示已在 Inbox。
      expect(screen.queryByRole("button", { name: "加入 Inbox" })).not.toBeInTheDocument();
      expect(screen.getAllByText("已在 Inbox").length).toBe(2);
    });
  });

  it("shows a distinct error when adoption is rejected", async () => {
    const user = userEvent.setup();
    adoptMock.mockRejectedValue(
      Object.assign(new Error("曾删除"), {
        code: "EPISODE_DELETED",
        response: { data: { error: { code: "EPISODE_DELETED", message: "这一集曾从个人库删除" } } },
      }),
    );
    renderDetail(<CollectionDetailContent collectionID={1} />);
    await screen.findByText("E185 芯片规律 × AI浪潮");

    await user.click(screen.getByRole("button", { name: "加入 Inbox" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/曾从个人库删除/);
    // 失败不伪造状态：条目仍是未收录且可重试。
    const rejectedItems = screen.getAllByTestId("collection-item");
    expect(within(rejectedItems[0]).getByText("未收录")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "加入 Inbox" })).toBeEnabled();
  });

  it("renders an invalid address notice for id 0", async () => {
    renderDetail(<CollectionDetailContent collectionID={0} />);
    await waitFor(() => {
      expect(screen.getByText("清单地址无效")).toBeInTheDocument();
    });
  });
});

function within2(element: HTMLElement) {
  return within(element);
}
