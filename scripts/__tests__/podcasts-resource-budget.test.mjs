import assert from "node:assert/strict";
import { readFile, stat } from "node:fs/promises";
import test from "node:test";

const MAX_TEXTURE_BYTES = 30 * 1024;
const MAX_NEWSREADER_LATIN_BYTES = 65 * 1024;

test("deferred paper texture stays within the podcasts cold-load budget", async () => {
  const texture = await stat(
    new URL(
      "../../frontend/src/assets/warm-paper-grid-texture-v1.jpg",
      import.meta.url,
    ),
  );

  assert.ok(
    texture.size <= MAX_TEXTURE_BYTES,
    `paper texture is ${texture.size} bytes; expected <= ${MAX_TEXTURE_BYTES}`,
  );
});

test("Newsreader uses the compact weight-only Latin asset", async () => {
  const layout = await readFile(
    new URL("../../frontend/src/app/layout.tsx", import.meta.url),
    "utf8",
  );
  const newsreaderLatin = await stat(
    new URL(
      "../../frontend/node_modules/@fontsource-variable/newsreader/files/newsreader-latin-wght-normal.woff2",
      import.meta.url,
    ),
  );

  assert.match(
    layout,
    /@fontsource-variable\/newsreader\/wght\.css/,
  );
  assert.doesNotMatch(
    layout,
    /@fontsource-variable\/newsreader\/standard\.css/,
  );
  assert.ok(
    newsreaderLatin.size <= MAX_NEWSREADER_LATIN_BYTES,
    `Newsreader Latin is ${newsreaderLatin.size} bytes; expected <= ${MAX_NEWSREADER_LATIN_BYTES}`,
  );
});
