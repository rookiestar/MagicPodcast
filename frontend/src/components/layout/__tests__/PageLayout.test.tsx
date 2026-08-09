import { render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import PageLayout from "../PageLayout";

vi.mock("../AppNavbar", () => ({
  default: () => <div data-testid="navbar" />,
}));

vi.mock("../MobileBottomNav", () => ({
  default: () => <div data-testid="mobile-nav" />,
}));

vi.mock("../PageToolbar", () => ({
  default: () => <div data-testid="toolbar" />,
}));

vi.mock("@/components/SearchSidebar", () => ({
  default: () => <div data-testid="search-sidebar" />,
}));

vi.mock("@/contexts/SearchContext", () => ({
  useSearch: () => ({
    isSearchOpen: false,
    openSearch: vi.fn(),
    closeSearch: vi.fn(),
  }),
}));

describe("PageLayout background", () => {
  it("does not let the default slate background override a custom root background", () => {
    const { container } = render(
      <PageLayout toolbar={false} rootClassName="editorial-page-shell">
        <div>content</div>
      </PageLayout>,
    );

    expect(container.firstElementChild).toHaveClass("editorial-page-shell");
    expect(container.firstElementChild).not.toHaveClass("bg-slate-50");
  });

  it("inlines the editorial first-paint canvas before external CSS arrives", () => {
    const { container } = render(
      <PageLayout toolbar={false} rootClassName="editorial-page-shell">
        <div>content</div>
      </PageLayout>,
    );

    const fallback = container.querySelector(
      "style[data-editorial-page-fallback]",
    );
    expect(fallback).toBeTruthy();
    expect(fallback?.textContent).toContain("background-color: #f7f1e5");
    expect(fallback?.textContent).toContain("min-height: 100vh");
  });

  it("keeps the slate background when no custom root background is supplied", () => {
    const { container } = render(
      <PageLayout toolbar={false}>
        <div>content</div>
      </PageLayout>,
    );

    expect(container.firstElementChild).toHaveClass("bg-slate-50");
  });
});
