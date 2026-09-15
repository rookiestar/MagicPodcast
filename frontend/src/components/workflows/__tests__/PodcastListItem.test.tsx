import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { PodcastListItem } from "../PodcastListItem";
import type { Podcast } from "@/types";

describe("PodcastListItem", () => {
  it("声明移动端和桌面端小图的真实显示尺寸", () => {
    const rectSpy = vi
      .spyOn(HTMLElement.prototype, "getBoundingClientRect")
      .mockReturnValue({
        top: 0,
        right: 48,
        bottom: 48,
        left: 0,
        width: 48,
        height: 48,
        x: 0,
        y: 0,
        toJSON: () => ({}),
      });
    render(
      <PodcastListItem
        podcast={{
          id: 1,
          title: "测试节目",
          cover_url: "https://i.typlog.com/workflow-cover.png",
        } as Podcast}
        isSelected={false}
        onAdd={vi.fn()}
        onRemove={vi.fn()}
        index={0}
      />,
    );

    expect(
      screen.getAllByRole("img", { name: "测试节目" }).map((image) =>
        image.getAttribute("sizes"),
      ),
    ).toEqual(["40px"]);
    rectSpy.mockRestore();
  });
});

it("整行与图标点击只切换一次，已选项有独立移除语义", () => {
  const add=vi.fn(),remove=vi.fn();
  const podcast={id:7,title:"选择测试",author:"作者"} as Podcast;
  const {rerender}=render(<PodcastListItem podcast={podcast} isSelected={false} onAdd={add} onRemove={remove} index={0}/>);
  fireEvent.click(screen.getByText("选择测试"));
  expect(add).toHaveBeenCalledExactlyOnceWith(7);
  rerender(<PodcastListItem podcast={podcast} isSelected onAdd={add} onRemove={remove} index={0}/>);
  const button=screen.getByRole("button",{name:"移除节目：选择测试"});
  expect(button).toHaveAttribute("aria-pressed","true");
  fireEvent.click(button.querySelector("svg")!);
  expect(remove).toHaveBeenCalledExactlyOnceWith(7);
});
