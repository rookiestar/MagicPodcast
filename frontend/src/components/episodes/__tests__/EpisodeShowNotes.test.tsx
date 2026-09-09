import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
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
        onToggle={() => undefined}
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
    ).not.toHaveAttribute("tabindex");
    expect(container.querySelector(".bg-gradient-to-t")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "收起" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
  });

  it("keeps the summary until an explicit toggle and does not collapse on hover", () => {
    const onToggle = vi.fn();
    render(
      <EpisodeShowNotes
        summary="三行摘要"
        link=""
        isExpanded={false}
        status="idle"
        onToggle={onToggle}
        onRetry={() => undefined}
      />,
    );

    expect(screen.getByText("三行摘要")).toBeVisible();
    expect(screen.getByRole("button", { name: "展开简介" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    fireEvent.mouseEnter(screen.getByText("三行摘要"));
    expect(onToggle).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "展开简介" }));
    expect(onToggle).toHaveBeenCalledTimes(1);
  });

  it("keeps the summary visible while loading or retrying", () => {
    const onRetry = vi.fn();
    const { rerender } = render(
      <EpisodeShowNotes
        summary="失败前的摘要"
        link=""
        isExpanded
        status="loading"
        onToggle={() => undefined}
        onRetry={onRetry}
      />,
    );

    expect(screen.getByText("失败前的摘要")).toBeVisible();
    expect(screen.getByRole("status")).toHaveTextContent("正在读取完整");

    rerender(
      <EpisodeShowNotes
        summary="失败前的摘要"
        link=""
        isExpanded
        status="error"
        onToggle={() => undefined}
        onRetry={onRetry}
      />,
    );
    expect(screen.getByText("失败前的摘要")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "重试全文" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("animates from the height stored on the previous commit, not a second post-commit measurement", () => {
    let bodyHeight = 80;
    const originalGetBoundingClientRect =
      HTMLElement.prototype.getBoundingClientRect;
    const getBoundingClientRect = vi
      .spyOn(HTMLElement.prototype, "getBoundingClientRect")
      .mockImplementation(function (this: HTMLElement) {
        const rect = originalGetBoundingClientRect.call(this);
        if (this.classList.contains("podcast-episode-show-notes-body")) {
          return {
            x: rect.x,
            y: rect.y,
            width: rect.width,
            height: bodyHeight,
            top: rect.top,
            left: rect.left,
            bottom: rect.top + bodyHeight,
            right: rect.right,
            toJSON() {
              return {};
            },
          } as DOMRect;
        }
        return rect;
      });
    Object.defineProperty(window, "matchMedia", {
      configurable: true,
      value: vi.fn((query: string) => ({
        matches: query.includes("prefers-reduced-motion") ? false : false,
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    });

    const { container, rerender } = render(
      <EpisodeShowNotes
        summary="三行摘要"
        link=""
        isExpanded={false}
        status="idle"
        onToggle={() => undefined}
        onRetry={() => undefined}
      />,
    );

    const body = container.querySelector(
      ".podcast-episode-show-notes-body",
    ) as HTMLElement;
    expect(body).not.toBeNull();

    bodyHeight = 240;
    rerender(
      <EpisodeShowNotes
        summary="三行摘要"
        link=""
        isExpanded
        status="success"
        document={{
          content: "<h2>章节</h2><p>全文段落</p>",
          format: "html",
        }}
        onToggle={() => undefined}
        onRetry={() => undefined}
      />,
    );

    expect(body.style.height).toBe("240px");
    getBoundingClientRect.mockRestore();
  });
});
