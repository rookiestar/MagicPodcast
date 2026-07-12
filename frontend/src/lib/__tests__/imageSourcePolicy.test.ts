import { describe, expect, it } from "vitest";
import {
  getSafeImageSource,
  isApprovedRemoteImageUrl,
  isSafeInlineImageData,
  sanitizeContentUrl,
} from "../imageSourcePolicy";

describe("imageSourcePolicy", () => {
  it("proxies an approved image host through the same-origin backend", () => {
    expect(
      getSafeImageSource("https://i.typlog.com/cover.jpg"),
    ).toBe("/images/proxy?url=https%3A%2F%2Fi.typlog.com%2Fcover.jpg");
  });

  it("rejects unreviewed hosts, userinfo, ports, and protocol-relative URLs", () => {
    expect(isApprovedRemoteImageUrl("https://evil.example/cover.jpg")).toBe(
      false,
    );
    expect(
      getSafeImageSource("https://i.typlog.com:8443/cover.jpg"),
    ).toBeUndefined();
    expect(
      getSafeImageSource("https://i.typlog.com@127.0.0.1/cover.jpg"),
    ).toBeUndefined();
    expect(getSafeImageSource("//127.0.0.1/cover.jpg")).toBeUndefined();
  });

  it("keeps bounded QR data but rejects oversized inline images", () => {
    const qr = "data:image/png;base64,abc123=";
    expect(isSafeInlineImageData(qr)).toBe(true);
    expect(getSafeImageSource(qr)).toBe(qr);
    expect(
      isSafeInlineImageData(`data:image/png;base64,${"a".repeat(1024 * 1024)}`),
    ).toBe(false);
  });

  it("allows ordinary links while rejecting active URL schemes", () => {
    expect(sanitizeContentUrl("https://example.com/article")).toBe(
      "https://example.com/article",
    );
    expect(sanitizeContentUrl("mailto:owner@example.com")).toBe(
      "mailto:owner@example.com",
    );
    expect(sanitizeContentUrl("javascript:alert(1)")).toBe("");
    expect(sanitizeContentUrl("data:text/html,<script>alert(1)</script>")).toBe(
      "",
    );
  });
});
