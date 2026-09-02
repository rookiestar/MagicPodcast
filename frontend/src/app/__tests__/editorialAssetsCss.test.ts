import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const globalsCss = readFileSync(resolve("src/app/globals.css"), "utf8");
const layoutSource = readFileSync(resolve("src/app/layout.tsx"), "utf8");

describe("editorial asset critical path", () => {
  it("keeps web fonts and the paper texture behind the ready class", () => {
    expect(globalsCss).toContain(":root.editorial-assets-ready");
    expect(globalsCss).toMatch(
      /:root\.editorial-typography-ready[\s\S]*--font-serif:\s*"Newsreader Variable"/,
    );
    const assetsReadyBlock =
      globalsCss.match(/:root\.editorial-assets-ready\s*{([\s\S]*?)\n  }/)?.[1] ?? "";
    expect(assetsReadyBlock).not.toContain("--font-serif");
    expect(assetsReadyBlock).not.toContain("--font-cjk-display");
    expect(globalsCss).toMatch(
      /:root\s*{[\s\S]*--editorial-paper-texture:\s*none/,
    );
  });

  it("uses the deferred texture token instead of eager background URLs", () => {
    const eagerTextureDeclarations =
      globalsCss.match(
        /background-image:\s*url\("\.\.\/assets\/warm-paper-grid-texture-v1\.jpg"\)/g,
      ) ?? [];

    expect(eagerTextureDeclarations).toHaveLength(0);
    expect(globalsCss).not.toContain("warm-paper-grid-texture-v1.jpg");
    expect(globalsCss).toContain(
      "background-image: var(--editorial-paper-texture)",
    );
  });

  it("keeps the podcast library and dynamic card titles on system CJK fonts", () => {
    expect(globalsCss).toMatch(
      /\.podcast-library-shell\s*{\s*--font-cjk-display:\s*var\(--font-sans\);\s*--font-serif:\s*var\(--font-latin-display\),\s*var\(--font-sans\);\s*--font-display:\s*var\(--font-serif\);\s*--font-display-bold:\s*var\(--font-serif\);/s,
    );
    expect(globalsCss).toMatch(
      /\.podcast-library-card-copy h3\s*{[\s\S]*font-family:\s*var\(--font-sans\)/,
    );
  });

  it("loads only the weight axis for Newsreader", () => {
    expect(layoutSource).toContain(
      '"@fontsource-variable/newsreader/wght.css"',
    );
    expect(layoutSource).not.toContain(
      '"@fontsource-variable/newsreader/standard.css"',
    );
  });
});
