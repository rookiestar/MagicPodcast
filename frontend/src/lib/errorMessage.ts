export function getErrorMessage(error: unknown) {
  if (error instanceof Error) {
    return error.message || "未知错误";
  }

  if (typeof error === "string") {
    return error || "未知错误";
  }

  if (
    error &&
    typeof error === "object" &&
    "message" in error &&
    typeof error.message === "string"
  ) {
    return error.message || "未知错误";
  }

  return "未知错误";
}
