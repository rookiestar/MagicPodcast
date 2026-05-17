import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import SortDrawer from "../SortDrawer";

const options = [
  { label: "最近更新", value: "recent_update" },
  { label: "名称", value: "title" },
];

describe("SortDrawer", () => {
  afterEach(() => {
    document.body.style.overflow = "";
  });

  it("does not render closed drawer content", () => {
    render(
      <SortDrawer
        isOpen={false}
        onClose={vi.fn()}
        currentSort="recent_update"
        onSortChange={vi.fn()}
        options={options}
      />,
    );

    expect(screen.queryByText("选择排序方式")).not.toBeInTheDocument();
  });

  it("locks page scroll while open", () => {
    const { unmount } = render(
      <SortDrawer
        isOpen
        onClose={vi.fn()}
        currentSort="recent_update"
        onSortChange={vi.fn()}
        options={options}
      />,
    );

    expect(document.body.style.overflow).toBe("hidden");
    unmount();
    expect(document.body.style.overflow).toBe("");
  });

  it("selects a sort option and closes the drawer", () => {
    const onSortChange = vi.fn();
    const onClose = vi.fn();

    render(
      <SortDrawer
        isOpen
        onClose={onClose}
        currentSort="recent_update"
        onSortChange={onSortChange}
        options={options}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "名称" }));

    expect(onSortChange).toHaveBeenCalledWith("title");
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
