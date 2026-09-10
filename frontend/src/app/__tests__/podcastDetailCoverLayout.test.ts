import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import postcss from "postcss";
import { describe, expect, it } from "vitest";

const cssRoot = postcss.parse(
  readFileSync(resolve("src/app/globals.css"), "utf8"),
  { from: "src/app/globals.css" },
);

function isInsideMedia(node: postcss.Node | undefined): boolean {
  if (!node) {
    return false;
  }
  if (node.type === "atrule" && (node as postcss.AtRule).name === "media") {
    return true;
  }
  return isInsideMedia(node.parent);
}

function getDeclarations(selector: string): Record<string, string> {
  const declarations: Record<string, string> = {};

  cssRoot.walkRules((rule) => {
    if (isInsideMedia(rule.parent) || !rule.selectors.includes(selector)) {
      return;
    }
    rule.walkDecls((declaration) => {
      declarations[declaration.prop] = declaration.value.trim();
    });
  });

  return declarations;
}

describe("podcast detail cover layout", () => {
  it("pins the desktop cover to a 96px square beside the heading", () => {
    const hero = getDeclarations(".podcast-reading-hero");
    const heading = getDeclarations(".podcast-reading-heading");
    const cover = getDeclarations(".podcast-reading-cover");
    const coverInner = getDeclarations(".podcast-reading-cover > div");

    expect(hero["grid-template-columns"]).toBe(
      "minmax(0, 1fr) minmax(280px, 330px)",
    );
    expect(heading.display).toBe("flex");
    expect(heading["align-items"]).toBe("center");
    expect(cover.width).toBe("96px");
    expect(cover.height).toBe("96px");
    expect(cover.flex).toBe("0 0 96px");
    expect(coverInner.width).toBe("96px");
    expect(coverInner.height).toBe("96px");
  });

  it("letterboxes non-square detail covers instead of stretching them", () => {
    const objectFitBySelector = new Map<string, string>();

    cssRoot.walkRules((rule) => {
      for (const selector of [
        ".podcast-reading-cover img",
        ".podcast-reading-mobile-cover img",
      ]) {
        if (!rule.selectors.includes(selector)) continue;
        rule.walkDecls("object-fit", (declaration) => {
          objectFitBySelector.set(selector, declaration.value.trim());
        });
      }
    });

    expect(objectFitBySelector.get(".podcast-reading-cover img")).toBe(
      "contain",
    );
    expect(objectFitBySelector.get(".podcast-reading-mobile-cover img")).toBe(
      "contain",
    );
  });

  it("does not restore a stretching cover column between tablet and desktop", () => {
    let tabletHeroColumns = "";

    cssRoot.walkAtRules("media", (mediaRule) => {
      if (!mediaRule.params.includes("max-width: 1120px")) {
        return;
      }
      mediaRule.walkRules((rule) => {
        if (!rule.selectors.includes(".podcast-reading-hero")) {
          return;
        }
        rule.walkDecls("grid-template-columns", (declaration) => {
          tabletHeroColumns = declaration.value.trim();
        });
      });
    });

    expect(tabletHeroColumns).toBe("minmax(0, 1fr)");
    expect(tabletHeroColumns).not.toMatch(/220px|280px/);
  });

  it("keeps the mobile cover in a fixed square independent of description height", () => {
    let mobileCoverWidth = "";
    let mobileCoverHeight = "";
    let mobileCoverFlex = "";

    cssRoot.walkAtRules("media", (mediaRule) => {
      if (!mediaRule.params.includes("max-width: 767px")) {
        return;
      }
      mediaRule.walkRules((rule) => {
        if (!rule.selectors.includes(".podcast-reading-mobile-cover")) {
          return;
        }
        rule.walkDecls((declaration) => {
          if (declaration.prop === "width") {
            mobileCoverWidth = declaration.value.trim();
          }
          if (declaration.prop === "height") {
            mobileCoverHeight = declaration.value.trim();
          }
          if (declaration.prop === "flex") {
            mobileCoverFlex = declaration.value.trim();
          }
        });
      });
    });

    expect(mobileCoverWidth).toBe("92px");
    expect(mobileCoverHeight).toBe("92px");
    expect(mobileCoverFlex).toBe("0 0 92px");
  });
});
