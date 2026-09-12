import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";

const { previewMock, applyMock } = vi.hoisted(() => ({
  previewMock: vi.fn(),
  applyMock: vi.fn(),
}));
vi.mock("@/lib/collections", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/collections")>()),
  refreshCollectionPreview: previewMock,
  applyCollectionRefresh: applyMock,
}));
import RefreshCollectionModal from "../RefreshCollectionModal";

const preview = {
  preview_id: "token",
  collection_id: 1,
  base_revision: 1,
  read_count: 0,
  changes: {
    added_count: 0,
    removed_count: 0,
    reordered_count: 0,
    recommendation_changed_count: 0,
    unchanged_count: 0,
  },
  removed: [],
};
beforeEach(() => vi.resetAllMocks());

it("aborts a pending read when dismissed", async () => {
  previewMock.mockImplementation(() => new Promise(() => {}));
  const onClose = vi.fn();
  render(
    <RefreshCollectionModal
      isOpen
      collectionID={1}
      onClose={onClose}
      onApplied={vi.fn()}
    />,
  );
  await userEvent.click(screen.getByRole("button", { name: "关闭" }));
  expect(onClose).toHaveBeenCalledOnce();
  expect(previewMock.mock.calls[0][1].aborted).toBe(true);
  expect(applyMock).not.toHaveBeenCalled();
});

it("allows a new preview after an actual Axios expiry response", async () => {
  previewMock.mockResolvedValue(preview);
  applyMock.mockRejectedValue({
    code: "ERR_BAD_REQUEST",
    response: {
      data: { error: { code: "PREVIEW_EXPIRED", message: "expired" } },
    },
  });
  render(
    <RefreshCollectionModal
      isOpen
      collectionID={1}
      onClose={vi.fn()}
      onApplied={vi.fn()}
    />,
  );
  expect(
    await screen.findByText("没有变化，确认后记录本次检查时间。"),
  ).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "应用刷新" }));
  expect(
    await screen.findByText("刷新预览已过期，请重新刷新。"),
  ).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "重新刷新" }));
  await waitFor(() => expect(previewMock).toHaveBeenCalledTimes(2));
  expect(
    await screen.findByText("没有变化，确认后记录本次检查时间。"),
  ).toBeInTheDocument();
});
