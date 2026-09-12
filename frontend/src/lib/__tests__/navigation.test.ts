import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  episodeIDFromHref,
  rememberEpisodeOrigin,
  attachEpisodeOrigin,
  closeTo,
  readEpisodeOrigin,
  episodeOriginIsPrevious,
  navigate,
  normalizeEpisodeQuery,
  parseEpisodeRoute,
  singleParam,
  updateQuery,
  useLocationHref,
  useUnsavedNavigation,
} from "../navigation";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  window.history.replaceState({}, "", "/");
});

describe("resource navigation", () => {
  it("attaches a same-tab list origin to the matching episode and keeps it across view changes", () => {
    window.history.replaceState({ custom: "preserved" }, "", "/search?q=Done&type=episodes");
    rememberEpisodeOrigin(2012, "episode-entry-search-2012");
    window.history.pushState({ custom: "preserved" }, "", "/episodes/2012?from=search");
    attachEpisodeOrigin(2012);
    const origin = readEpisodeOrigin(2012)!;
    expect(origin.href).toBe("/search?q=Done&type=episodes");
    expect(episodeOriginIsPrevious(origin)).toBe(true);
    updateQuery({ tab: "notes" });
    expect(readEpisodeOrigin(2012)).toEqual(origin);
    expect(readEpisodeOrigin(2013)).toBeUndefined();
    navigate("/episodes/2013?tab=transcript");
    expect(readEpisodeOrigin(2013)?.href).toBe(origin.href);
    expect(window.history.state.custom).toBe("preserved");
  });

  it("rejects malformed resource identities without guessing", () => {
    for (const href of [
      "/episodes/0",
      "/episodes/-1",
      "/episodes/1abc",
      "/episodes/1.5",
      "/episodes/9007199254740992",
    ])
      expect(episodeIDFromHref(href)).toBeNull();
    expect(episodeIDFromHref("/episodes/66413?tab=notes")).toBe(66413);
    expect(
      singleParam(new URLSearchParams("person=1&person=2"), "person"),
    ).toBeNull();
  });
  it("updates several view fields atomically and retains framework metadata", () => {
    window.history.replaceState(
      { __NA: true, custom: "preserve" },
      "",
      "/episodes/4",
    );
    const { result } = renderHook(useLocationHref);
    const push = vi.spyOn(window.history, "pushState");
    act(() => {
      updateQuery({ tab: "transcript", artifact: "transcript" });
    });
    expect(push).toHaveBeenCalledTimes(1);
    expect(result.current).toBe(
      "/episodes/4?tab=transcript&artifact=transcript",
    );
    expect(window.history.state.custom).toBe("preserve");
    // Next restores its own private flags through its patched history API.
    act(() => {
      updateQuery({ tab: "transcript", artifact: "transcript" });
    });
    expect(push).toHaveBeenCalledTimes(1);
  });
  it("normalizes invalid single view values without creating history", () => {
    window.history.replaceState(
      {},
      "",
      "/episodes/4?tab=notes&tab=transcript&artifact=bad&unrelated=ok",
    );
    const push = vi.spyOn(window.history, "pushState");
    normalizeEpisodeQuery();
    expect(window.location.search).toBe("?unrelated=ok");
    expect(
      parseEpisodeRoute(
        window.location.href.replace(window.location.origin, ""),
      ).tab,
    ).toBe("show-notes");
    expect(push).not.toHaveBeenCalled();
  });
  it("guards back navigation from a directly loaded dirty page", () => {
    window.history.replaceState({}, "", "/episodes/4?tab=notes");
    vi.stubGlobal("confirm", vi.fn(() => false));
    renderHook(() => useUnsavedNavigation(true, (href) => episodeIDFromHref(href) === 4));
    act(() => {
      window.history.replaceState({}, "", "/inbox");
      window.dispatchEvent(new PopStateEvent("popstate", { state: {} }));
    });
    expect(window.location.pathname).toBe("/episodes/4");
    expect(window.location.search).toBe("?tab=notes");
  });

  it("preserves an unknown browser destination when a dirty page rejects Back", async () => {
    window.history.replaceState({}, "", "/discovery");
    window.history.pushState({}, "", "/episodes/4?tab=notes");
    const confirm = vi.fn(() => false);
    vi.stubGlobal("confirm", confirm);
    renderHook(() =>
      useUnsavedNavigation(true, (href) => episodeIDFromHref(href) === 4),
    );

    act(() => {
      window.history.back();
    });
    await waitFor(() => expect(window.location.pathname).toBe("/episodes/4"));

    confirm.mockReturnValue(true);
    act(() => {
      window.history.back();
    });
    await waitFor(() => expect(window.location.pathname).toBe("/discovery"));
  });

  it("closes through replaced search refinements without reopening them", () => {
    window.history.replaceState({}, "", "/discovery");
    navigate("/search");
    updateQuery({ q: "first" }, true);
    updateQuery({ type: "episodes" }, true);
    const back = vi.spyOn(window.history, "back");

    expect(closeTo("/discovery")).toBe(true);
    expect(back).toHaveBeenCalledTimes(1);
  });

  it("does not change URL when leaving an unsaved editor is rejected", () => {
    window.history.replaceState({}, "", "/episodes/4?tab=notes");
    renderHook(() =>
      useUnsavedNavigation(true, (href) => episodeIDFromHref(href) === 4),
    );
    const confirm = vi.fn(() => false);
    vi.stubGlobal("confirm", confirm);
    expect(navigate("/episodes/5")).toBe(false);
    expect(window.location.pathname).toBe("/episodes/4");
    expect(navigate("/episodes/4?tab=transcript")).toBe(true);
    expect(confirm).toHaveBeenCalledTimes(1);
  });
});
