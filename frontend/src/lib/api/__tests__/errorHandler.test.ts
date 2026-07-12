import { describe, expect, it } from "vitest";
import { getApiErrorMessage } from "../errorHandler";

describe("getApiErrorMessage", () => {
  it("reads the stable nested API error shape", () => {
    expect(
      getApiErrorMessage({
        error: { code: "RATE_LIMITED", message: "请求过于频繁，请稍后再试" },
        message: "ignored",
      }),
    ).toBe("请求过于频繁，请稍后再试");
  });

  it("keeps compatibility with a flat error shape", () => {
    expect(getApiErrorMessage({ message: "请求参数错误" })).toBe("请求参数错误");
  });
});
