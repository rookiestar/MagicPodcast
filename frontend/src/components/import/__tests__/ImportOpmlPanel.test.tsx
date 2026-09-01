import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import ImportOpmlPanel from "../ImportOpmlPanel";

function renderPanel(disabled: boolean) {
  return render(
    <ImportOpmlPanel
      file={null}
      disabled={disabled}
      importing={false}
      onFileChange={vi.fn()}
      onImport={vi.fn()}
    />,
  );
}

describe("ImportOpmlPanel", () => {
  it("marks the file picker as unavailable while an operation is running", () => {
    renderPanel(true);

    const picker = screen.getByText("选择 OPML 文件").closest("label");
    expect(picker).toHaveAttribute("aria-disabled", "true");
    expect(picker).toHaveClass("is-disabled");
    expect(screen.getByLabelText("选择 OPML 文件")).toBeDisabled();
  });

  it("keeps the file picker interactive when no operation is running", () => {
    renderPanel(false);

    const picker = screen.getByText("选择 OPML 文件").closest("label");
    expect(picker).not.toHaveAttribute("aria-disabled", "true");
    expect(picker).not.toHaveClass("is-disabled");
    expect(screen.getByLabelText("选择 OPML 文件")).not.toBeDisabled();
  });
});
