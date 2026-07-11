import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { debugDebug, debugLog, isDebugLogEnabled } from "../debugLog";

describe("debugLog", () => {
  beforeEach(() => {
    vi.spyOn(console, "log").mockImplementation(() => undefined);
    vi.spyOn(console, "debug").mockImplementation(() => undefined);
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.restoreAllMocks();
  });

  it("suppresses debug logs outside development", () => {
    vi.stubEnv("NODE_ENV", "production");

    debugLog("hidden log");
    debugDebug("hidden debug");

    expect(isDebugLogEnabled()).toBe(false);
    expect(console.log).not.toHaveBeenCalled();
    expect(console.debug).not.toHaveBeenCalled();
  });

  it("allows debug logs in development", () => {
    vi.stubEnv("NODE_ENV", "development");

    debugLog("visible log");
    debugDebug("visible debug");

    expect(isDebugLogEnabled()).toBe(true);
    expect(console.log).toHaveBeenCalledWith("visible log");
    expect(console.debug).toHaveBeenCalledWith("visible debug");
  });
});
