import { fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
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
  afterEach(() => {
    document.body.style.overflow = "";
    document.body.style.touchAction = "";
    document.documentElement.style.overflow = "";
  });

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

  it("starts at full-screen fit, locks the page, and supports zoom reset", () => {
    document.body.style.overflow = "scroll";
    document.documentElement.style.overflow = "auto";
    render(
      <MinutesSummaryView
        artifactSetId={7}
        content="# 纪要"
        mode="visual"
        visualItems={visuals}
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: /放大查看画板：飞书智能纪要画板/ }),
    );
    const lightbox = screen.getByRole("dialog", { name: "画板预览" });
    expect(lightbox.parentElement).toBe(document.body);
    expect(document.body.style.overflow).toBe("hidden");
    expect(document.documentElement.style.overflow).toBe("hidden");
    expect(screen.getByText("100%")).toBeVisible();
    expect(screen.getByRole("button", { name: "适配全屏" })).toBeDisabled();

    fireEvent.click(screen.getByRole("button", { name: "放大" }));
    expect(screen.getByText("125%")).toBeVisible();
    expect(screen.getByRole("button", { name: "适配全屏" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "适配全屏" }));
    expect(screen.getByText("100%")).toBeVisible();
    expect(screen.getByRole("button", { name: "适配全屏" })).toBeDisabled();

    const wheelDefaultAllowed = fireEvent.wheel(
      lightbox.querySelector("[data-zoomed]")!,
      {
        deltaY: -100,
        clientX: 320,
        clientY: 180,
      },
    );
    expect(wheelDefaultAllowed).toBe(false);
    expect(screen.getByText("125%")).toBeVisible();

    const viewport = lightbox.querySelector("[data-zoomed]")!;
    const lightboxImage = within(lightbox).getByRole("img", {
      name: "飞书智能纪要画板",
    });
    Object.defineProperties(viewport, {
      clientWidth: { configurable: true, value: 800 },
      clientHeight: { configurable: true, value: 600 },
    });
    Object.defineProperties(lightboxImage, {
      offsetWidth: { configurable: true, value: 800 },
      offsetHeight: { configurable: true, value: 600 },
    });
    fireEvent.pointerDown(viewport, {
      pointerId: 1,
      pointerType: "mouse",
      button: 0,
      clientX: 100,
      clientY: 100,
    });
    fireEvent.pointerMove(viewport, {
      pointerId: 1,
      pointerType: "mouse",
      clientX: 140,
      clientY: 120,
    });
    fireEvent.pointerUp(viewport, { pointerId: 1, pointerType: "mouse" });
    const imageFrame = lightbox.querySelector<HTMLElement>(
      '[style*="translate3d"]',
    );
    expect(imageFrame?.style.transform).toBe("translate3d(40px, 20px, 0)");

    fireEvent.pointerDown(viewport, {
      pointerId: 2,
      pointerType: "touch",
      clientX: 100,
      clientY: 100,
    });
    fireEvent.pointerDown(viewport, {
      pointerId: 3,
      pointerType: "touch",
      clientX: 200,
      clientY: 100,
    });
    fireEvent.pointerMove(viewport, {
      pointerId: 3,
      pointerType: "touch",
      clientX: 300,
      clientY: 100,
    });
    expect(screen.getByText("250%")).toBeVisible();
    fireEvent.pointerUp(viewport, { pointerId: 2, pointerType: "touch" });
    fireEvent.pointerUp(viewport, { pointerId: 3, pointerType: "touch" });

    fireEvent.click(screen.getByRole("button", { name: "关闭" }));
    expect(document.body.style.overflow).toBe("scroll");
    expect(document.documentElement.style.overflow).toBe("auto");
  });

  it("constrains a wide image to its rendered contain bounds", () => {
    render(
      <MinutesSummaryView
        artifactSetId={7}
        content="# 纪要"
        mode="visual"
        visualItems={visuals}
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: /放大查看画板：飞书智能纪要画板/ }),
    );
    const lightbox = screen.getByRole("dialog", { name: "画板预览" });
    const viewport = lightbox.querySelector("[data-zoomed]")!;
    const lightboxImage = within(lightbox).getByRole("img", {
      name: "飞书智能纪要画板",
    });
    Object.defineProperties(viewport, {
      clientWidth: { configurable: true, value: 800 },
      clientHeight: { configurable: true, value: 600 },
    });
    Object.defineProperties(lightboxImage, {
      offsetWidth: { configurable: true, value: 800 },
      offsetHeight: { configurable: true, value: 600 },
      naturalWidth: { configurable: true, value: 2400 },
      naturalHeight: { configurable: true, value: 600 },
    });

    fireEvent.click(within(lightbox).getByRole("button", { name: "放大" }));
    fireEvent.pointerDown(viewport, {
      pointerId: 1,
      pointerType: "mouse",
      button: 0,
      clientX: 100,
      clientY: 100,
    });
    fireEvent.pointerMove(viewport, {
      pointerId: 1,
      pointerType: "mouse",
      clientX: 1400,
      clientY: 1000,
    });
    fireEvent.pointerUp(viewport, { pointerId: 1, pointerType: "mouse" });

    expect(
      lightbox.querySelector<HTMLElement>('[style*="translate3d"]')?.style
        .transform,
    ).toBe("translate3d(100px, 0px, 0)");
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
