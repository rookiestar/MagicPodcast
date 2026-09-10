import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Podcast, Tag } from "@/types";
import {
  DesktopPodcastDetailInfo,
  MobilePodcastDetailInfo,
} from "../PodcastDetailInfo";

const tagMocks = vi.hoisted(() => ({
  availableTags: [
    { id: 1, name: "科技", color: "#2563eb" },
    { id: 2, name: "AI", color: "#16a34a" },
  ] as Tag[],
  ensureAvailableTags: vi.fn(),
  create: vi.fn(),
}));

vi.mock("@/hooks/useAvailableTags", () => ({
  useAvailableTags: () => ({
    availableTags: tagMocks.availableTags,
    loading: false,
    ensureAvailableTags: tagMocks.ensureAvailableTags,
    appendAvailableTag: vi.fn(),
  }),
}));

vi.mock("@/lib/api", () => ({
  tagApi: {
    create: (...args: unknown[]) => tagMocks.create(...args),
  },
}));

vi.mock("../PodcastCover", () => ({
  default: ({
    title,
    coverUrl,
    sizes,
  }: {
    title: string;
    coverUrl?: string;
    sizes?: string;
  }) => (
    <div
      data-testid="podcast-cover"
      data-cover-url={coverUrl ?? ""}
      data-sizes={sizes}
    >
      {title}
    </div>
  ),
}));

const podcast: Podcast = {
  id: 1,
  xyz_id: "podcast-1",
  title: "测试播客",
  description: "简介内容",
  author: "作者",
  cover_url: "",
  episode_count: 12,
  newest_episode_date: new Date(2026, 7, 29, 13, 36, 45).toISOString(),
  created_at: "2026-01-01T00:00:00Z",
  is_subscribed: true,
  is_dead: false,
  link: "https://example.com",
};

const tag: Tag = {
  id: 1,
  name: "科技",
  color: "#2563eb",
};

const baseProps = {
  podcast,
  tags: [tag],
  notes: "",
  isEditingNotes: false,
  isSavingNotes: false,
  isUpdatingTags: false,
  onNotesChange: vi.fn(),
  onEditNotes: vi.fn(),
  onSaveNotes: vi.fn(),
  onCancelNotesEdit: vi.fn(),
  onTagsChange: vi.fn(),
};

describe("PodcastDetailInfo", () => {
  it("renders compact title metadata to the minute without the archive kicker", () => {
    render(<DesktopPodcastDetailInfo {...baseProps} />);

    expect(screen.queryByText("个人播客库 · 节目档案")).not.toBeInTheDocument();
    expect(screen.queryByText("主播")).not.toBeInTheDocument();
    expect(
      screen.getByText("作者 · 12 集 · 更新于 2026/08/29 13:36"),
    ).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "测试播客" })).toBeInTheDocument();
  });

  it("keeps invalid dates honest and hides misleading playback", () => {
    render(
      <DesktopPodcastDetailInfo
        {...baseProps}
        podcast={{
          ...podcast,
          newest_episode_date: "not-a-date",
          newest_enclosure_url: "",
          newest_enclosure_duration: 0,
        }}
      />,
    );

    expect(screen.getByText("作者 · 12 集 · 更新于 未知")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "播放最新一集" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText(/分.*秒/)).not.toBeInTheDocument();
  });

  it("removes the header playback and management heading while keeping metadata", () => {
    render(<DesktopPodcastDetailInfo {...baseProps} podcast={{...podcast, newest_enclosure_url:"https://example.com/latest.mp3", newest_enclosure_duration:125}} />);
    expect(screen.queryByRole("button", { name:"播放最新一集" })).not.toBeInTheDocument();
    expect(screen.queryByText("2分5秒")).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name:"标签与备注" })).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name:"标签与备注" })).toBeInTheDocument();
  });

  it("clamps long descriptions and leaves short copy uncollapsed", () => {
    const { rerender } = render(<DesktopPodcastDetailInfo {...baseProps} />);
    expect(screen.getByText("简介内容")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "查看全文" }),
    ).not.toBeInTheDocument();

    const longDescription = "很长的节目简介。".repeat(30);
    rerender(
      <DesktopPodcastDetailInfo
        {...baseProps}
        podcast={{ ...podcast, description: longDescription }}
      />,
    );

    const toggle = screen.getByRole("button", { name: "查看全文" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(toggle);
    expect(screen.getByRole("button", { name: "收起" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
    expect(screen.getByText(longDescription)).toBeInTheDocument();
  });

  it("disables note controls while notes are being saved", () => {
    render(
      <DesktopPodcastDetailInfo
        {...baseProps}
        isEditingNotes
        isSavingNotes
        notes="临时备注"
      />,
    );

    expect(screen.getByPlaceholderText("添加备注...")).toBeDisabled();
    expect(screen.getByRole("button", { name: "保存中..." })).toBeDisabled();
    expect(screen.getByRole("button", { name: "取消" })).toBeDisabled();
  });

  it("offers a direct empty-notes entry and keeps existing notes editable", () => {
    const onEditNotes = vi.fn();
    const { rerender } = render(
      <DesktopPodcastDetailInfo {...baseProps} onEditNotes={onEditNotes} />,
    );

    fireEvent.click(screen.getByRole("button", { name: "＋ 添加备注" }));
    expect(onEditNotes).toHaveBeenCalledTimes(1);
    expect(screen.queryByText("暂无备注")).not.toBeInTheDocument();

    rerender(
      <DesktopPodcastDetailInfo
        {...baseProps}
        notes="已有备注"
        onEditNotes={onEditNotes}
      />,
    );
    expect(screen.getByText("已有备注")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "编辑" })).toBeInTheDocument();
  });

  it("opens the tag panel from the compact add control", () => {
    const onTagsChange = vi.fn();
    render(
      <DesktopPodcastDetailInfo {...baseProps} onTagsChange={onTagsChange} />,
    );

    fireEvent.click(screen.getByRole("button", { name: "＋ 添加标签" }));
    expect(screen.getByRole("dialog", { name: "添加标签" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("option", { name: /AI/ }));
    expect(onTagsChange).toHaveBeenCalledWith([
      tag,
      { id: 2, name: "AI", color: "#16a34a" },
    ]);
  });

  it("presents the desktop detail as a reading surface with adjacent management", () => {
    render(<DesktopPodcastDetailInfo {...baseProps} />);

    expect(
      screen.getByRole("article", { name: "测试播客" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("region", { name: "标签与备注" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("heading", {name: "标签与备注"})).not.toBeInTheDocument();
  });

  it("keeps the desktop cover in a fixed heading square independent of description length", () => {
    const { container, rerender } = render(
      <DesktopPodcastDetailInfo {...baseProps} />,
    );

    const heading = container.querySelector(".podcast-reading-heading");
    const cover = heading?.querySelector(".podcast-reading-cover");
    const description = container.querySelector(".podcast-reading-description");
    const renderedCover = screen.getByTestId("podcast-cover");

    expect(heading).not.toBeNull();
    expect(cover).not.toBeNull();
    expect(heading).toContainElement(screen.getByRole("heading", { level: 1 }));
    expect(description).not.toContainElement(cover as HTMLElement);
    expect(renderedCover).toHaveAttribute("data-sizes", "96px");
    expect(container.querySelector(".podcast-reading-hero")?.children).toHaveLength(
      2,
    );

    rerender(
      <DesktopPodcastDetailInfo
        {...baseProps}
        podcast={{
          ...podcast,
          description: "很长的节目简介。".repeat(40),
        }}
      />,
    );

    const nextHeading = container.querySelector(".podcast-reading-heading");
    const nextCover = nextHeading?.querySelector(".podcast-reading-cover");
    expect(nextCover).not.toBeNull();
    expect(nextHeading).toContainElement(nextCover as HTMLElement);
    expect(screen.getByTestId("podcast-cover")).toHaveAttribute(
      "data-sizes",
      "96px",
    );
    expect(screen.getByText("很长的节目简介。".repeat(40))).toBeInTheDocument();
  });

  it("keeps title, metadata, playback, website, tags and notes available on desktop", () => {
    render(
      <DesktopPodcastDetailInfo
        {...baseProps}
        notes="已有备注"
        podcast={{
          ...podcast,
          newest_enclosure_url: "https://example.com/latest.mp3",
        }}
      />,
    );

    expect(screen.getByRole("heading", { name: "测试播客" })).toBeInTheDocument();
    expect(screen.getByText(/作者 · 12 集 · 更新于/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "播放最新一集" })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: /节目官网/ })).toHaveAttribute(
      "href",
      "https://example.com",
    );
    expect(screen.getByText("科技")).toBeInTheDocument();
    expect(screen.getByText("已有备注")).toBeInTheDocument();
  });

  it("renders a desktop cover slot when the podcast has no cover url", () => {
    render(<DesktopPodcastDetailInfo {...baseProps} />);

    expect(screen.getByTestId("podcast-cover")).toHaveAttribute(
      "data-cover-url",
      "",
    );
    expect(
      document.querySelector(".podcast-reading-heading .podcast-reading-cover"),
    ).not.toBeNull();
  });

  it("keeps the mobile cover in the existing fixed header slot and stacks management", () => {
    const { container } = render(<MobilePodcastDetailInfo {...baseProps} />);

    const mobileCover = container.querySelector(".podcast-reading-mobile-cover");
    expect(mobileCover).not.toBeNull();
    expect(mobileCover).toContainElement(screen.getByTestId("podcast-cover"));
    expect(container.querySelector(".podcast-reading-heading")).toBeNull();
    expect(screen.getByTestId("podcast-cover")).toHaveAttribute(
      "data-sizes",
      "96px",
    );
    expect(
      screen.getByRole("region", { name: "节目管理" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "展开详细信息" }),
    ).not.toBeInTheDocument();
    expect(
      within(screen.getByRole("article", { name: "测试播客" })).getByText(
        "作者 · 12 集 · 更新于 2026/08/29 13:36",
      ),
    ).toBeInTheDocument();
  });
});
