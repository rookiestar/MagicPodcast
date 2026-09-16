import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ImportTask } from "@/lib/api/importTasks";
import ImportRetryConfirmationDialog from "../ImportRetryConfirmationDialog";

const task: ImportTask = {
  id: 1,
  status: "interrupted",
  file_name: "cosmos.opml",
  total: 10,
  processed: 4,
  success_count: 4,
  pending_count: 0,
  conflict_count: 0,
  merged_count: 0,
  unchanged_count: 0,
  skipped_count: 0,
  failed_count: 0,
  error_message: "",
  started_at: "2026-09-16T00:00:00Z",
};

describe("ImportRetryConfirmationDialog", () => {
  it("requires the confirmation text before submitting", () => {
    const onConfirm = vi.fn();
    render(
      <ImportRetryConfirmationDialog
        task={task}
        retryableCount={6}
        disabled={false}
        confirmationText="RETRY IMPORT"
        onCancel={vi.fn()}
        onConfirm={onConfirm}
      />,
    );

    const submit = screen.getByRole("button", { name: "确认重试" });
    expect(submit).toBeDisabled();
    fireEvent.change(screen.getByLabelText(/输入确认文字/), { target: { value: "WRONG" } });
    expect(submit).toBeDisabled();
    fireEvent.change(screen.getByLabelText(/输入确认文字/), { target: { value: "RETRY IMPORT" } });
    expect(submit).toBeEnabled();
    fireEvent.click(submit);
    expect(onConfirm).toHaveBeenCalledWith("RETRY IMPORT");
  });

  it("cancels without submitting", () => {
    const onCancel = vi.fn();
    render(
      <ImportRetryConfirmationDialog
        task={task}
        retryableCount={6}
        disabled={false}
        confirmationText="RETRY IMPORT"
        onCancel={onCancel}
        onConfirm={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "取消" }));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });
});
