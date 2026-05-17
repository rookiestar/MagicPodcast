import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import PrefetchLink from "../PrefetchLink";
import { prefetchPodcastData, prefetchWorkflowData } from "@/lib/prefetch";

vi.mock("@/lib/prefetch", () => ({
  prefetchPodcastData: vi.fn(),
  prefetchWorkflowData: vi.fn(),
}));

const prefetchPodcast = vi.mocked(prefetchPodcastData);
const prefetchWorkflow = vi.mocked(prefetchWorkflowData);

describe("PrefetchLink", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("prefetches podcast data after the hover delay", async () => {
    render(
      <PrefetchLink href="/podcasts/1" prefetchId={1}>
        Podcast
      </PrefetchLink>,
    );

    fireEvent.mouseEnter(screen.getByRole("link", { name: "Podcast" }));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(99);
    });
    expect(prefetchPodcast).not.toHaveBeenCalled();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(prefetchPodcast).toHaveBeenCalledWith(1);
  });

  it("cancels the scheduled prefetch when the pointer leaves", async () => {
    render(
      <PrefetchLink href="/podcasts/1" prefetchId={1}>
        Podcast
      </PrefetchLink>,
    );

    const link = screen.getByRole("link", { name: "Podcast" });
    fireEvent.mouseEnter(link);
    fireEvent.mouseLeave(link);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(100);
    });

    expect(prefetchPodcast).not.toHaveBeenCalled();
  });

  it("clears the scheduled prefetch when unmounted", async () => {
    const { unmount } = render(
      <PrefetchLink href="/podcasts/1" prefetchId={1}>
        Podcast
      </PrefetchLink>,
    );

    fireEvent.mouseEnter(screen.getByRole("link", { name: "Podcast" }));
    unmount();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(100);
    });

    expect(prefetchPodcast).not.toHaveBeenCalled();
  });

  it("prefetches workflow data for workflow links", async () => {
    render(
      <PrefetchLink href="/workflows/1" prefetchId={7} prefetchType="workflow">
        Workflow
      </PrefetchLink>,
    );

    fireEvent.mouseEnter(screen.getByRole("link", { name: "Workflow" }));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(100);
    });

    expect(prefetchWorkflow).toHaveBeenCalledWith(7);
    expect(prefetchPodcast).not.toHaveBeenCalled();
  });
});
