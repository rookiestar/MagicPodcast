import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Tag } from "@/types";
import SelectedTagList from "../SelectedTagList";

const tags: Tag[] = [
  { id: 1, name: "科技", color: "#2563eb" },
  { id: 2, name: "AI", color: "#16a34a" },
];

describe("SelectedTagList", () => {
  it("renders selected tags with remove buttons", () => {
    render(
      <SelectedTagList
        selectedTags={tags}
        showSelectedTags
        onRemoveTag={vi.fn()}
      />,
    );

    expect(screen.getByText("科技")).toBeInTheDocument();
    expect(screen.getByText("AI")).toBeInTheDocument();
    expect(screen.getByTitle('移除 "科技" 标签')).toBeInTheDocument();
  });

  it("calls remove callback with the tag id", () => {
    const onRemoveTag = vi.fn();

    render(
      <SelectedTagList
        selectedTags={tags}
        showSelectedTags
        onRemoveTag={onRemoveTag}
      />,
    );

    fireEvent.click(screen.getByTitle('移除 "AI" 标签'));

    expect(onRemoveTag).toHaveBeenCalledWith(2);
  });

  it("renders nothing when selected tags are hidden", () => {
    const { container } = render(
      <SelectedTagList
        selectedTags={tags}
        showSelectedTags={false}
        onRemoveTag={vi.fn()}
      />,
    );

    expect(container).toBeEmptyDOMElement();
  });
});
