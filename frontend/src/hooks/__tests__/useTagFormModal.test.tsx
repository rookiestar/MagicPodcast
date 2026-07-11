import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { DEFAULT_TAG_COLOR } from "@/lib/tagFormState";
import { useTagFormModal } from "../useTagFormModal";

function renderTagFormModal(overrides = {}) {
  return renderHook((props) => useTagFormModal(props), {
    initialProps: {
      isOpen: true,
      initialData: undefined,
      onClose: vi.fn(),
      onSubmit: vi.fn().mockResolvedValue(undefined),
      ...overrides,
    },
  });
}

describe("useTagFormModal", () => {
  it("initializes values when opened", () => {
    const { result } = renderTagFormModal({
      initialData: { name: "科技", color: "#2563eb" },
    });

    expect(result.current.name).toBe("科技");
    expect(result.current.color).toBe("#2563eb");
    expect(result.current.error).toBe("");
  });

  it("validates before submit", async () => {
    const onSubmit = vi.fn();
    const { result } = renderTagFormModal({ onSubmit });

    await act(async () => {
      await result.current.submit();
    });

    expect(result.current.error).toBe("标签名称不能为空");
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("submits normalized values and closes on success", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    const onClose = vi.fn();
    const { result } = renderTagFormModal({ onSubmit, onClose });

    await act(async () => {
      result.current.setName(" 科技 ");
      result.current.setColor("#2563eb");
    });

    await act(async () => {
      await result.current.submit();
    });

    expect(onSubmit).toHaveBeenCalledWith({
      name: "科技",
      color: "#2563eb",
    });
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(result.current.name).toBe("");
    expect(result.current.color).toBe(DEFAULT_TAG_COLOR);
  });

  it("keeps the modal open and shows submit errors", async () => {
    const onSubmit = vi.fn().mockRejectedValue(new Error("保存失败"));
    const onClose = vi.fn();
    const { result } = renderTagFormModal({ onSubmit, onClose });

    await act(async () => {
      result.current.setName("科技");
    });

    await act(async () => {
      await result.current.submit();
    });

    expect(result.current.error).toBe("保存失败");
    expect(onClose).not.toHaveBeenCalled();
  });

  it("submits from Cmd+Enter", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    const { result } = renderTagFormModal({ onSubmit });

    await act(async () => {
      result.current.setName("科技");
    });

    await act(async () => {
      result.current.handleKeyboardSubmit("Enter", true);
    });

    expect(onSubmit).toHaveBeenCalledWith({
      name: "科技",
      color: DEFAULT_TAG_COLOR,
    });
  });
});
