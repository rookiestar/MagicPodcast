import { describe, expect, it } from "vitest";
import {
  DEFAULT_TAG_COLOR,
  getTagFormInitialValues,
  getTagFormPayload,
  getTagFormSubmitError,
  getTagFormSubmitLabel,
  getTagFormTitle,
  shouldDisableTagFormSubmit,
  shouldSubmitTagFormByKeyboard,
  validateTagFormName,
} from "../tagFormState";

describe("tagFormState", () => {
  it("builds initial form values", () => {
    expect(getTagFormInitialValues()).toEqual({
      name: "",
      color: DEFAULT_TAG_COLOR,
    });
    expect(
      getTagFormInitialValues({ name: "科技", color: "#2563eb" }),
    ).toEqual({
      name: "科技",
      color: "#2563eb",
    });
  });

  it("builds title and submit labels from mode and loading state", () => {
    expect(getTagFormTitle("create")).toBe("新建");
    expect(getTagFormTitle("edit")).toBe("编辑");
    expect(getTagFormSubmitLabel("create", false)).toBe("创建");
    expect(getTagFormSubmitLabel("edit", false)).toBe("保存");
    expect(getTagFormSubmitLabel("edit", true)).toBe("保存中...");
  });

  it("validates and normalizes submitted values", () => {
    expect(validateTagFormName(" ")).toBe("标签名称不能为空");
    expect(validateTagFormName("科技")).toBe("");
    expect(getTagFormPayload(" 科技 ", "#2563eb")).toEqual({
      name: "科技",
      color: "#2563eb",
    });
    expect(shouldDisableTagFormSubmit(" ", false)).toBe(true);
    expect(shouldDisableTagFormSubmit("科技", true)).toBe(true);
    expect(shouldDisableTagFormSubmit("科技", false)).toBe(false);
  });

  it("normalizes submit errors and keyboard shortcuts", () => {
    expect(getTagFormSubmitError(new Error("失败"))).toBe("失败");
    expect(getTagFormSubmitError("failed")).toBe("操作失败，请重试");
    expect(shouldSubmitTagFormByKeyboard("Enter", true)).toBe(true);
    expect(shouldSubmitTagFormByKeyboard("Enter", false)).toBe(false);
    expect(shouldSubmitTagFormByKeyboard("Escape", true)).toBe(false);
  });
});
