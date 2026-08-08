import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import MarkdownViewer from "../MarkdownViewer";

describe("MarkdownViewer", () => {
  it("uses the shared rich-text scale without a second prose system", () => {
    const { container, rerender } = render(
      <MarkdownViewer content="# 阅读报告" className="custom-class" />,
    );

    expect(container.firstElementChild).toHaveClass(
      "editorial-rich-text",
      "editorial-rich-text--reading",
      "custom-class",
    );
    expect(container.firstElementChild).not.toHaveClass("prose");
    expect(screen.getByRole("heading", { name: "阅读报告" })).not.toHaveClass(
      "text-2xl",
    );

    rerender(<MarkdownViewer content="# 紧凑报告" density="compact" />);

    expect(container.firstElementChild).toHaveClass(
      "editorial-rich-text",
      "editorial-rich-text--compact",
    );
  });

  it("removes raw active HTML and unapproved images while keeping Markdown", () => {
    const { container } = render(
      <MarkdownViewer
        content={[
          "# 安全报告",
          "",
          "- 正常列表项",
          "",
          "[正常链接](https://example.com/article)",
          "",
          "![恶意图片](https://evil.example/track.png)",
          "",
          '<script>window.__xss = true</script><img src="x" onerror="alert(1)">',
          "",
          "[危险链接](javascript:alert(1))",
        ].join("\n")}
      />,
    );

    expect(screen.getByRole("heading", { name: "安全报告" })).toBeInTheDocument();
    expect(screen.getByText("正常列表项")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "正常链接" })).toHaveAttribute(
      "href",
      "https://example.com/article",
    );
    expect(container.querySelector("script")).toBeNull();
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("[onerror]")).toBeNull();
    expect(screen.queryByRole("link", { name: "危险链接" })).toBeNull();
  });

  it("keeps a reviewed image source behind the bounded proxy", () => {
    render(
      <MarkdownViewer content="![封面](https://i.typlog.com/cover.png)" />,
    );

    const optimizerUrl = new URL(
      screen.getByAltText("封面").getAttribute("src") ?? "",
      "http://localhost",
    );
    expect(optimizerUrl.pathname).toBe("/_next/image");
    expect(optimizerUrl.searchParams.get("url")).toBe(
      "/images/proxy?url=https%3A%2F%2Fi.typlog.com%2Fcover.png",
    );
  });

  it("keeps bounded report QR images as inline PNGs", () => {
    render(
      <MarkdownViewer content="![二维码](data:image/png;base64,abc123=)" />,
    );

    expect(screen.getByAltText("二维码")).toHaveAttribute(
      "src",
      "data:image/png;base64,abc123=",
    );
  });
});
