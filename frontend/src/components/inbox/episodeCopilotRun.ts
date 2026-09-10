import type {
  EpisodeCopilotActivity,
  EpisodeCopilotStage,
  EpisodeCopilotStageTimings,
  EpisodeCopilotStreamEvent,
} from "@/types/episodeCopilot";

/**
 * Client model of one question's runtime execution. The seven user-visible
 * stages are fixed; real stream events drive every transition, and the model
 * never invents model actions, percentages, or ETAs.
 */

export const stageOrder: ReadonlyArray<{
  id: EpisodeCopilotStage;
  label: string;
}> = [
  { id: "read_context", label: "读取单集上下文" },
  { id: "library_search", label: "检索库内发言" },
  { id: "research_runtime", label: "检索 Runtime" },
  { id: "public_research", label: "核对公开资料" },
  { id: "source_validation", label: "校验公开来源" },
  { id: "answer_runtime", label: "回答 Runtime" },
  { id: "compose_answer", label: "组织回答" },
  { id: "citation_validation", label: "核验引用" },
];

export type StageStatus =
  | "pending"
  | "running"
  | "done"
  | "failed"
  | "cancelled"
  | "skipped";

export interface StageProgress {
  id: EpisodeCopilotStage;
  label: string;
  status: StageStatus;
  startedAt: number | null;
  endedAt: number | null;
}

export interface RunActivity {
  id: string;
  ordinal: number;
  stage: EpisodeCopilotStage;
  category: EpisodeCopilotActivity["category"];
  state: EpisodeCopilotActivity["state"];
  text: string;
  observedAt: number;
  elapsedMs?: number;
  metadata?: Record<string, string>;
  receivedAt: number;
}

export type RunOutcome = "running" | "completed" | "failed" | "cancelled";

export interface RunFinish {
  outcome: Exclude<RunOutcome, "running">;
  firstContentMs?: number;
  totalMs?: number;
  stageTimings?: EpisodeCopilotStageTimings;
  errorCode?: string;
  errorMessage?: string;
}

export interface CopilotRunState {
  startedAt: number;
  stages: StageProgress[];
  activities: RunActivity[];
  // Activities beyond the display budget are merged into a counter instead
  // of silently disappearing or growing the DOM without bound.
  mergedActivities: number;
  lastActivityAt: number | null;
  verifiedSources: number | null;
  sourceStatusText: string | null;
  degradedResearch: boolean;
  sourceConflicts: boolean;
  finish: RunFinish | null;
  announcement: string | null;
}

// The service caps progress per execution, and the client additionally
// merges the oldest activities beyond this budget so a long question can
// never grow the list without bound.
const maxActivities = 120;

export function createRunState(startedAt: number, persona = false): CopilotRunState {
  return {
    startedAt,
    stages: stageOrder.filter((stage) => persona || stage.id !== "library_search").map((stage) => ({
      id: stage.id,
      label: stage.label,
      status: "pending",
      startedAt: null,
      endedAt: null,
    })),
    activities: [],
    mergedActivities: 0,
    lastActivityAt: null,
    verifiedSources: null,
    sourceStatusText: null,
    degradedResearch: false,
    sourceConflicts: false,
    finish: null,
    announcement: null,
  };
}

function stageIndex(state: CopilotRunState, id: EpisodeCopilotStage) {
  return state.stages.findIndex((stage) => stage.id === id);
}

function firstRunningIndex(state: CopilotRunState) {
  return state.stages.findIndex((stage) => stage.status === "running");
}

function firstPendingIndex(state: CopilotRunState) {
  return state.stages.findIndex((stage) => stage.status === "pending");
}

function withStageStarted(
  state: CopilotRunState,
  id: EpisodeCopilotStage,
  now: number,
): CopilotRunState {
  const index = stageIndex(state, id);
  if (index < 0) return state;
  const stages = state.stages.map((stage, position) => {
    if (position < index && stage.status === "running") {
      // Stages run in order: a later stage starting closes an earlier one
      // that was still marked running.
      return { ...stage, status: "done" as const, endedAt: now };
    }
    if (position !== index || stage.status !== "pending") return stage;
    return { ...stage, status: "running" as const, startedAt: now };
  });
  const label = state.stages[index].label;
  return { ...state, stages, announcement: `进入阶段：${label}` };
}

function withStageEnded(
  state: CopilotRunState,
  id: EpisodeCopilotStage,
  status: Extract<StageStatus, "done" | "failed" | "cancelled">,
  now: number,
): CopilotRunState {
  const index = stageIndex(state, id);
  if (index < 0) return state;
  const stages = state.stages.map((stage, position) => {
    if (position === index && stage.status !== "done") {
      return { ...stage, status, endedAt: now };
    }
    if (position < index && stage.status === "running") {
      return { ...stage, status: "done" as const, endedAt: now };
    }
    return stage;
  });
  return { ...state, stages };
}

function upsertActivity(
  state: CopilotRunState,
  activity: RunActivity,
): CopilotRunState {
  const existingIndex = state.activities.findIndex(
    (entry) => entry.id === activity.id,
  );
  let activities: RunActivity[];
  let mergedActivities = state.mergedActivities;
  if (existingIndex >= 0) {
    // Same activity updates in place and keeps its original position.
    activities = state.activities.map((entry, position) =>
      position === existingIndex ? activity : entry,
    );
  } else {
    activities = [...state.activities, activity];
  }
  if (activities.length > maxActivities) {
    const overflow = activities.length - maxActivities;
    activities = activities.slice(overflow);
    mergedActivities += overflow;
  }
  return {
    ...state,
    activities,
    mergedActivities,
    lastActivityAt: activity.receivedAt,
  };
}

function withFinish(
  state: CopilotRunState,
  finish: RunFinish,
  now: number,
): CopilotRunState {
  const stages = state.stages.map((stage) => {
    if (stage.status === "running") {
      return {
        ...stage,
        status: finish.outcome === "cancelled" ? ("cancelled" as const) : ("done" as const),
        endedAt: now,
      };
    }
    if (finish.outcome !== "completed" || stage.status !== "pending") {
      return stage;
    }
    // Persona questions may finish without invoking public research.
    if (state.stages.some((entry) => entry.id === "library_search") &&
        (stage.id === "research_runtime" || stage.id === "public_research")) {
      return { ...stage, status: "skipped" as const };
    }
    // Preserve the existing ordinary-question stage contract.
    return { ...stage, status: "done" as const, endedAt: stage.endedAt ?? now };
  });
  return { ...state, stages, finish };
}

export function applyStreamEvent(
  state: CopilotRunState,
  event: EpisodeCopilotStreamEvent,
  receivedAt: number,
): CopilotRunState {
  if (state.finish) return state;
  if (event.type === "context") {
    return withStageEnded(state, "read_context", "done", receivedAt);
  }
  if (event.type === "complete") {
    const completed = withFinish(
      state,
      {
        outcome: "completed",
        firstContentMs: event.first_content_ms,
        totalMs: event.total_ms,
        stageTimings: event.stage_timings,
      },
      receivedAt,
    );
    return { ...completed, announcement: "回答完成" };
  }
  if (event.type === "error") {
    const failingIndex =
      firstRunningIndex(state) >= 0
        ? firstRunningIndex(state)
        : firstPendingIndex(state);
    let next = state;
    if (failingIndex >= 0) {
      next = withStageEnded(
        state,
        state.stages[failingIndex].id,
        "failed",
        receivedAt,
      );
    }
    return {
      ...next,
      finish: {
        outcome: "failed",
        errorCode: event.code,
        errorMessage: event.message,
      },
      announcement: event.message ? `执行失败：${event.message}` : "执行失败",
    };
  }
  if (event.type !== "status" || !event.activity) return state;

  const activity = event.activity;
  let next = upsertActivity(
    state,
    {
      id: activity.id,
      ordinal: activity.ordinal,
      stage: activity.stage,
      category: activity.category,
      state: activity.state,
      text: activity.text ?? "",
      observedAt: Date.parse(activity.observed_at) || receivedAt,
      elapsedMs: activity.elapsed_ms,
      metadata: activity.metadata,
      receivedAt,
    },
  );

  const stageId = activity.stage;
  if (activity.category === "stage") {
    // Deterministic service activities carry the stage lifecycle.
    if (activity.state === "started") {
      next = withStageStarted(next, stageId, receivedAt);
    } else if (activity.state === "completed") {
      next = withStageEnded(next, stageId, "done", receivedAt);
    } else if (activity.state === "failed") {
      next = withStageEnded(next, stageId, "failed", receivedAt);
    }
  } else if (stageId !== "read_context") {
    // Forwarded runtime activities prove the stage is doing real work; the
    // stage itself closes when the service announces the next one.
    const index = stageIndex(next, stageId);
    if (index >= 0 && next.stages[index].status === "pending") {
      next = withStageStarted(next, stageId, receivedAt);
    }
  }

  if (stageId === "source_validation") {
    if (activity.state === "completed") {
      const sourceText = activity.text ?? "";
      const verified = /已验证 (\d+) 个公开来源/.exec(sourceText);
      next = {
        ...next,
        verifiedSources: verified ? Number(verified[1]) : 0,
        sourceStatusText: sourceText,
        sourceConflicts: sourceText.includes("来源冲突"),
      };
    } else if (activity.state === "failed") {
      next = {
        ...next,
        verifiedSources: 0,
        sourceStatusText: activity.text ?? "公开资料检索降级",
        degradedResearch: true,
      };
    }
  }
  return next;
}

/** The first readable answer proves the compose stage is streaming. */
export function applyAnswerDelta(
  state: CopilotRunState,
  receivedAt: number,
): CopilotRunState {
  if (state.finish) return state;
  const index = stageIndex(state, "compose_answer");
  if (index >= 0 && state.stages[index].status === "pending") {
    return withStageStarted(state, "compose_answer", receivedAt);
  }
  return state;
}

export function cancelRun(state: CopilotRunState, now: number): CopilotRunState {
  if (state.finish) return state;
  const runningIndex = firstRunningIndex(state);
  const next =
    runningIndex >= 0
      ? withStageEnded(
          state,
          state.stages[runningIndex].id,
          "cancelled",
          now,
        )
      : state;
  return {
    ...next,
    finish: { outcome: "cancelled" },
    announcement: "已取消",
  };
}
