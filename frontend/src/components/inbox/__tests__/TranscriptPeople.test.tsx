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

describe("人物识别进度与恢复", () => {
  it("shows real phases, keeps the same request when hidden and cancels before readback", async () => {
    let report!: NonNullable<Parameters<typeof episodeCopilotApi.preparePeople>[2]>;
    let resolve!: (value: EpisodePeoplePayload) => void;
    let signal!: AbortSignal;
    vi.mocked(episodeCopilotApi.preparePeople).mockImplementation((_id, s, callback) => {
      report = callback!; signal = s!;
      return new Promise(done => { resolve = done; });
    });
    render(<Player />);
    fireEvent.click(await screen.findByRole("button", { name: "识别人物" }));
    expect(episodeCopilotApi.preparePeople).not.toHaveBeenCalled();
    const start = screen.getByRole("button", { name: "开始识别" });
    fireEvent.click(start); fireEvent.click(start);
    expect(episodeCopilotApi.preparePeople).toHaveBeenCalledTimes(1);
    act(() => report({ type: "stage", episode_id: 7, request_id: "a", source_version: "artifact-8", stage: "identify" }));
    expect(screen.getByText("正在识别出场人物")).toBeInTheDocument();
    act(() => report({ type: "heartbeat", episode_id: 7, request_id: "a", source_version: "artifact-8" }));
    expect(screen.getByText("正在识别出场人物")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "关闭人物核对" }));
    expect(signal.aborted).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "查看进度" }));
    expect(screen.getByText("正在识别出场人物")).toBeInTheDocument();
    vi.mocked(episodeCopilotApi.getPeople).mockResolvedValueOnce(empty);
    fireEvent.click(screen.getByRole("button", { name: "取消识别" }));
    expect(signal.aborted).toBe(true);
    await screen.findByText(/已请求取消/);
    await act(async () => resolve(proposal));
    expect(screen.queryByRole("button", { name: "编辑匹配 小林" })).not.toBeInTheDocument();
    expect(episodeCopilotApi.preparePeople).toHaveBeenCalledTimes(1);
  });

  it("recovers a committed draft after losing completion without applying or rerunning", async () => {
    render(<Player />);
    fireEvent.click(await screen.findByRole("button", { name: "识别人物" }));
    vi.mocked(episodeCopilotApi.preparePeople).mockRejectedValueOnce(new Error("disconnected"));
    vi.mocked(episodeCopilotApi.getPeople).mockResolvedValueOnce(proposal);
    fireEvent.click(screen.getByRole("button", { name: "开始识别" }));
    await screen.findByText("已核对：草稿已保存，等待你确认");
    expect(screen.getAllByRole("button", { name: "Speaker 1" })).toHaveLength(2);
    expect(episodeCopilotApi.reviewPeople).not.toHaveBeenCalled();
    expect(episodeCopilotApi.preparePeople).toHaveBeenCalledTimes(1);
  });

  it("blocks retry until uncertain persistence is read back successfully", async () => {
    render(<Player />);
    fireEvent.click(await screen.findByRole("button", { name: "识别人物" }));
    vi.mocked(episodeCopilotApi.preparePeople).mockRejectedValueOnce(new Error("disconnected"));
    vi.mocked(episodeCopilotApi.getPeople).mockRejectedValueOnce(new Error("offline"));
    fireEvent.click(screen.getByRole("button", { name: "开始识别" }));
    await screen.findByText(/结果状态尚未确认/);
    expect(screen.getByRole("button", { name: "开始识别" })).toBeDisabled();
    vi.mocked(episodeCopilotApi.getPeople).mockResolvedValueOnce(empty);
    fireEvent.click(screen.getByRole("button", { name: "重新读取" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "开始识别" })).toBeEnabled());
    expect(episodeCopilotApi.preparePeople).toHaveBeenCalledTimes(1);
  });

  it("cancels on leaving an episode and ignores its late completion", async () => {
    let resolve!: (value: EpisodePeoplePayload) => void;
    let signal!: AbortSignal;
    vi.mocked(episodeCopilotApi.preparePeople).mockImplementation((_id, s) => {
      signal = s!; return new Promise(done => { resolve = done; });
    });
    const view = render(<Player />);
    fireEvent.click(await screen.findByRole("button", { name: "识别人物" }));
    fireEvent.click(screen.getByRole("button", { name: "开始识别" }));
    vi.mocked(episodeCopilotApi.getPeople).mockResolvedValue({ ...empty, episode_id: 9, source_version: "artifact-10" });
    view.rerender(<TranscriptAudioPlayer episodeId={9} artifactSetId={10} segments={segments} mediaAvailable={false} playbackRate={1} onPlaybackRateChange={() => {}} />);
    expect(signal.aborted).toBe(true);
    await act(async () => resolve(proposal));
    fireEvent.click(await screen.findByRole("button", { name: "识别人物" }));
    expect(screen.queryByRole("button", { name: "编辑匹配 小林" })).not.toBeInTheDocument();
  });
});

it("cannot apply or edit a current server draft while displaying another source version", async () => {
  vi.mocked(episodeCopilotApi.getPeople).mockResolvedValue({ ...proposal,
    source_version: "artifact-99", draft: { ...proposal.draft!, source_version: "artifact-99" } });
  render(<Player />);
  fireEvent.click(await screen.findByRole("button", { name: "继续核对" }));
  expect(screen.getByRole("button", { name: "确认并应用" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "编辑匹配 小林" })).toBeDisabled();
  expect(screen.getByText(/来源已变化/)).toBeInTheDocument();
  expect(screen.getByText("本次将更新 0 段发言")).toBeInTheDocument();
  expect(episodeCopilotApi.reviewPeople).not.toHaveBeenCalled();
});

it("keeps manual entry available when identification saves an empty draft", async () => {
  vi.mocked(episodeCopilotApi.preparePeople).mockResolvedValue({ ...proposal, draft: { ...proposal.draft!, matches: [] } });
  render(<Player />);
  fireEvent.click(await screen.findByRole("button", { name: "识别人物" }));
  expect(episodeCopilotApi.preparePeople).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "开始识别" }));
  await screen.findByText("没有识别出可核对的人物，可直接编辑逐字稿中的 Speaker。");
  expect(screen.getByRole("button", { name: "填写姓名" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "确认并应用" })).toBeDisabled();
});

it("submits only the selected fragments and preserves edits after a revision conflict", async () => {
  vi.mocked(episodeCopilotApi.getPeople).mockResolvedValue(proposal);
  vi.mocked(episodeCopilotApi.reviewPeople).mockRejectedValue(new Error("人物资料已更新，请重新核对"));
  render(<Player />);
  fireEvent.click(await screen.findByRole("button", { name: "继续核对" }));
  fireEvent.click(screen.getByText("核对匹配范围 · 主持人 · 2 段"));
  fireEvent.click(screen.getByRole("checkbox", { name: "应该保持休息。" }));
  expect(screen.getByText("本次将更新 1 段发言")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "确认并应用" }));
  await screen.findByText("人物资料已更新，请重新核对");
  expect(episodeCopilotApi.reviewPeople).toHaveBeenCalledWith(7, expect.objectContaining({ matches: [expect.objectContaining({ orders: [1] })] }), true);
  fireEvent.click(screen.getByRole("button", { name: "重新读取" }));
  await screen.findByText("草稿尚未保存，请先保存或放弃修改。");
  expect(screen.getByText("本次将更新 1 段发言")).toBeInTheDocument();
  expect(episodeCopilotApi.getPeople).toHaveBeenCalledTimes(1);
});

it("retains expanded evidence when locating the transcript and reopening review", async () => {
  vi.mocked(episodeCopilotApi.getPeople).mockResolvedValue(proposal);
  render(<Player />);
  fireEvent.click(await screen.findByRole("button", { name: "继续核对" }));
  const summary = screen.getByText("核对匹配范围 · 主持人 · 2 段");
  fireEvent.click(summary);
  expect(summary.closest("details")).toHaveAttribute("open");
  fireEvent.click(screen.getAllByRole("button", { name: "定位 / 试听" })[0]);
  expect(screen.queryByRole("dialog", { name: "人物与发言核对" })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "继续核对" }));
  expect(screen.getByText("核对匹配范围 · 主持人 · 2 段").closest("details")).toHaveAttribute("open");
});
