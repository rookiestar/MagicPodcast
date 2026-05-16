import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { podcastApi } from "@/lib/api";
import { toast } from "@/lib/toast";
import type { Tag } from "@/types";
import {
  getTagChanges,
  usePodcastMetadataEditing,
} from "../usePodcastMetadataEditing";

vi.mock("@/lib/api", () => ({
  podcastApi: {
    addTag: vi.fn(),
    removeTag: vi.fn(),
    updateNotes: vi.fn(),
  },
}));

vi.mock("@/lib/toast", () => ({
  toast: {
    error: vi.fn(),
  },
}));

const addTag = vi.mocked(podcastApi.addTag);
const removeTag = vi.mocked(podcastApi.removeTag);
const updateNotes = vi.mocked(podcastApi.updateNotes);
const showError = vi.mocked(toast.error);

const techTag: Tag = { id: 1, name: "科技", color: "#2563eb" };
const businessTag: Tag = { id: 2, name: "商业", color: "#16a34a" };
const aiTag: Tag = { id: 3, name: "AI", color: "#7c3aed" };

function deferred<T = void>() {
  let resolve: (value: T | PromiseLike<T>) => void = () => {};
  const promise = new Promise<T>((promiseResolve) => {
    resolve = promiseResolve;
  });

  return { promise, resolve };
}

describe("getTagChanges", () => {
  it("detects tags to add and remove", () => {
    const result = getTagChanges([techTag, businessTag], [businessTag, aiTag]);

    expect(result.toAdd).toEqual([aiTag]);
    expect(result.toRemove).toEqual([techTag]);
  });
});

describe("usePodcastMetadataEditing", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  function renderEditingHook(options?: { tags?: Tag[]; swrNotes?: string }) {
    return {
      mutateTags: vi.fn(),
      mutateNotes: vi.fn(),
      ...renderHook(() =>
        usePodcastMetadataEditing({
          podcastId: 1,
          tags: options?.tags ?? [techTag],
          swrNotes: options?.swrNotes ?? "旧备注",
          mutateTags: vi.fn(),
          mutateNotes: vi.fn(),
        }),
      ),
    };
  }

  it("syncs notes from cached podcast notes", async () => {
    const { result, rerender } = renderHook(
      ({ swrNotes }) =>
        usePodcastMetadataEditing({
          podcastId: 1,
          tags: [techTag],
          swrNotes,
          mutateTags: vi.fn(),
          mutateNotes: vi.fn(),
        }),
      { initialProps: { swrNotes: "旧备注" } },
    );

    expect(result.current.notes).toBe("旧备注");

    rerender({ swrNotes: "新备注" });

    await waitFor(() => expect(result.current.notes).toBe("新备注"));
  });

  it("updates added and removed tags", async () => {
    const mutateTags = vi.fn();
    const { result } = renderHook(() =>
      usePodcastMetadataEditing({
        podcastId: 1,
        tags: [techTag],
        swrNotes: "",
        mutateTags,
        mutateNotes: vi.fn(),
      }),
    );

    await act(async () => {
      await result.current.handleTagsChange([businessTag]);
    });

    expect(mutateTags).toHaveBeenNthCalledWith(
      1,
      { tags: [businessTag] },
      false,
    );
    expect(addTag).toHaveBeenCalledWith(1, businessTag.id);
    expect(removeTag).toHaveBeenCalledWith(1, techTag.id);
    expect(mutateTags).toHaveBeenLastCalledWith();
  });

  it("rolls back tag cache when updating tags fails", async () => {
    const mutateTags = vi.fn();
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    addTag.mockRejectedValueOnce(new Error("failed"));

    const { result } = renderHook(() =>
      usePodcastMetadataEditing({
        podcastId: 1,
        tags: [techTag],
        swrNotes: "",
        mutateTags,
        mutateNotes: vi.fn(),
      }),
    );

    await act(async () => {
      await result.current.handleTagsChange([businessTag]);
    });

    expect(showError).toHaveBeenCalledWith("标签更新失败: failed");
    expect(mutateTags).toHaveBeenLastCalledWith();
    consoleSpy.mockRestore();
  });

  it("saves notes and leaves editing mode", async () => {
    const mutateNotes = vi.fn();
    const { result } = renderHook(() =>
      usePodcastMetadataEditing({
        podcastId: 1,
        tags: [techTag],
        swrNotes: "旧备注",
        mutateTags: vi.fn(),
        mutateNotes,
      }),
    );

    act(() => {
      result.current.setIsEditingNotes(true);
      result.current.setNotes("新备注");
    });

    await act(async () => {
      await result.current.handleNotesSave();
    });

    expect(mutateNotes).toHaveBeenNthCalledWith(
      1,
      { id: 1, notes: "新备注" },
      false,
    );
    expect(updateNotes).toHaveBeenCalledWith(1, "新备注");
    expect(result.current.isEditingNotes).toBe(false);
  });

  it("prevents duplicate note saves while one save is running", async () => {
    const saveRun = deferred();
    updateNotes.mockReturnValue(saveRun.promise);

    const { result } = renderHook(() =>
      usePodcastMetadataEditing({
        podcastId: 1,
        tags: [techTag],
        swrNotes: "旧备注",
        mutateTags: vi.fn(),
        mutateNotes: vi.fn(),
      }),
    );

    act(() => {
      result.current.setIsEditingNotes(true);
      result.current.setNotes("新备注");
    });

    act(() => {
      void result.current.handleNotesSave();
      void result.current.handleNotesSave();
    });

    expect(updateNotes).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(result.current.isSavingNotes).toBe(true));

    await act(async () => {
      saveRun.resolve();
      await saveRun.promise;
    });

    await waitFor(() => expect(result.current.isSavingNotes).toBe(false));
  });

  it("prevents duplicate tag updates while one update is running", async () => {
    const tagRun = deferred();
    addTag.mockReturnValue(tagRun.promise);

    const { result } = renderHook(() =>
      usePodcastMetadataEditing({
        podcastId: 1,
        tags: [techTag],
        swrNotes: "",
        mutateTags: vi.fn(),
        mutateNotes: vi.fn(),
      }),
    );

    act(() => {
      void result.current.handleTagsChange([techTag, businessTag]);
      void result.current.handleTagsChange([techTag, aiTag]);
    });

    expect(addTag).toHaveBeenCalledTimes(1);
    expect(addTag).toHaveBeenCalledWith(1, businessTag.id);
    await waitFor(() => expect(result.current.isUpdatingTags).toBe(true));

    await act(async () => {
      tagRun.resolve();
      await tagRun.promise;
    });

    await waitFor(() => expect(result.current.isUpdatingTags).toBe(false));
  });

  it("restores notes when editing is cancelled", () => {
    const { result } = renderEditingHook();

    act(() => {
      result.current.setIsEditingNotes(true);
      result.current.setNotes("临时内容");
      result.current.cancelNotesEdit();
    });

    expect(result.current.notes).toBe("旧备注");
    expect(result.current.isEditingNotes).toBe(false);
  });
});
