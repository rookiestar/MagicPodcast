import { describe, expect, it } from "vitest";
import {
  canUseNextImage,
  getOptimizedImageUrl,
  isOptimizableImageUrl,
  optimizeHtmlImageSources,
} from "../imageOptimization";

describe("imageOptimization", () => {
  it("builds a fixed-size optimizer URL for remote images", () => {
    expect(getOptimizedImageUrl("https://i.typlog.com/cover.jpg")).toBe(
      "/images/proxy?url=https%3A%2F%2Fi.typlog.com%2Fcover.jpg",
    );
  });

  it("supports custom width and quality", () => {
    expect(getOptimizedImageUrl("/cover.jpg", 256, 60)).toBe(
      "/_next/image?url=%2Fcover.jpg&w=256&q=60",
    );
  });

  it("rejects remote hosts outside the reviewed image set", () => {
    expect(getOptimizedImageUrl("https://example.com/cover.jpg")).toBe("");
  });

  it("leaves relative file names and data URLs untouched", () => {
    expect(getOptimizedImageUrl("cover.jpg")).toBe("cover.jpg");
    expect(getOptimizedImageUrl("data:image/png;base64,abc")).toBe(
      "data:image/png;base64,abc",
    );
  });

  it("detects sources that Next Image can render directly", () => {
    expect(canUseNextImage("/cover.jpg")).toBe(true);
    expect(canUseNextImage("https://example.com/cover.jpg")).toBe(false);
    expect(canUseNextImage("data:image/png;base64,abc")).toBe(true);
    expect(canUseNextImage("cover.jpg")).toBe(false);
  });

  it("detects URLs that should go through the optimizer URL", () => {
    expect(isOptimizableImageUrl("/cover.jpg")).toBe(true);
    expect(isOptimizableImageUrl("/images/proxy?url=x")).toBe(false);
    expect(isOptimizableImageUrl("/_next/image?url=x&w=128&q=75")).toBe(false);
    expect(isOptimizableImageUrl("data:image/png;base64,abc")).toBe(false);
  });

  it("rewrites image sources inside HTML", () => {
    expect(
      optimizeHtmlImageSources(
        '<p><img src="https://i.typlog.com/a.jpg" alt="a"><img src="data:image/png;base64,abc"></p>',
        768,
        80,
      ),
    ).toBe(
      '<p><img src="/images/proxy?url=https%3A%2F%2Fi.typlog.com%2Fa.jpg" alt="a"><img src="data:image/png;base64,abc"></p>',
    );
  });

  it("removes disallowed remote image sources from HTML", () => {
    expect(
      optimizeHtmlImageSources(
        '<p><img src="https://evil.example/track.gif" alt="bad"></p>',
      ),
    ).toBe('<p><img  alt="bad"></p>');
  });
});
