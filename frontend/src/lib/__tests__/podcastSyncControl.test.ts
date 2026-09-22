import { describe, expect, it, vi } from "vitest";
import {
  getPodcastSyncControl,
  shouldPollPodcastSync,
} from "../podcastSyncControl";
import type { PodcastHistorySyncTask } from "@/types";

function makeTask(overrides: Partial<PodcastHistorySyncTask>) {
  return {
    id: 12,
    podcast_id: 7,
    trigger: "manual" as const,
    status: "pending" as const,
    attempts: 0,
    next_retry_at: null,
    total_known: null,
    processed_count: 0,
    created_count: 0,
    updated_count: 0,
    failed_count: 0,
    error_message: "",
    source_note: "",
    started_at: null,
    finished_at: null,
    ...overrides,
  };
}

const onStart = vi.fn();

// #465：详情页同步入口的按钮文案、禁用与进度契约。
describe("getPodcastSyncControl", () => {
  it("offers a visible entry when never synced, distinguishing empty history", () => {
    const empty = getPodcastSyncControl({
      podcast: { episode_count: 0 },
      task: null,
      onStart,
    });
    expect(empty.label).toBe("同步历史单集");
    expect(empty.disabled).toBe(false);

    const withEpisodes = getPodcastSyncControl({
      podcast: { episode_count: 30 },
      task: null,
      onStart,
    });
    expect(withEpisodes.label).toBe("同步单集");
  });

  it("blocks duplicate submissions while queued or running and shows real progress", () => {
    const queued = getPodcastSyncControl({
      podcast: { episode_count: 0 },
      task: makeTask({ status: "queued" }),
      onStart,
    });
    expect(queued.label).toBe("排队中…");
    expect(queued.disabled).toBe(true);
    expect(queued.inFlight).toBe(true);

    const running = getPodcastSyncControl({
      podcast: { episode_count: 0 },
      task: makeTask({
        status: "running",
        processed_count: 320,
        total_known: 1200,
      }),
      onStart,
    });
    expect(running.label).toBe("同步中…");
    expect(running.disabled).toBe(true);
    expect(running.progressText).toBe("已处理 320 / 1200");

    const unknownTotal = getPodcastSyncControl({
      podcast: { episode_count: 0 },
      task: makeTask({
        status: "running",
        processed_count: 45,
        total_known: null,
      }),
      onStart,
    });
    // 总量未知只展示已处理数量，不伪造百分比。
    expect(unknownTotal.progressText).toBe("已处理 45 集");
  });

  it("offers retry with the failure reason after a failure", () => {
    const control = getPodcastSyncControl({
      podcast: { episode_count: 3 },
      task: makeTask({
        status: "failed",
        attempts: 3,
        error_message: "upstream timeout",
      }),
      onStart,
    });
    expect(control.label).toBe("重试同步");
    expect(control.disabled).toBe(false);
    expect(control.errorMessage).toBe("upstream timeout");
  });

  it("offers continue after a partial pass and keeps the empty-source note after completion", () => {
    const partial = getPodcastSyncControl({
      podcast: { episode_count: 1000 },
      task: makeTask({ status: "partial", processed_count: 1000 }),
      onStart,
    });
    expect(partial.label).toBe("继续同步");

    const completedEmpty = getPodcastSyncControl({
      podcast: { episode_count: 0 },
      task: makeTask({
        status: "completed",
        source_note: "源当前未提供可获取单集",
      }),
      onStart,
    });
    expect(completedEmpty.label).toBe("同步历史单集");
    expect(completedEmpty.sourceNote).toBe("源当前未提供可获取单集");
  });
});

it("allows manual takeover of a pending automatic task", () => {
  const control = getPodcastSyncControl({
    podcast: { episode_count: 0 },
    task: makeTask({ status: "pending", trigger: "workflow" }),
    onStart,
  });
  expect(control.disabled).toBe(false);
  control.onStart();
  expect(onStart).toHaveBeenCalled();
});
it.each(["failed", "partial"] as const)(
  "continues polling %s tasks with scheduled retries",
  (status) => {
    expect(
      shouldPollPodcastSync(
        makeTask({ status, next_retry_at: "2026-09-22T01:00:00Z" }),
      ),
    ).toBe(true);
    expect(
      shouldPollPodcastSync(makeTask({ status, next_retry_at: null })),
    ).toBe(false);
  },
);
