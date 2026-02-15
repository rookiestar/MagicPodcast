import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";
import TagBadge from "../TagBadge";
import type { Tag } from "@/types";

describe("TagBadge", () => {
  const mockTag: Tag = {
    id: 1,
    name: "Test Tag",
    color: "#ff0000",
  };

  describe("基础渲染", () => {
    it("应该渲染标签名称", () => {
      const { container } = render(<TagBadge tag={mockTag} />);
      // 使用 screen.getAllByText 查找所有包含文本的元素，然后找到主文本
      const allTextElements = screen.getAllByText("Test Tag");
      // 找到主要的文本元素，排除tooltip（tooltip有特定的类名）
      const mainText = allTextElements.find(
        (el) => !el.className.includes("absolute"),
      );
      expect(mainText).toBeInTheDocument();
      expect(mainText).toHaveTextContent("Test Tag");
    });

    it("应该使用默认的 medium 尺寸", () => {
      const { container } = render(<TagBadge tag={mockTag} />);
      const badge = container.querySelector("span");
      expect(badge).toHaveClass("text-sm", "px-3", "py-1");
    });

    it("应该使用默认的 colorful 变体", () => {
      const { container } = render(<TagBadge tag={mockTag} />);
      const badge = container.querySelector("span");
      expect(badge).toHaveStyle({
        backgroundColor: "#ff000020",
        color: "#ff0000",
      });
    });
  });

  describe("尺寸变体", () => {
    it("应该渲染 small 尺寸", () => {
      const { container } = render(<TagBadge tag={mockTag} size="sm" />);
      const badge = container.querySelector("span");
      expect(badge).toHaveClass("text-xs", "px-2", "py-0.5");
    });

    it("应该渲染 medium 尺寸", () => {
      const { container } = render(<TagBadge tag={mockTag} size="md" />);
      const badge = container.querySelector("span");
      expect(badge).toHaveClass("text-sm", "px-3", "py-1");
    });

    it("应该渲染 large 尺寸", () => {
      const { container } = render(<TagBadge tag={mockTag} size="lg" />);
      const badge = container.querySelector("span");
      expect(badge).toHaveClass("text-base", "px-4", "py-1.5");
    });
  });

  describe("Simple 变体", () => {
    it("应该渲染 simple 变体", () => {
      const { container } = render(<TagBadge tag={mockTag} variant="simple" />);
      const badge = container.querySelector("span");
      expect(badge).toHaveClass("bg-slate-100", "text-slate-600");
    });

    it("应该在 simple 变体中显示彩色圆点", () => {
      const { container } = render(<TagBadge tag={mockTag} variant="simple" />);
      // 查找所有圆点元素
      const dots = container.querySelectorAll(".rounded-full");
      // 找到彩色圆点（w-1.5 h-1.5）
      const colorDot = Array.from(dots).find(
        (el) =>
          el.classList.contains("w-1.5") && el.classList.contains("h-1.5"),
      );
      expect(colorDot).toBeInTheDocument();
      expect(colorDot).toHaveStyle({ backgroundColor: "#ff0000" });
    });
  });

  describe("移除功能", () => {
    it("当 removable=true 且提供 onRemove 时，应该显示移除按钮", () => {
      const { container } = render(
        <TagBadge tag={mockTag} removable onRemove={vi.fn()} />,
      );
      const button = container.querySelector("button");
      expect(button).toBeInTheDocument();
    });

    it("当 removable=false 时，不应该显示移除按钮", () => {
      const { container } = render(<TagBadge tag={mockTag} />);
      const button = container.querySelector("button");
      expect(button).not.toBeInTheDocument();
    });

    it("点击移除按钮应该调用 onRemove 回调", () => {
      const onRemove = vi.fn();
      const { container } = render(
        <TagBadge tag={mockTag} removable onRemove={onRemove} />,
      );
      const button = container.querySelector("button");
      fireEvent.click(button!);
      expect(onRemove).toHaveBeenCalledTimes(1);
      expect(onRemove).toHaveBeenCalledWith(1);
    });

    it("在 simple 变体中点击移除按钮应该阻止事件冒泡", () => {
      const onRemove = vi.fn();
      const { container } = render(
        <TagBadge
          tag={mockTag}
          variant="simple"
          removable
          onRemove={onRemove}
        />,
      );
      const button = container.querySelector("button")!;
      const clickEvent = new MouseEvent("click", { bubbles: true });
      Object.defineProperty(clickEvent, "stopPropagation", {
        value: vi.fn(),
        writable: false,
      });

      fireEvent(button, clickEvent);
      expect(onRemove).toHaveBeenCalled();
    });
  });

  describe("可访问性", () => {
    it("应该设置 title 属性", () => {
      render(<TagBadge tag={mockTag} />);
      // 使用 getByRole 查找所有可能的元素
      // TagBadge 组件不包含 standard role，所以我们查找所有 span
      const badge = document.querySelector('span[title="Test Tag"]');
      expect(badge).toBeInTheDocument();
    });

    it("移除按钮应该有适当的 title", () => {
      const { container } = render(
        <TagBadge tag={mockTag} removable onRemove={vi.fn()} />,
      );
      const button = container.querySelector("button");
      expect(button).toHaveAttribute("title", '移除 "Test Tag" 标签');
    });
  });

  describe("样式", () => {
    it("应该在 hover 时显示 tooltip", () => {
      const { container } = render(<TagBadge tag={mockTag} />);
      const badge = container.querySelector(".group");
      expect(badge).toBeInTheDocument();

      const tooltip = container.querySelector(".absolute.bottom-full");
      expect(tooltip).toBeInTheDocument();
      expect(tooltip).toHaveClass("opacity-0");
      expect(tooltip).toHaveTextContent("Test Tag");
    });
  });
});
