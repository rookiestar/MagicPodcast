import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { EpisodeShowNotes } from "../EpisodeShowNotes";

describe("EpisodeShowNotes", () => {
  it("uses the compact rich-text density when expanded", () => {
    const { container } = render(
      <EpisodeShowNotes
        html="<h2>章节</h2><p>单集简介</p>"
        link="https://example.com/episode"
        isExpanded
      />,
    );

    expect(container.querySelector(".editorial-rich-text")).toHaveClass(
      "editorial-rich-text--compact",
      "line-clamp-3",
      "md:line-clamp-none",
    );
    expect(container.querySelector(".editorial-rich-text")).not.toHaveClass(
      "prose",
    );
  });
});
