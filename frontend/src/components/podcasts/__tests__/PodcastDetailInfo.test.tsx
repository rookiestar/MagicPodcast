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
  default: ({ title }: { title: string }) => <div>{title}</div>,
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
});
