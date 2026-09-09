import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { createEpisodeShowNotesStore } from "@/lib/episodeShowNotesStore";
import { useEpisodeShowNotes } from "../useEpisodeShowNotes";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((nextResolve, nextReject) => {
    resolve = nextResolve;
    reject = nextReject;
  });
  return { promise, resolve, reject };
}

describe("useEpisodeShowNotes", () => {
  it("does not load until the user explicitly expands", async () => {
    const loader = vi.fn().mockResolvedValue({
      episode_id: 1,
      show_notes_document: { content: "全文", format: "markdown" },
    });
    const store = createEpisodeShowNotesStore(loader);
    const { result } = renderHook(() => useEpisodeShowNotes(1, true, store));

    expect(result.current.isExpanded).toBe(false);
    expect(loader).not.toHaveBeenCalled();

    await act(async () => {
      result.current.toggle();
    });

    await waitFor(() => expect(result.current.status).toBe("success"));
    expect(result.current.isExpanded).toBe(true);
    expect(loader).toHaveBeenCalledTimes(1);
    expect(loader).toHaveBeenCalledWith(1);
  });

  it("reuses a successful document and does not reopen after collapse", async () => {
    const pending = deferred<{
      episode_id: number;
      show_notes_document: { content: string; format: "markdown" };
    }>();
    const loader = vi.fn(() => pending.promise);
    const store = createEpisodeShowNotesStore(loader);
    const { result } = renderHook(() => useEpisodeShowNotes(4, true, store));

    act(() => {
      result.current.toggle();
    });
    await waitFor(() => expect(loader).toHaveBeenCalledTimes(1));
    act(() => {
      result.current.toggle();
    });
    expect(result.current.isExpanded).toBe(false);

    await act(async () => {
      pending.resolve({
        episode_id: 4,
        show_notes_document: { content: "迟到全文", format: "markdown" },
      });
    });

    expect(result.current.isExpanded).toBe(false);
    expect(result.current.document?.content).toBe("迟到全文");

    act(() => {
      result.current.toggle();
    });
    expect(result.current.isExpanded).toBe(true);
    expect(result.current.status).toBe("success");
    expect(loader).toHaveBeenCalledTimes(1);
  });

  it("does not mix a late response into another episode", async () => {
    const pending = new Map<
      number,
      ReturnType<
        typeof deferred<{
          episode_id: number;
          show_notes_document: { content: string; format: "markdown" };
        }>
      >
    >();
    const loader = vi.fn((episodeId: number) => {
      const next = deferred<{
        episode_id: number;
        show_notes_document: { content: string; format: "markdown" };
      }>();
      pending.set(episodeId, next);
      return next.promise;
    });
    const store = createEpisodeShowNotesStore(loader);
    const { result, rerender } = renderHook(
      ({ episodeId }) => useEpisodeShowNotes(episodeId, true, store),
      { initialProps: { episodeId: 1 } },
    );

    act(() => {
      result.current.toggle();
    });
    await waitFor(() => expect(pending.has(1)).toBe(true));
    rerender({ episodeId: 2 });
    expect(result.current.isExpanded).toBe(false);
    act(() => {
      result.current.toggle();
    });
    await waitFor(() => expect(pending.has(2)).toBe(true));

    await act(async () => {
      pending.get(2)?.resolve({
        episode_id: 2,
        show_notes_document: { content: "B", format: "markdown" },
      });
    });
    await waitFor(() => expect(result.current.document?.content).toBe("B"));

    await act(async () => {
      pending.get(1)?.resolve({
        episode_id: 1,
        show_notes_document: { content: "A", format: "markdown" },
      });
    });
    expect(result.current.document?.content).toBe("B");
  });
});
