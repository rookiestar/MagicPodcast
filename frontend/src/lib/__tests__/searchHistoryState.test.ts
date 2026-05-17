import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  addToSearchHistory,
  clearSearchHistory,
  getSearchHistory,
} from "../searchHistoryState";

const memoryStorage = new Map<string, string>();

function installLocalStorageMock() {
  memoryStorage.clear();

  const localStorageMock = {
    getItem: vi.fn((key: string) => memoryStorage.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => {
      memoryStorage.set(key, value);
    }),
    removeItem: vi.fn((key: string) => {
      memoryStorage.delete(key);
    }),
    clear: vi.fn(() => {
      memoryStorage.clear();
    }),
  };

  Object.defineProperty(globalThis, "localStorage", {
    value: localStorageMock,
    configurable: true,
  });
  Object.defineProperty(window, "localStorage", {
    value: localStorageMock,
    configurable: true,
  });
}

describe("searchHistoryState", () => {
  beforeEach(() => {
    installLocalStorageMock();
  });

  it("deduplicates and caps search history", () => {
    ["a", "b", "c", "d", "e", "f"].forEach(addToSearchHistory);

    expect(addToSearchHistory("c")).toEqual(["c", "f", "e", "d", "b", "a"]);
    expect(addToSearchHistory("g")).toEqual(["g", "c", "f", "e", "d", "b"]);
    expect(getSearchHistory()).toEqual(["g", "c", "f", "e", "d", "b"]);
  });

  it("ignores invalid local storage content", () => {
    localStorage.setItem("podcast_search_history", '{"bad":true}');

    expect(getSearchHistory()).toEqual([]);
  });

  it("clears saved search history", () => {
    addToSearchHistory("科技");
    clearSearchHistory();

    expect(getSearchHistory()).toEqual([]);
  });
});
