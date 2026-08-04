import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import LoadingLayout from "../LoadingLayout";

describe("LoadingLayout tone", () => {
  it("keeps the slate baseline by default (no podcast regression)", () => {
    const { container } = render(
      <LoadingLayout title="我的订阅" description="加载中...">
        <div>children</div>
      </LoadingLayout>,
    );

    expect(container.querySelector(".editorial-loading")).toBeNull();
    expect(container.querySelector(".bg-slate-50")).toBeTruthy();
    // 工具栏骨架沿用 slate 描边
    expect(container.querySelector(".editorial-loading-bar")).toBeNull();
  });

  it("adopts the editorial shell classes when tone is editorial", () => {
    const { container } = render(
      <LoadingLayout tone="editorial" title="标签管理" description="加载中...">
        <div>children</div>
      </LoadingLayout>,
    );

    expect(container.querySelector(".editorial-loading")).toBeTruthy();
    // 顶部导航骨架使用 editorial 条
    expect(container.querySelector(".editorial-loading-bar.is-navbar")).toBeTruthy();
    // 工具栏骨架使用 editorial 条（带 2px 墨线）
    const bars = container.querySelectorAll(".editorial-loading-bar");
    expect(bars.length).toBeGreaterThanOrEqual(2);
    // 内容块骨架采用暖纸基调
    expect(container.querySelector(".editorial-loading-block")).toBeTruthy();
  });

  it("wraps editorial toolbar actions within a narrow viewport", () => {
    const { container } = render(
      <LoadingLayout
        tone="editorial"
        showBack
        title="加载中..."
        rightContent={
          <div>
            <div>操作一</div>
            <div>操作二</div>
          </div>
        }
      >
        <div>children</div>
      </LoadingLayout>,
    );

    expect(container.querySelector(".editorial-loading-toolbar-row")).toHaveClass(
      "flex-wrap",
    );
    expect(
      container.querySelector(".editorial-loading-toolbar-main"),
    ).toHaveClass("min-w-0");
    expect(
      container.querySelector(".editorial-loading-toolbar-actions"),
    ).toHaveClass("max-w-full");
  });
});
