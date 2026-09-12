import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import WorkflowFormModal from "../WorkflowFormModal";
import { navigate } from "@/lib/navigation";

afterEach(()=>{vi.unstubAllGlobals();window.history.replaceState({},"","/");});
it("preserves an unsaved workflow form when close or URL navigation is rejected",async()=>{
  window.history.replaceState({},"","/workflows?dialog=create");
  const close=vi.fn();
  const confirm=vi.fn(()=>false);
  vi.stubGlobal("confirm",confirm);
  render(<WorkflowFormModal isOpen onClose={close} onSuccess={()=>{}} />);
  const input=screen.getByPlaceholderText("例如: 每日科技播客抓取");
  await waitFor(()=>expect(input).toHaveValue(""));
  fireEvent.change(input,{target:{value:"未保存的 URL 工作流"}});
  fireEvent.click(screen.getByRole("button",{name:"关闭"}));
  expect(confirm).toHaveBeenCalledTimes(1);
  expect(close).not.toHaveBeenCalled();
  expect(input).toHaveValue("未保存的 URL 工作流");
  expect(navigate("/workflows")).toBe(false);
  expect(window.location.search).toBe("?dialog=create");
});
it("closes an unchanged form without a false unsaved warning",()=>{
  const close=vi.fn();const confirm=vi.fn(()=>false);vi.stubGlobal("confirm",confirm);
  render(<WorkflowFormModal isOpen onClose={close} onSuccess={()=>{}} />);
  fireEvent.click(screen.getByRole("button",{name:"关闭"}));
  expect(confirm).not.toHaveBeenCalled();expect(close).toHaveBeenCalledTimes(1);
});
