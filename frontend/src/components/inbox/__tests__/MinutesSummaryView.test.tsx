import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import MinutesSummaryView from "../MinutesSummaryView";
import type { MinutesVisualItem } from "@/types/processing";

const visuals: MinutesVisualItem[] = [
  {
    type: "whiteboard",
    media_id: "whiteboard",
    media_type: "image/png",
    width: 320,
    height: 180,
    sha256: "a".repeat(64),
    alt: "飞书智能纪要画板",
  },
  {
    type: "image",
    media_id: "image-1",
    media_type: "image/png",
    width: 640,
    height: 480,
    sha256: "b".repeat(64),
    summary: "图片说明",
    alt: "普通图片",
  },
];

describe("MinutesSummaryView", () => {
  it("renders ordered visuals and opens only the clicked image", () => {
    render(
      <MinutesSummaryView
        artifactSetId={7}
        content="# 纪要\n\n文字总结"
        mode="visual"
        visualItems={visuals}
      />,
    );

    const images = screen.getAllByRole("img");
    expect(images).toHaveLength(2);
    expect(images[0]).toHaveAccessibleName("飞书智能纪要画板");
    expect(images[1]).toHaveAccessibleName("普通图片");
    expect(screen.getByText("图片说明")).toBeVisible();

    fireEvent.click(
      screen.getByRole("button", { name: /放大查看图片：普通图片/ }),
    );
    const lightbox = screen.getByRole("dialog", { name: "图片预览" });
    expect(within(lightbox).getAllByRole("img")).toHaveLength(1);
    expect(within(lightbox).getByRole("img")).toHaveAccessibleName("普通图片");

    fireEvent.keyDown(lightbox, { key: "Escape" });
    expect(
      screen.queryByRole("dialog", { name: "图片预览" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /放大查看图片：普通图片/ }),
    ).toHaveFocus();
  });

  it("keeps other visuals available when one thumbnail fails", () => {
    render(
      <MinutesSummaryView
        artifactSetId={7}
        content="# 纪要"
        mode="visual"
        visualItems={visuals}
      />,
    );
    fireEvent.error(screen.getAllByRole("img")[0]);
    expect(screen.getByRole("status")).toHaveTextContent("画板暂时无法加载");
    expect(screen.getByRole("img", { name: "普通图片" })).toBeVisible();
  });
});
