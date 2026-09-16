import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("next/navigation", () => ({
  useParams: () => ({ id: "1" }),
  useSearchParams: () => new URLSearchParams(),
  usePathname: () => "/workflows/1",
  useRouter: () => ({
    push: vi.fn(),
    replace: vi.fn(),
    prefetch: vi.fn(),
    back: vi.fn(),
  }),
}));

// Expose modal open state without loading the dynamically-imported form.
vi.mock("next/dynamic", () => ({
  default: () =>
    ({ isOpen }: { isOpen?: boolean }) =>
      isOpen ? <div role="dialog">编辑工作流弹窗</div> : null,
}));

vi.mock("@/components/layout/PageLayout", () => ({
  default: ({ rootClassName, className, toolbar, children }: any) => (
    <div className={rootClassName}>
      {toolbar && (
        <div className={toolbar.className} data-testid="toolbar">
          {toolbar.title}
          <div data-testid="toolbar-mobile">{toolbar.rightContent}</div>
          <div data-testid="toolbar-desktop">{toolbar.rightContent}</div>
        </div>
      )}
      <div className={className}>{children}</div>
    </div>
  ),
}));

// Mutable hook state so a single suite can exercise the ready + error branches.
const state = {
  workflow: {
    id: 1,
    name: "编辑工作流",
    description: "详情编辑感验收",
    schedule: "0 8 * * *",
    scope_type: "all_subscribed",
    scope_config: {},
    rules_config: {},
    is_enabled: true,
    stats: { total_jobs: 1, total_episodes: 2 },
  } as any,
  isLoading: false,
  isError: false,
};

vi.mock("@/hooks/useWorkflowSWR", () => ({
  useWorkflow: () => ({
    workflow: state.workflow,
    isLoading: state.isLoading,
    isError: state.isError,
    mutate: vi.fn(),
  }),
  useWorkflowJobs: () => ({
    jobs: [],
    pagination: null,
    isLoading: false,
    isError: false,
    mutate: vi.fn(),
  }),
}));

vi.mock("@/hooks/useWorkflowActions", () => ({
  useWorkflowActions: () => ({
    handleToggle: vi.fn(),
    handleTrigger: vi.fn(),
    handleDelete: vi.fn(),
  }),
}));

vi.mock("@/hooks/useJobExpansion", () => ({
  useJobExpansion: () => ({
    selectedJobId: null,
    jobDetails: {},
    loadingJobId: null,
    fetchJobDetail: vi.fn(),
  }),
}));

vi.mock("@/lib/prefetch", () => ({
  prefetchWorkflowJobsSummary: vi.fn(),
}));
vi.mock("@/lib/api", () => ({ podcastApi: { batchGet: vi.fn() } }));
vi.mock("@/lib/api/workflow", () => ({ workflowApi: {} }));
vi.mock("@/lib/api/scheduler", () => ({ schedulerApi: { reload: vi.fn() } }));
vi.mock("@/components/podcasts/PodcastCover", () => ({
  default: () => <div data-testid="cover" />,
}));
vi.mock("@/components/layout/LoadingLayout", () => ({
  default: ({ children }: any) => <div>{children}</div>,
}));
vi.mock("@/components/ui/Skeleton", () => ({
  WorkflowDetailSkeleton: () => <div data-testid="skeleton" />,
}));
vi.mock("@/components/ui/StatusBadge", () => ({
  WorkflowStatusBadge: () => <span data-testid="wb" />,
  JobStatusBadge: () => <span data-testid="jb" />,
}));

import WorkflowDetailPage from "../page";

describe("workflow detail editorial chrome (#53)", () => {
  it("adopts the editorial shell, toolbar, section + editorial tabs with standard tab semantics", () => {
    state.workflow = {
      id: 1,
      name: "编辑工作流",
      description: "详情编辑感验收",
      schedule: "0 8 * * *",
      scope_type: "all_subscribed",
      scope_config: {},
      rules_config: {},
      is_enabled: true,
      stats: { total_jobs: 1, total_episodes: 2 },
    };
    state.isLoading = false;
    state.isError = false;

    const { container } = render(<WorkflowDetailPage />);

    expect(container.querySelector(".editorial-page-shell")).toBeTruthy();
    expect(container.querySelector(".workflow-detail")).toBeTruthy();
    expect(container.querySelector(".editorial-page-toolbar")).toBeTruthy();

    const tabs = container.querySelector('.editorial-tabs');
    expect(tabs).toBeTruthy();
    expect(tabs?.getAttribute("role")).toBe("tablist");

    const overview = screen.getByRole("tab", { name: /概览/ });
    expect(overview.className).toContain("editorial-tab");
    expect(overview.getAttribute("aria-selected")).toBe("true");
    expect(overview.getAttribute("aria-controls")).toBeTruthy();

    const jobs = screen.getByRole("tab", { name: /执行历史/ });
    expect(jobs.getAttribute("aria-selected")).toBe("false");
  });

  it("keeps execution visible and secondary actions in the menu", () => {
    render(<WorkflowDetailPage />);
    expect(screen.getAllByRole("button", { name: "执行" }).length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: "删除" })).not.toBeInTheDocument();
    const more = screen.getAllByRole("button", { name: /更多操作/ });
    fireEvent.click(more[0]);
    expect(screen.getByRole("button", { name: "删除" })).toBeInTheDocument();
    fireEvent.keyDown(screen.getByRole("button", { name: "删除" }), { key: "Escape" });
    expect(screen.queryByRole("button", { name: "删除" })).not.toBeInTheDocument();
    expect(more[0]).toHaveFocus();
  });

  it("opens edit modal from the shared action menu", () => {
    render(<WorkflowDetailPage />);
    fireEvent.click(screen.getAllByRole("button", { name: /更多操作/ })[0]);
    fireEvent.click(screen.getByRole("button", { name: "编辑" }));
    expect(screen.getByRole("dialog")).toHaveTextContent("编辑工作流弹窗");
  });

  it("renders the editorial error state when the workflow fails to load", () => {
    state.workflow = null;
    state.isLoading = false;
    state.isError = true;

    const { container } = render(<WorkflowDetailPage />);

    expect(
      container.querySelector(".editorial-state.is-error"),
    ).toBeTruthy();
    // Error heading + message both render the failure copy.
    expect(screen.getAllByText(/加载失败/).length).toBeGreaterThan(0);
    expect(
      container.querySelector(".editorial-btn--ghost"),
    ).toBeTruthy();
  });
});
