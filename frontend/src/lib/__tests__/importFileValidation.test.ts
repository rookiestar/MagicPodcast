import { describe, expect, it } from "vitest";
import {
  isOpmlFileSizeAllowed,
  isValidOpmlFile,
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

describe("OPML file type validation", () => {
  it("accepts opml/xml extensions regardless of case", () => {
    expect(isValidOpmlFile({ name: "subs.opml", type: "" })).toBe(true);
    expect(isValidOpmlFile({ name: "subs.OPML", type: "" })).toBe(true);
    expect(isValidOpmlFile({ name: "subs.Xml", type: "" })).toBe(true);
  });

  it("accepts a mismatched or empty MIME when the extension is valid", () => {
    expect(isValidOpmlFile({ name: "subs.opml", type: "video/mp4" })).toBe(true);
    expect(isValidOpmlFile({ name: "subs.xml", type: "" })).toBe(true);
  });

  it("rejects files without a valid extension even when MIME looks right", () => {
    expect(isValidOpmlFile({ name: "subs.txt", type: "text/xml" })).toBe(false);
    expect(isValidOpmlFile({ name: "subs", type: "application/xml" })).toBe(false);
  });
});
