import { describe, expect, it } from "vitest";
import { getOptimizedImageUrl } from "../imageOptimization";

describe("imageOptimization", () => {
  it("builds a fixed-size optimizer URL for remote images", () => {
    expect(getOptimizedImageUrl("https://example.com/cover.jpg")).toBe(
      "/_next/image?url=https%3A%2F%2Fexample.com%2Fcover.jpg&w=128&q=75",
    );
  });

  it("supports custom width and quality", () => {
    expect(getOptimizedImageUrl("https://example.com/cover.jpg", 256, 60)).toBe(
      "/_next/image?url=https%3A%2F%2Fexample.com%2Fcover.jpg&w=256&q=60",
    );
  });

  it("leaves relative file names and data URLs untouched", () => {
    expect(getOptimizedImageUrl("cover.jpg")).toBe("cover.jpg");
    expect(getOptimizedImageUrl("data:image/png;base64,abc")).toBe(
      "data:image/png;base64,abc",
    );
  });
});
