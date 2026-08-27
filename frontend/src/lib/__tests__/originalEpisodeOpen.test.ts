import { describe, expect, it } from "vitest";
import {
  getSafeOriginalUrl,
  planOriginalEpisodeOpen,
} from "../originalEpisodeOpen";

const XYZ_ID = "6a8cf80a1352af56ff3b7e2d";

describe("planOriginalEpisodeOpen", () => {
  it("offers recovery for Xiaoyuzhou www episode URLs with RSS tracking", () => {
    const openUrl = `https://www.xiaoyuzhoufm.com/episode/${XYZ_ID}?utm_source=rss`;
    expect(planOriginalEpisodeOpen(openUrl)).toEqual({
      recovery: true,
      openUrl,
      retryUrl: `https://www.xiaoyuzhoufm.com/episode/${XYZ_ID}`,
      appUrl: `cosmos://page.cos/episode/${XYZ_ID}`,
      copyText: `https://www.xiaoyuzhoufm.com/episode/${XYZ_ID}`,
    });
  });

  it("offers recovery for the web subdomain and ignores a trailing slash", () => {
    const openUrl = `https://web.xiaoyuzhoufm.com/episode/${XYZ_ID}/`;
    expect(planOriginalEpisodeOpen(openUrl)).toEqual({
      recovery: true,
      openUrl,
      retryUrl: `https://www.xiaoyuzhoufm.com/episode/${XYZ_ID}`,
      appUrl: `cosmos://page.cos/episode/${XYZ_ID}`,
      copyText: `https://www.xiaoyuzhoufm.com/episode/${XYZ_ID}`,
    });
  });

  it("does not offer recovery for non-episode Xiaoyuzhou paths or extra segments", () => {
    expect(
      planOriginalEpisodeOpen(
        `https://www.xiaoyuzhoufm.com/podcast/${XYZ_ID}`,
      ),
    ).toEqual({
      recovery: false,
      openUrl: `https://www.xiaoyuzhoufm.com/podcast/${XYZ_ID}`,
    });
    expect(
      planOriginalEpisodeOpen(
        `https://www.xiaoyuzhoufm.com/episode/${XYZ_ID}/comments`,
      ),
    ).toEqual({
      recovery: false,
      openUrl: `https://www.xiaoyuzhoufm.com/episode/${XYZ_ID}/comments`,
    });
    expect(
      planOriginalEpisodeOpen("https://www.xiaoyuzhoufm.com/episode/"),
    ).toEqual({
      recovery: false,
      openUrl: "https://www.xiaoyuzhoufm.com/episode/",
    });
  });

  it("does not offer recovery for other hosts or unsafe schemes", () => {
    expect(
      planOriginalEpisodeOpen("https://example.com/episode/201"),
    ).toEqual({
      recovery: false,
      openUrl: "https://example.com/episode/201",
    });
    expect(planOriginalEpisodeOpen("javascript:alert(1)")).toEqual({
      recovery: false,
      openUrl: "javascript:alert(1)",
    });
    expect(
      planOriginalEpisodeOpen("data:text/html,<script>alert(1)</script>"),
    ).toEqual({
      recovery: false,
      openUrl: "data:text/html,<script>alert(1)</script>",
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
