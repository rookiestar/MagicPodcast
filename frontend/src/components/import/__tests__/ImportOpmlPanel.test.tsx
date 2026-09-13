import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import ImportOpmlPanel from "../ImportOpmlPanel";
import SyncLogStats from "../SyncLogStats";
import { computeSyncStats } from "@/lib/syncLogState";

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

it("shows pending imports separately from failures and skipped subscriptions", () => {
  const stats = computeSyncStats([{id: "summary", type: "summary", message: "导入完成", timestamp: "12:00:00", data: {operation: "import", total_podcasts: 2, success_podcasts: 0, failed_podcasts: 0, skipped_podcasts: 0, stub_podcasts: 2}}]);
  render(<SyncLogStats stats={stats} />);
  expect(screen.getByText("待同步").parentElement).toHaveTextContent("2");
  expect(screen.getByText("跳过").parentElement).toHaveTextContent("0");
  expect(screen.getByText("失败").parentElement).toHaveTextContent("0");
});
