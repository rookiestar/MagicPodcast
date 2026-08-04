import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import PodcastListSortControls from "../PodcastListSortControls";
import ResponsivePodcastCard from "../ResponsivePodcastCard";
import type { Podcast } from "@/types";

const sortOptions = [
  { label: "最近更新", value: "recent_update" as const },
  { label: "名称", value: "title" as const },
];

describe("podcast library chrome", () => {
  it("labels the desktop sort control as an editorial sort mode", () => {
    render(
      <PodcastListSortControls
        sortBy="recent_update"
        options={sortOptions}
        onSortChange={vi.fn()}
      />,
    );

    expect(
      screen.getByRole("combobox", { name: "排序方式" }),
    ).toBeInTheDocument();
    expect(document.querySelector(".podcast-sort-desktop-icon")).toBeTruthy();
    expect(document.querySelector(".podcast-sort-desktop-copy")).toBeNull();
  });

  it("uses the established New marker copy for recent podcasts", () => {
    const podcast = {
      id: 1,
      title: "测试节目",
      author: "测试作者",
      description: "测试简介",
      cover_url: "/warm-paper-grid-texture.png",
      newest_episode_date: new Date().toISOString(),
      episode_count: 3,
      tags: [],
    } as Podcast;

    render(
      <ResponsivePodcastCard
        podcast={podcast}
        index={0}
        priority="high"
        detailUrl="/podcasts/1"
        isMobile={false}
      />,
    );

    expect(screen.getByText("New")).toBeInTheDocument();
    expect(screen.queryByText("新")).not.toBeInTheDocument();
  });
});
