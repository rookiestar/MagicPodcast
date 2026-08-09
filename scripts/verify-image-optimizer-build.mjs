#!/usr/bin/env node

import { readFileSync } from "node:fs";
import path from "node:path";

const buildDirectory = process.argv[2];
const expectedPath = process.argv[3] || "/_next/image.webp";

if (!buildDirectory) {
  console.error(
    "用法: node scripts/verify-image-optimizer-build.mjs <build-directory> [expected-path]",
  );
  process.exit(2);
}

const manifestPath = path.join(
  path.resolve(buildDirectory),
  "required-server-files.json",
);

let manifest;
try {
  manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
} catch (error) {
  console.error(`无法读取前端构建配置: ${manifestPath}`);
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
}

const actualPath = manifest?.config?.images?.path;
if (actualPath !== expectedPath) {
  console.error(
    `图片优化路径不一致: expected=${expectedPath} actual=${actualPath || "missing"}`,
  );
  process.exit(1);
}

console.log(`image_optimizer_path=${actualPath}`);
