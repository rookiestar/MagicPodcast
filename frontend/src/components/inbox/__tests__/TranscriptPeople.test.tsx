import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import TranscriptAudioPlayer from "../TranscriptAudioPlayer";
import { episodeCopilotApi } from "@/lib/api/episodeCopilot";
import type { EpisodePeoplePayload } from "@/types/episodeCopilot";

vi.mock("@/lib/api/episodeCopilot", () => ({
  episodeCopilotApi: {
    getPeople: vi.fn(),
    preparePeople: vi.fn(),
    peopleDrafts: vi.fn(),
    reviewPeople: vi.fn(),
    manualPerson: vi.fn(),
  },
}));
const segments = [
  {
    order: 1,
    speaker: "Speaker 1",
    start_ms: 0,
    text: "我是小林，今天聊工作。",
  },
  { order: 2, speaker: "Speaker 1", start_ms: 5000, text: "应该保持休息。" },
];
const empty: EpisodePeoplePayload = {
  episode_id: 7,
  source_version: "artifact-8",
  revision: 0,
  index_ready: false,
  people: [],
  attributions: [],
};
const proposal: EpisodePeoplePayload = {
  ...empty,
  revision: 1,
  draft: {
    id: 1,
    revision: 1,
    source_version: "artifact-8",
    outdated: false,
    matches: [
      {
        key: "host",
        display_name: "小林",
        role: "host",
        speaker_label: "Speaker 1",
        orders: [1, 2],
        selected: true,
        uncertain: false,
      },
    ],
  },
};
const applied: EpisodePeoplePayload = {
  ...proposal,
  revision: 3,
  index_ready: true,
  people: [
    {
      id: 9,
      display_name: "林老师",
      aliases: [],
      identity_note: "",
      role: "host",
      status: "confirmed",
      status_reason: "",
    },
  ],
  attributions: segments.map((s) => ({
    id: s.order,
    person_id: 9,
    display_name: "林老师",
    source_kind: "transcript",
    source_version: "artifact-8",
    fragment_order: s.order,
    speaker_label: s.speaker,
    start_ms: s.start_ms,
    text: s.text,
    status: "confirmed",
    user_confirmed: true,
  })),
};
function Player({ mediaAvailable = false }: { mediaAvailable?: boolean } = {}) {
  return (
    <TranscriptAudioPlayer
      episodeId={7}
      artifactSetId={8}
      segments={segments}
      mediaAvailable={mediaAvailable}
      playbackRate={1}
      onPlaybackRateChange={() => {}}
    />
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(episodeCopilotApi.getPeople).mockResolvedValue(empty);
  vi.mocked(episodeCopilotApi.peopleDrafts).mockResolvedValue([]);
  vi.mocked(episodeCopilotApi.preparePeople).mockResolvedValue(proposal);
});
describe("逐字稿人物确认", () => {
  it("keeps suggestions separate, saves edits and applies only on explicit confirmation", async () => {
    const change = vi.fn();
    window.addEventListener("episode-people-changed", change);
    render(<Player />);
    fireEvent.click(await screen.findByRole("button", { name: "识别人物" }));
    fireEvent.click(screen.getByRole("button", { name: "开始识别" }));
    fireEvent.click(await screen.findByRole("button", { name: "编辑匹配 小林" }));
    const name = await screen.findByRole("textbox", { name: "姓名 host" });
    expect(screen.getAllByRole("button", { name: "Speaker 1" })).toHaveLength(
      2,
    );
    expect(change).not.toHaveBeenCalled();
    fireEvent.change(name, { target: { value: "林老师" } });
    fireEvent.doubleClick(screen.getAllByRole("button", { name: "Speaker 1" })[0]);
    expect(screen.queryByRole("dialog", { name: "编辑发言人物" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "关闭人物核对" }));
    expect(screen.queryByRole("dialog", { name: "人物与发言核对" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "继续编辑" }));
    expect(screen.getByRole("textbox", { name: "姓名 host" })).toHaveValue("林老师");
    vi.mocked(episodeCopilotApi.reviewPeople).mockResolvedValueOnce({
      ...proposal,
      revision: 2,
      draft: {
        ...proposal.draft!,
        revision: 2,
        matches: [{ ...proposal.draft!.matches[0], display_name: "林老师" }],
      },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存草稿" }));
    await waitFor(() =>
      expect(episodeCopilotApi.reviewPeople).toHaveBeenCalledWith(
        7,
        expect.objectContaining({
          matches: [expect.objectContaining({ display_name: "林老师" })],
        }),
        false,
      ),
    );
    expect(screen.getAllByRole("button", { name: "Speaker 1" })).toHaveLength(
      2,
    );
    vi.mocked(episodeCopilotApi.reviewPeople).mockResolvedValueOnce(applied);
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "确认并应用" }),
      ).not.toBeDisabled(),
    );
    fireEvent.click(screen.getByRole("button", { name: "确认并应用" }));
    await waitFor(() =>
      expect(screen.getAllByRole("button", { name: "林老师" })).toHaveLength(2),
    );
    expect(change).toHaveBeenCalledTimes(1);
    window.removeEventListener("episode-people-changed", change);
  });
  it("supports direct manual naming, cancellation and fragment scope without a prior recognition", async () => {
    render(<Player />);
    await waitFor(() => expect(episodeCopilotApi.getPeople).toHaveBeenCalled());
    fireEvent.doubleClick(
      screen.getAllByRole("button", { name: "Speaker 1" })[0],
    );
    let dialog = screen.getByRole("dialog", { name: "编辑发言人物" });
    fireEvent.change(
      within(dialog).getByRole("textbox", { name: "姓名或称呼" }),
      { target: { value: "小林" } },
    );
    fireEvent.blur(within(dialog).getByRole("textbox", { name: "姓名或称呼" }));
    expect(episodeCopilotApi.manualPerson).not.toHaveBeenCalled();
    fireEvent.keyDown(dialog, { key: "Escape" });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "编辑Speaker 1，片段 1" }),
    );
    dialog = screen.getByRole("dialog");
    fireEvent.change(
      within(dialog).getByRole("textbox", { name: "姓名或称呼" }),
      { target: { value: "林老师" } },
    );
    fireEvent.click(within(dialog).getByRole("radio", { name: "仅此段" }));
    vi.mocked(episodeCopilotApi.manualPerson).mockResolvedValue({
      ...applied,
      attributions: applied.attributions.slice(0, 1),
    });
    fireEvent.click(
      within(dialog).getByRole("button", { name: "确认应用到 1 段" }),
    );
    await waitFor(() =>
      expect(episodeCopilotApi.manualPerson).toHaveBeenCalledWith(
        7,
        expect.objectContaining({
          fragment_order: 1,
          scope: "fragment",
          person_id: 0,
          display_name: "林老师",
          revision: 0,
          source_version: "artifact-8",
        }),
      ),
    );
    await screen.findByRole("button", { name: "林老师" });
    expect(
      screen.getByRole("button", { name: "Speaker 1" }),
    ).toBeInTheDocument();
  });
  it("keeps the transcript readable during slow recognition and ignores a cancelled result", async () => {
    let finish!: (p: EpisodePeoplePayload) => void;
    vi.mocked(episodeCopilotApi.preparePeople).mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    render(<Player />);
    fireEvent.click(await screen.findByRole("button", { name: "识别人物" }));
    fireEvent.click(screen.getByRole("button", { name: "开始识别" }));
    expect(screen.getByText(segments[0].text)).toBeVisible();
    fireEvent.click(await screen.findByRole("button", { name: "取消识别" }));
    await act(async () => finish(proposal));
    expect(
      screen.queryByRole("textbox", { name: "姓名 host" }),
    ).not.toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Speaker 1" })).toHaveLength(
      2,
    );
  });
  it("distinguishes an old applied record from a later manual correction", async () => {
    vi.mocked(episodeCopilotApi.getPeople).mockResolvedValue({
      ...applied,
      draft: {
        ...proposal.draft!,
        matches: [
          {
            ...proposal.draft!.matches[0],
            person_id: 9,
            display_name: "林老师",
            applied: true,
            selected: false,
          },
        ],
      },
      attributions: [
        { ...applied.attributions[0], person_id: 10, display_name: "另一位" },
        applied.attributions[1],
      ],
    });
    render(<Player />);
    fireEvent.click(await screen.findByRole("button", { name: "管理" }));
    expect(screen.getByText(/曾应用，当前归属已有调整/)).toBeInTheDocument();
    expect(screen.getByText(/当前：另一位、林老师/)).toBeInTheDocument();
  });
  it("editing a speaker does not arm or play an available audio stream", async () => {
    const play = vi
      .spyOn(HTMLMediaElement.prototype, "play")
      .mockResolvedValue();
    const { container } = render(<Player mediaAvailable />);
    await waitFor(() => expect(episodeCopilotApi.getPeople).toHaveBeenCalled());
    fireEvent.doubleClick(
      screen.getAllByRole("button", { name: "Speaker 1" })[0],
    );
    expect(
      screen.getByRole("dialog", { name: "编辑发言人物" }),
    ).toBeInTheDocument();
    expect(container.querySelector("audio")).not.toHaveAttribute("src");
    expect(play).not.toHaveBeenCalled();
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    play.mockRestore();
  });
});

it("lists an unknown speaker in a modal and supports manual naming without recognition", async () => {
  render(<Player />);
  fireEvent.click(await screen.findByRole("button", { name: "识别人物" }));
  const dialog = screen.getByRole("dialog", { name: "人物与发言核对" });
  expect(dialog).toHaveAttribute("aria-modal", "true");
  expect(within(dialog).getByText("姓名待确认")).toBeVisible();
  expect(episodeCopilotApi.preparePeople).not.toHaveBeenCalled();
  fireEvent.click(within(dialog).getByRole("button", { name: "填写姓名" }));
  expect(screen.getByRole("dialog", { name: "编辑发言人物" })).toBeVisible();
  fireEvent.keyDown(screen.getByRole("dialog", { name: "编辑发言人物" }), { key: "Escape" });
  expect(dialog).toBeVisible();
});
it("keeps a running request alive when closing and reopening the modal", async () => {
  let complete!: (p: EpisodePeoplePayload) => void;
  vi.mocked(episodeCopilotApi.preparePeople).mockImplementation(() => new Promise(resolve => { complete = resolve; }));
  render(<Player />);
  fireEvent.click(await screen.findByRole("button", { name: "识别人物" }));
  fireEvent.click(screen.getByRole("button", { name: "开始识别" }));
  const signal = vi.mocked(episodeCopilotApi.preparePeople).mock.calls[0][1];
  fireEvent.keyDown(screen.getByRole("dialog", { name: "人物与发言核对" }), { key: "Escape" });
  expect(signal?.aborted).toBe(false);
  fireEvent.click(screen.getByRole("button", { name: "查看进度" }));
  expect(episodeCopilotApi.preparePeople).toHaveBeenCalledTimes(1);
  await act(async () => complete(proposal));
  expect(screen.getByRole("button", { name: "编辑匹配 小林" })).toBeVisible();
});
