import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Workflow } from "@/types";

// PageLayout passthrough: surfaces rootClassName / className / toolbar.className
// + toolbar.rightContent so the editorial chrome contract is observable.
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
          {toolbar.rightContent}
        </div>
      )}
      <div className={className}>{children}</div>
    </div>
  ),
}));

// Kill the dynamically-imported wizard modal in unit tests.
vi.mock("next/dynamic", () => ({ default: () => () => null }));

let workflowsValue: Workflow[] = [];
const mutate = vi.fn();
vi.mock("@/hooks/useWorkflowSWR", () => ({
  useWorkflows: () => ({
    workflows: workflowsValue,
    isLoading: false,
    isError: false,
    mutate,
  }),
}));

vi.mock("@/lib/api", () => ({ workflowApi: {} }));
vi.mock("@/lib/api/errorHandler", () => ({ showSuccess: vi.fn() }));
vi.mock("@/lib/confirmation", () => ({
  requestTypedConfirmation: vi.fn(() => null),
}));
vi.mock("@/components/common/PrefetchLink", () => ({
  default: ({ children, prefetchType, prefetchId, prefetch, ...rest }: any) => (
    <a {...rest}>{children}</a>
  ),
}));
vi.mock("@/components/ui/StatusBadge", () => ({
  WorkflowStatusBadge: () => <span data-testid="status-badge" />,
}));
vi.mock("@/components/workflows/WorkflowActionMenu", () => ({
  default: () => <div data-testid="action-menu" />,
}));
vi.mock("@/lib/timeUtils", () => ({ formatDateTime: () => "时间" }));

import WorkflowsPage from "../page";

describe("workflows list editorial chrome (#53)", () => {
  it("does not repeat the global home navigation in the page toolbar", () => {
    render(<WorkflowsPage />);

    expect(screen.queryByRole("link", { name: "返回首页" })).not.toBeInTheDocument();
  });

  it("adopts the editorial shell, toolbar, section classes and a primary create button", () => {
    workflowsValue = [];
    const { container } = render(<WorkflowsPage />);

    expect(container.querySelector(".editorial-page-shell")).toBeTruthy();
    expect(container.querySelector(".editorial-page-toolbar")).toBeTruthy();
    expect(container.querySelector(".workflow-page")).toBeTruthy();

    const createBtn = screen.getByRole("button", { name: "+ 创建工作流" });
    expect(createBtn.className).toContain("editorial-btn--primary");

    // Editorial empty state with a create CTA.
    const state = screen.getByText("暂无工作流").closest(".editorial-state");
    expect(state).toBeTruthy();
    expect(screen.getByRole("button", { name: "创建工作流" })).toBeTruthy();

    // Workflow sorting follows the established podcast sorting interaction.
    expect(container.querySelector(".podcast-sort-desktop")).toBeTruthy();
    expect(container.querySelector(".podcast-sort-desktop-icon")).toBeTruthy();
    expect(screen.getByRole("combobox", { name: "排序方式" })).toBeInTheDocument();

    const mobileSortButton = screen.getByRole("button", { name: "排序" });
    fireEvent.click(mobileSortButton);
    expect(
      screen.getByRole("dialog", { name: "排序方式" }),
    ).toBeInTheDocument();
    const nextExecutionOption = screen.getByRole("button", {
      name: "下次执行",
    });
    expect(nextExecutionOption).toBeInTheDocument();
    fireEvent.click(nextExecutionOption);
    expect(screen.getByRole("combobox", { name: "排序方式" })).toHaveValue(
      "execution",
    );
    expect(window.location.search).toBe("?sort_by=execution");
    window.history.replaceState({}, "", "/");
  });

  it("renders workflow cards with the editorial card class", () => {
    workflowsValue = [
      {
        id: 1,
        name: "每日精选",
        description: "",
        schedule: "0 8 * * *",
        scope_type: "all_subscribed",
        scope_config: {},
        rules_config: {},
        is_enabled: true,
        stats: { total_jobs: 1, total_episodes: 2 },
      } as unknown as Workflow,
    ];
    const { container } = render(<WorkflowsPage />);
    expect(container.querySelector(".workflow-list")).toBeTruthy();
    expect(container.querySelector(".workflow-card")).toBeTruthy();
    expect(container.querySelector(".workflow-card-body")).toBeTruthy();
    expect(container.querySelector(".workflow-card-metadata")).toBeTruthy();
    expect(container.querySelector(".workflow-card-actions")).toBeTruthy();
  });
});
