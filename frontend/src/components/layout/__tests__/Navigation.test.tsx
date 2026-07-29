import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import AppNavbar from "../AppNavbar";
import MobileBottomNav from "../MobileBottomNav";

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
    expect(
      screen.getAllByRole("link", { name: "首页" })[0],
    ).toHaveAttribute("aria-current", "page");
    for (const emoji of forbiddenEmoji) {
      expect(container).not.toHaveTextContent(emoji);
    }
  });
});
