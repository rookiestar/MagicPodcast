import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { requestTypedConfirmation } from "../confirmation";

describe("requestTypedConfirmation", () => {
  beforeEach(() => {
    Object.defineProperty(window, "confirm", {
      configurable: true,
      writable: true,
      value: vi.fn(),
    });
    Object.defineProperty(window, "prompt", {
      configurable: true,
      writable: true,
      value: vi.fn(),
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns the exact phrase only after impact acknowledgement", () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    vi.spyOn(window, "prompt").mockReturnValue("CLEAR CACHE");

    expect(
      requestTypedConfirmation({
        action: "清空缓存",
        impact: "后续请求会重新加载数据",
        phrase: "CLEAR CACHE",
      }),
    ).toBe("CLEAR CACHE");
    expect(window.confirm).toHaveBeenCalledWith(
      expect.stringContaining("后续请求会重新加载数据"),
    );
  });

  it("returns null and never prompts for text when cancelled", () => {
    vi.spyOn(window, "confirm").mockReturnValue(false);
    const prompt = vi.spyOn(window, "prompt");

    expect(
      requestTypedConfirmation({
        action: "删除工作流",
        impact: "不可恢复",
        phrase: "DELETE WORKFLOW 1",
      }),
    ).toBeNull();
    expect(prompt).not.toHaveBeenCalled();
  });

  it("rejects a mismatched phrase", () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    vi.spyOn(window, "prompt").mockReturnValue("DELETE WORKFLOW 2");

    expect(
      requestTypedConfirmation({
        action: "删除工作流",
        impact: "不可恢复",
        phrase: "DELETE WORKFLOW 1",
      }),
    ).toBeNull();
  });
});
