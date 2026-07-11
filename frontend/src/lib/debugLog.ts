export function isDebugLogEnabled() {
  return process.env.NODE_ENV === "development";
}

export function debugLog(...args: unknown[]) {
  if (!isDebugLogEnabled()) return;
  console.log(...args);
}

export function debugDebug(...args: unknown[]) {
  if (!isDebugLogEnabled()) return;
  console.debug(...args);
}
