import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import PageToolbar from "../PageToolbar";

describe("PageToolbar mobile title layout", () => {
  it("keeps a dynamic title inside a shrinkable main row", () => {
    const longTitle = "一个足够长的动态工作流名称，用于验证移动端标题不会撑破页面";

    const { container } = render(
      <PageToolbar
        breadcrumbs={[{ label: "返回列表", href: "/workflows" }]}
        title={longTitle}
        rightContent={<button type="button">更多</button>}
      />,
    );

    const title = screen.getAllByRole("heading", { name: longTitle })[0];
    for (const renderedTitle of screen.getAllByRole("heading", {
      name: longTitle,
    })) {
      expect(renderedTitle).toHaveAttribute(
        "data-editorial-display-text",
        "true",
      );
    }
    const mainRow = title.closest(".page-toolbar-mobile-main");

    expect(mainRow).not.toBeNull();
    expect(mainRow).toHaveClass("min-w-0");
    expect(title).toHaveClass("min-w-0");
    expect(container.querySelector(".page-toolbar-mobile-main")).toHaveClass(
      "flex",
    );
    expect(container.querySelector(".page-toolbar-title-group")).toHaveClass(
      "editorial-title-group",
    );
  });

  it("keeps an opted-in mobile description with the title", () => {
    const { container } = render(
      <PageToolbar
        title="我的订阅"
        description="共 487 个节目"
        mobileDescription="共 487 个节目"
      />,
    );

    expect(
      container.querySelector(".page-toolbar-mobile-description"),
    ).toHaveTextContent("共 487 个节目");
    expect(
      container.querySelector(".page-toolbar-desktop .page-toolbar-description"),
    ).toHaveTextContent("共 487 个节目");
  });
});
