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

it.each([600, 61, 1])("编辑时长 %s 秒无损保存", async (seconds) => {
  const {workflowApi}=await import("@/lib/api");
  const update=vi.spyOn(workflowApi,"update").mockResolvedValue({} as never);
  vi.stubGlobal("confirm",()=>true);
  vi.stubGlobal("prompt",()=>"UPDATE WORKFLOW 7");
  const workflow={id:7,name:"时长测试",description:"",schedule:"0 0 8 * * *",scope_type:"all_subscribed",scope_config:{},rules_config:{min_duration:seconds},is_enabled:true} as import("@/types").Workflow;
  render(<WorkflowFormModal isOpen workflow={workflow} onClose={()=>{}} onSuccess={()=>{}}/>);
  fireEvent.click(screen.getAllByRole("button",{name:"下一步"})[0]);
  fireEvent.click(screen.getAllByRole("button",{name:"下一步"})[0]);
  expect(screen.getByRole("spinbutton",{name:"最小时长（分钟）"})).toHaveValue(seconds/60);
  fireEvent.click(screen.getAllByRole("button",{name:"下一步"})[0]);
  fireEvent.click(screen.getAllByRole("button",{name:"保存并启用"})[0]);
  await waitFor(()=>expect(update).toHaveBeenCalledWith(7,expect.objectContaining({rules_config:expect.objectContaining({min_duration:seconds})})));
  update.mockRestore();
});

it("分钟输入拒绝负数，支持小数并提交秒；保存失败不丢失输入",async()=>{
 const {workflowApi}=await import("@/lib/api");
 const create=vi.spyOn(workflowApi,"create").mockRejectedValueOnce(new Error("保存失败")).mockResolvedValueOnce({} as never);
 const close=vi.fn();render(<WorkflowFormModal isOpen onClose={close} onSuccess={()=>{}}/>);
 fireEvent.change(screen.getByPlaceholderText("例如: 每日科技播客抓取"),{target:{value:"分钟提交"}});
 fireEvent.click(screen.getAllByRole("button",{name:"下一步"})[0]);fireEvent.click(screen.getAllByRole("button",{name:"下一步"})[0]);
 const input=screen.getByRole("spinbutton",{name:"最小时长（分钟）"});
 fireEvent.change(input,{target:{value:"-1"}});fireEvent.click(screen.getAllByRole("button",{name:"下一步"})[0]);
 expect(screen.getByRole("alert")).toHaveTextContent("有效分钟数");expect(create).not.toHaveBeenCalled();
 fireEvent.change(input,{target:{value:"1.5"}});fireEvent.click(screen.getAllByRole("button",{name:"下一步"})[0]);
 fireEvent.click(screen.getAllByRole("button",{name:"创建并启用"})[0]);
 await waitFor(()=>expect(create).toHaveBeenCalledTimes(1));expect(close).not.toHaveBeenCalled();
 fireEvent.click(screen.getAllByRole("button",{name:"上一步"})[0]);expect(screen.getByRole("spinbutton",{name:"最小时长（分钟）"})).toHaveValue(1.5);
 fireEvent.click(screen.getAllByRole("button",{name:"下一步"})[0]);fireEvent.click(screen.getAllByRole("button",{name:"创建并启用"})[0]);
 await waitFor(()=>expect(create).toHaveBeenCalledTimes(2));expect(create).toHaveBeenLastCalledWith(expect.objectContaining({rules_config:expect.objectContaining({min_duration:90})}));
 create.mockRestore();
});

it.each([
 ["0 8 * * *","每天 08:00"], ["0 30 7 * * *","每天 07:30"], ["0 0 6 * * 1","每周一 06:00"],
 ["0 0 6 2,16 * *","0 0 6 2,16 * *"], ["0 0 6 * 1 *","0 0 6 * 1 *"], ["0 0 6 * * 1-5","0 0 6 * * 1-5"], ["30 0 6 * * *","30 0 6 * * *"]
])("定时展示保留语义 %s",async(expression,expected)=>{
 const {formatWorkflowSchedule}=await import("../workflowFormConstants");expect(formatWorkflowSchedule(expression)).toBe(expected);
});
