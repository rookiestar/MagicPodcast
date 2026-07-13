import { describe, expect, it } from "vitest";
import {
  isOpmlFileSizeAllowed,
  MAX_OPML_FILE_SIZE_BYTES,
} from "../importFileValidation";

describe("OPML upload size validation", () => {
  it("allows the configured maximum", () => {
    expect(isOpmlFileSizeAllowed({ size: MAX_OPML_FILE_SIZE_BYTES })).toBe(true);
  });

  it("rejects files above the configured maximum", () => {
    expect(isOpmlFileSizeAllowed({ size: MAX_OPML_FILE_SIZE_BYTES + 1 })).toBe(false);
  });
});
