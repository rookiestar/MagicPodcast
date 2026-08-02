import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { CompactLogo, Logo } from "../Logo";

describe("Logo", () => {
  it("uses the branded image asset and keeps the product name accessible", () => {
    render(<Logo />);

    const brand = screen.getByRole("img", { name: "MagicPodcast" });
    const mark = brand.querySelector("img");

    expect(mark).not.toBeNull();
    expect(mark?.getAttribute("src")).toContain(
      "/brand/magicpodcast-tuning-mark.png",
    );
    const name = brand.querySelector(".magic-wordmark-name");
    expect(name).toHaveTextContent("MagicPodcast");
    expect(name?.children).toHaveLength(0);
    expect(screen.queryByText("PODCAST · 01")).not.toBeInTheDocument();
  });

  it("supports compact and icon-only navigation treatments", () => {
    const { rerender } = render(<CompactLogo />);

    expect(screen.getByRole("img", { name: "MagicPodcast" })).toHaveAttribute(
      "data-variant",
      "compact",
    );

    rerender(<Logo variant="icon-only" />);

    expect(screen.getByRole("img", { name: "MagicPodcast" })).toHaveAttribute(
      "data-variant",
      "icon-only",
    );
  });
});
