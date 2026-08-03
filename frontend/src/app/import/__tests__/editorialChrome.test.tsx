import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

// PageLayout passthrough: surfaces rootClassName / className / toolbar.className
// so the editorial chrome contract is observable.
vi.mock("@/components/layout/PageLayout", () => ({
  default: ({ rootClassName, className, toolbar, children }: any) => (
    <div className={rootClassName}>
      {toolbar && (
        <div className={toolbar.className} data-testid="toolbar">
          {toolbar.breadcrumbs?.map((item: any) => (
            <a key={item.label} href={item.href}>
              {item.label}
            </a>
          ))}
          {toolbar.title}
          {toolbar.rightContent}
        </div>
      )}
      <div className={className}>{children}</div>
    </div>
  ),
}));

// Drive the page chrome without engaging SSE / localStorage (those flows are
// covered by page.test.tsx). We only need the shell + tab + CTA to mount.
vi.mock("@/hooks/useSyncLogSession", () => ({
  useSyncLogSession: () => ({
    logs: [],
    logMode: "import",
    filter: "all",
    setFilter: vi.fn(),
    restoredMode: null,
    stats: {
      total: 0,
      success: 0,
      errors: 0,
      skips: 0,
      skipNoUpdate: 0,
    },
    filteredLogs: [],
    addLog: vi.fn(),
    startLogSession: vi.fn(),
    clearLogSession: vi.fn(),
  }),
}));

vi.mock("@/hooks/useStableLogScroll", () => ({
  useStableLogScroll: () => ({
    autoScroll: true,
    logContainerRef: { current: null },
    logEndRef: { current: null },
    handleLogScroll: vi.fn(),
    resetLogScroll: vi.fn(),
    resumeAutoScroll: vi.fn(),
  }),
}));

vi.mock("@/hooks/useImportSyncOperations", () => ({
  useImportSyncOperations: () => ({
    file: null,
    importing: false,
    syncing: false,
    handleFileChange: vi.fn(),
    handleImport: vi.fn(),
    handleSync: vi.fn(),
  }),
}));

import ImportPage from "../page";

describe("import page editorial chrome (#53)", () => {
  it("does not repeat the global home navigation in the page toolbar", () => {
    render(<ImportPage />);

    expect(screen.queryByRole("link", { name: "返回首页" })).not.toBeInTheDocument();
  });

  it("adopts the editorial shell, toolbar and import-page scope classes", () => {
    const { container } = render(<ImportPage />);
    expect(container.querySelector(".editorial-page-shell")).toBeTruthy();
    expect(container.querySelector(".editorial-page-toolbar")).toBeTruthy();
    expect(container.querySelector(".import-page")).toBeTruthy();
  });

  it("renders the editorialized import tab and primary CTA", () => {
    const { container } = render(<ImportPage />);
    expect(screen.getByRole("tab", { name: "导入 OPML" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "开始导入" })).toBeTruthy();
    expect(container.querySelector(".import-workspace")).toBeTruthy();
    expect(container.querySelector(".import-operation-panel")).toBeTruthy();
    expect(container.querySelector(".import-log-column")).toBeTruthy();
  });
});
