import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { EpisodeShowNotes } from "../EpisodeShowNotes";

function Reader({ status = "success" }: { status?: "success" | "loading" | "error" }) {
  const [open, setOpen] = useState(false);
  return <EpisodeShowNotes title="测试单集" summary="三行摘要" link="" isExpanded={open}
    status={status} document={{content:"<h2>正文标题</h2><p>全文末段</p>",format:"html"}}
    onToggle={() => setOpen(v => !v)} onRetry={() => {}} />;
}
describe("EpisodeShowNotes reader", () => {
  it("keeps the card summary and opens a named modal only on request", () => {
    render(<Reader />);
    fireEvent.mouseEnter(screen.getByText("三行摘要"));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", {name:"阅读简介"}));
    const dialog = screen.getByRole("dialog", {name:"测试单集"});
    expect(within(dialog).getByText("全文末段")).toBeVisible();
    expect(screen.getByText("三行摘要")).toBeVisible();
    expect(screen.getByRole("button", {name:"关闭简介"})).toHaveFocus();
    fireEvent.keyDown(screen.getByRole("button", {name:"关闭简介"}), {key:"Tab", shiftKey:true});
    expect(screen.getByRole("region", {name:"完整 Show Notes"})).toHaveFocus();
    fireEvent.keyDown(screen.getByRole("region", {name:"完整 Show Notes"}), {key:"Tab"});
    expect(screen.getByRole("button", {name:"关闭简介"})).toHaveFocus();
  });
  it.each(["button", "cancel", "backdrop"])("closes by %s and restores focus and body scroll", async method => {
    render(<Reader />);
    const trigger = screen.getByRole("button", {name:"阅读简介"});
    const overflow = document.body.style.overflow;
    fireEvent.click(trigger);
    const dialog = screen.getByRole("dialog");
    if (method === "button") fireEvent.click(screen.getByRole("button", {name:"关闭简介"}));
    else if (method === "cancel") fireEvent(dialog, new Event("cancel", {cancelable:true}));
    else fireEvent.click(dialog, {clientX:-1, clientY:-1});
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
    expect(document.body.style.overflow).toBe(overflow);
  });
  it("preserves readable summary during loading and failure and retries", () => {
    const retry = vi.fn();
    const {rerender} = render(<EpisodeShowNotes title="慢单集" summary="摘要" link="" isExpanded status="loading" onToggle={() => {}} onRetry={retry}/>);
    expect(within(screen.getByRole("dialog")).getByText("摘要")).toBeVisible();
    expect(screen.getByRole("status")).toBeVisible();
    rerender(<EpisodeShowNotes title="慢单集" summary="摘要" link="" isExpanded status="error" onToggle={() => {}} onRetry={retry}/>);
    expect(within(screen.getByRole("dialog")).getByText("摘要")).toBeVisible();
    fireEvent.click(screen.getByRole("button", {name:"重试全文"}));
    expect(retry).toHaveBeenCalledTimes(1);
  });
});
