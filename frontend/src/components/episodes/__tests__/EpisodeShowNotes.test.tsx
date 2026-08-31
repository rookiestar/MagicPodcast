import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { EpisodeShowNotes } from "../EpisodeShowNotes";

describe("EpisodeShowNotes", () => {
  it("uses the compact rich-text density when expanded", () => {
    const { container } = render(
      <EpisodeShowNotes
        summary="单集简介"
        link="https://example.com/episode"
        isExpanded
        status="success"
        document={{
          content: "<h2>章节</h2><p>单集简介</p>",
          format: "html",
        }}
        onRetry={() => undefined}
      />,
    );

    expect(container.querySelector(".editorial-rich-text")).toHaveClass(
      "editorial-rich-text--compact",
    );
    expect(container.querySelector(".editorial-rich-text")).not.toHaveClass(
      "prose",
    );
    expect(
      container.querySelector(".podcast-episode-show-notes-reader"),
    ).toHaveAttribute("tabindex", "0");
    expect(container.querySelector(".bg-gradient-to-t")).not.toBeInTheDocument();
  });
});
