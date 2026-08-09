import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const globalsCss = readFileSync(resolve("src/app/globals.css"), "utf8");

describe("editorial asset critical path", () => {
  it("keeps web fonts and the paper texture behind the ready class", () => {
    expect(globalsCss).toContain(":root.editorial-assets-ready");
    expect(globalsCss).toMatch(
      /:root\.editorial-assets-ready[\s\S]*--font-serif:\s*"Newsreader Variable"/,
    );
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
});
