import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ReportModal from "@/components/workflows/ReportModal";
import { reportStatsSamples } from "@/lib/reportStatsSamples";

const getMock = vi.fn();

vi.mock("@/lib/api/client", () => ({
  api: {
    get: (...args: unknown[]) => getMock(...args),
  },
}));

vi.mock("@/lib/api", () => ({
  workflowApi: {
    regenerateLLMSummary: vi.fn(),
  },
}));

vi.mock("@/components/workflows/MarkdownViewer", () => ({
  default: ({ content }: { content: string }) => (
    <div data-testid="markdown-body">{content}</div>
  ),
}));

vi.mock("@/lib/toast", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

describe("ReportModal stats line", () => {
  beforeEach(() => {
    getMock.mockReset();
  });

  it.each(reportStatsSamples)(
    "shows the same compact stats line for $name",
    async (sample) => {
      getMock.mockResolvedValue({
        data: {
          data: {
            id: 1,
            job_id: 9,
            title: "执行报告",
            content: sample.content,
            summary: "摘要",
            episodes_count: sample.stats.episodes_count,
            podcasts_count: sample.stats.podcasts_count,
            generated_at: "2026-09-10T08:00:00Z",
            format: "markdown",
            file_size: 1200,
            llm_error:
              sample.stats.ai_status === "not_generated" ||
              sample.stats.ai_status === "incomplete"
                ? "empty"
                : "",
            report_stats: sample.stats,
          },
        },
      });

      render(
        <ReportModal isOpen onClose={() => undefined} jobId={9} jobStatus="completed" />,
      );

      const stats = await screen.findByTestId("report-stats-line");
      expect(stats).toHaveTextContent(sample.stats.line);
      expect(stats.textContent).not.toMatch(/(?:^|[^\d])0 Token/);
      expect(screen.getByRole("heading", { name: "执行报告" })).toBeInTheDocument();

      await waitFor(() => {
        const markdown = screen.getByTestId("markdown-body").textContent || "";
        expect(markdown).not.toContain("🕐 执行");
        expect(markdown).not.toContain("📡 Feed覆盖");
        if (sample.content.includes("用户普通引用块")) {
          expect(markdown).toContain("用户普通引用块");
        }
      });
    },
  );
});


it("restores keyboard focus after retry removes the focused button", async () => {
  getMock.mockReset();
  getMock.mockRejectedValueOnce({ response: { status: 503 } });
  getMock.mockResolvedValue({ data: { data: {
    id: 1, job_id: 9, title: "报告", content: "恢复的正文", summary: "",
    episodes_count: 1, podcasts_count: 1, generated_at: "2026-09-12T08:00:00Z",
    format: "markdown", file_size: 10,
  } } });
  const close = vi.fn();
  render(<ReportModal isOpen onClose={close} jobId={9} jobStatus="completed" />);
  const retry = await screen.findByRole("button", { name: "重试" });
  retry.focus();
  fireEvent.click(retry);
  await screen.findByText("恢复的正文");
  await waitFor(() => expect(screen.getByRole("dialog").contains(document.activeElement)).toBe(true));
  fireEvent.keyDown(document.activeElement!, { key: "Escape" });
  expect(close).toHaveBeenCalledTimes(1);
});
