import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import TagFormModal from "../TagFormModal";

describe("TagFormModal", () => {
  it("uses the shared editorial dialog shell", () => {
    const { container } = render(
      <TagFormModal
        isOpen
        mode="create"
        onClose={vi.fn()}
        onSubmit={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    expect(
      screen.getByRole("dialog", { name: "新建" }),
    ).toHaveAttribute("aria-modal", "true");
    expect(screen.getByText("标签管理")).toBeInTheDocument();
    expect(
      container.querySelector(".editorial-modal-heading small"),
    ).toHaveTextContent("新建");
    expect(screen.getByRole("button", { name: "关闭" })).toBeInTheDocument();
  });
});
