import assert from "node:assert/strict";
import { once } from "node:events";
import http from "node:http";
import { spawn } from "node:child_process";
import test from "node:test";

const ONE_MEBIBYTE = 1024 * 1024;

function listen(server) {
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      server.off("error", reject);
      resolve(server.address().port);
    });
  });
}

function runAudit(baseUrl) {
  const child = spawn(
    process.execPath,
    [
      "scripts/performance-audit.mjs",
      "--base-url",
      baseUrl,
      "--api-url",
      baseUrl,
      "--runs",
      "1",
      "--warmup-runs",
      "0",
      "--asset-warn-bytes",
      String(900_000),
      "--strict",
      "--json",
    ],
    {
      cwd: new URL("../..", import.meta.url),
      stdio: ["ignore", "pipe", "pipe"],
    },
  );

  let stdout = "";
  let stderr = "";
  child.stdout.setEncoding("utf8");
  child.stderr.setEncoding("utf8");
  child.stdout.on("data", (chunk) => {
    stdout += chunk;
  });
  child.stderr.on("data", (chunk) => {
    stderr += chunk;
  });

  return once(child, "close").then(([code]) => ({
    code,
    report: JSON.parse(stdout),
    stderr,
  }));
}

test("strict mode gates encoded browser-equivalent bytes, not HEAD source size", async (t) => {
  const compressedAsset = Buffer.alloc(100_000, 1);
  const server = http.createServer((request, response) => {
    const path = new URL(request.url, "http://localhost").pathname;

    if (path.startsWith("/api/") || path === "/health") {
      response.setHeader("content-type", "application/json");
      response.end(JSON.stringify({ data: [] }));
      return;
    }

    if (path === "/_next/static/large.css") {
      response.setHeader("content-type", "text/css");
      response.setHeader("x-encoded-content-length", String(compressedAsset.length));
      response.setHeader("x-source-size", String(2.3 * ONE_MEBIBYTE));
      response.end(compressedAsset);
      return;
    }

    response.setHeader("content-type", "text/html");
    response.end(
      '<!doctype html><link rel="stylesheet" href="/_next/static/large.css">',
    );
  });
  const port = await listen(server);
  t.after(() => server.close());

  const result = await runAudit(`http://127.0.0.1:${port}`);

  assert.equal(result.code, 0, result.stderr);
  const podcasts = result.report.pages.find((page) => page.name === "podcasts");
  assert.equal(podcasts.assetBytes, compressedAsset.length);
  assert.ok(podcasts.assetSourceBytes > 2 * ONE_MEBIBYTE);
  assert.equal(podcasts.status, "OK");
});
