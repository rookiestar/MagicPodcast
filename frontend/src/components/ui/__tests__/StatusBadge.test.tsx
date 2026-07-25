import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { JobStatusBadge } from "../StatusBadge";

describe("JobStatusBadge", () => {
  it("shows partial jobs as partially completed instead of pending", () => {
    render(<JobStatusBadge status="partial" />);

    expect(screen.getByText("部分完成")).toBeInTheDocument();
    expect(screen.queryByText("等待中")).not.toBeInTheDocument();
  });

  it("shows finalizing jobs as report generation", () => {
    render(<JobStatusBadge status="finalizing" />);

    expect(screen.getByText("生成报告")).toBeInTheDocument();
  });

  it("does not disguise an unknown backend status as pending", () => {
    render(<JobStatusBadge status="backend_status_drift" />);

    expect(screen.getByText("状态未知")).toBeInTheDocument();
    expect(screen.queryByText("等待中")).not.toBeInTheDocument();
  });
});
