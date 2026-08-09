import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it } from "vitest";

const projectRoot = path.resolve(process.cwd(), "..");
const verifier = path.join(
  projectRoot,
  "scripts",
  "verify-image-optimizer-build.mjs",
);
const temporaryDirectories: string[] = [];

function createBuild(imageOptimizerPath: string) {
  const buildDirectory = mkdtempSync(
    path.join(tmpdir(), "magicpodcast-image-build-"),
  );
  temporaryDirectories.push(buildDirectory);
  writeFileSync(
    path.join(buildDirectory, "required-server-files.json"),
    JSON.stringify({
      config: {
        images: {
          path: imageOptimizerPath,
        },
      },
    }),
  );
  return buildDirectory;
}

afterEach(() => {
  for (const directory of temporaryDirectories.splice(0)) {
    rmSync(directory, { force: true, recursive: true });
  }
});

describe("image optimizer build guard", () => {
  it("accepts the production image optimizer path", () => {
    const buildDirectory = createBuild("/_next/image.webp");

    expect(() =>
      execFileSync(process.execPath, [
        verifier,
        buildDirectory,
        "/_next/image.webp",
      ], { stdio: "ignore" }),
    ).not.toThrow();
  });

  it("rejects a build that silently falls back to another path", () => {
    const buildDirectory = createBuild("/_next/image");

    expect(() =>
      execFileSync(process.execPath, [
        verifier,
        buildDirectory,
        "/_next/image.webp",
      ], { stdio: "ignore" }),
    ).toThrow();
  });
});
