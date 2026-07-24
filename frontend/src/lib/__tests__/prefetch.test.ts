import { beforeEach, describe, expect, it, vi } from "vitest";
import { mutate } from "swr";
import { apiClient } from "../fetcher";
import {
  prefetchPodcastData,
  prefetchWorkflowData,
  prefetchWorkflowJobsSummary,
} from "../prefetch";

vi.mock("swr", () => ({
  mutate: vi.fn(),
}));

vi.mock("../fetcher", () => ({
  apiClient: {
    get: vi.fn(),
  },
}));

const get = vi.mocked(apiClient.get);
const swrMutate = vi.mocked(mutate);

function success(data: unknown) {
  return {
    data: {
      success: true,
      data,
    },
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((innerResolve) => {
    resolve = innerResolve;
  });

  return { promise, resolve };
}

describe("prefetch", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("prefetches podcast detail data that is actually read from cache", async () => {
    get
      .mockResolvedValueOnce(success({ id: 1, title: "Podcast" }))
      .mockResolvedValueOnce(success({ tags: [] }))
      .mockResolvedValueOnce(success({ id: 1, notes: "note" }));

    await prefetchPodcastData(1);

    expect(get).toHaveBeenCalledTimes(3);
    expect(get).toHaveBeenNthCalledWith(1, "/api/v1/podcasts/1");
    expect(get).toHaveBeenNthCalledWith(2, "/api/v1/podcasts/1/tags");
    expect(get).toHaveBeenNthCalledWith(3, "/api/v1/podcasts/1/notes");
    expect(swrMutate).toHaveBeenCalledWith(
      "/api/v1/podcasts/1",
      { id: 1, title: "Podcast" },
      false,
    );
    expect(swrMutate).toHaveBeenCalledWith(
      "/api/v1/podcasts/1/tags",
      { tags: [] },
      false,
    );
    expect(swrMutate).toHaveBeenCalledWith(
      "/api/v1/podcasts/1/notes",
      { id: 1, notes: "note" },
      false,
    );
  });

  it("deduplicates concurrent podcast prefetches for the same podcast", async () => {
    const podcast = deferred<ReturnType<typeof success>>();
    const tags = deferred<ReturnType<typeof success>>();
    const notes = deferred<ReturnType<typeof success>>();
    get
      .mockReturnValueOnce(podcast.promise)
      .mockReturnValueOnce(tags.promise)
      .mockReturnValueOnce(notes.promise);

    const firstPrefetch = prefetchPodcastData(1);
    const secondPrefetch = prefetchPodcastData(1);

    expect(get).toHaveBeenCalledTimes(3);

    podcast.resolve(success({ id: 1, title: "Podcast" }));
    tags.resolve(success({ tags: [] }));
    notes.resolve(success({ id: 1, notes: "note" }));

    await Promise.all([firstPrefetch, secondPrefetch]);

    expect(get).toHaveBeenCalledTimes(3);
    expect(swrMutate).toHaveBeenCalledTimes(3);
  });

  it("allows a later podcast prefetch after the previous one settles", async () => {
    get
      .mockResolvedValueOnce(success({ id: 1, title: "Podcast" }))
      .mockResolvedValueOnce(success({ tags: [] }))
      .mockResolvedValueOnce(success({ id: 1, notes: "note" }))
      .mockResolvedValueOnce(success({ id: 1, title: "Podcast" }))
      .mockResolvedValueOnce(success({ tags: [] }))
      .mockResolvedValueOnce(success({ id: 1, notes: "note" }));

    await prefetchPodcastData(1);
    await prefetchPodcastData(1);

    expect(get).toHaveBeenCalledTimes(6);
  });

  it("keeps workflow prefetch behavior unchanged", async () => {
    get
      .mockResolvedValueOnce(success({ id: 7, name: "Workflow" }))
      .mockResolvedValueOnce(success({ jobs: [] }));

    await prefetchWorkflowData(7);

    expect(get).toHaveBeenCalledTimes(2);
    expect(get).toHaveBeenNthCalledWith(1, "/api/v1/workflows/7");
    expect(get).toHaveBeenNthCalledWith(
      2,
      "/api/v1/workflows/7/jobs?page=1&page_size=10&view=summary",
    );
    expect(swrMutate).toHaveBeenCalledWith(
      "/api/v1/workflows/7",
      { id: 7, name: "Workflow" },
      false,
    );
    expect(swrMutate).toHaveBeenCalledWith(
      "/api/v1/workflows/7/jobs?page=1&page_size=10&view=summary",
      { jobs: [] },
      false,
    );
  });

  it("prefetches only first-page jobs summary for history tab intent", async () => {
    get.mockResolvedValueOnce(success({ jobs: [{ id: 1 }], pagination: {} }));

    await prefetchWorkflowJobsSummary(9);

    expect(get).toHaveBeenCalledTimes(1);
    expect(get).toHaveBeenCalledWith(
      "/api/v1/workflows/9/jobs?page=1&page_size=10&view=summary",
    );
    expect(swrMutate).toHaveBeenCalledWith(
      "/api/v1/workflows/9/jobs?page=1&page_size=10&view=summary",
      { jobs: [{ id: 1 }], pagination: {} },
      false,
    );
  });
});
