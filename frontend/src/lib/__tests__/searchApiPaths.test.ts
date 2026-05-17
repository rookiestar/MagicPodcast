import { describe, expect, it } from "vitest";
import { buildSearchPath } from "../searchApiPaths";

describe("searchApiPaths", () => {
  it("builds the base search path", () => {
    expect(buildSearchPath({ q: "科技" })).toBe(
      "/api/v1/search?q=%E7%A7%91%E6%8A%80",
    );
  });

  it("builds full search paths with stable query ordering", () => {
    expect(
      buildSearchPath({
        q: "podcast",
        type: "episodes",
        tag_id: [1, 52],
        page: 2,
        page_size: 20,
        episode_page: 3,
        episode_page_size: 30,
      }),
    ).toBe(
      "/api/v1/search?q=podcast&type=episodes&tag_id=1&tag_id=52&page=2&page_size=20&episode_page=3&episode_page_size=30",
    );
  });

  it("supports a single tag id", () => {
    expect(buildSearchPath({ q: "podcast", tag_id: 7 })).toBe(
      "/api/v1/search?q=podcast&tag_id=7",
    );
  });
});
