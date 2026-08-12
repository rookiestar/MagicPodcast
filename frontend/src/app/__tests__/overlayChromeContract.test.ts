import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import postcss from "postcss";
import { describe, expect, it } from "vitest";

const cssRoot = postcss.parse(
  readFileSync(resolve("src/app/globals.css"), "utf8"),
  { from: "src/app/globals.css" },
);

function declarations(
  selector: string,
  includeMediaRules = true,
): Record<string, string> {
  const result: Record<string, string> = {};

  cssRoot.walkRules((rule) => {
    if (!rule.selectors.includes(selector)) return;
    if (!includeMediaRules) {
      if (rule.parent?.type === "atrule" && rule.parent.name === "media") return;
    }
    rule.walkDecls((declaration) => {
      result[declaration.prop] = declaration.value.trim();
    });
  });

  return result;
}

describe("overlay chrome contract", () => {
  it("keeps the search sheet compact and single-column", () => {
    expect(declarations(".search-workbench", false)).toMatchObject({
      "max-width": "min(640px, 50vw)",
      "border-left": "1px solid #171717",
    });
    expect(declarations(".search-podcast-results")).not.toHaveProperty(
      "grid-template-columns",
    );
    expect(declarations(".search-podcast-result", false)).toMatchObject({
      display: "grid",
      "grid-template-columns": "72px minmax(0, 1fr)",
    });
  });

  it("shares the 44px close control across modal surfaces", () => {
    expect(declarations(".search-workbench-close")).toMatchObject({
      width: "44px",
      height: "44px",
      border: "1px solid #171717",
    });
    expect(declarations(".editorial-modal-close")).toMatchObject({
      width: "44px",
      height: "44px",
      border: "1px solid #171717",
    });
  });
});
