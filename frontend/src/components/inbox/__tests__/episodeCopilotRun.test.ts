import { describe, expect, it } from "vitest";
import type { EpisodeCopilotStreamEvent } from "@/types/episodeCopilot";
import {
  applyAnswerDelta,
  applyStreamEvent,
  cancelRun,
  createRunState,
  stageOrder,
} from "../episodeCopilotRun";

function statusEvent(
  activity: Record<string, unknown>,
): EpisodeCopilotStreamEvent {
  return {
    type: "status",
    transcript_used: false,
    private_note_included: false,
    activity: activity as unknown as EpisodeCopilotStreamEvent["activity"],
  } as EpisodeCopilotStreamEvent;
}

function runtimeActivity(
  id: string,
  ordinal: number,
  state: string,
): Record<string, unknown> {
  return {
    id,
    ordinal,
    stage: "public_research",
    category: "web_search",
    state,
    text: `activity-${ordinal}`,
    observed_at: "2026-09-06T12:00:00Z",
  };
}

describe("episodeCopilotRun state machine", () => {
  it("starts with seven fixed pending stages", () => {
    const state = createRunState(1000);
    expect(state.stages.map((stage) => stage.id)).toEqual(
      stageOrder.map((stage) => stage.id),
    );
    expect(state.stages.every((stage) => stage.status === "pending")).toBe(
      true,
    );
    expect(state.finish).toBeNull();
  });

  it("marks earlier running stages done when a later stage starts", () => {
    let state = createRunState(0);
    state = applyStreamEvent(
      state,
      statusEvent({
        id: "stage:research_runtime",
        ordinal: 1,
        stage: "research_runtime",
        category: "stage",
        state: "started",
        observed_at: "2026-09-06T12:00:00Z",
      }),
      10,
    );
    expect(
      state.stages.find((stage) => stage.id === "research_runtime")?.status,
    ).toBe("running");
    state = applyStreamEvent(
      state,
      statusEvent({
        id: "stage:answer_runtime",
        ordinal: 2,
        stage: "answer_runtime",
        category: "stage",
        state: "started",
        observed_at: "2026-09-06T12:00:00Z",
      }),
      20,
    );
    expect(
      state.stages.find((stage) => stage.id === "research_runtime")?.status,
    ).toBe("done");
    expect(
      state.stages.find((stage) => stage.id === "answer_runtime")?.status,
    ).toBe("running");
    expect(state.announcement).toBe("进入阶段：回答 Runtime");
  });

  it("updates the same activity in place and merges beyond the bounded budget", () => {
    let state = createRunState(0);
    state = applyStreamEvent(
      state,
      statusEvent(runtimeActivity("research:a1", 1, "started")),
      10,
    );
    expect(state.activities.length).toBe(1);
    state = applyStreamEvent(
      state,
      statusEvent(runtimeActivity("research:a1", 1, "completed")),
      20,
    );
    expect(state.activities.length).toBe(1);
    expect(state.activities[0]?.state).toBe("completed");
    expect(state.lastActivityAt).toBe(20);

    // A long stream stays bounded: the oldest activities merge into a count.
    for (let index = 2; index <= 200; index += 1) {
      state = applyStreamEvent(
        state,
        statusEvent(runtimeActivity(`research:a${index}`, index, "started")),
        20 + index,
      );
    }
    expect(state.activities.length).toBeLessThanOrEqual(120);
    expect(state.mergedActivities).toBeGreaterThan(0);
    expect(state.activities.at(-1)?.id).toBe("research:a200");
  });

  it("surfaces verified counts and degradation from the service announcements", () => {
    let state = createRunState(0);
    state = applyStreamEvent(
      state,
      statusEvent({
        id: "stage:source_validation",
        ordinal: 1,
        stage: "source_validation",
        category: "stage",
        state: "completed",
        text: "已验证 3 个公开来源。",
        observed_at: "2026-09-06T12:00:00Z",
      }),
      10,
    );
    expect(state.verifiedSources).toBe(3);
    expect(state.degradedResearch).toBe(false);

    state = applyStreamEvent(
      state,
      statusEvent({
        id: "stage:source_validation",
        ordinal: 2,
        stage: "source_validation",
        category: "stage",
        state: "failed",
        text: "公开资料检索失败，将仅依据单集内部内容回答。",
        observed_at: "2026-09-06T12:00:00Z",
      }),
      20,
    );
    expect(state.verifiedSources).toBe(0);
    expect(state.degradedResearch).toBe(true);
  });

  it("closes the run on complete, error, and cancellation with stage words", () => {
    let state = createRunState(0);
    state = applyAnswerDelta(state, 10);
    expect(
      state.stages.find((stage) => stage.id === "compose_answer")?.status,
    ).toBe("running");
    state = applyStreamEvent(
      state,
      {
        type: "complete",
        first_content_ms: 100,
        total_ms: 400,
        transcript_used: false,
        private_note_included: false,
      },
      20,
    );
    expect(state.finish?.outcome).toBe("completed");
    expect(state.stages.every((stage) => stage.status === "done")).toBe(true);

    state = applyStreamEvent(
      state,
      {
        type: "status",
        transcript_used: false,
        private_note_included: false,
        activity: {
          id: "stage:answer_runtime",
          ordinal: 9,
          stage: "answer_runtime",
          category: "stage",
          state: "started",
          observed_at: "2026-09-06T12:00:00Z",
        } as EpisodeCopilotStreamEvent["activity"],
      },
      30,
    );
    // Events after a terminal outcome are ignored.
    expect(state.finish?.outcome).toBe("completed");

    let failing = createRunState(0);
    failing = applyStreamEvent(
      failing,
      {
        type: "error",
        message: "本地 Codex Runtime 暂时无法完成回答",
        code: "execution_failed",
        retryable: true,
        transcript_used: false,
        private_note_included: false,
      },
      10,
    );
    expect(failing.finish?.outcome).toBe("failed");
    expect(failing.stages[0]?.status).toBe("failed");

    let cancelled = createRunState(0);
    cancelled = applyAnswerDelta(cancelled, 5);
    cancelled = cancelRun(cancelled, 15);
    expect(cancelled.finish?.outcome).toBe("cancelled");
    expect(
      cancelled.stages.find((stage) => stage.id === "compose_answer")?.status,
    ).toBe("cancelled");
    expect(cancelled.announcement).toBe("已取消");
  });
});
