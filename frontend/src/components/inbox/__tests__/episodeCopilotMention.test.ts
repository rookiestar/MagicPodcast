import { afterEach, describe, expect, it, vi } from "vitest";
import {
  filterPeople,
  jumpToLibrarySource,
  mentionDraft,
  parseLibrarySources,
  replaceMention,
} from "../episodeCopilotMention";

const people = [
  {
    id: 9,
    display_name: "张三",
    aliases: ["老张"],
    identity_note: "技术漫谈主播",
    role: "host" as const,
    status: "confirmed" as const,
    status_reason: "",
  },
];

describe("episodeCopilotMention", () => {
  it("detects @ drafts and nickname matches", () => {
    expect(mentionDraft("hello @老", 9)).toEqual({ start: 6, query: "老" });
    expect(filterPeople(people, "老").map((person) => person.id)).toEqual([9]);
    expect(replaceMention("hello @老", { start: 6, query: "老" }, 9)).toBe(
      "hello ",
    );
  });

  it("parses library source locators", () => {
    const sources = parseLibrarySources(
      "- [库内 S1] 单集 81 · 2025-03-12 · 片段 3\n",
    );
    expect(sources).toEqual([
      { index: 1, episodeId: 81, date: "2025-03-12", fragmentOrder: 3 },
    ]);
  });

  it("opens transcript tabs and scrolls to the matching fragment", async () => {
    const transcriptTab = document.createElement("button");
    transcriptTab.id = "detail-tab-transcript";
    const artifactTab = document.createElement("button");
    artifactTab.id = "processing-artifact-tab-transcript";
    const root = document.createElement("div");
    root.setAttribute("data-copilot-source", "transcript");
    root.setAttribute("data-copilot-episode-id", "81");
    const fragment = document.createElement("article");
    fragment.setAttribute("data-fragment-order", "3");
    fragment.scrollIntoView = vi.fn();
    root.append(fragment);
    document.body.append(transcriptTab, artifactTab, root);
    const clickTranscript = vi.spyOn(transcriptTab, "click");
    const clickArtifact = vi.spyOn(artifactTab, "click");

    await jumpToLibrarySource({ episodeId: 81, fragmentOrder: 3 });

    expect(clickTranscript).toHaveBeenCalled();
    expect(clickArtifact).toHaveBeenCalled();
    expect(fragment.scrollIntoView).toHaveBeenCalledWith({ block: "center" });
  });

  it("opens another episode before scrolling to a library source", async () => {
    const openEpisode = vi.fn(async (episodeId: number) => {
      const root = document.createElement("div");
      root.setAttribute("data-copilot-source", "transcript");
      root.setAttribute("data-copilot-episode-id", String(episodeId));
      const fragment = document.createElement("article");
      fragment.setAttribute("data-fragment-order", "1");
      fragment.scrollIntoView = vi.fn();
      root.append(fragment);
      document.body.append(root);
    });
    const transcriptTab = document.createElement("button");
    transcriptTab.id = "detail-tab-transcript";
    const artifactTab = document.createElement("button");
    artifactTab.id = "processing-artifact-tab-transcript";
    document.body.append(transcriptTab, artifactTab);

    await jumpToLibrarySource(
      { episodeId: 99, fragmentOrder: 1 },
      { openEpisode },
    );

    expect(openEpisode).toHaveBeenCalledWith(99);
    const fragment = document.querySelector(
      '[data-copilot-episode-id="99"] [data-fragment-order="1"]',
    ) as HTMLElement;
    expect(fragment.scrollIntoView).toHaveBeenCalledWith({ block: "center" });
  });
});

afterEach(() => {
  document.body.replaceChildren();
});
