import { describe, expect, it } from "vitest";
import {
  inlineApiErrorConfig,
  shouldPresentApiErrorGlobally,
} from "../client";

describe("API error presentation", () => {
  it("keeps locally rendered failures out of global toast handling", () => {
    expect(shouldPresentApiErrorGlobally(inlineApiErrorConfig)).toBe(false);
  });

  it("keeps the existing global presentation by default", () => {
    expect(shouldPresentApiErrorGlobally()).toBe(true);
  });
});
