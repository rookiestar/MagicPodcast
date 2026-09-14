import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { previewMock, confirmMock, fetchDetailMock } = vi.hoisted(() => ({
  previewMock: vi.fn(),
  confirmMock: vi.fn(),
  fetchDetailMock: vi.fn(),
}));

vi.mock("@/lib/collections", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/collections")>();
  return {
    ...actual,
    previewCollection: previewMock,
    confirmCollectionImport: confirmMock,
    fetchCollectionDetail: fetchDetailMock,
  };
});

import ImportCollectionModal from "../ImportCollectionModal";
import type { CollectionPreview } from "@/types/collection";

const SAMPLE_URL =
  "https://www.xiaoyuzhoufm.com/collection/episode/6a20323b78a52c96d821a769";

const CAMPAIGN_URL = "https://collection.xiaoyuzhoufm.com/wavesfilm2026";

const ACTIVITY_URL = "https://h5.xiaoyuzhoufm.com/xyz-activity/forgenz";

function makePreview(overrides: Partial<CollectionPreview> = {}): CollectionPreview {
  return {
    preview_id: "preview-token-1",
    platform: "xiaoyuzhoufm",
    external_id: "6a20323b78a52c96d821a769",
    title: "穿透半导体迷雾",
    description: "半导体产业盘点。",
    author: "小宇宙领航员",
    source_url: SAMPLE_URL,
    total_known: false,
    read_count: 2,
    duplicate: false,
    existing_collection_id: null,
    items: [
      {
        position: 0,
        external_episode_id: "e1",
        episode_title: "E185 芯片规律 × AI浪潮",
        podcast_title: "投资实战派",
        podcast_author: "wong永庆",
        recommendation: "存储芯片为何五年内持续短缺？",
        duration: 4692,
        published_at: "2026-06-01T00:00:00Z",
        episode_url: "https://www.xiaoyuzhoufm.com/episode/e1",
        pay_type: "FREE",
        is_private_media: false,
      },
      {
        position: 1,
        external_episode_id: "e2",
        episode_title: "No.24 芯片江湖之中国半导体劫起",
        podcast_title: "半拿铁 | 商业沉浮录",
        podcast_author: "刘飞",
        recommendation: "",
        duration: 3989,
        published_at: "2022-11-02T00:00:00Z",
        episode_url: "https://www.xiaoyuzhoufm.com/episode/e2",
        pay_type: "FREE",
        is_private_media: false,
      },
    ],
    ...overrides,
  };
}

function axiosLikeError(code: string, message: string) {
  // collectionErrorMessage 读取 axios 错误的 response.data.error.message。
  return Object.assign(new Error(message), {
    code: "ERR_BAD_REQUEST",
    response: { data: { error: { code, message } } },
  });
}

function renderModal(onImported = vi.fn()) {
  const onClose = vi.fn();
  render(
    <ImportCollectionModal
      isOpen
      onClose={onClose}
      onImported={onImported}
    />,
  );
  return { onClose, onImported };
}

describe("ImportCollectionModal", () => {
  it("shows source and merged counts and retains different recommendations", async () => {
    const preview = makePreview({source_item_count: 3, duplicate_item_count: 1});
    preview.items[0].recommendation = "第一条\n\n第二条";
    previewMock.mockResolvedValue(preview);
    renderModal();
    await userEvent.type(screen.getByLabelText("小宇宙单集清单链接"), SAMPLE_URL);
    await userEvent.click(screen.getByRole("button", {name: "预览"}));
    expect(await screen.findByText(/已读取 3 个条目，合并 1 个重复条目，共 2 集/)).toBeInTheDocument();
    expect(screen.getByText(/第一条/).textContent).toContain("第一条\n\n第二条");
    expect(screen.getByRole("button", {name: "导入清单"})).toBeEnabled();
  });
  beforeEach(() => {
    vi.clearAllMocks();
    // resetAllMocks 清除上一用例的 rejected 实现。
    vi.resetAllMocks();
  });

  it("previews a URL and shows the read count and original recommendations", async () => {
    const user = userEvent.setup();
    previewMock.mockResolvedValue(makePreview());
    renderModal();

    await user.type(screen.getByLabelText("小宇宙单集清单链接"), SAMPLE_URL);
    await user.click(screen.getByRole("button", { name: "预览" }));

    expect(await screen.findByText("穿透半导体迷雾")).toBeInTheDocument();
    expect(screen.getByText(/作者：小宇宙领航员/)).toBeInTheDocument();
    expect(screen.getByText(/已读取 2 集/)).toBeInTheDocument();
    // 原始推荐语随条目展示。
    expect(screen.getByText(/存储芯片为何五年内持续短缺？/)).toBeInTheDocument();
    // 缺失推荐语的条目不编造替代文案。
    expect(screen.getByText(/No.24 芯片江湖之中国半导体劫起/)).toBeInTheDocument();
  });

  it("accepts an h5 activity page URL as a collection source", async () => {
    const user = userEvent.setup();
    previewMock.mockResolvedValue(
      makePreview({
        external_id: "activity:forgenz",
        source_url: ACTIVITY_URL,
        title: "00后的宇宙必听｜给正在长大的你",
        author: "",
      }),
    );
    renderModal();

    // 弹窗提示明确活动页链接也受支持。
    expect(screen.getByText(/h5\.xiaoyuzhoufm\.com\/xyz-activity\/…/)).toBeInTheDocument();
    await user.type(screen.getByLabelText("小宇宙单集清单链接"), ACTIVITY_URL);
    await user.click(screen.getByRole("button", { name: "预览" }));

    expect(await screen.findByText(/00后的宇宙必听/)).toBeInTheDocument();
    expect(previewMock).toHaveBeenCalledWith(
      ACTIVITY_URL,
      expect.anything(),
    );
  });

  it("accepts a campaign topic page URL as a collection source", async () => {
    const user = userEvent.setup();
    previewMock.mockResolvedValue(
      makePreview({
        external_id: "wavesfilm2026",
        source_url: CAMPAIGN_URL,
        title: "海浪电影周 播客特别企划：世界在每个清晨重启",
        author: "",
      }),
    );
    renderModal();

    // 弹窗提示明确两种链接格式都受支持。
    expect(
      screen.getByText(/collection\.xiaoyuzhoufm\.com\/…/),
    ).toBeInTheDocument();
    await user.type(screen.getByLabelText("小宇宙单集清单链接"), CAMPAIGN_URL);
    await user.click(screen.getByRole("button", { name: "预览" }));

    expect(await screen.findByText(/海浪电影周/)).toBeInTheDocument();
    expect(previewMock).toHaveBeenCalledWith(
      CAMPAIGN_URL,
      expect.anything(),
    );
    // 专题没有公开作者时不编造。
    expect(screen.getByText(/作者：未提供/)).toBeInTheDocument();
  });

  it("confirms the previewed version and reports the result", async () => {
    const user = userEvent.setup();
    previewMock.mockResolvedValue(makePreview());
    confirmMock.mockResolvedValue({ duplicate: false, collection_id: 9 });
    const { onImported } = renderModal();

    await user.type(screen.getByLabelText("小宇宙单集清单链接"), SAMPLE_URL);
    await user.click(screen.getByRole("button", { name: "预览" }));
    await screen.findByText("穿透半导体迷雾");
    await user.click(screen.getByRole("button", { name: "导入清单" }));

    await waitFor(() => {
      expect(confirmMock).toHaveBeenCalledWith("preview-token-1");
      expect(onImported).toHaveBeenCalledWith({ duplicate: false, collectionID: 9 });
    });
  });

  it("warns on a duplicate source and does not confirm it again", async () => {
    const user = userEvent.setup();
    previewMock.mockResolvedValue(
      makePreview({ duplicate: true, existing_collection_id: 9 }),
    );
    const { onClose, onImported } = renderModal();

    await user.type(screen.getByLabelText("小宇宙单集清单链接"), SAMPLE_URL);
    await user.click(screen.getByRole("button", { name: "预览" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "这份清单已经导入过",
    );
    expect(screen.queryByRole("button", { name: "导入清单" })).toBeNull();
    fetchDetailMock.mockResolvedValue(makePreview());
    await user.click(screen.getByRole("button", { name: "打开已有清单" }));

    expect(confirmMock).not.toHaveBeenCalled();
    expect(fetchDetailMock).toHaveBeenCalledWith(9);
    expect(onImported).toHaveBeenCalledWith({ duplicate: true, collectionID: 9 });
    expect(onClose).toHaveBeenCalled();
  });

  it("keeps the preview and allows re-import when the duplicate was deleted", async () => {
    const user = userEvent.setup();
    previewMock.mockResolvedValue(
      makePreview({ duplicate: true, existing_collection_id: 9 }),
    );
    fetchDetailMock.mockRejectedValue(
      axiosLikeError("COLLECTION_NOT_FOUND", "清单不存在"),
    );
    confirmMock.mockResolvedValue({ duplicate: false, collection_id: 10 });
    const { onClose, onImported } = renderModal();

    await user.type(screen.getByLabelText("小宇宙单集清单链接"), SAMPLE_URL);
    await user.click(screen.getByRole("button", { name: "预览" }));
    await user.click(await screen.findByRole("button", { name: "打开已有清单" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "已有清单已删除，可以重新导入这份预览。",
    );
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "导入清单" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "导入清单" }));
    await waitFor(() => {
      expect(confirmMock).toHaveBeenCalledWith("preview-token-1");
      expect(onImported).toHaveBeenCalledWith({ duplicate: false, collectionID: 10 });
    });
  });

  it("keeps a confirmation race visible until the user opens the existing collection", async () => {
    const user = userEvent.setup();
    previewMock.mockResolvedValue(makePreview());
    confirmMock.mockResolvedValue({ duplicate: true, collection_id: 9 });
    const { onClose, onImported } = renderModal();
    await user.type(screen.getByLabelText("小宇宙单集清单链接"), SAMPLE_URL);
    await user.click(screen.getByRole("button", { name: "预览" }));
    await user.click(await screen.findByRole("button", { name: "导入清单" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("这份清单已经导入过");
    expect(screen.getAllByRole("alert")).toHaveLength(1);
    expect(onClose).not.toHaveBeenCalled();
    expect(onImported).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "打开已有清单" }));
    expect(confirmMock).toHaveBeenCalledTimes(1);
    expect(onImported).toHaveBeenCalledWith({ duplicate: true, collectionID: 9 });
  });

  it("surfaces an expired preview and asks for a fresh preview", async () => {
    const user = userEvent.setup();
    previewMock.mockResolvedValue(makePreview());
    confirmMock.mockRejectedValue(
      Object.assign(new Error("预览已过期"), { code: "PREVIEW_EXPIRED" }),
    );
    renderModal();

    await user.type(screen.getByLabelText("小宇宙单集清单链接"), SAMPLE_URL);
    await user.click(screen.getByRole("button", { name: "预览" }));
    await screen.findByText("穿透半导体迷雾");
    await user.click(screen.getByRole("button", { name: "导入清单" }));

    expect(
      await screen.findByText("预览已过期，请重新预览后再导入。"),
    ).toBeInTheDocument();
    // 回到链接输入步骤，可重新预览。
    expect(await screen.findByLabelText("小宇宙单集清单链接")).toBeInTheDocument();
  });

  it("shows a distinct failure message and keeps the modal usable", async () => {
    const user = userEvent.setup();
    previewMock.mockRejectedValue(
      axiosLikeError("SOURCE_FORBIDDEN", "来源拒绝了读取（可能需要登录或已限制访问），未保存任何清单"),
    );
    renderModal();

    await user.type(screen.getByLabelText("小宇宙单集清单链接"), SAMPLE_URL);
    await user.click(screen.getByRole("button", { name: "预览" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/来源拒绝了读取/);
    expect(screen.getByRole("button", { name: "预览" })).toBeEnabled();
  });

  it("closes on Escape and via the change-link action", async () => {
    const user = userEvent.setup();
    previewMock.mockResolvedValue(makePreview());
    const { onClose } = renderModal();

    await user.type(screen.getByLabelText("小宇宙单集清单链接"), SAMPLE_URL);
    await user.click(screen.getByRole("button", { name: "预览" }));
    await screen.findByText("穿透半导体迷雾");

    await user.click(screen.getByRole("button", { name: "修改链接" }));
    expect(screen.getByLabelText("小宇宙单集清单链接")).toBeInTheDocument();

    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(onClose).toHaveBeenCalled();
    });
  });
});
