#!/usr/bin/env node

import { performance } from "node:perf_hooks";

const DEFAULTS = {
  baseUrl: "http://localhost:3000",
  apiUrl: "http://localhost:8080",
  runs: 3,
  warmupRuns: 1,
  timeoutMs: 10000,
  pageWarnMs: 2500,
  apiWarnMs: 800,
  assetWarnBytes: 1_500_000,
  strict: false,
  json: false,
};

function parseArgs(argv) {
  const options = { ...DEFAULTS };

  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    const [name, inlineValue] = arg.split("=", 2);
    const nextValue = () => inlineValue ?? argv[++index];

    switch (name) {
      case "--base-url":
        options.baseUrl = nextValue();
        break;
      case "--api-url":
        options.apiUrl = nextValue();
        break;
      case "--runs":
        options.runs = Number.parseInt(nextValue(), 10);
        break;
      case "--warmup-runs":
        options.warmupRuns = Number.parseInt(nextValue(), 10);
        break;
      case "--timeout-ms":
        options.timeoutMs = Number.parseInt(nextValue(), 10);
        break;
      case "--page-warn-ms":
        options.pageWarnMs = Number.parseInt(nextValue(), 10);
        break;
      case "--api-warn-ms":
        options.apiWarnMs = Number.parseInt(nextValue(), 10);
        break;
      case "--asset-warn-bytes":
        options.assetWarnBytes = Number.parseInt(nextValue(), 10);
        break;
      case "--strict":
        options.strict = true;
        break;
      case "--json":
        options.json = true;
        break;
      case "--help":
      case "-h":
        printHelp();
        process.exit(0);
      default:
        throw new Error(`Unknown option: ${arg}`);
    }
  }

  if (!Number.isInteger(options.runs) || options.runs < 1) {
    throw new Error("--runs must be a positive integer");
  }
  if (!Number.isInteger(options.warmupRuns) || options.warmupRuns < 0) {
    throw new Error("--warmup-runs must be a non-negative integer");
  }

  options.baseUrl = trimTrailingSlash(options.baseUrl);
  options.apiUrl = trimTrailingSlash(options.apiUrl);
  return options;
}

function printHelp() {
  console.log(`
Usage:
  node scripts/performance-audit.mjs [options]

Options:
  --base-url <url>           Frontend URL. Default: ${DEFAULTS.baseUrl}
  --api-url <url>            Backend URL. Default: ${DEFAULTS.apiUrl}
  --runs <n>                 Samples per page/API. Default: ${DEFAULTS.runs}
  --warmup-runs <n>          Warmup requests before sampling. Default: ${DEFAULTS.warmupRuns}
  --timeout-ms <n>           Request timeout. Default: ${DEFAULTS.timeoutMs}
  --page-warn-ms <n>         Page warning threshold. Default: ${DEFAULTS.pageWarnMs}
  --api-warn-ms <n>          API warning threshold. Default: ${DEFAULTS.apiWarnMs}
  --asset-warn-bytes <n>     Static asset warning threshold. Default: ${DEFAULTS.assetWarnBytes}
  --strict                   Exit non-zero on warnings, not only failures
  --json                     Print machine-readable JSON
`);
}

function trimTrailingSlash(value) {
  return String(value || "").replace(/\/+$/, "");
}

function joinUrl(baseUrl, path) {
  if (/^https?:\/\//i.test(path)) return path;
  const cleanPath = path.startsWith("/") ? path : `/${path}`;
  return `${baseUrl}${cleanPath}`;
}

async function timedFetch(url, options = {}) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), options.timeoutMs);
  const startedAt = performance.now();

  try {
    const response = await fetch(url, {
      method: options.method || "GET",
      headers: options.headers,
      signal: controller.signal,
    });
    const headersAt = performance.now();
    const arrayBuffer = await response.arrayBuffer();
    const endedAt = performance.now();
    const body = Buffer.from(arrayBuffer);

    return {
      ok: response.ok,
      status: response.status,
      statusText: response.statusText,
      url: response.url,
      ttfbMs: headersAt - startedAt,
      totalMs: endedAt - startedAt,
      bytes: body.length,
      headers: Object.fromEntries(response.headers.entries()),
      body,
    };
  } catch (error) {
    const endedAt = performance.now();
    return {
      ok: false,
      status: 0,
      statusText: "FETCH_ERROR",
      url,
      ttfbMs: endedAt - startedAt,
      totalMs: endedAt - startedAt,
      bytes: 0,
      headers: {},
      body: Buffer.alloc(0),
      error: error?.name === "AbortError" ? "timeout" : error?.message || String(error),
    };
  } finally {
    clearTimeout(timeout);
  }
}

async function fetchJson(baseUrl, path, options) {
  const result = await timedFetch(joinUrl(baseUrl, path), {
    timeoutMs: options.timeoutMs,
    headers: { accept: "application/json" },
  });

  if (!result.ok) return { result, json: null };

  try {
    return { result, json: JSON.parse(result.body.toString("utf8")) };
  } catch (error) {
    return { result: { ...result, ok: false, error: `invalid json: ${error.message}` }, json: null };
  }
}

function extractItems(payload) {
  if (Array.isArray(payload)) return payload;
  if (Array.isArray(payload?.data)) return payload.data;
  if (Array.isArray(payload?.data?.data)) return payload.data.data;
  if (Array.isArray(payload?.data?.items)) return payload.data.items;
  if (Array.isArray(payload?.data?.podcasts)) return payload.data.podcasts;
  if (Array.isArray(payload?.data?.workflows)) return payload.data.workflows;
  if (Array.isArray(payload?.items)) return payload.items;
  return [];
}

function firstIdFrom(payload) {
  const item = extractItems(payload).find((entry) => entry?.id ?? entry?.ID);
  return item?.id ?? item?.ID ?? null;
}

async function discoverTargets(options) {
  const [podcasts, workflows] = await Promise.all([
    fetchJson(options.apiUrl, "/api/v1/podcasts?page=1&page_size=1&view=summary", options),
    fetchJson(options.apiUrl, "/api/v1/workflows?page=1&page_size=1", options),
  ]);

  const podcastId = firstIdFrom(podcasts.json);
  const workflowId = firstIdFrom(workflows.json);

  const pages = [
    { name: "home", path: "/" },
    { name: "podcasts", path: "/podcasts" },
    { name: "tags", path: "/tags" },
    { name: "import", path: "/import" },
    { name: "workflows", path: "/workflows" },
  ];

  if (podcastId) pages.splice(2, 0, { name: "podcast-detail", path: `/podcasts/${podcastId}` });
  if (workflowId) pages.push({ name: "workflow-detail", path: `/workflows/${workflowId}` });

  const apiProbes = [
    { name: "health", path: "/health" },
    { name: "podcasts-summary", path: "/api/v1/podcasts?page=1&page_size=15&view=summary" },
    { name: "tags", path: "/api/v1/tags" },
    { name: "workflows", path: "/api/v1/workflows?page=1&page_size=10&view=summary" },
    {
      name: "search",
      path: "/api/v1/search?q=podcast&type=all&page=1&page_size=20&episode_page=1&episode_page_size=20&include_totals=false",
    },
  ];

  if (podcastId) {
    apiProbes.push(
      { name: "podcast-detail", path: `/api/v1/podcasts/${podcastId}` },
      { name: "podcast-episodes", path: `/api/v1/podcasts/${podcastId}/episodes?page=1&page_size=10&view=summary` },
      { name: "podcast-tags", path: `/api/v1/podcasts/${podcastId}/tags` },
      { name: "podcast-notes", path: `/api/v1/podcasts/${podcastId}/notes` },
    );
  }

  if (workflowId) {
    apiProbes.push(
      { name: "workflow-detail", path: `/api/v1/workflows/${workflowId}` },
      { name: "workflow-jobs", path: `/api/v1/workflows/${workflowId}/jobs?page=1&page_size=10&view=summary` },
    );
  }

  return { pages, apiProbes, discovered: { podcastId, workflowId } };
}

function extractStaticAssets(html) {
  const assets = new Set();
  const attributePattern = /\b(?:href|src)=["']([^"']+)["']/g;
  let match;

  while ((match = attributePattern.exec(html)) !== null) {
    const rawValue = match[1].replaceAll("&amp;", "&");
    if (
      rawValue.startsWith("/_next/") ||
      rawValue.startsWith("/favicon") ||
      rawValue.startsWith("/icons/") ||
      rawValue.startsWith("/manifest")
    ) {
      assets.add(rawValue);
    }
  }

  return [...assets];
}

async function estimateAssetBytes(baseUrl, assetPaths, options, cache) {
  let totalBytes = 0;
  const details = [];

  for (const path of assetPaths) {
    if (!cache.has(path)) {
      const result = await timedFetch(joinUrl(baseUrl, path), {
        method: "HEAD",
        timeoutMs: options.timeoutMs,
      });
      const contentLength = Number.parseInt(result.headers["content-length"] || "", 10);

      if (result.ok && Number.isFinite(contentLength)) {
        cache.set(path, { bytes: contentLength, status: result.status });
      } else {
        const fallback = await timedFetch(joinUrl(baseUrl, path), { timeoutMs: options.timeoutMs });
        cache.set(path, { bytes: fallback.bytes, status: fallback.status, error: fallback.error });
      }
    }

    const entry = cache.get(path);
    totalBytes += entry.bytes || 0;
    details.push({ path, ...entry });
  }

  return { totalBytes, details };
}

async function auditPages(targets, options) {
  const assetCache = new Map();
  const results = [];

  for (const target of targets) {
    const samples = [];
    let html = "";

    await warmupTarget(options.baseUrl, target.path, {
      timeoutMs: options.timeoutMs,
      headers: { accept: "text/html,application/xhtml+xml" },
      runs: options.warmupRuns,
    });

    for (let run = 0; run < options.runs; run += 1) {
      const result = await timedFetch(joinUrl(options.baseUrl, target.path), {
        timeoutMs: options.timeoutMs,
        headers: { accept: "text/html,application/xhtml+xml" },
      });
      samples.push(compactFetchResult(result));

      if (result.ok && result.body.length > 0) {
        html = result.body.toString("utf8");
      }
    }

    const assetPaths = html ? extractStaticAssets(html) : [];
    const assets = await estimateAssetBytes(options.baseUrl, assetPaths, options, assetCache);
    const stats = summarizeSamples(samples);

    results.push({
      ...target,
      ...stats,
      htmlBytes: latestSuccessfulBytes(samples),
      assetCount: assetPaths.length,
      assetBytes: assets.totalBytes,
      status: classifyPage(stats, assets.totalBytes, options),
      samples,
    });
  }

  return results;
}

async function auditApis(targets, options) {
  const results = [];

  for (const target of targets) {
    const samples = [];

    await warmupTarget(options.apiUrl, target.path, {
      timeoutMs: options.timeoutMs,
      headers: { accept: "application/json" },
      runs: options.warmupRuns,
    });

    for (let run = 0; run < options.runs; run += 1) {
      const result = await timedFetch(joinUrl(options.apiUrl, target.path), {
        timeoutMs: options.timeoutMs,
        headers: { accept: "application/json" },
      });
      samples.push(compactFetchResult(result));
    }

    const stats = summarizeSamples(samples);
    results.push({
      ...target,
      ...stats,
      bytes: latestSuccessfulBytes(samples),
      status: classifyApi(stats, options),
      samples,
    });
  }

  return results;
}

async function warmupTarget(baseUrl, path, options) {
  for (let run = 0; run < options.runs; run += 1) {
    await timedFetch(joinUrl(baseUrl, path), {
      timeoutMs: options.timeoutMs,
      headers: options.headers,
    });
  }
}

function compactFetchResult(result) {
  return {
    ok: result.ok,
    status: result.status,
    ttfbMs: round(result.ttfbMs),
    totalMs: round(result.totalMs),
    bytes: result.bytes,
    error: result.error,
  };
}

function latestSuccessfulBytes(samples) {
  const sample = [...samples].reverse().find((entry) => entry.ok);
  return sample?.bytes || 0;
}

function summarizeSamples(samples) {
  const totalTimes = samples.map((sample) => sample.totalMs);
  const ttfbTimes = samples.map((sample) => sample.ttfbMs);
  const statusCodes = [...new Set(samples.map((sample) => sample.status))].sort((a, b) => a - b);

  return {
    ok: samples.every((sample) => sample.ok),
    statuses: statusCodes,
    avgTotalMs: round(avg(totalTimes)),
    p95TotalMs: round(percentile(totalTimes, 0.95)),
    avgTtfbMs: round(avg(ttfbTimes)),
    p95TtfbMs: round(percentile(ttfbTimes, 0.95)),
    maxTotalMs: round(Math.max(...totalTimes)),
  };
}

function classifyPage(stats, assetBytes, options) {
  if (!stats.ok) return "FAIL";
  if (stats.avgTotalMs > options.pageWarnMs) return "SLOW";
  if (assetBytes > options.assetWarnBytes) return "HEAVY";
  return "OK";
}

function classifyApi(stats, options) {
  if (!stats.ok) return "FAIL";
  if (stats.avgTotalMs > options.apiWarnMs) return "SLOW";
  return "OK";
}

function avg(values) {
  if (values.length === 0) return 0;
  return values.reduce((sum, value) => sum + value, 0) / values.length;
}

function percentile(values, ratio) {
  if (values.length === 0) return 0;
  const sorted = [...values].sort((a, b) => a - b);
  const index = Math.min(sorted.length - 1, Math.ceil(sorted.length * ratio) - 1);
  return sorted[index];
}

function round(value) {
  return Math.round(value * 10) / 10;
}

function formatMs(value) {
  return `${Math.round(value)}ms`;
}

function formatBytes(bytes) {
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)}MB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)}KB`;
  return `${bytes}B`;
}

function formatStatuses(statuses) {
  return statuses.join(",");
}

function printTable(columns, rows) {
  const widths = columns.map((column) =>
    Math.max(column.label.length, ...rows.map((row) => String(column.value(row)).length)),
  );

  const header = columns.map((column, index) => column.label.padEnd(widths[index])).join("  ");
  const divider = widths.map((width) => "-".repeat(width)).join("  ");
  console.log(header);
  console.log(divider);

  for (const row of rows) {
    console.log(columns.map((column, index) => String(column.value(row)).padEnd(widths[index])).join("  "));
  }
}

function printReport(report) {
  console.log("MagicPodcast performance audit");
  console.log(`Frontend: ${report.options.baseUrl}`);
  console.log(`Backend:  ${report.options.apiUrl}`);
  console.log(`Runs:     ${report.options.runs}`);
  console.log(`Warmup:   ${report.options.warmupRuns}`);
  console.log("");

  console.log("Pages");
  printTable(
    [
      { label: "status", value: (row) => row.status },
      { label: "name", value: (row) => row.name },
      { label: "path", value: (row) => row.path },
      { label: "http", value: (row) => formatStatuses(row.statuses) },
      { label: "avg", value: (row) => formatMs(row.avgTotalMs) },
      { label: "p95", value: (row) => formatMs(row.p95TotalMs) },
      { label: "html", value: (row) => formatBytes(row.htmlBytes) },
      { label: "assets", value: (row) => `${row.assetCount}/${formatBytes(row.assetBytes)}` },
    ],
    report.pages,
  );
  console.log("");

  console.log("Slow API top 5");
  printTable(
    [
      { label: "status", value: (row) => row.status },
      { label: "name", value: (row) => row.name },
      { label: "path", value: (row) => row.path },
      { label: "http", value: (row) => formatStatuses(row.statuses) },
      { label: "avg", value: (row) => formatMs(row.avgTotalMs) },
      { label: "p95", value: (row) => formatMs(row.p95TotalMs) },
      { label: "bytes", value: (row) => formatBytes(row.bytes) },
    ],
    [...report.apis].sort((a, b) => b.avgTotalMs - a.avgTotalMs).slice(0, 5),
  );
  console.log("");

  if (report.failures.length > 0 || report.warnings.length > 0) {
    console.log("Findings");
    for (const finding of [...report.failures, ...report.warnings]) {
      console.log(`- ${finding}`);
    }
  } else {
    console.log("Findings");
    console.log("- No failures or warnings.");
  }
}

function collectFindings(pages, apis, options) {
  const failures = [];
  const warnings = [];

  for (const page of pages) {
    if (page.status === "FAIL") {
      failures.push(`Page ${page.name} returned HTTP ${formatStatuses(page.statuses)} (${page.path})`);
    } else if (page.status === "SLOW") {
      warnings.push(`Page ${page.name} averaged ${formatMs(page.avgTotalMs)} (${page.path})`);
    } else if (page.status === "HEAVY") {
      warnings.push(`Page ${page.name} static assets total ${formatBytes(page.assetBytes)} (${page.path})`);
    }
  }

  for (const api of apis) {
    if (api.status === "FAIL") {
      failures.push(`API ${api.name} returned HTTP ${formatStatuses(api.statuses)} (${api.path})`);
    } else if (api.status === "SLOW") {
      warnings.push(`API ${api.name} averaged ${formatMs(api.avgTotalMs)} (${api.path})`);
    }
  }

  const shouldFail = failures.length > 0 || (options.strict && warnings.length > 0);
  return { failures, warnings, shouldFail };
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const targets = await discoverTargets(options);
  const [pages, apis] = await Promise.all([auditPages(targets.pages, options), auditApis(targets.apiProbes, options)]);
  const findings = collectFindings(pages, apis, options);
  const report = {
    generatedAt: new Date().toISOString(),
    options,
    discovered: targets.discovered,
    pages,
    apis,
    failures: findings.failures,
    warnings: findings.warnings,
  };

  if (options.json) {
    console.log(JSON.stringify(report, null, 2));
  } else {
    printReport(report);
  }

  process.exitCode = findings.shouldFail ? 1 : 0;
}

main().catch((error) => {
  console.error(error?.stack || error?.message || String(error));
  process.exitCode = 1;
});
