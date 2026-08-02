import { StrictMode } from "react";
import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useUrlState } from "../useUrlState";

describe("useUrlState", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    window.history.replaceState({}, "", "/");
  });

  it("updates browser history once for a functional state update", () => {
    const replaceState = vi.spyOn(window.history, "replaceState");
    const { result } = renderHook(() => useUrlState("tag_id", [] as number[], {
      isArray: true,
    }), {
      wrapper: StrictMode,
    });

    act(() => {
      result.current[1]((previous) => [...previous, 1]);
    });

    expect(result.current[0]).toEqual([1]);
    expect(replaceState).toHaveBeenCalledTimes(1);
  });
});
