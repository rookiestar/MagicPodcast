import { renderHook, waitFor } from "@testing-library/react";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
  getColumnsForViewportWidth,
  getPageSizeForViewportWidth,
  useBreakpoint,
} from "@/hooks/useBreakpoint";

function BreakpointProbe() {
  const { isReady } = useBreakpoint();
  return createElement("span", null, isReady ? "ready" : "pending");
}

describe("useBreakpoint", () => {
  it.each([
    [375, 1, 10],
    [640, 2, 8],
    [768, 3, 9],
    [1024, 4, 12],
    [1280, 5, 15],
  ])(
    "maps viewport %spx to %s columns and page size %s before effects",
    (width, columns, pageSize) => {
      expect(getColumnsForViewportWidth(width)).toBe(columns);
      expect(getPageSizeForViewportWidth(width)).toBe(pageSize);
    },
  );

  it("客户端挂载并读取视口后才标记页大小已确定", async () => {
    expect(renderToStaticMarkup(createElement(BreakpointProbe))).toContain(
      "pending",
    );

    Object.defineProperty(window, "innerWidth", {
      configurable: true,
      value: 375,
    });
    const { result } = renderHook(() => useBreakpoint());

    await waitFor(() => expect(result.current.isReady).toBe(true));
    expect(result.current.columns).toBe(1);
  });
});
