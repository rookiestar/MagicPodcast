import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import TagFormFooter from "../TagFormFooter";

function renderFooter(overrides = {}) {
  const props = {
    loading: false,
    submitDisabled: false,
    buttonText: "保存",
    onClose: vi.fn(),
    onSubmit: vi.fn(),
    ...overrides,
  };

  return {
    props,
    ...render(<TagFormFooter {...props} />),
  };
}

describe("TagFormFooter", () => {
  it("calls cancel and submit callbacks", () => {
    const { props } = renderFooter();

    fireEvent.click(screen.getByRole("button", { name: "取消" }));
    fireEvent.click(screen.getByRole("button", { name: "保存" }));

    expect(props.onClose).toHaveBeenCalledTimes(1);
    expect(props.onSubmit).toHaveBeenCalledTimes(1);
  });

  it("disables buttons from loading and submit state", () => {
    renderFooter({ loading: true, submitDisabled: true, buttonText: "保存中..." });

    expect(screen.getByRole("button", { name: "取消" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "保存中..." })).toBeDisabled();
    expect(screen.getByText("提示：按 Cmd+Enter 快速保存")).toBeInTheDocument();
  });
});
