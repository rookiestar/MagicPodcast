import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import VirtualPodcastGrid from "../VirtualPodcastGrid";
import type { Podcast } from "@/types";

vi.mock("@tanstack/react-virtual", () => ({
  useWindowVirtualizer: () => ({
    getVirtualItems: () => [],
    getTotalSize: () => 0,
    measureElement: vi.fn(),
    options: { scrollMargin: 0 },
  }),
}));

function podcast(id: number): Podcast {
  return {
    id,
    xyz_id: `xyz-${id}`,
    title: `服务端播客 ${id}`,
    description: "无需等待客户端脚本的首批内容",
    author: "作者",
    cover_url: `https://i.typlog.com/server-cover-${id}.png`,
    episode_count: 1,
    newest_episode_date: "2026-08-09T00:00:00Z",
    created_at: "2026-08-09T00:00:00Z",
    is_subscribed: true,
    is_dead: false,
  };
}

describe("VirtualPodcastGrid server fallback", () => {
  it("renders the first batch as real responsive card links before virtualization starts", () => {
    const html = renderToString(
      <VirtualPodcastGrid
        podcasts={Array.from({ length: 10 }, (_, index) =>
          podcast(index + 1),
        )}
        columns={5}
        isMobile={false}
        listStateKey="podcasts"
        sortBy="recent_update"
        selectedTagIds={[]}
      />,
    );

    expect(html).toContain('href="/podcasts/1?sort_by=recent_update"');
    expect(html).toContain("服务端播客 10");
    expect(html).toContain("podcast-library-card is-mobile");
  });

  it("puts the bounded first cover batch in HTML with only the first row eager", () => {
    const html = renderToString(
      <VirtualPodcastGrid
        podcasts={Array.from({ length: 15 }, (_, index) =>
          podcast(index + 1),
        )}
        columns={5}
        isMobile={false}
        listStateKey="podcasts"
        sortBy="recent_update"
        selectedTagIds={[]}
      />,
    );

    expect(html.match(/<img/g)).toHaveLength(10);
    expect(html.match(/loading="eager"/g)).toHaveLength(5);
    expect(html.match(/loading="lazy"/g)).toHaveLength(5);
  });
});
