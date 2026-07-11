import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Tag } from "@/types";
import TagSuggestionsDropdown from "../TagSuggestionsDropdown";

const tags: Tag[] = [
  { id: 1, name: "科技", color: "#2563eb" },
  { id: 2, name: "AI", color: "#16a34a" },
];

function renderDropdown(overrides = {}) {
  const props = {
    filteredTags: tags,
    inputValue: "a",
    availableTags: tags,
    loading: false,
    highlightedIndex: -1,
    disabled: false,
    onClose: vi.fn(),
    onSelectTag: vi.fn(),
    onCreateTag: vi.fn(),
    onHighlightTag: vi.fn(),
    ...overrides,
  };

  return {
    props,
    ...render(<TagSuggestionsDropdown {...props} />),
  };
}

describe("TagSuggestionsDropdown", () => {
  it("renders matching tags and select callbacks", () => {
    const { props } = renderDropdown();

    fireEvent.click(screen.getByRole("button", { name: "AI" }));

    expect(screen.getByText("匹配的标签")).toBeInTheDocument();
    expect(props.onSelectTag).toHaveBeenCalledWith(tags[1]);
  });

  it("creates a tag when there are no matches", () => {
    const { props } = renderDropdown({
      filteredTags: [],
      inputValue: "新标签",
    });

    fireEvent.click(screen.getByRole("button", { name: /创建/ }));

    expect(props.onCreateTag).toHaveBeenCalledWith("新标签");
  });

  it("shows fallback states for all selected and no available tags", () => {
    const { rerender } = renderDropdown({
      filteredTags: [],
      inputValue: "",
      availableTags: tags,
    });

    expect(screen.getByText("所有标签都已选择")).toBeInTheDocument();

    rerender(
      <TagSuggestionsDropdown
        filteredTags={[]}
        inputValue=""
        availableTags={[]}
        loading={false}
        highlightedIndex={-1}
        disabled={false}
        onClose={vi.fn()}
        onSelectTag={vi.fn()}
        onCreateTag={vi.fn()}
        onHighlightTag={vi.fn()}
      />,
    );

    expect(
      screen.getByText("暂无可用标签，输入名称创建新标签"),
    ).toBeInTheDocument();
  });

  it("keeps highlighted tag styling and mouse highlight callback", () => {
    const { props } = renderDropdown({ highlightedIndex: 1 });
    const aiButton = screen.getByRole("button", { name: "AI" });

    expect(aiButton).toHaveClass("border-l-4");
    fireEvent.mouseEnter(aiButton);
    expect(props.onHighlightTag).toHaveBeenCalledWith(1);
  });
});
