import { describe, expect, it } from "vitest";
import { resolveApiBaseUrl } from "../apiBaseUrl";

describe("resolveApiBaseUrl", () => {
  it("always keeps browser requests same-origin", () => {
    expect(resolveApiBaseUrl(true, "http://192.168.1.10:8080")).toBe("");
  });

  it("uses only a server-side loopback default", () => {
    expect(resolveApiBaseUrl(false, "")).toBe("http://127.0.0.1:8080");
    expect(resolveApiBaseUrl(false, "http://127.0.0.1:18080")).toBe("http://127.0.0.1:18080");
  });

  it("rejects a non-loopback server address", () => {
    expect(() => resolveApiBaseUrl(false, "http://192.168.1.10:8080")).toThrow(
      "BACKEND_URL must use an HTTP loopback address",
    );
  });
});
