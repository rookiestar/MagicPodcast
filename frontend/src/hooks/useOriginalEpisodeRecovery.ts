import { useCallback, useState } from "react";
import {
  openOriginalEpisodeTab,
  type OriginalEpisodeOpenPlan,
  type OriginalEpisodeRecoveryPlan,
} from "@/lib/originalEpisodeOpen";

const COPY_FAILED_MESSAGE = "复制失败，请改用重试或用小宇宙打开。";

export function useOriginalEpisodeRecovery() {
  const [state, setState] = useState<{
    key: string | number;
    plan: OriginalEpisodeRecoveryPlan;
  } | null>(null);
  const [copyError, setCopyError] = useState<string | null>(null);

  const activate = useCallback(
    (key: string | number, plan: OriginalEpisodeOpenPlan) => {
      if (plan.recovery) {
        setState({ key, plan });
        setCopyError(null);
        return;
      }
      setState(null);
      setCopyError(null);
    },
    [],
  );

  const dismiss = useCallback(() => {
    setState(null);
    setCopyError(null);
  }, []);

  const retry = useCallback(() => {
    if (state) {
      openOriginalEpisodeTab(state.plan.retryUrl);
    }
  }, [state]);

  const openApp = useCallback(() => {
    if (state) {
      openOriginalEpisodeTab(state.plan.appUrl);
    }
  }, [state]);

  const copy = useCallback(async () => {
    if (!state) {
      return;
    }
    try {
      await navigator.clipboard.writeText(state.plan.copyText);
      setCopyError(null);
    } catch {
      setCopyError(COPY_FAILED_MESSAGE);
    }
  }, [state]);

  return {
    activeKey: state?.key ?? null,
    plan: state?.plan ?? null,
    copyError,
    activate,
    dismiss,
    retry,
    openApp,
    copy,
  };
}

export type OriginalEpisodeRecoveryController = ReturnType<
  typeof useOriginalEpisodeRecovery
>;
