import assert from "node:assert/strict";
import { once } from "node:events";
import http from "node:http";
import { spawn } from "node:child_process";
import test from "node:test";
import { gzipSync } from "node:zlib";

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

test("strict mode counts chunked gzip bytes before Node decompression", async (t) => {
  const sourceAsset = Buffer.alloc(Math.ceil(2.3 * ONE_MEBIBYTE), 1);
  const compressedAsset = gzipSync(sourceAsset);
  const server = http.createServer((request, response) => {
    const path = new URL(request.url, "http://localhost").pathname;

    if (path.startsWith("/api/") || path === "/health") {
      response.setHeader("content-type", "application/json");
      response.end(JSON.stringify({ data: [] }));
      return;
    }

    if (path === "/_next/static/chunked.css") {
      response.setHeader("content-type", "text/css");
      response.setHeader("content-encoding", "gzip");
      response.write(compressedAsset.subarray(0, 100));
      response.end(compressedAsset.subarray(100));
      return;
    }

    response.setHeader("content-type", "text/html");
    response.end(
      '<!doctype html><link rel="stylesheet" href="/_next/static/chunked.css">',
    );
  });
  const port = await listen(server);
  t.after(() => server.close());

  const result = await runAudit(`http://127.0.0.1:${port}`);

  assert.equal(result.code, 0, result.stderr);
  const podcasts = result.report.pages.find((page) => page.name === "podcasts");
  assert.equal(podcasts.assetBytes, compressedAsset.length);
  assert.equal(podcasts.assetSourceBytes, sourceAsset.length);
  assert.equal(podcasts.status, "OK");
});

test("static bundle estimate excludes responsive image fallback URLs", async (t) => {
  const bundle = Buffer.alloc(100_000, 1);
  const fallbackImage = Buffer.alloc(ONE_MEBIBYTE, 2);
  const server = http.createServer((request, response) => {
    const url = new URL(request.url, "http://localhost");

    if (url.pathname.startsWith("/api/") || url.pathname === "/health") {
      response.setHeader("content-type", "application/json");
      response.end(JSON.stringify({ data: [] }));
      return;
    }

    if (url.pathname === "/_next/static/app.js") {
      response.setHeader("content-type", "application/javascript");
      response.end(bundle);
      return;
    }

    if (url.pathname === "/_next/image.webp") {
      response.setHeader("content-type", "image/webp");
      response.end(fallbackImage);
      return;
    }

    response.setHeader("content-type", "text/html");
    response.end(
      [
        '<!doctype html><script src="/_next/static/app.js"></script>',
        '<img src="/_next/image.webp?url=%2Fcover.jpg&w=1920&q=75"',
        'srcset="/_next/image.webp?url=%2Fcover.jpg&w=256&q=75 256w">',
      ].join(""),
    );
  });
  const port = await listen(server);
  t.after(() => server.close());

  const result = await runAudit(`http://127.0.0.1:${port}`);

  assert.equal(result.code, 0, result.stderr);
  const podcasts = result.report.pages.find((page) => page.name === "podcasts");
  assert.equal(podcasts.assetCount, 1);
  assert.equal(podcasts.assetBytes, bundle.length);
  assert.equal(podcasts.status, "OK");
});
