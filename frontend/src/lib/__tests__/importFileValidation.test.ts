import { describe, expect, it } from "vitest";
import { isValidOpmlFile } from "../importFileValidation";

describe("importFileValidation", () => {
  it("accepts OPML and XML files by extension or MIME type", () => {
    expect(isValidOpmlFile({ name: "feeds.opml", type: "" })).toBe(true);
    expect(isValidOpmlFile({ name: "feeds.xml", type: "" })).toBe(true);
    expect(isValidOpmlFile({ name: "feeds", type: "text/opml" })).toBe(true);
    expect(isValidOpmlFile({ name: "feeds", type: "application/xml" })).toBe(
      true,
    );
  });

  it("rejects unrelated file types", () => {
    expect(isValidOpmlFile({ name: "notes.txt", type: "text/plain" })).toBe(
      false,
    );
  });
});
