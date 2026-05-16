import { useCallback, useRef } from "react";

interface UseExclusiveAsyncActionOptions {
  isBlocked: boolean;
}

export function useExclusiveAsyncAction({
  isBlocked,
}: UseExclusiveAsyncActionOptions) {
  const runningRef = useRef(false);

  return useCallback(
    async (action: () => Promise<void>) => {
      if (runningRef.current || isBlocked) {
        return false;
      }

      runningRef.current = true;
      try {
        await action();
        return true;
      } finally {
        runningRef.current = false;
      }
    },
    [isBlocked],
  );
}
