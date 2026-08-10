import { readdirSync, readFileSync } from "node:fs";
import { extname, join, resolve } from "node:path";
import postcss from "postcss";
import { describe, expect, it } from "vitest";

const globalsCss = readFileSync(resolve("src/app/globals.css"), "utf8");
const cssRoot = postcss.parse(globalsCss, {
  from: "src/app/globals.css",
});

function readApplicationSource(directory: string): string {
  return readdirSync(directory, { withFileTypes: true })
    .filter((entry) => entry.name !== "__tests__")
    .map((entry) => {
      const path = join(directory, entry.name);
      if (entry.isDirectory()) {
        return readApplicationSource(path);
      }

      return [".js", ".jsx", ".ts", ".tsx"].includes(extname(entry.name))
        ? readFileSync(path, "utf8")
        : "";
    })
    .join("\n");
}

function getDeclarations(selector: string): Record<string, string> {
  const declarations: Record<string, string> = {};

  cssRoot.walkRules((rule) => {
    if (rule.selector !== selector) return;
    rule.walkDecls((declaration) => {
      declarations[declaration.prop] = declaration.value.trim();
    });
  });

  return declarations;
}

function getDeclarationsForSelector(selector: string): Record<string, string> {
  const declarations: Record<string, string> = {};

  cssRoot.walkRules((rule) => {
    if (!rule.selectors.includes(selector)) return;
    rule.walkDecls((declaration) => {
      declarations[declaration.prop] = declaration.value.trim();
    });
  });

  return declarations;
}

const applicationSource = readApplicationSource(resolve("src"));

describe("typography contract", () => {
  it("keeps UI and metadata weights within the approved range", () => {
    const disallowedCssWeights: string[] = [];

    cssRoot.walkDecls((declaration) => {
      const value = declaration.value.trim();
      const hasHeavyWeight =
        declaration.prop === "font-weight" &&
        /^(650|700|800|900)$/.test(value);
      const hasHeavyShorthand =
        declaration.prop === "font" &&
        /^(650|700|800|900)\b/.test(value);

      if (hasHeavyWeight || hasHeavyShorthand) {
        disallowedCssWeights.push(
          `${declaration.parent?.toString() ?? "unknown"}: ${declaration.toString()}`,
        );
      }
    });

    expect(disallowedCssWeights).toEqual([]);
    expect(applicationSource).not.toMatch(
      /\bfont-(?:bold|extrabold|black)\b/,
    );
    expect(applicationSource).not.toMatch(
      /\bfont-\[(?:650|700|800|900)\]\b/,
    );
  });

  it("keeps the sort drawer heading and controls on their intended roles", () => {
    expect(getDeclarations(".podcast-sort-drawer-heading h3")).toMatchObject({
      "font-family": "var(--font-cjk-display)",
      "font-weight": "400",
    });
    expect(getDeclarations(".podcast-sort-options button")).toMatchObject({
      "font-family": "var(--font-sans)",
      "font-weight": "600",
    });
    expect(getDeclarations(".podcast-sort-cancel")).toMatchObject({
      "font-family": "var(--font-sans)",
      "font-weight": "600",
    });
  });

  it("keeps primary navigation and workflow reports on scoped semantic roles", () => {
    expect(getDeclarations(".type-nav")).toMatchObject({
      "font-family": "var(--font-sans)",
      "font-size": "var(--type-nav-size)",
      "font-weight": "600",
      "line-height": "var(--type-nav-leading)",
    });
    expect(getDeclarationsForSelector(".app-navbar-links a")).toMatchObject({
      "font-size": "var(--type-nav-size)",
      "line-height": "var(--type-nav-leading)",
    });
    expect(
      getDeclarationsForSelector(".mobile-bottom-nav a"),
    ).toMatchObject({
      "font-size": "var(--type-nav-size)",
      "line-height": "var(--type-nav-leading)",
    });
    expect(getDeclarations(".editorial-rich-text--report")).toMatchObject({
      "font-size": "var(--type-report-size)",
      "line-height": "var(--type-report-leading)",
    });
  });

  it("reserves sub-11px text for decorative kickers and markers", () => {
    const approvedTinySelectors = new Set([
      ".editorial-kicker",
      ".import-page .import-eyebrow",
      ".podcast-library-card-cover > .podcast-library-card-new",
      ".podcast-reading-kicker",
      ".search-workbench-kicker small",
    ]);
    const tinySelectors = new Set<string>();

    cssRoot.walkDecls((declaration) => {
      const value = declaration.value.trim();
      const pixelSize = declaration.prop === "font-size"
        ? value.match(/^([0-9.]+)px$/)
        : null;
      const remSize = declaration.prop === "font-size"
        ? value.match(/^([0-9.]+)rem$/)
        : null;
      const shorthandSize = declaration.prop === "font"
        ? value.match(/\b([0-9.]+)px\//)
        : null;
      const sizeInPixels = pixelSize
        ? Number(pixelSize[1])
        : remSize
          ? Number(remSize[1]) * 16
          : shorthandSize
            ? Number(shorthandSize[1])
            : null;

      if (sizeInPixels !== null && sizeInPixels < 11) {
        const selector =
          declaration.parent?.type === "rule"
            ? declaration.parent.selector
            : "unknown";
        tinySelectors.add(selector);
      }
    });

    expect([...tinySelectors].sort()).toEqual(
      [...approvedTinySelectors].sort(),
    );
    expect(applicationSource).not.toMatch(/\btext-\[(?:9|10)px\]\b/);
  });

  it("uses only the approved font-family tokens", () => {
    const approvedFamily =
      /^var\(--font-(?:body|cjk-display|display|display-bold|latin-display|mono|sans|serif)\)$/;
    const invalidFamilies: string[] = [];

    cssRoot.walkDecls((declaration) => {
      const value = declaration.value.trim();
      if (
        declaration.prop === "font-family" &&
        !approvedFamily.test(value)
      ) {
        invalidFamilies.push(value);
      }
      if (
        declaration.prop === "font" &&
        value !== "inherit" &&
        !value.includes("var(--font-")
      ) {
        invalidFamilies.push(value);
      }
    });

    expect(invalidFamilies).toEqual([]);
  });
});
