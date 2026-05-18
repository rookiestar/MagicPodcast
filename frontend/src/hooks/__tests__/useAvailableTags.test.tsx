import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Tag } from "@/types";
import { availableTagCache } from "@/lib/tagAvailabilityCache";
import { useAvailableTags } from "../useAvailableTags";

const tags: Tag[] = [
  { id: 1, name: "科技", color: "#2563eb" },
  { id: 2, name: "AI", color: "#16a34a" },
];

describe("useAvailableTags", () => {
  afterEach(() => {
    availableTagCache.clear();
    vi.restoreAllMocks();
  });

  it("loads available tags from the shared cache", async () => {
    availableTagCache.replace(tags);
    const { result } = renderHook(() => useAvailableTags());

    await act(async () => {
      await result.current.ensureAvailableTags();
    });

    expect(result.current.loading).toBe(false);
    expect(result.current.availableTags).toBe(tags);
  });

  it("adds a newly created tag to local state and shared cache", async () => {
    availableTagCache.replace(tags);
    const { result } = renderHook(() => useAvailableTags());
    const newTag = { id: 3, name: "生活", color: "#f97316" };

    await act(async () => {
      await result.current.ensureAvailableTags();
    });

    act(() => {
      result.current.appendAvailableTag(newTag);
    });

    expect(result.current.availableTags).toEqual([...tags, newTag]);

    const nextHook = renderHook(() => useAvailableTags());
    await act(async () => {
      await nextHook.result.current.ensureAvailableTags();
    });

    expect(nextHook.result.current.availableTags).toEqual([...tags, newTag]);
  });

  it("clears loading state when loading tags fails", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    vi.spyOn(availableTagCache, "load").mockRejectedValueOnce(
      new Error("failed"),
    );
    const { result } = renderHook(() => useAvailableTags());

    await act(async () => {
      await result.current.ensureAvailableTags();
    });

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.availableTags).toEqual([]);
    expect(errorSpy).toHaveBeenCalled();
  });
});
