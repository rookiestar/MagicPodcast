import { StrictMode } from "react";
import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { navigate } from "@/lib/navigation";
import { useUrlState } from "../useUrlState";

describe("useUrlState", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    window.history.replaceState({}, "", "/");
  });

  it("reads navigation updates and preserves framework state", () => {
    window.history.replaceState({ custom: "retained" }, "", "/podcasts?sort_by=title");
    const { result } = renderHook(() => useUrlState("sort_by", "recent_update", { replace: false }));
    expect(result.current[0]).toBe("title");
    act(() => { result.current[1]("episode_count"); });
    expect(window.history.state.custom).toBe("retained");
    act(() => { navigate("/podcasts?sort_by=title"); });
    expect(result.current[0]).toBe("title");
  });

  it("updates browser history once for a functional state update", () => {
    const replaceState = vi.spyOn(window.history, "replaceState");
    const { result } = renderHook(() => useUrlState("tag_id", [] as string[], {
      isArray: true,
    }), {
      wrapper: StrictMode,
    });

    act(() => {
      result.current[1]((previous) => [...previous, "1"]);
    });

    expect(result.current[0]).toEqual(["1"]);
    expect(replaceState).toHaveBeenCalledTimes(1);
  });
});
