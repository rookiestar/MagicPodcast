import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import RichText from "../RichText";

describe("RichText", () => {
  it("uses the shared reading scale and supports report and compact densities", () => {
    const { container, rerender } = render(
      <RichText html="<p>阅读正文</p>" className="custom-class" />,
    );

    expect(container.firstElementChild).toHaveClass(
      "rich-text",
      "editorial-rich-text",
      "editorial-rich-text--reading",
      "custom-class",
    );
    expect(container.firstElementChild).not.toHaveClass("prose");

    rerender(<RichText html="<p>报告正文</p>" density="report" />);

    expect(container.firstElementChild).toHaveClass(
      "editorial-rich-text",
      "editorial-rich-text--report",
    );
    expect(container.firstElementChild).not.toHaveClass(
      "editorial-rich-text--reading",
    );

    rerender(<RichText html="<p>紧凑正文</p>" density="compact" />);

    expect(container.firstElementChild).toHaveClass(
      "editorial-rich-text",
      "editorial-rich-text--compact",
    );
    expect(container.firstElementChild).not.toHaveClass(
      "editorial-rich-text--reading",
    );
  });

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
    expect(optimizerUrl.pathname).toBe("/_next/image.webp");
    expect(optimizerUrl.searchParams.get("url")).toBe(
      "/images/proxy?url=https%3A%2F%2Fi.typlog.com%2Fcover.png",
    );
  });
});
