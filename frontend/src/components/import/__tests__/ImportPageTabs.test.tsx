import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import ImportPageTabs from "../ImportPageTabs";

describe("ImportPageTabs", () => {
  it("removes hover styling while the import operation is running", () => {
    render(
      <ImportPageTabs
        activeTab="import"
        disabled
        onChange={vi.fn()}
      />,
    );

    const syncTab = screen.getByRole("tab", { name: "同步元数据" });
    expect(syncTab).toBeDisabled();
    expect(syncTab).toHaveClass("cursor-not-allowed");
    expect(syncTab.className).not.toContain("hover:");
  });

  it("keeps hover styling on the enabled inactive tab", () => {
    render(
      <ImportPageTabs
        activeTab="import"
        disabled={false}
        onChange={vi.fn()}
      />,
    );

    const syncTab = screen.getByRole("tab", { name: "同步元数据" });
    expect(syncTab).not.toBeDisabled();
    expect(syncTab.className).toContain("hover:");
  });
});
