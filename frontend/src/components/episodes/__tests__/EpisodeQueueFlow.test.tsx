import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { episodeApi } from "@/lib/api";
import { consumptionApi } from "@/lib/api/consumption";
import { usePodcastEpisodes } from "@/hooks/usePodcastEpisodes";
import type { Episode } from "@/types";
import type { ConsumptionItem } from "@/types/consumption";
import EpisodeListSection from "../EpisodeListSection";

vi.mock("@/lib/api", () => ({
  episodeApi: { listByPodcast: vi.fn(), getShowNotes: vi.fn() },
}));
vi.mock("@/lib/api/consumption", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/consumption")>();
  return { ...actual, consumptionApi: { ...actual.consumptionApi, setQueue: vi.fn() } };
});

function episode(id = 1, queue: Episode["queue_state"] = null): Episode {
  return {
    id, queue_state: queue, podcast_id: 1, guid: String(id), title: `单集 ${id}`,
    episode_no: "", medium_url: "", link: "", image_url: "", show_notes: "简介",
    published_date: "2026-09-01T00:00:00Z", duration: 60, enclosure_type: "",
    enclosure_length: 0, my_rate: 0, notes: "",
  };
}
function canonical(queue: ConsumptionItem["queue_state"]): ConsumptionItem {
  return {
    episode_id: 1, episode_title: "单集 1", queue_state: queue,
  } as ConsumptionItem;
}
function page(episodes: Episode[], page = 1, has_more = false) {
  return { episodes, pagination: { page, page_size: 20, total: 2, total_pages: 2, has_more } };
}
function Harness() {
  const state = usePodcastEpisodes({ podcastId: 1, enabled: true });
  return <>
    <EpisodeListSection {...state} loadMoreRef={() => {}} onQueueChange={state.updateEpisodeQueue} />
    <button onClick={() => void state.loadMoreEpisodes()}>加载下一页</button>
  </>;
}
async function openMenu(label = "加入队列") {
  fireEvent.click(await screen.findByRole("button", { name: `${label}，单集 1，打开队列菜单` }));
}
const focusError = {
  response: { status: 409, data: { error: {
    code: "FOCUS_LIMIT_CONFIRMATION_REQUIRED", message: "需要确认", current_count: 7, focus_limit: 7,
  } } },
};

describe("episode list action queue flow", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(episodeApi.listByPodcast).mockResolvedValue(page([episode()]));
    vi.mocked(episodeApi.getShowNotes).mockResolvedValue({
      episode_id: 1, show_notes_document: { format: "html", content: "<p>完整正文</p>" },
    });
  });

  it.each(["inbox", "focus", "someday"] as const)("adds an uncollected episode to %s and reads it back on remount", async (target) => {
    vi.mocked(consumptionApi.setQueue).mockResolvedValue(canonical(target));
    const view = render(<Harness />);
    await openMenu();
    const label = { inbox: "Inbox", focus: "Focus", someday: "Someday" }[target];
    fireEvent.click(screen.getByRole("menuitem", { name: label }));
    expect(await screen.findByRole("button", { name: `${label}，单集 1，打开队列菜单` })).toBeVisible();
    expect(consumptionApi.setQueue).toHaveBeenCalledWith(1, target, { acknowledgeFocusLimit: false });
    view.unmount();
    vi.mocked(episodeApi.listByPodcast).mockResolvedValue(page([episode(1, target)]));
    render(<Harness />);
    expect(await screen.findByRole("button", { name: `${label}，单集 1，打开队列菜单` })).toBeVisible();
  });

  it("keeps the confirmed queue during slow and failed writes, then retries without losing loaded pages", async () => {
    vi.mocked(episodeApi.listByPodcast)
      .mockResolvedValueOnce(page([episode(1, "inbox")], 1, true))
      .mockResolvedValueOnce(page([episode(2, "someday")], 2));
    let reject!: (error: Error) => void;
    vi.mocked(consumptionApi.setQueue).mockImplementationOnce(() => new Promise((_, no) => { reject = no; }))
      .mockResolvedValueOnce(canonical("someday"));
    render(<Harness />);
    await openMenu("Inbox");
    fireEvent.click(screen.getByRole("menuitem", { name: "Someday" }));
    const trigger = screen.getByRole("button", { name: "Inbox，单集 1，打开队列菜单" });
    expect(trigger).toHaveAttribute("aria-disabled", "true");
    fireEvent.click(trigger);
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "加载下一页" }));
    expect(await screen.findByRole("button", { name: "Someday，单集 2，打开队列菜单" })).toBeVisible();
    await act(async () => reject(new Error("暂时无法保存")));
    expect(screen.getByRole("alert")).toHaveTextContent("暂时无法保存");
    expect(trigger).toHaveTextContent("Inbox");
    fireEvent.click(within(screen.getByRole("alert")).getByRole("button", { name: "重试" }));
    expect(await screen.findByRole("button", { name: "Someday，单集 1，打开队列菜单" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Someday，单集 2，打开队列菜单" })).toBeVisible();
  });

  it("requires explicit Focus confirmation, traps dialog focus, and cancellation preserves the queue", async () => {
    vi.mocked(consumptionApi.setQueue).mockRejectedValueOnce(focusError).mockRejectedValueOnce(focusError)
      .mockResolvedValueOnce(canonical("focus"));
    render(<Harness />);
    await openMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: "Focus" }));
    const dialog = await screen.findByRole("alertdialog");
    const user = userEvent.setup();
    const cancel = within(dialog).getByRole("button", { name: "保持原队列" });
    expect(cancel).toHaveFocus();
    await user.tab({ shift: true });
    expect(within(dialog).getByRole("button", { name: "仍加入 Focus" })).toHaveFocus();
    fireEvent.click(cancel);
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "加入队列，单集 1，打开队列菜单" })).toHaveFocus();
    expect(consumptionApi.setQueue).toHaveBeenCalledTimes(1);
    await openMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: "Focus" }));
    fireEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "仍加入 Focus" }));
    expect(await screen.findByRole("button", { name: "Focus，单集 1，打开队列菜单" })).toBeVisible();
    expect(consumptionApi.setQueue).toHaveBeenLastCalledWith(1, "focus", { acknowledgeFocusLimit: true });
  });

  it("offers only other action queues and labels Done moves as reprocessing", async () => {
    vi.mocked(episodeApi.listByPodcast).mockResolvedValue(page([episode(1, "done")]));
    vi.mocked(consumptionApi.setQueue).mockResolvedValue(canonical("inbox"));
    render(<Harness />);
    await openMenu("Done");
    expect(screen.getByRole("menu", { name: "重新处理，加入" })).toBeVisible();
    expect(screen.queryByRole("menuitem", { name: /Done|完成/ })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("menuitem", { name: "Inbox" }));
    await openMenu("Inbox");
    expect(screen.queryByRole("menuitem", { name: "Inbox" })).not.toBeInTheDocument();
    expect(screen.getAllByRole("menuitem")).toHaveLength(2);
  });

  it("navigates the queue menu with a keyboard and restores focus on Escape", async () => {
    render(<Harness />);
    await openMenu();
    const user = userEvent.setup();
    expect(screen.getByRole("menuitem", { name: "Inbox" })).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("menuitem", { name: "Focus" })).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "加入队列，单集 1，打开队列菜单" })).toHaveFocus();
  });

  it("reports an ordinary Focus error inline without asking for capacity acknowledgement", async () => {
    vi.mocked(consumptionApi.setQueue).mockRejectedValue(new Error("服务不可用"));
    render(<Harness />);
    await openMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: "Focus" }));
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("服务不可用"));
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });
  it("presents concurrent Focus confirmations one at a time with the correct episode and focus", async () => {
    vi.mocked(episodeApi.listByPodcast).mockResolvedValue(page([episode(1), episode(2)]));
    const rejections: Array<(error: unknown) => void> = [];
    vi.mocked(consumptionApi.setQueue)
      .mockImplementationOnce(() => new Promise((_, reject) => rejections.push(reject)))
      .mockImplementationOnce(() => new Promise((_, reject) => rejections.push(reject)))
      .mockResolvedValueOnce({ ...canonical("focus"), episode_id: 2 });

    render(<Harness />);
    await openMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: "Focus" }));
    fireEvent.click(screen.getByRole("button", { name: "加入队列，单集 2，打开队列菜单" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Focus" }));

    await act(async () => rejections.forEach((reject) => reject(focusError)));
    expect(screen.getAllByRole("alertdialog")).toHaveLength(1);
    expect(screen.getByRole("alertdialog")).toHaveTextContent("加入《单集 1》");
    expect(within(screen.getByRole("alertdialog")).getByRole("button", { name: "保持原队列" })).toHaveFocus();
    expect(screen.getByRole("button", { name: "加入队列，单集 2，打开队列菜单" })).toHaveAttribute("aria-disabled", "true");

    fireEvent.click(within(screen.getByRole("alertdialog")).getByRole("button", { name: "保持原队列" }));
    expect(screen.getAllByRole("alertdialog")).toHaveLength(1);
    expect(screen.getByRole("alertdialog")).toHaveTextContent("加入《单集 2》");
    expect(within(screen.getByRole("alertdialog")).getByRole("button", { name: "保持原队列" })).toHaveFocus();

    fireEvent.click(within(screen.getByRole("alertdialog")).getByRole("button", { name: "仍加入 Focus" }));
    expect(await screen.findByRole("button", { name: "Focus，单集 2，打开队列菜单" })).toBeVisible();
    expect(screen.getByRole("button", { name: "加入队列，单集 1，打开队列菜单" })).toHaveAttribute("aria-disabled", "false");
    expect(consumptionApi.setQueue).toHaveBeenLastCalledWith(2, "focus", { acknowledgeFocusLimit: true });
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  it("ignores a late Focus response for an episode removed from the list", async () => {
    let reject!: (error: unknown) => void;
    vi.mocked(consumptionApi.setQueue)
      .mockImplementationOnce(() => new Promise((_, no) => { reject = no; }))
      .mockRejectedValueOnce(focusError);
    const props = {
      episodesLoading: false, isLoadingMore: false, hasMoreEpisodes: false,
      totalEpisodes: 2, loadMoreRef: () => {},
    };
    const view = render(<EpisodeListSection {...props} episodes={[episode(1), episode(2)]} />);
    await openMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: "Focus" }));
    view.rerender(<EpisodeListSection {...props} episodes={[episode(2)]} />);
    await act(async () => reject(focusError));
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "加入队列，单集 2，打开队列菜单" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Focus" }));
    expect(await screen.findByRole("alertdialog")).toHaveTextContent("加入《单集 2》");
  });

});
