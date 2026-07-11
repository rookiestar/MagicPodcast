import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Tag } from "@/types";
import { tagApi } from "@/lib/api";
import { useTagInputActions } from "../useTagInputActions";

vi.mock("@/lib/api", () => ({
  tagApi: {
    create: vi.fn(),
  },
}));

const tags: Tag[] = [
  { id: 1, name: "科技", color: "#2563eb" },
  { id: 2, name: "AI", color: "#16a34a" },
];

function renderActions(overrides = {}) {
  return renderHook((props) => useTagInputActions(props), {
    initialProps: {
      selectedTags: [tags[0]],
      onTagsChange: vi.fn(),
      disabled: false,
      appendAvailableTag: vi.fn(),
      resetAfterTagChange: vi.fn(),
      ...overrides,
    },
  });
}

describe("useTagInputActions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("adds and removes selected tags", () => {
    const onTagsChange = vi.fn();
    const resetAfterTagChange = vi.fn();
    const { result } = renderActions({ onTagsChange, resetAfterTagChange });

    act(() => {
      result.current.addTag(tags[1]);
    });

    expect(onTagsChange).toHaveBeenCalledWith(tags);
    expect(resetAfterTagChange).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.removeTag(tags[0].id);
    });

    expect(onTagsChange).toHaveBeenLastCalledWith([]);
  });

  it("creates a tag and appends it to the available tag cache", async () => {
    const newTag = { id: 3, name: "生活", color: "#f97316" };
    const onTagsChange = vi.fn();
    const appendAvailableTag = vi.fn();
    const resetAfterTagChange = vi.fn();
    vi.mocked(tagApi.create).mockResolvedValue(newTag);
    const { result } = renderActions({
      onTagsChange,
      appendAvailableTag,
      resetAfterTagChange,
    });

    await act(async () => {
      await result.current.createTag(" 生活 ");
    });

    expect(tagApi.create).toHaveBeenCalledWith({
      name: "生活",
      color: expect.any(String),
    });
    expect(onTagsChange).toHaveBeenCalledWith([tags[0], newTag]);
    expect(appendAvailableTag).toHaveBeenCalledWith(newTag);
    expect(resetAfterTagChange).toHaveBeenCalledTimes(1);
  });

  it("runs submit actions without changing state for none actions", () => {
    const onTagsChange = vi.fn();
    const { result } = renderActions({ onTagsChange });

    act(() => {
      result.current.submitTagAction({ type: "select", tag: tags[1] });
      result.current.submitTagAction({ type: "none" });
    });

    expect(onTagsChange).toHaveBeenCalledTimes(1);
    expect(onTagsChange).toHaveBeenCalledWith(tags);
  });

  it("does nothing while disabled", async () => {
    const onTagsChange = vi.fn();
    const appendAvailableTag = vi.fn();
    const resetAfterTagChange = vi.fn();
    const { result } = renderActions({
      disabled: true,
      onTagsChange,
      appendAvailableTag,
      resetAfterTagChange,
    });

    await act(async () => {
      result.current.addTag(tags[1]);
      result.current.removeTag(tags[0].id);
      await result.current.createTag("生活");
    });

    expect(onTagsChange).not.toHaveBeenCalled();
    expect(appendAvailableTag).not.toHaveBeenCalled();
    expect(resetAfterTagChange).not.toHaveBeenCalled();
    expect(tagApi.create).not.toHaveBeenCalled();
  });
});
