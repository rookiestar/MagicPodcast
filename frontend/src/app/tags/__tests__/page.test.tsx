import { act, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Tag } from "@/types";

const mockTagState = vi.hoisted(() => ({ tags: [] as Tag[] }));

// PageLayout passthrough: applies rootClassName / className / toolbar.className
// so the editorial chrome contract is observable in the DOM.
vi.mock("@/components/layout/PageLayout", () => ({
  default: ({ rootClassName, className, toolbar, children }: any) => (
    <div className={rootClassName}>
      {toolbar && (
        <div className={toolbar.className} data-testid="toolbar">
          {toolbar.breadcrumbs?.map((item: any) => (
            <a key={item.label} href={item.href}>
              {item.label}
            </a>
          ))}
          {toolbar.rightContent}
        </div>
      )}
      <div className={className}>{children}</div>
    </div>
  ),
}));

const tagMutate = vi.fn();
vi.mock("@/hooks/useTagSWR", () => ({
  useTags: () => ({ tags: mockTagState.tags, isLoading: false, isError: false, mutate: tagMutate }),
}));

vi.mock("@/hooks/usePodcastSWR", () => ({
  usePodcast: () => ({ podcast: null, isLoading: false, isError: false }),
  usePodcastTags: () => ({ tags: [] as Tag[], mutate: vi.fn() }),
}));

vi.mock("@/lib/api", () => ({
  tagApi: { create: vi.fn(), update: vi.fn(), delete: vi.fn() },
  podcastApi: { addTag: vi.fn(), removeTag: vi.fn() },
}));

vi.mock("@/lib/toast", () => ({
  toast: { error: vi.fn(), success: vi.fn(), warning: vi.fn() },
}));

vi.mock("@/components/podcasts/PodcastCover", () => ({
  default: ({ title }: any) => <div>{title}</div>,
}));

vi.mock("@/components/tags/TagInput", () => ({
  default: () => <div data-testid="tag-input" />,
}));

vi.mock("pinyin-pro", () => ({ pinyin: () => "Z" }));

import TagsPage from "../page";

beforeEach(() => {
  mockTagState.tags = [];
});

async function settleAsyncEffects() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

describe("tags page editorial chrome (#53)", () => {
  it("adopts the editorial shell, toolbar and section classes", async () => {
    const { container } = render(<TagsPage />);
    await settleAsyncEffects();

    expect(container.querySelector(".editorial-page-shell")).toBeTruthy();
    expect(container.querySelector(".editorial-page-toolbar")).toBeTruthy();
    expect(container.querySelector(".tag-page")).toBeTruthy();
  });

  it("does not repeat the global home navigation in the page toolbar", async () => {
    render(<TagsPage />);
    await settleAsyncEffects();

    expect(screen.queryByRole("link", { name: "返回首页" })).not.toBeInTheDocument();
  });

  it("renders the editorial empty state with the create CTA", async () => {
    render(<TagsPage />);
    await settleAsyncEffects();

    const state = screen.getByText("还没有创建任何标签").closest(".editorial-state");
    expect(state).toBeTruthy();
    expect(screen.getByRole("button", { name: "创建第一个标签" })).toBeTruthy();
  });

  it("uses the editorial primary button for 新建 and a segmented sort control", async () => {
    render(<TagsPage />);
    await settleAsyncEffects();

    const createBtn = screen.getByRole("button", { name: "新建" });
    expect(createBtn.className).toContain("editorial-btn--primary");

    const segmented = document.querySelector(".editorial-segmented");
    expect(segmented).toBeTruthy();
    const popularity = screen.getByRole("button", { name: "热度" });
    const alphabetical = screen.getByRole("button", { name: "字母" });
    // default sort is popularity → active
    expect(popularity.getAttribute("aria-pressed")).toBe("true");
    expect(alphabetical.getAttribute("aria-pressed")).toBe("false");
  });

  it("separates page actions from the tag sort choices", async () => {
    render(<TagsPage />);
    await settleAsyncEffects();

    const actions = document.querySelector(".tag-toolbar-primary");
    const sortGroup = screen.getByRole("group", { name: "标签排序" });

    expect(actions).toContainElement(screen.getByRole("button", { name: "新建" }));
    expect(actions).toContainElement(screen.getByRole("button", { name: "多选" }));
    expect(actions).not.toContainElement(sortGroup);
    expect(sortGroup).toHaveClass("tag-toolbar-sort");
    expect(screen.getByText("排序")).toHaveClass("tag-toolbar-sort-label");
  });

  it("keeps tag editing and deletion as sibling controls", async () => {
    mockTagState.tags = [
      { id: 1, name: "技术与产品", color: "#a85432", podcast_count: 3 },
    ];

    render(<TagsPage />);
    await settleAsyncEffects();

    const editButton = await screen.findByRole("button", {
      name: "编辑标签 技术与产品",
    });
    const card = editButton.closest(".tag-card");

    expect(card).not.toBeNull();
    expect(card).not.toHaveAttribute("role", "button");
    expect(card?.querySelectorAll("button")).toHaveLength(2);
    expect(card?.querySelector('[role="button"]')).toBeNull();
  });

  it("keeps tag cards wide enough and exposes the full name on hover", async () => {
    mockTagState.tags = [
      { id: 1, name: "技术与产品", color: "#a85432", podcast_count: 3 },
    ];

    const { container } = render(<TagsPage />);
    await settleAsyncEffects();

    expect(container.querySelector(".tag-grid")).toBeTruthy();
    expect(
      container.querySelector(".tag-card-name > span"),
    ).toHaveAttribute("title", "技术与产品");
  });
});
