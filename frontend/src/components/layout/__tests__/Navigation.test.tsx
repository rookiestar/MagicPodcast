import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import AppNavbar from "../AppNavbar";
import MobileBottomNav from "../MobileBottomNav";
import PageToolbar from "../PageToolbar";

vi.mock("next/navigation", () => ({
  usePathname: () => "/",
}));

const forbiddenEmoji = ["🏠", "🎙️", "🏷️", "⚡", "📥", "🔍", "🔄"];

describe("global navigation", () => {
  it("keeps core features reachable with text and non-color active state", () => {
    const { container } = render(
      <>
        <AppNavbar />
        <MobileBottomNav />
      </>,
    );

    for (const label of ["首页", "播客", "标签", "工作流", "导入"]) {
      expect(screen.getAllByRole("link", { name: label })).toHaveLength(2);
    }
    expect(screen.getAllByRole("button", { name: "搜索" })).toHaveLength(2);
    const desktopSearch = container.querySelector(".app-navbar-search");
    expect(desktopSearch).not.toBeNull();
    expect(desktopSearch).toHaveTextContent("");
    expect(desktopSearch?.querySelector("svg")).toHaveAttribute(
      "aria-hidden",
      "true",
    );
    expect(
      screen.getAllByRole("link", { name: "首页" })[0],
    ).toHaveAttribute("aria-current", "page");
    for (const emoji of forbiddenEmoji) {
      expect(container).not.toHaveTextContent(emoji);
    }
  });

  it("gives editorial page headers a shared alignment contract", () => {
    const { container } = render(
      <PageToolbar
        breadcrumbs={[{ label: "返回首页", href: "/" }]}
        title="我的订阅"
        description="共 4 个节目"
        className="editorial-page-toolbar"
      />,
    );

    expect(container.querySelector(".page-toolbar-desktop")).toHaveClass(
      "editorial-inset-rule",
    );
    expect(
      screen.getAllByRole("heading", { name: "我的订阅" })[1],
    ).toHaveClass("editorial-section-title");
    expect(container.querySelector(".page-toolbar-title-group")).not.toBeNull();
  });
});
