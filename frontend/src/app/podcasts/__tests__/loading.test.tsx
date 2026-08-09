import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import PodcastsLoading from "../loading";

describe("podcasts route loading shell", () => {
  it("uses the editorial background from the first rendered frame", () => {
    const { container } = render(<PodcastsLoading />);

    expect(container.querySelector(".editorial-loading")).toBeTruthy();
    expect(container.querySelector(".bg-slate-50")).toBeNull();
  });
});
