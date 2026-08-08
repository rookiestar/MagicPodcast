import { afterEach, describe, expect, it, vi } from "vitest";

type Rewrite = {
  source: string;
  destination: string;
};

describe("frontend backend proxy", () => {
  const originalMockFlag = process.env.MAGICPODCAST_FRONTEND_MOCK_API;
  const originalBackendUrl = process.env.BACKEND_URL;

  afterEach(() => {
    if (originalMockFlag === undefined) {
      delete process.env.MAGICPODCAST_FRONTEND_MOCK_API;
    } else {
      process.env.MAGICPODCAST_FRONTEND_MOCK_API = originalMockFlag;
    }
    if (originalBackendUrl === undefined) {
      delete process.env.BACKEND_URL;
    } else {
      process.env.BACKEND_URL = originalBackendUrl;
    }
    vi.resetModules();
  });

  it("forwards API, image, and health requests even when the legacy mock flag is set", async () => {
    process.env.MAGICPODCAST_FRONTEND_MOCK_API = "1";
    process.env.BACKEND_URL = "http://127.0.0.1:18080";
    vi.resetModules();

    const configModule = await import("../../../next.config.js");
    const config = (configModule.default ?? configModule) as unknown as {
      rewrites: () => Promise<Rewrite[]>;
    };
    const rewrites = await config.rewrites();

    expect(rewrites).toEqual(
      expect.arrayContaining([
        {
          source: "/api/v1/:path*",
          destination: "http://127.0.0.1:18080/api/v1/:path*",
        },
        {
          source: "/images/:path*",
          destination: "http://127.0.0.1:18080/images/:path*",
        },
        {
          source: "/health",
          destination: "http://127.0.0.1:18080/health",
        },
      ]),
    );
  });

  it("uses finite responsive image buckets and modern formats", async () => {
    const configModule = await import("../../../next.config.js");
    const config = (configModule.default ?? configModule) as unknown as {
      images: {
        deviceSizes: number[];
        imageSizes: number[];
        formats: string[];
        localPatterns: Array<{ pathname: string; search?: string }>;
      };
    };
    const widths = [
      ...config.images.imageSizes,
      ...config.images.deviceSizes,
    ].sort((left, right) => left - right);

    expect(widths.find((width) => width >= 32 * 2)).toBe(96);
    expect(widths.find((width) => width >= 228 * 2)).toBe(512);
    expect(config.images.formats).toEqual(["image/avif", "image/webp"]);
    expect(config.images.localPatterns).toEqual([
      { pathname: "/**", search: "" },
      { pathname: "/images/proxy" },
    ]);
  });
});
