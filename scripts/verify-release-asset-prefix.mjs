#!/usr/bin/env node

import { readdir, readFile } from "node:fs/promises";
import path from "node:path";

const [buildDir, expectedPrefix] = process.argv.slice(2);

if (!buildDir || !expectedPrefix) {
  console.error("usage: verify-release-asset-prefix.mjs <build-dir> <asset-prefix>");
  process.exit(2);
}

async function collectHtmlFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const entryPath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...(await collectHtmlFiles(entryPath)));
    } else if (entry.isFile() && entry.name.endsWith(".html")) {
      files.push(entryPath);
    }
  }
  return files;
}

const htmlRoot = path.join(buildDir, "server");
const htmlFiles = await collectHtmlFiles(htmlRoot);
const staticPrefix = `${expectedPrefix.replace(/\/$/, "")}/_next/static/`;
const references = [];

for (const file of htmlFiles) {
  const html = await readFile(file, "utf8");
  for (const match of html.matchAll(/(?:src|href)="([^"]*\/_next\/static\/[^\"]+)"/g)) {
    references.push({ file, reference: match[1] });
  }
}

if (references.length === 0) {
  console.error(`no static asset references found under ${htmlRoot}`);
  process.exit(1);
}

const invalid = references.filter(({ reference }) => !reference.startsWith(staticPrefix));
if (invalid.length > 0) {
  for (const { file, reference } of invalid.slice(0, 5)) {
    console.error(`${path.relative(buildDir, file)}: ${reference}`);
  }
  console.error(
    `found ${invalid.length} static references outside release prefix ${staticPrefix}`,
  );
  process.exit(1);
}

console.log(
  `verified ${references.length} static references under ${staticPrefix}`,
);
