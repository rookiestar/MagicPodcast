import { describe, expect, it } from "vitest";
import {
  getSafeOriginalUrl,
  ORIGINAL_EPISODE_MISSING_TEXT,
  ORIGINAL_EPISODE_REJECTED_TEXT,
  originalEpisodeAccessText,
  planOriginalEpisodeAccess,
  planSafeOriginalEpisodeOpen,
} from "../originalEpisodeOpen";

const XYZ_ID = "6a8cf80a1352af56ff3b7e2d";

describe("planSafeOriginalEpisodeOpen", () => {
  it("offers recovery for Xiaoyuzhou www episode URLs with RSS tracking", () => {
    const openUrl = `https://www.xiaoyuzhoufm.com/episode/${XYZ_ID}?utm_source=rss`;
    expect(planSafeOriginalEpisodeOpen(openUrl)).toEqual({
      recovery: true,
      openUrl,
      retryUrl: `https://www.xiaoyuzhoufm.com/episode/${XYZ_ID}`,
      appUrl: `cosmos://page.cos/episode/${XYZ_ID}`,
      copyText: `https://www.xiaoyuzhoufm.com/episode/${XYZ_ID}`,
    });
  });

  it("offers recovery for the web subdomain and ignores a trailing slash", () => {
    const openUrl = `https://web.xiaoyuzhoufm.com/episode/${XYZ_ID}/`;
    expect(planSafeOriginalEpisodeOpen(openUrl)).toEqual({
      recovery: true,
      openUrl,
      retryUrl: `https://www.xiaoyuzhoufm.com/episode/${XYZ_ID}`,
      appUrl: `cosmos://page.cos/episode/${XYZ_ID}`,
      copyText: `https://www.xiaoyuzhoufm.com/episode/${XYZ_ID}`,
    });
  });

  it("does not offer recovery for non-episode Xiaoyuzhou paths or extra segments", () => {
    expect(
      planSafeOriginalEpisodeOpen(
        `https://www.xiaoyuzhoufm.com/podcast/${XYZ_ID}`,
      ),
    ).toEqual({
      recovery: false,
      openUrl: `https://www.xiaoyuzhoufm.com/podcast/${XYZ_ID}`,
    });
    expect(
      planSafeOriginalEpisodeOpen(
        `https://www.xiaoyuzhoufm.com/episode/${XYZ_ID}/comments`,
      ),
    ).toEqual({
      recovery: false,
      openUrl: `https://www.xiaoyuzhoufm.com/episode/${XYZ_ID}/comments`,
    });
    expect(
      planSafeOriginalEpisodeOpen("https://www.xiaoyuzhoufm.com/episode/"),
    ).toEqual({
      recovery: false,
      openUrl: "https://www.xiaoyuzhoufm.com/episode/",
    });
  });

  it("does not offer recovery for other hosts or unsafe schemes", () => {
    expect(
      planSafeOriginalEpisodeOpen("https://example.com/episode/201"),
    ).toEqual({
      recovery: false,
      openUrl: "https://example.com/episode/201",
    });
    expect(planSafeOriginalEpisodeOpen("javascript:alert(1)")).toBeNull();
    expect(
      planSafeOriginalEpisodeOpen("data:text/html,<script>alert(1)</script>"),
    ).toBeNull();
  });

  it("keeps recovery for any nonempty Xiaoyuzhou episode path segment", () => {
    const episodeId = "a".repeat(129);
    const openUrl = `https://www.xiaoyuzhoufm.com/episode/${episodeId}`;

    expect(planSafeOriginalEpisodeOpen(openUrl)).toEqual({
      recovery: true,
      openUrl,
      retryUrl: openUrl,
      appUrl: `cosmos://page.cos/episode/${episodeId}`,
      copyText: openUrl,
    });
  });
});

describe("getSafeOriginalUrl", () => {
  it("keeps ordinary http(s) links and strips active schemes", () => {
    expect(getSafeOriginalUrl(`https://www.xiaoyuzhoufm.com/episode/${XYZ_ID}`)).toBe(
      `https://www.xiaoyuzhoufm.com/episode/${XYZ_ID}`,
    );
    expect(getSafeOriginalUrl("javascript:alert(1)")).toBe("");
    expect(getSafeOriginalUrl("data:text/html,<p>x</p>")).toBe("");
  });
});

describe("planOriginalEpisodeAccess", () => {
  it("opens legal absolute HTTPS and HTTP addresses with a host", () => {
    const httpsAccess = planOriginalEpisodeAccess(
      "https://hosting.wavpub.cn/pie/ep229/",
    );
    expect(httpsAccess.state).toBe("openable");
    assertOpenable(httpsAccess, "https://hosting.wavpub.cn/pie/ep229/");

    const httpAccess = planOriginalEpisodeAccess("http://example.com/episode");
    expect(httpAccess.state).toBe("openable");
    assertOpenable(httpAccess, "http://example.com/episode");
  });

  it("treats empty, undefined, and blank values as missing", () => {
    expect(planOriginalEpisodeAccess("")).toEqual({ state: "missing" });
    expect(planOriginalEpisodeAccess(null)).toEqual({ state: "missing" });
    expect(planOriginalEpisodeAccess(undefined)).toEqual({ state: "missing" });
    expect(planOriginalEpisodeAccess("   ")).toEqual({ state: "missing" });
    expect(originalEpisodeAccessText({ state: "missing" })).toBe(
      ORIGINAL_EPISODE_MISSING_TEXT,
    );
  });

  it("rejects incomplete schemes, dangerous protocols, and parse failures", () => {
    for (const value of [
      "https://",
      "http://",
      "javascript:alert(1)",
      "data:text/html,<script>alert(1)</script>",
      "mailto:hello@example.com",
      "tel:+8612345678901",
      "not a url",
      "/episode/relative",
      "//protocol-relative.example.com/episode",
    ]) {
      const access = planOriginalEpisodeAccess(value);
      expect(access.state, value).toBe("rejected");
      expect(
        originalEpisodeAccessText(access as { state: "rejected" }),
        value,
      ).toBe(ORIGINAL_EPISODE_REJECTED_TEXT);
    }
  });

  it("never exposes an openable navigation for rejected or missing values", () => {
    for (const value of [
      "",
      "   ",
      "https://",
      "javascript:alert(1)",
      "data:text/html,<p>x</p>",
    ]) {
      const access = planOriginalEpisodeAccess(value);
      if (access.state !== "openable") {
        expect("openUrl" in access && Boolean(access.openUrl)).toBe(false);
      } else {
        expect.fail(`value ${value} must not be openable`);
      }
    }
  });
});

function assertOpenable(
  access: ReturnType<typeof planOriginalEpisodeAccess>,
  openUrl: string,
) {
  if (access.state !== "openable") {
    throw new Error(`expected openable access, got ${access.state}`);
  }
  expect(access.openUrl).toBe(openUrl);
  expect(access.plan.openUrl).toBe(openUrl);
}
