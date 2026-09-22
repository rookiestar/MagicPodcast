import { describe, expect, it } from "vitest";
import { getRelativeTime, isValidDisplayDate } from "../timeUtils";

// #463：缺失、零值与无效日期不得再渲染出「125 年前」这类年代差。
describe("isValidDisplayDate", () => {
  it("rejects null, empty, unparsable and zero-epoch placeholder dates", () => {
    expect(isValidDisplayDate(null)).toBe(false);
    expect(isValidDisplayDate(undefined)).toBe(false);
    expect(isValidDisplayDate("")).toBe(false);
    expect(isValidDisplayDate("not-a-date")).toBe(false);
    expect(isValidDisplayDate("0001-01-01T00:00:00Z")).toBe(false);
    expect(isValidDisplayDate("1901-01-01T00:00:00Z")).toBe(false);
  });

  it("accepts real dates", () => {
    expect(isValidDisplayDate("2026-09-21T00:00:00Z")).toBe(true);
    expect(isValidDisplayDate("1970-01-02T00:00:00Z")).toBe(true);
  });
});

describe("getRelativeTime", () => {
  it("returns empty string for missing or invalid dates", () => {
    expect(getRelativeTime(null)).toBe("");
    expect(getRelativeTime(undefined)).toBe("");
    expect(getRelativeTime("not-a-date")).toBe("");
  });

  it("returns empty string for the zero-value date instead of 年代差", () => {
    const text = getRelativeTime("0001-01-01T00:00:00Z");
    expect(text).toBe("");
    expect(text).not.toContain("年前");
  });

  it("keeps relative phrasing for valid dates", () => {
    const twoHoursAgo = new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString();
    expect(getRelativeTime(twoHoursAgo)).toBe("2小时前");
  });
});
