import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Tag } from "@/types";
import { PodcastTagPicker } from "../PodcastTagPicker";

const tagMocks = vi.hoisted(() => ({
  availableTags: [
    { id: 1, name: "科技", color: "#2563eb" },
    { id: 2, name: "AI", color: "#16a34a" },
  ] as Tag[],
  ensureAvailableTags: vi.fn(),
  create: vi.fn(),
}));

vi.mock("@/hooks/useAvailableTags", () => ({
  useAvailableTags: () => ({
    availableTags: tagMocks.availableTags,
    loading: false,
    ensureAvailableTags: tagMocks.ensureAvailableTags,
    appendAvailableTag: vi.fn(),
  }),
}));

vi.mock("@/lib/api", () => ({
  tagApi: {
    create: (...args: unknown[]) => tagMocks.create(...args),
  },
}));

const selected: Tag[] = [{ id: 1, name: "科技", color: "#2563eb" }];

describe("PodcastTagPicker", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    tagMocks.create.mockResolvedValue({
      id: 9,
      name: "新品",
      color: "#f97316",
    });
  });

  it("keeps the closed state to selected chips plus an add control", () => {
    render(
      <PodcastTagPicker
        tags={selected}
        onTagsChange={vi.fn()}
      />,
    );

    expect(screen.getByText("科技")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "＋ 添加标签" })).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "添加标签" })).not.toBeInTheDocument();
  });

  it("checks selected tags, searches without auto-creating, and creates only from the explicit action", async () => {
    const onTagsChange = vi.fn();
    render(
      <PodcastTagPicker tags={selected} onTagsChange={onTagsChange} />,
    );

    fireEvent.click(screen.getByRole("button", { name: "＋ 添加标签" }));
    const search = screen.getByPlaceholderText("搜索标签");
    expect(screen.getByRole("option", { name: /科技/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );

    fireEvent.change(search, { target: { value: "AI" } });
    expect(screen.getByRole("option", { name: /AI/ })).toBeInTheDocument();
    expect(tagMocks.create).not.toHaveBeenCalled();

    fireEvent.change(search, { target: { value: "新品" } });
    fireEvent.click(screen.getByRole("option", { name: "创建『新品』" }));
    await waitFor(() => {
      expect(tagMocks.create).toHaveBeenCalledWith({
        name: "新品",
        color: expect.any(String),
      });
    });
  });

  it("removes a chip and unchecks a selected option", () => {
    const onTagsChange = vi.fn();
    render(
      <PodcastTagPicker tags={selected} onTagsChange={onTagsChange} />,
    );

    fireEvent.click(screen.getByRole("button", { name: "删除标签 科技" }));
    expect(onTagsChange).toHaveBeenCalledWith([]);

    fireEvent.click(screen.getByRole("button", { name: "＋ 添加标签" }));
    fireEvent.click(screen.getByRole("option", { name: /科技/ }));
    expect(onTagsChange).toHaveBeenLastCalledWith([]);
  });

  it("supports keyboard open, move, confirm, and escape focus return", () => {
    const onTagsChange = vi.fn();
    render(
      <PodcastTagPicker tags={[]} onTagsChange={onTagsChange} />,
    );

    const add = screen.getByRole("button", { name: "＋ 添加标签" });
    fireEvent.click(add);
    const search = screen.getByPlaceholderText("搜索标签");
    fireEvent.keyDown(search, { key: "ArrowDown" });
    fireEvent.keyDown(search, { key: "Enter" });
    expect(onTagsChange).toHaveBeenCalledWith([
      { id: 2, name: "AI", color: "#16a34a" },
    ]);

    fireEvent.keyDown(search, { key: "Escape" });
    expect(screen.queryByRole("dialog", { name: "添加标签" })).not.toBeInTheDocument();
    expect(add).toHaveFocus();
  });

  it.each(["", "新品"])("closes with Escape from a focused option for query %s", async (query) => {
    const user = userEvent.setup();
    const onTagsChange = vi.fn();
    render(<PodcastTagPicker tags={selected} onTagsChange={onTagsChange} />);

    const add = screen.getByRole("button", { name: "＋ 添加标签" });
    await user.click(add);
    const search = screen.getByPlaceholderText("搜索标签");
    await waitFor(() => expect(search).toHaveFocus());
    if (query) await user.type(search, query);
    await user.tab();
    expect(screen.getAllByRole("option")[0]).toHaveFocus();

    await user.keyboard("{Escape}");

    expect(screen.queryByRole("dialog", { name: "添加标签" })).not.toBeInTheDocument();
    expect(add).toHaveFocus();
    expect(onTagsChange).not.toHaveBeenCalled();
    expect(tagMocks.create).not.toHaveBeenCalled();
  });

  it("disables tag changes while saving", () => {
    render(
      <PodcastTagPicker
        tags={selected}
        isUpdatingTags
        onTagsChange={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "＋ 添加标签" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "删除标签 科技" })).toBeDisabled();
  });

  it("keeps long tag names readable", () => {
    render(
      <PodcastTagPicker
        tags={[
          {
            id: 8,
            name: "超长主题标签用来确认完整名称仍然可读",
            color: "#111111",
          },
        ]}
        onTagsChange={vi.fn()}
      />,
    );

    expect(
      screen.getByTitle("超长主题标签用来确认完整名称仍然可读"),
    ).toHaveTextContent("超长主题标签用来确认完整名称仍然可读");
  });
});
