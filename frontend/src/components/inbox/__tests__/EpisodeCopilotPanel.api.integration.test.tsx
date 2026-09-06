import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import EpisodeCopilotPanel from "../EpisodeCopilotPanel";
import { episodeCopilotApi } from "@/lib/api/episodeCopilot";
import type { ConsumptionItem } from "@/types/consumption";

const item: ConsumptionItem = {
  episode_id: 301,
  podcast_id: 30,
  podcast_title: "测试节目",
  podcast_author: "测试作者",
  podcast_cover_url: "",
  episode_title: "档位错误集成测试",
  episode_no: "301",
  duration: 1200,
  published_date: "2026-09-06T08:00:00Z",
  show_notes: "Profile errors must remain non-retryable.",
  original_url: "https://example.com/episode/301",
  image_url: "",
  notes: "",
  tags: [],
  queue_state: "focus",
};

function errorStream() {
  const encoder = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      controller.enqueue(
        encoder.encode(
          'data: {"type":"error","message":"所选档位不可用",' +
            '"code":"profile_unavailable","retryable":false,' +
            '"transcript_used":false,"private_note_included":false,' +
            '"profile_id":"quick"}\n\n',
        ),
      );
      controller.close();
    },
  });
}

describe("EpisodeCopilotPanel API integration", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("keeps profile_unavailable non-retryable through the real API client", async () => {
    vi.spyOn(episodeCopilotApi, "getContext").mockResolvedValue({
      episode_id: 301,
      show_notes_available: true,
      transcript_available: false,
      private_note_available: false,
      profiles: [
        {
          id: "quick",
          model: "gpt-5.6-sol",
          effort: "medium",
          service_tier: "priority",
          service_tier_name: "Fast",
          is_default: false,
        },
        {
          id: "balanced",
          model: "gpt-5.6-luna",
          effort: "max",
          service_tier: "priority",
          service_tier_name: "Fast",
          is_default: true,
        },
        {
          id: "deep",
          model: "gpt-5.6-sol",
          effort: "xhigh",
          service_tier: "",
          service_tier_name: "Standard",
          is_default: false,
        },
      ],
      default_profile_id: "balanced",
    });
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(errorStream(), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    render(<EpisodeCopilotPanel item={item} />);
    const group = await screen.findByTestId("copilot-profiles");
    fireEvent.click(within(group).getByRole("radio", { name: /快速/ }));
    fireEvent.change(screen.getByRole("textbox", { name: "向单集助手提问" }), {
      target: { value: "检查不可用档位" },
    });
    fireEvent.click(screen.getByRole("button", { name: "提问" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "所选档位不可用",
    );
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(
      screen.queryByRole("button", { name: "重试" }),
    ).not.toBeInTheDocument();
  });
});
