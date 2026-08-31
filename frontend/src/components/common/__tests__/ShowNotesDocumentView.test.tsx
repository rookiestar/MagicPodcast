import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ShowNotesDocumentView } from "../ShowNotesDocumentView";

describe("ShowNotesDocumentView", () => {
  it("uses the existing safe HTML renderer", () => {
    render(
      <ShowNotesDocumentView
        document={{
          content:
            '<h2>HTML 章节</h2><a href="javascript:alert(1)">危险链接</a>',
          format: "html",
        }}
      />,
    );

    expect(screen.getByRole("heading", { name: "HTML 章节" })).toBeVisible();
    expect(screen.queryByRole("link", { name: "危险链接" })).not.toBeInTheDocument();
  });

  it("uses the existing safe Markdown renderer", () => {
    render(
      <ShowNotesDocumentView
        document={{
          content: "## Markdown 章节\n\n**重点**\n\n---",
          format: "markdown",
        }}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "Markdown 章节" }),
    ).toBeVisible();
    expect(screen.getByText("重点").tagName).toBe("STRONG");
    expect(document.querySelector("hr")).toBeInTheDocument();
  });

  it("renders the caller's explicit empty state", () => {
    render(
      <ShowNotesDocumentView
        document={{ content: " \n ", format: "markdown" }}
        emptyFallback={<p>暂无 Show Notes</p>}
      />,
    );

    expect(screen.getByText("暂无 Show Notes")).toBeVisible();
  });
});
