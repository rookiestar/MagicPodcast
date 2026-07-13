export interface TypedConfirmationSpec {
  action: string;
  impact: string;
  phrase: string;
}
/**
 * Show the impact first, then require a short exact phrase. Returning null is
 * the cancellation contract: callers must not start a request or task.
 */
export function requestTypedConfirmation(
  spec: TypedConfirmationSpec,
): string | null {
  if (typeof window === "undefined") {
    return null;
  }

  const acknowledged = window.confirm(
    `${spec.action}\n\n影响：${spec.impact}\n\n继续操作请点击“确定”，否则点击“取消”。`,
  );
  if (!acknowledged) {
    return null;
  }

  const entered = window.prompt(
    `请输入确认文字：${spec.phrase}`,
    "",
  );
  return entered?.trim() === spec.phrase ? spec.phrase : null;
}
