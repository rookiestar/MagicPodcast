import { describe, expect, it, vi } from "vitest";
import { createEpisodeShowNotesStore } from "../episodeShowNotesStore";

function payload(episodeId: number, content = `Episode ${episodeId}`) {
  return {
    episode_id: episodeId,
    show_notes_document: { content, format: "markdown" as const },
  };
}

describe("createEpisodeShowNotesStore", () => {
  it("deduplicates concurrent reads and reuses a successful document", async () => {
    let resolveRequest!: (value: ReturnType<typeof payload>) => void;
    const loader = vi.fn(
      () => new Promise<ReturnType<typeof payload>>((resolve) => {
        resolveRequest = resolve;
      }),
    );
    const store = createEpisodeShowNotesStore(loader);

    const first = store.load(7);
    const second = store.load(7);
    expect(second).toBe(first);
    expect(loader).toHaveBeenCalledTimes(1);

    resolveRequest(payload(7));
    await expect(first).resolves.toEqual(payload(7).show_notes_document);
    await expect(store.load(7)).resolves.toEqual(payload(7).show_notes_document);
    expect(loader).toHaveBeenCalledTimes(1);
  });

  it("does not cache failures and permits an explicit retry", async () => {
    const loader = vi
      .fn()
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce(payload(8, "retry success"));
    const store = createEpisodeShowNotesStore(loader);

    await expect(store.load(8)).rejects.toThrow("offline");
    expect(store.get(8)).toBeUndefined();
    await expect(store.load(8)).resolves.toEqual({
      content: "retry success",
      format: "markdown",
    });
    expect(loader).toHaveBeenCalledTimes(2);
  });

  it("rejects a response for a different episode", async () => {
    const store = createEpisodeShowNotesStore(
      vi.fn().mockResolvedValue(payload(99)),
    );

    await expect(store.load(9)).rejects.toThrow("identity mismatch");
    expect(store.get(9)).toBeUndefined();
    expect(store.get(99)).toBeUndefined();
  });

  it("bounds the page-session cache with least-recently-used eviction", async () => {
    const loader = vi.fn((episodeId: number) =>
      Promise.resolve(payload(episodeId)),
    );
    const store = createEpisodeShowNotesStore(loader, 2);

    await store.load(1);
    await store.load(2);
    expect(store.get(1)).toBeDefined();
    await store.load(3);

    expect(store.get(2)).toBeUndefined();
    expect(store.get(1)).toBeDefined();
    expect(store.get(3)).toBeDefined();
  });
});
