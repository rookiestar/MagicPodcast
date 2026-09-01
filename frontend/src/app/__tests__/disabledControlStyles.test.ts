import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import postcss from "postcss";
import { describe, expect, it } from "vitest";

const globalsCss = readFileSync(resolve("src/app/globals.css"), "utf8");
const cssRoot = postcss.parse(globalsCss, { from: "src/app/globals.css" });

function declarations(selector: string): Record<string, string> {
  const result: Record<string, string> = {};

  cssRoot.walkRules((rule) => {
    if (rule.selector !== selector) return;
    rule.walkDecls((declaration) => {
      result[declaration.prop] = declaration.value.trim();
    });
  });

  return result;
}

describe("disabled control style contract", () => {
  it("distinguishes unavailable Discovery actions from busy actions", () => {
    expect(declarations(".discovery-action-button:disabled")).toMatchObject({
      cursor: "not-allowed",
    });
    expect(
      declarations('.discovery-action-button:disabled[aria-busy="true"]'),
    ).toMatchObject({ cursor: "wait" });
  });

  it("gates shared button hover and press feedback to enabled controls", () => {
    for (const selector of [
      ".editorial-btn:hover:not(:disabled)",
      ".editorial-btn:active:not(:disabled)",
      ".editorial-btn--primary:hover:not(:disabled)",
      ".editorial-btn--solid:hover:not(:disabled)",
      ".editorial-btn--ghost:hover:not(:disabled)",
      ".editorial-btn--danger:hover:not(:disabled)",
      ".editorial-btn--link:hover:not(:disabled)",
      ".workflow-page .workflow-card-action:hover:not(:disabled)",
      '.workflow-page .workflow-card button[title="执行"]:hover:not(:disabled)',
    ]) {
      expect(declarations(selector)).not.toEqual({});
    }
  });

  it("neutralizes the disabled OPML file picker hover state", () => {
    expect(declarations(".import-page .import-file-picker.is-disabled")).toMatchObject({
      cursor: "not-allowed",
    });
    expect(
      declarations(".import-page .import-file-picker.is-disabled:hover"),
    ).toMatchObject({
      color: "#171717",
      background: "transparent",
    });
  });
});
