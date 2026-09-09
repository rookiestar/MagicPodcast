import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Podcast, Tag } from "@/types";
import {
  DesktopPodcastDetailInfo,
  MobilePodcastDetailInfo,
} from "../PodcastDetailInfo";

vi.mock("@/components/RichText", () => ({
  default: ({ html }: { html: string }) => <div>{html}</div>,
}));

vi.mock("@/components/tags/TagInput", () => ({
  default: ({ disabled }: { disabled?: boolean }) => (
    <input aria-label="标签输入" disabled={disabled} />
  ),
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
  newest_episode_date: "2026-01-01T00:00:00Z",
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
  it("toggles mobile detail expansion without querying the document by id", () => {
    const { container } = render(<MobilePodcastDetailInfo {...baseProps} />);

    const toggle = screen.getByRole("button", { name: "展开详细信息" });
    const details = container.querySelector("details");

    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(details).not.toHaveAttribute("open");

    fireEvent.click(toggle);

    expect(
      screen.getByRole("button", { name: "收起详细信息" }),
    ).toHaveAttribute("aria-expanded", "true");
    expect(details).toHaveAttribute("open");
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

  it("presents the desktop detail as a reading surface with adjacent management", () => {
    render(<DesktopPodcastDetailInfo {...baseProps} />);

    expect(
      screen.getByRole("article", { name: "测试播客" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("region", { name: "标签与备注" }),
    ).toBeInTheDocument();
    expect(screen.getByText("标签与备注")).toBeInTheDocument();
    expect(screen.queryByText("个人管理")).not.toBeInTheDocument();
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
    expect(heading).toContainElement(screen.getByText("主播"));
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
    expect(screen.getByText("作者")).toBeInTheDocument();
    expect(screen.getByText("12")).toBeInTheDocument();
    expect(screen.getByText("最近更新")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /播放最新一集/ })).toBeInTheDocument();
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

  it("keeps the mobile cover in the existing fixed header slot", () => {
    const { container } = render(<MobilePodcastDetailInfo {...baseProps} />);

    const mobileCover = container.querySelector(".podcast-reading-mobile-cover");
    expect(mobileCover).not.toBeNull();
    expect(mobileCover).toContainElement(screen.getByTestId("podcast-cover"));
    expect(container.querySelector(".podcast-reading-heading")).toBeNull();
    expect(screen.getByTestId("podcast-cover")).toHaveAttribute(
      "data-sizes",
      "96px",
    );
  });
});
