import { render, screen } from "@testing-library/react";
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
    ).toEqual(["48px", "40px"]);
    rectSpy.mockRestore();
  });
});
