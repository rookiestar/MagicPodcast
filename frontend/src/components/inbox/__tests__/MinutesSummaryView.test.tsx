import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import MinutesSummaryView from "../MinutesSummaryView";
import type { MinutesInlineImage, MinutesVisualItem } from "@/types/processing";

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

const inlineImages: MinutesInlineImage[] = [
  {
    media_id: "image-1",
    section: "summary",
    anchor_text: "段落",
    anchor_occurrence: 1,
  },
];

function expectBefore(first: Element, second: Element) {
  expect(
    first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).not.toBe(0);
}

describe("MinutesSummaryView", () => {
  it("renders only the whiteboard in visual mode", () => {
    render(
      <MinutesSummaryView
        artifactSetId={7}
        content="# 纪要\n\n文字总结"
        mode="visual"
        visualItems={visuals}
      />,
    );

    const images = screen.getAllByRole("img");
    expect(images).toHaveLength(1);
    expect(images[0]).toHaveAccessibleName("飞书智能纪要画板");
    expect(
      screen.queryByRole("img", { name: "普通图片" }),
    ).not.toBeInTheDocument();
  });

  it("renders body images inline in minutes mode and opens the clicked image", () => {
    render(
      <MinutesSummaryView
        artifactSetId={7}
        content="# 纪要\n\n段落\n\n后续正文"
        mode="minutes"
        visualItems={visuals}
        inlineImages={inlineImages}
      />,
    );

    expect(screen.queryByRole("img", { name: "飞书智能纪要画板" })).toBeNull();
    expect(screen.getByRole("img", { name: "普通图片" })).toBeVisible();
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

  it("keeps repeated-anchor images beside the correct body occurrence", () => {
    render(
      <MinutesSummaryView
        artifactSetId={7}
        content={"# 纪要\n\n重复段落\n\n中间正文\n\n重复段落\n\n结尾"}
        mode="minutes"
        visualItems={[
          ...visuals,
          { ...visuals[1], media_id: "image-2", alt: "第二张图片" },
          { ...visuals[1], media_id: "image-3", alt: "第三张图片" },
        ]}
        inlineImages={[
          {
            media_id: "image-1",
            section: "summary",
            anchor_text: "重复段落",
            anchor_occurrence: 1,
          },
          {
            media_id: "image-2",
            section: "summary",
            anchor_text: "重复段落",
            anchor_occurrence: 1,
          },
          {
            media_id: "image-3",
            section: "summary",
            anchor_text: "重复段落",
            anchor_occurrence: 2,
          },
        ]}
      />,
    );

    const images = screen.getAllByRole("img");
    const repeatedParagraphs = screen.getAllByText("重复段落");
    expect(images).toHaveLength(3);
    expect(images.map((image) => image.getAttribute("alt"))).toEqual([
      "普通图片",
      "第二张图片",
      "第三张图片",
    ]);
    expectBefore(repeatedParagraphs[0], images[0]);
    expectBefore(images[0], images[1]);
    expectBefore(images[1], screen.getByText("中间正文"));
    expectBefore(repeatedParagraphs[1], images[2]);
    expectBefore(images[2], screen.getByText("结尾"));
  });

  it("does not let one missing anchor move later matched images", () => {
    render(
      <MinutesSummaryView
        artifactSetId={7}
        content={"# 纪要\n\n第一段\n\n匹配段落\n\n结尾"}
        mode="minutes"
        visualItems={[
          ...visuals,
          { ...visuals[1], media_id: "image-2", alt: "匹配图片" },
        ]}
        inlineImages={[
          {
            media_id: "image-1",
            section: "summary",
            anchor_text: "缺失段落",
            anchor_occurrence: 1,
          },
          {
            media_id: "image-2",
            section: "summary",
            anchor_text: "匹配段落",
            anchor_occurrence: 1,
          },
        ]}
      />,
    );

    const missingAnchorImage = screen.getByRole("img", { name: "普通图片" });
    const matchedImage = screen.getByRole("img", { name: "匹配图片" });
    const ending = screen.getByText("结尾");
    expectBefore(screen.getByText("匹配段落"), matchedImage);
    expectBefore(matchedImage, ending);
    expectBefore(ending, missingAnchorImage);
  });

  it("renders section-start and anchored decision images in their section", () => {
    render(
      <MinutesSummaryView
        artifactSetId={7}
        content="# 纪要\n\n正文"
        mode="minutes"
        decisions={["采用方案 A", "保持方案 B"]}
        visualItems={[
          ...visuals,
          { ...visuals[1], media_id: "image-2", alt: "决策图片" },
        ]}
        inlineImages={[
          {
            media_id: "image-1",
            section: "decisions",
            section_start: true,
          },
          {
            media_id: "image-2",
            section: "decisions",
            anchor_text: "采用方案 A",
            anchor_occurrence: 1,
          },
        ]}
      />,
    );

    const decisionHeading = screen.getByRole("heading", { name: "关键决策" });
    const firstDecision = screen.getByText("采用方案 A").closest("li");
    const startImage = screen.getByRole("img", { name: "普通图片" });
    const decisionImage = screen.getByRole("img", { name: "决策图片" });
    expect(firstDecision).not.toBeNull();
    expectBefore(decisionHeading, startImage);
    expectBefore(startImage, firstDecision!);
    expect(firstDecision).toContainElement(decisionImage);
  });

  it("keeps legacy body images in the minutes tab when placement is unavailable", () => {
    render(
      <MinutesSummaryView
        artifactSetId={7}
        content="# 纪要\n\n正文"
        mode="minutes"
        visualItems={[visuals[1]]}
      />,
    );

    expect(screen.getByRole("img", { name: "普通图片" })).toBeVisible();
    expect(screen.queryByRole("img", { name: "飞书智能纪要画板" })).toBeNull();
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
    expect(
      screen.queryByRole("img", { name: "普通图片" }),
    ).not.toBeInTheDocument();
  });
});
