import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import RichText from "../RichText";

describe("RichText", () => {
  it("sanitizes scripts, events, unsafe styles, and unapproved images", () => {
    const { container } = render(
      <RichText
        html={[
          '<h2>节目简介</h2>',
          '<p><a href="https://example.com" target="_blank">正常链接</a></p>',
          '<img src="https://i.typlog.com/cover.png" onerror="alert(1)" style="background:url(javascript:alert(1))">',
          '<img src="https://evil.example/track.png">',
          '<script>alert(1)</script><iframe src="https://evil.example/embed"></iframe>',
        ].join(" ")}
      />,
    );

    expect(screen.getByRole("heading", { name: "节目简介" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "正常链接" })).toHaveAttribute(
      "href",
      "https://example.com",
    );
    expect(container.querySelector("script")).toBeNull();
    expect(container.querySelector("iframe")).toBeNull();
    expect(container.querySelector("[onerror]")).toBeNull();
    expect(container.querySelector("[style]")).toBeNull();
    expect(container.querySelector('img[src*="evil.example"]')).toBeNull();
    const approvedImage = container.querySelector("img");
    const optimizerUrl = new URL(
      approvedImage?.getAttribute("src") ?? "",
      "http://localhost",
    );
    expect(optimizerUrl.pathname).toBe("/_next/image");
    expect(optimizerUrl.searchParams.get("url")).toBe(
      "/images/proxy?url=https%3A%2F%2Fi.typlog.com%2Fcover.png",
    );
  });
});
