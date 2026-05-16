import { useCallback, useLayoutEffect, useRef, useState } from "react";

const DEFAULT_BOTTOM_THRESHOLD = 24;

export function isLogContainerNearBottom(
  container: Pick<
    HTMLDivElement,
    "clientHeight" | "scrollHeight" | "scrollTop"
  >,
  threshold = DEFAULT_BOTTOM_THRESHOLD,
) {
  return (
    container.scrollHeight - container.scrollTop - container.clientHeight <=
    threshold
  );
}

export function useStableLogScroll(itemCount: number) {
  const [autoScroll, setAutoScroll] = useState(true);
  const logContainerRef = useRef<HTMLDivElement>(null);
  const logEndRef = useRef<HTMLDivElement>(null);
  const manualLogScrollTopRef = useRef<number | null>(null);

  useLayoutEffect(() => {
    const container = logContainerRef.current;
    if (!container) return;

    if (!autoScroll) {
      if (manualLogScrollTopRef.current !== null) {
        container.scrollTop = manualLogScrollTopRef.current;
      }
      return;
    }

    requestAnimationFrame(() => {
      if (logEndRef.current && autoScroll) {
        logEndRef.current.scrollIntoView({ behavior: "auto", block: "end" });
      }
    });
  }, [itemCount, autoScroll]);

  const resetLogScroll = useCallback(() => {
    manualLogScrollTopRef.current = null;
    setAutoScroll(true);
  }, []);

  const resumeAutoScroll = useCallback(() => {
    manualLogScrollTopRef.current = null;
    setAutoScroll(true);
    requestAnimationFrame(() => {
      if (logEndRef.current) {
        logEndRef.current.scrollIntoView({ behavior: "smooth", block: "end" });
      }
    });
  }, []);

  const handleLogScroll = useCallback(() => {
    const container = logContainerRef.current;
    if (!container) return;

    if (isLogContainerNearBottom(container)) {
      manualLogScrollTopRef.current = null;
      setAutoScroll(true);
      return;
    }

    manualLogScrollTopRef.current = container.scrollTop;
    setAutoScroll(false);
  }, []);

  return {
    autoScroll,
    logContainerRef,
    logEndRef,
    handleLogScroll,
    resetLogScroll,
    resumeAutoScroll,
  };
}
