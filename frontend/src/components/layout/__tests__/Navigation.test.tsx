import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import AppNavbar from "../AppNavbar";
import MobileBottomNav from "../MobileBottomNav";
import PageToolbar from "../PageToolbar";

vi.mock("next/navigation", () => ({
  usePathname: () => "/",
}));

vi.mock("@/lib/api/consumption", () => ({
  consumptionApi: {
    getSummary: vi.fn().mockResolvedValue({
      counts: { inbox: 0, focus: 1, someday: 0, done: 4 },
      focus_limit: 7,
      focus_over_limit: false,
    }),
  },
}));

const forbiddenEmoji = ["🏠", "🎙️", "🏷️", "⚡", "📥", "🔍", "🔄"];

describe("global navigation", () => {
  it("keeps core features reachable with one desktop Inbox and five mobile destinations", async () => {
    const { container } = render(
      <>
        <AppNavbar />
        <MobileBottomNav />
      </>,
    );

    for (const label of ["首页", "播客", "标签", "工作流"]) {
      expect(screen.getAllByRole("link", { name: label })).toHaveLength(2);
    }
    expect(screen.getByRole("link", { name: "导入" })).toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: "Inbox" })).toHaveLength(2);
    expect(screen.getAllByRole("button", { name: "搜索" })).toHaveLength(2);
    expect(await screen.findByTitle("0 项待处理")).toHaveTextContent("0");
    const desktopSearch = container.querySelector(".app-navbar-search");
    expect(desktopSearch).not.toBeNull();
    expect(desktopSearch).toHaveTextContent("");
    expect(desktopSearch?.querySelector("svg")).toHaveAttribute(
      "aria-hidden",
      "true",
    );
    expect(
      container.querySelector(
        ".mobile-bottom-nav > div > .mobile-global-search",
      ),
    ).not.toBeNull();
    expect(screen.getAllByRole("link", { name: "首页" })[0]).toHaveAttribute(
      "aria-current",
      "page",
    );
    for (const emoji of forbiddenEmoji) {
      expect(container).not.toHaveTextContent(emoji);
    }
  });

  it("gives editorial page headers a shared alignment contract", () => {
    const { container } = render(
      <PageToolbar
        breadcrumbs={[{ label: "返回列表", href: "/podcasts" }]}
        title="我的订阅"
        description="共 4 个节目"
        className="editorial-page-toolbar"
      />,
    );

    expect(container.querySelector(".page-toolbar-desktop")).toHaveClass(
      "editorial-inset-rule",
    );
    expect(screen.getAllByRole("heading", { name: "我的订阅" })[1]).toHaveClass(
      "editorial-section-title",
    );
    expect(container.querySelector(".page-toolbar-title-group")).not.toBeNull();
    expect(screen.getByRole("link", { name: "返回列表" })).toHaveAttribute(
      "href",
      "/podcasts",
    );
  });
});
