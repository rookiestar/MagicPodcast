import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import EpisodeDetailPage from "../EpisodeDetailPage";
import { navigate } from "@/lib/navigation";
import type { ConsumptionItem } from "@/types/consumption";

const mocks = vi.hoisted(() => ({ getItem: vi.fn(), replace: vi.fn() }));
vi.mock("next/navigation", () => ({ usePathname: () => window.location.pathname, useRouter: () => ({ replace: mocks.replace }) }));
vi.mock("@/components/layout/PageLayout", () => ({ default: ({ children }: { children: React.ReactNode }) => children }));
vi.mock("@/lib/api/consumption", () => ({
  consumptionApi: { getItem: mocks.getItem },
  getConsumptionErrorDetails: (error: { status?: number }) => error,
  requiresFocusConfirmation: () => false,
}));
vi.mock("../ConsumptionDetailPanel", () => ({ default: ({ item, routeState }: { item: ConsumptionItem; routeState: { tab: string } }) => <section aria-label="单集内容">{item.episode_title} · {routeState.tab}</section> }));
vi.mock("../FocusLimitDialog", () => ({ default: () => null }));

function item(id: number) { return { episode_id: id, episode_title: `单集 ${id}`, queue_state: null }; }
beforeEach(() => { mocks.getItem.mockReset(); window.history.replaceState({}, "", "/episodes/42?tab=notes"); });

describe("independent episode page", () => {
  it("reads an unassigned episode directly by ID and restores the requested tab", async () => {
    mocks.getItem.mockResolvedValue(item(42));
    render(<EpisodeDetailPage />);
    expect(await screen.findByRole("region", { name: "单集内容" })).toHaveTextContent("单集 42 · notes");
    expect(mocks.getItem).toHaveBeenCalledWith(42);
  });
  it("never fetches a guessed ID for an invalid resource address", async () => {
    window.history.replaceState({}, "", "/episodes/42bad");
    render(<EpisodeDetailPage />);
    expect(await screen.findByRole("alert")).toHaveTextContent("单集地址无效");
    expect(mocks.getItem).not.toHaveBeenCalled();
  });
  it("ignores an earlier episode response that arrives after navigation", async () => {
    let resolveOld: (value: ReturnType<typeof item>) => void = () => {};
    mocks.getItem.mockImplementation((id: number) => id === 42 ? new Promise((resolve) => { resolveOld = resolve; }) : Promise.resolve(item(id)));
    render(<EpisodeDetailPage />);
    act(() => { navigate("/episodes/43?tab=transcript"); });
    expect(await screen.findByRole("region", { name: "单集内容" })).toHaveTextContent("单集 43 · transcript");
    await act(async () => { resolveOld(item(42)); });
    expect(screen.getByRole("region", { name: "单集内容" })).toHaveTextContent("单集 43 · transcript");
  });
  it("keeps the target on failure and retries only its read", async () => {
    mocks.getItem.mockRejectedValueOnce({ status: 503 }).mockResolvedValue(item(42));
    render(<EpisodeDetailPage />);
    expect(await screen.findByRole("alert")).toHaveTextContent("单集读取失败");
    expect(window.location.search).toBe("?tab=notes");
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    expect(await screen.findByRole("region", { name: "单集内容" })).toHaveTextContent("单集 42 · notes");
  });
});
