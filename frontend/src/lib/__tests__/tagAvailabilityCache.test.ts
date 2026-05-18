import { describe, expect, it, vi } from "vitest";
import type { Tag } from "@/types";
import { createAvailableTagCache } from "../tagAvailabilityCache";

const tags: Tag[] = [
  { id: 1, name: "科技", color: "#2563eb" },
  { id: 2, name: "AI", color: "#16a34a" },
];

describe("tagAvailabilityCache", () => {
  it("shares concurrent tag list requests", async () => {
    const fetchAvailableTags = vi.fn().mockResolvedValue(tags);
    const cache = createAvailableTagCache(fetchAvailableTags);

    const [first, second] = await Promise.all([cache.load(), cache.load()]);

    expect(fetchAvailableTags).toHaveBeenCalledTimes(1);
    expect(first).toBe(tags);
    expect(second).toBe(tags);
  });

  it("returns the cached tag list after the first load", async () => {
    const fetchAvailableTags = vi.fn().mockResolvedValue(tags);
    const cache = createAvailableTagCache(fetchAvailableTags);

    await cache.load();
    const cached = await cache.load();

    expect(fetchAvailableTags).toHaveBeenCalledTimes(1);
    expect(cached).toBe(tags);
  });

  it("can replace and clear the cached tag list", async () => {
    const fetchAvailableTags = vi.fn().mockResolvedValue(tags);
    const cache = createAvailableTagCache(fetchAvailableTags);
    const replacement = [{ id: 3, name: "生活", color: "#f97316" }];

    expect(cache.replace(replacement)).toBe(replacement);
    expect(await cache.load()).toBe(replacement);

    cache.clear();
    expect(await cache.load()).toBe(tags);
    expect(fetchAvailableTags).toHaveBeenCalledTimes(1);
  });

  it("retries after a failed request", async () => {
    const fetchAvailableTags = vi
      .fn()
      .mockRejectedValueOnce(new Error("failed"))
      .mockResolvedValueOnce(tags);
    const cache = createAvailableTagCache(fetchAvailableTags);

    await expect(cache.load()).rejects.toThrow("failed");
    await expect(cache.load()).resolves.toBe(tags);
    expect(fetchAvailableTags).toHaveBeenCalledTimes(2);
  });
});
