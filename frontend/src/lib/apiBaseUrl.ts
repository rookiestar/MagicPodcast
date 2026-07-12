const LOOPBACK_BACKEND_URL = "http://127.0.0.1:8080";

export function resolveApiBaseUrl(
  isBrowser: boolean,
  backendUrl = process.env.BACKEND_URL,
): string {
  if (isBrowser) {
    return "";
  }

  const resolvedUrl = new URL(backendUrl || LOOPBACK_BACKEND_URL);
  const isLoopback =
    resolvedUrl.hostname === "127.0.0.1" ||
    resolvedUrl.hostname === "[::1]" ||
    resolvedUrl.hostname === "::1";

  if (resolvedUrl.protocol !== "http:" || !isLoopback) {
    throw new Error("BACKEND_URL must use an HTTP loopback address");
  }

  return resolvedUrl.origin;
}

export const apiBaseUrl = resolveApiBaseUrl(typeof window !== "undefined");
