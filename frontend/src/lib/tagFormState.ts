export const DEFAULT_TAG_COLOR = "#3B82F6";

export type TagFormMode = "create" | "edit";

export interface TagFormValues {
  name: string;
  color: string;
}

export function getTagFormInitialValues(initialData?: TagFormValues) {
  return {
    name: initialData?.name ?? "",
    color: initialData?.color ?? DEFAULT_TAG_COLOR,
  };
}

export function getTagFormTitle(mode: TagFormMode) {
  return mode === "create" ? "新建" : "编辑";
}

export function getTagFormSubmitLabel(mode: TagFormMode, loading: boolean) {
  if (loading) {
    return "保存中...";
  }

  return mode === "create" ? "创建" : "保存";
}

export function validateTagFormName(name: string) {
  return name.trim() ? "" : "标签名称不能为空";
}

export function getTagFormPayload(name: string, color: string) {
  return {
    name: name.trim(),
    color,
  };
}

export function shouldDisableTagFormSubmit(name: string, loading: boolean) {
  return loading || !name.trim();
}

export function getTagFormSubmitError(error: unknown) {
  return error instanceof Error ? error.message : "操作失败，请重试";
}

export function shouldSubmitTagFormByKeyboard(
  key: string,
  metaKey: boolean,
) {
  return key === "Enter" && metaKey;
}
