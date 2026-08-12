#!/usr/bin/env node

import { spawn } from "node:child_process";
import { access, mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const DEFAULTS = {
  baseUrl: "http://localhost:3000",
  path: "/podcasts",
  runs: 3,
  timeoutMs: 30_000,
  coldBudgetBytes: 900 * 1024,
  warmBudgetBytes: 50 * 1024,
  browserBin: process.env.CHROME_BIN || "",
  strict: false,
  json: false,
};

const CHROME_CANDIDATES = [
  "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
  "/Applications/Chromium.app/Contents/MacOS/Chromium",
  "/usr/bin/google-chrome",
  "/usr/bin/google-chrome-stable",
  "/usr/bin/chromium",
  "/usr/bin/chromium-browser",
];
const PODCASTS_FORBIDDEN_FONT_PATTERN = /lxgwwenkaigbscreen/i;
const IMAGE_ERROR_TRACKER_SCRIPT = `(() => {
  window.__magicPodcastImageErrorUrls = [];
  document.addEventListener("error", (event) => {
    const target = event.target;
    if (target instanceof HTMLImageElement) {
      window.__magicPodcastImageErrorUrls.push(
        target.currentSrc || target.src || "<unknown image>",
      );
    }
  }, true);
})()`;

function parseArgs(argv) {
  const options = { ...DEFAULTS };

  for (let index = 0; index < argv.length; index += 1) {
    const [name, inlineValue] = argv[index].split("=", 2);
    const nextValue = () => inlineValue ?? argv[++index];

    switch (name) {
      case "--base-url":
        options.baseUrl = nextValue();
        break;
      case "--path":
        options.path = nextValue();
        break;
      case "--runs":
        options.runs = Number.parseInt(nextValue(), 10);
        break;
      case "--timeout-ms":
        options.timeoutMs = Number.parseInt(nextValue(), 10);
        break;
      case "--cold-budget-bytes":
        options.coldBudgetBytes = Number.parseInt(nextValue(), 10);
        break;
      case "--warm-budget-bytes":
        options.warmBudgetBytes = Number.parseInt(nextValue(), 10);
        break;
      case "--browser-bin":
        options.browserBin = nextValue();
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
        break;
      default:
        throw new Error(`Unknown option: ${name}`);
    }
  }

  for (const [name, value, minimum] of [
    ["--runs", options.runs, 1],
    ["--timeout-ms", options.timeoutMs, 1],
    ["--cold-budget-bytes", options.coldBudgetBytes, 1],
    ["--warm-budget-bytes", options.warmBudgetBytes, 0],
  ]) {
    if (!Number.isInteger(value) || value < minimum) {
      throw new Error(`${name} must be an integer >= ${minimum}`);
    }
  }

  options.baseUrl = String(options.baseUrl || "").replace(/\/+$/, "");
  options.path = options.path.startsWith("/")
    ? options.path
    : `/${options.path}`;
  return options;
}

function printHelp() {
  console.log(`
Usage:
  node scripts/podcasts-resource-audit.mjs [options]

Options:
  --base-url <url>             Frontend URL. Default: ${DEFAULTS.baseUrl}
  --path <path>                Page path. Default: ${DEFAULTS.path}
  --runs <n>                   Cold/warm browser pairs. Default: ${DEFAULTS.runs}
  --timeout-ms <n>             Per navigation timeout. Default: ${DEFAULTS.timeoutMs}
  --cold-budget-bytes <n>      Cold transfer P95 budget. Default: ${DEFAULTS.coldBudgetBytes}
  --warm-budget-bytes <n>      Warm transfer P95 budget. Default: ${DEFAULTS.warmBudgetBytes}
  --browser-bin <path>         Chrome/Chromium executable. Defaults to CHROME_BIN or auto-detection
  --strict                     Exit non-zero when a transfer budget is exceeded
  --json                       Print machine-readable JSON
`);
}

async function findBrowser(explicitPath) {
  const candidates = explicitPath
    ? [explicitPath]
    : CHROME_CANDIDATES;

  for (const candidate of candidates) {
    try {
      await access(candidate);
      return candidate;
    } catch {
      // Try the next known Chrome/Chromium location.
    }
  }

  throw new Error(
    "Chrome/Chromium not found. Set CHROME_BIN or pass --browser-bin.",
  );
}

class PipeCdpClient {
  constructor(browserProcess) {
    this.browserProcess = browserProcess;
    this.writer = browserProcess.stdio[3];
    this.reader = browserProcess.stdio[4];
    this.nextId = 1;
    this.buffer = "";
    this.pending = new Map();
    this.eventListeners = new Set();

    this.reader.setEncoding("utf8");
    this.reader.on("data", (chunk) => this.handleData(chunk));
    this.reader.on("error", (error) => this.rejectAll(error));
    this.browserProcess.once("exit", (code, signal) => {
      this.rejectAll(
        new Error(
          `Chrome exited before CDP completed (code=${code}, signal=${signal})`,
        ),
      );
    });
  }

  handleData(chunk) {
    this.buffer += chunk;
    let delimiterIndex = this.buffer.indexOf("\0");

    while (delimiterIndex !== -1) {
      const rawMessage = this.buffer.slice(0, delimiterIndex);
      this.buffer = this.buffer.slice(delimiterIndex + 1);
      if (rawMessage) {
        this.handleMessage(JSON.parse(rawMessage));
      }
      delimiterIndex = this.buffer.indexOf("\0");
    }
  }

  handleMessage(message) {
    if (message.id) {
      const pending = this.pending.get(message.id);
      if (!pending) return;
      this.pending.delete(message.id);
      clearTimeout(pending.timeout);
      if (message.error) {
        pending.reject(
          new Error(
            `${pending.method}: ${message.error.message || "CDP error"}`,
          ),
        );
      } else {
        pending.resolve(message.result || {});
      }
      return;
    }

    for (const listener of this.eventListeners) {
      listener(message);
    }
  }

  rejectAll(error) {
    for (const pending of this.pending.values()) {
      clearTimeout(pending.timeout);
      pending.reject(error);
    }
    this.pending.clear();
  }

  send(method, params = {}, sessionId, timeoutMs = 10_000) {
    const id = this.nextId++;
    const message = { id, method, params };
    if (sessionId) message.sessionId = sessionId;

    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error(`${method} timed out after ${timeoutMs}ms`));
      }, timeoutMs);
      this.pending.set(id, { method, resolve, reject, timeout });
      this.writer.write(`${JSON.stringify(message)}\0`);
    });
  }

  onEvent(listener) {
    this.eventListeners.add(listener);
    return () => this.eventListeners.delete(listener);
  }

  waitForEvent(method, sessionId, timeoutMs) {
    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        unsubscribe();
        reject(new Error(`${method} timed out after ${timeoutMs}ms`));
      }, timeoutMs);
      const unsubscribe = this.onEvent((message) => {
        if (
          message.method === method &&
          (!sessionId || message.sessionId === sessionId)
        ) {
          clearTimeout(timeout);
          unsubscribe();
          resolve(message.params || {});
        }
      });
    });
  }
}

class ResourceTracker {
  constructor(client, sessionId) {
    this.sessionId = sessionId;
    this.requests = new Map();
    this.unsubscribe = client.onEvent((message) =>
      this.handleEvent(message),
    );
  }

  reset() {
    this.requests.clear();
  }

  handleEvent(message) {
    if (message.sessionId !== this.sessionId) return;
    const params = message.params || {};
    const requestId = params.requestId;

    if (message.method === "Network.requestWillBeSent") {
      const url = params.request?.url || "";
      if (!/^https?:/i.test(url)) return;
      this.requests.set(requestId, {
        url,
        type: params.type || "Other",
        status: 0,
        encodedBytes: 0,
        failed: false,
        finished: false,
        fromCache: false,
      });
      return;
    }

    const request = this.requests.get(requestId);
    if (!request) return;

    if (message.method === "Network.responseReceived") {
      request.type = params.type || request.type;
      request.status = params.response?.status || 0;
      request.failed = request.status >= 400;
      request.fromCache = Boolean(
        params.response?.fromDiskCache ||
          params.response?.fromPrefetchCache ||
          params.response?.fromServiceWorker,
      );
    } else if (message.method === "Network.requestServedFromCache") {
      request.fromCache = true;
    } else if (message.method === "Network.loadingFinished") {
      request.encodedBytes = Math.max(
        0,
        Math.round(params.encodedDataLength || 0),
      );
      request.finished = true;
    } else if (message.method === "Network.loadingFailed") {
      request.failed = true;
      request.finished = true;
    }
  }

  get inflightCount() {
    let count = 0;
    for (const request of this.requests.values()) {
      if (!request.finished) count += 1;
    }
    return count;
  }

  summarize() {
    const byType = {};
    const fontUrls = [];
    const failedUrls = [];
    let transferBytes = 0;
    let requestCount = 0;
    let cachedCount = 0;
    let failedCount = 0;

    for (const request of this.requests.values()) {
      if (!request.finished) continue;
      requestCount += 1;
      transferBytes += request.encodedBytes;
      byType[request.type] =
        (byType[request.type] || 0) + request.encodedBytes;
      if (request.type === "Font") {
        fontUrls.push(request.url);
      }
      if (request.fromCache || request.encodedBytes === 0) cachedCount += 1;
      if (request.failed) {
        failedCount += 1;
        failedUrls.push({
          url: request.url,
          status: request.status,
        });
      }
    }

    return {
      transferBytes,
      requestCount,
      cachedCount,
      failedCount,
      byType,
      fontUrls,
      failedUrls,
    };
  }

  close() {
    this.unsubscribe();
  }
}

async function launchBrowser(browserBin) {
  const profileDirectory = await mkdtemp(
    join(tmpdir(), "magicpodcast-resource-audit-"),
  );
  const browserProcess = spawn(
    browserBin,
    [
      "--headless=new",
      "--remote-debugging-pipe",
      "--no-first-run",
      "--no-default-browser-check",
      "--disable-background-networking",
      "--disable-component-update",
      "--disable-default-apps",
      "--disable-extensions",
      "--disable-sync",
      "--metrics-recording-only",
      "--no-proxy-server",
      "--window-size=1440,1200",
      `--user-data-dir=${profileDirectory}`,
      "about:blank",
    ],
    {
      stdio: ["ignore", "ignore", "pipe", "pipe", "pipe"],
    },
  );
  let stderr = "";
  browserProcess.stderr.setEncoding("utf8");
  browserProcess.stderr.on("data", (chunk) => {
    stderr = `${stderr}${chunk}`.slice(-4_000);
  });

  const client = new PipeCdpClient(browserProcess);
  try {
    const { targetId } = await client.send(
      "Target.createTarget",
      { url: "about:blank" },
      undefined,
      15_000,
    );
    const { sessionId } = await client.send("Target.attachToTarget", {
      targetId,
      flatten: true,
    });
    await Promise.all([
      client.send("Page.enable", {}, sessionId),
      client.send("Runtime.enable", {}, sessionId),
      client.send("Network.enable", {}, sessionId),
      client.send(
        "Page.addScriptToEvaluateOnNewDocument",
        { source: IMAGE_ERROR_TRACKER_SCRIPT },
        sessionId,
      ),
    ]);
    return {
      browserProcess,
      client,
      sessionId,
      profileDirectory,
      getStderr: () => stderr,
    };
  } catch (error) {
    browserProcess.kill("SIGTERM");
    await rm(profileDirectory, { recursive: true, force: true });
    throw new Error(`${error.message}\n${stderr}`.trim());
  }
}

async function removeProfileDirectory(profileDirectory) {
  for (let attempt = 0; attempt < 5; attempt += 1) {
    try {
      await rm(profileDirectory, { recursive: true, force: true });
      return;
    } catch (error) {
      if (error?.code !== "ENOTEMPTY" || attempt === 4) {
        throw error;
      }
      await new Promise((resolve) =>
        setTimeout(resolve, 100 * 2 ** attempt),
      );
    }
  }
}

async function evaluatePageState(client, sessionId) {
  const { result } = await client.send(
    "Runtime.evaluate",
    {
      expression: `(() => {
        const editorialPage = document.querySelector(".editorial-page-shell");
        const brokenImageUrls = Array.from(document.images)
          .filter((image) =>
            image.complete &&
            Boolean(image.currentSrc) &&
            image.naturalWidth === 0
          )
          .map((image) => image.currentSrc);
        const imageErrorUrls =
          window.__magicPodcastImageErrorUrls || [];
        return {
          readyState: document.readyState,
          hasPodcastPage: Boolean(document.querySelector(".podcast-library-page")),
          hasError: Boolean(document.querySelector(".editorial-state.is-error")),
          editorialReady: !editorialPage ||
            document.documentElement.classList.contains("editorial-assets-ready"),
          fontsReady: !document.fonts || document.fonts.status === "loaded",
          brokenImageUrls,
          imageErrorUrls
        };
      })()`,
      returnByValue: true,
    },
    sessionId,
  );
  return result?.value || {};
}

async function waitForSettled(
  client,
  sessionId,
  tracker,
  timeoutMs,
) {
  const deadline = Date.now() + timeoutMs;
  let quietSince = 0;
  let lastState = {};

  while (Date.now() < deadline) {
    lastState = await evaluatePageState(client, sessionId);
    if (lastState.imageErrorUrls?.length > 0) {
      throw new Error(
        `Image decoding failed: ${JSON.stringify(lastState.imageErrorUrls)}`,
      );
    }
    const ready =
      lastState.readyState === "complete" &&
      lastState.hasPodcastPage &&
      !lastState.hasError &&
      lastState.editorialReady &&
      lastState.fontsReady &&
      lastState.brokenImageUrls?.length === 0 &&
      tracker.inflightCount === 0;

    if (ready) {
      quietSince ||= Date.now();
      if (Date.now() - quietSince >= 1_000) return lastState;
    } else {
      quietSince = 0;
    }

    await new Promise((resolve) => setTimeout(resolve, 200));
  }

  throw new Error(
    `Page did not settle within ${timeoutMs}ms: ${JSON.stringify({
      ...lastState,
      inflightCount: tracker.inflightCount,
    })}`,
  );
}

async function navigateAndMeasure(
  client,
  sessionId,
  tracker,
  action,
  timeoutMs,
) {
  tracker.reset();
  const loaded = client.waitForEvent(
    "Page.loadEventFired",
    sessionId,
    timeoutMs,
  );
  await action();
  await loaded;
  await waitForSettled(client, sessionId, tracker, timeoutMs);
  return tracker.summarize();
}

async function measureColdWarmPair(browserBin, targetUrl, timeoutMs) {
  const browser = await launchBrowser(browserBin);
  const tracker = new ResourceTracker(browser.client, browser.sessionId);

  try {
    const cold = await navigateAndMeasure(
      browser.client,
      browser.sessionId,
      tracker,
      () =>
        browser.client.send(
          "Page.navigate",
          { url: targetUrl },
          browser.sessionId,
          timeoutMs,
        ),
      timeoutMs,
    );
    const warm = await navigateAndMeasure(
      browser.client,
      browser.sessionId,
      tracker,
      () =>
        browser.client.send(
          "Page.reload",
          { ignoreCache: false },
          browser.sessionId,
          timeoutMs,
        ),
      timeoutMs,
    );
    return { cold, warm };
  } catch (error) {
    throw new Error(`${error.message}\n${browser.getStderr()}`.trim());
  } finally {
    tracker.close();
    try {
      await browser.client.send("Browser.close", {}, undefined, 2_000);
    } catch {
      browser.browserProcess.kill("SIGTERM");
    }
    if (browser.browserProcess.exitCode === null) {
      await Promise.race([
        new Promise((resolve) =>
          browser.browserProcess.once("exit", resolve),
        ),
        new Promise((resolve) => setTimeout(resolve, 2_000)),
      ]);
    }
    if (browser.browserProcess.exitCode === null) {
      browser.browserProcess.kill("SIGTERM");
    }
    await removeProfileDirectory(browser.profileDirectory);
  }
}

function percentile(values, ratio) {
  if (values.length === 0) return 0;
  const sorted = [...values].sort((a, b) => a - b);
  const index = Math.min(
    sorted.length - 1,
    Math.ceil(sorted.length * ratio) - 1,
  );
  return sorted[index];
}

export function buildBudgetSummary(
  runs,
  coldBudgetBytes,
  warmBudgetBytes,
) {
  const coldBytes = runs.map((run) => run.cold.transferBytes);
  const warmBytes = runs.map((run) => run.warm.transferBytes);
  const unexpectedFontUrls = [
    ...new Set(
      runs.flatMap((run) =>
        [...(run.cold.fontUrls || []), ...(run.warm.fontUrls || [])].filter(
          (url) => PODCASTS_FORBIDDEN_FONT_PATTERN.test(url),
        ),
      ),
    ),
  ];
  const coldFailedCount = runs.reduce(
    (total, run) => total + (run.cold.failedCount || 0),
    0,
  );
  const warmFailedCount = runs.reduce(
    (total, run) => total + (run.warm.failedCount || 0),
    0,
  );
  const coldP95Bytes = percentile(coldBytes, 0.95);
  const warmP95Bytes = percentile(warmBytes, 0.95);

  return {
    coldP95Bytes,
    warmP95Bytes,
    coldBudgetBytes,
    warmBudgetBytes,
    coldFailedCount,
    warmFailedCount,
    unexpectedFontUrls,
    coldWithinBudget: coldP95Bytes < coldBudgetBytes,
    warmWithinBudget: warmP95Bytes < warmBudgetBytes,
    requestsSucceeded: coldFailedCount === 0 && warmFailedCount === 0,
    fontPolicyPassed: unexpectedFontUrls.length === 0,
  };
}

function formatBytes(bytes) {
  if (bytes >= 1024 * 1024) {
    return `${(bytes / 1024 / 1024).toFixed(2)}MB`;
  }
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)}KB`;
  return `${bytes}B`;
}

function printReport(report) {
  console.log("MagicPodcast /podcasts browser resource audit");
  console.log(`Target: ${report.targetUrl}`);
  console.log(`Runs:   ${report.runs.length}`);
  console.log("");

  report.runs.forEach((run, index) => {
    console.log(
      `Run ${index + 1}: cold ${formatBytes(run.cold.transferBytes)} (${run.cold.requestCount} requests), warm ${formatBytes(run.warm.transferBytes)} (${run.warm.requestCount} requests)`,
    );
  });
  console.log("");
  console.log(
    `Cold P95: ${formatBytes(report.summary.coldP95Bytes)} / ${formatBytes(report.summary.coldBudgetBytes)}`,
  );
  console.log(
    `Warm P95: ${formatBytes(report.summary.warmP95Bytes)} / ${formatBytes(report.summary.warmBudgetBytes)}`,
  );
  if (!report.summary.requestsSucceeded) {
    console.log(
      `Failed requests: ${report.summary.coldFailedCount} cold / ${report.summary.warmFailedCount} warm`,
    );
  }
  if (!report.summary.fontPolicyPassed) {
    console.log(
      `Unexpected podcast-list fonts: ${report.summary.unexpectedFontUrls.join(", ")}`,
    );
  }
  console.log(
    report.summary.coldWithinBudget &&
      report.summary.warmWithinBudget &&
      report.summary.requestsSucceeded &&
      report.summary.fontPolicyPassed
      ? "Result: PASS"
      : "Result: FAIL",
  );
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const browserBin = await findBrowser(options.browserBin);
  const targetUrl = `${options.baseUrl}${options.path}`;
  const runs = [];

  for (let run = 0; run < options.runs; run += 1) {
    runs.push(
      await measureColdWarmPair(
        browserBin,
        targetUrl,
        options.timeoutMs,
      ),
    );
  }

  const summary = buildBudgetSummary(
    runs,
    options.coldBudgetBytes,
    options.warmBudgetBytes,
  );
  const report = {
    generatedAt: new Date().toISOString(),
    targetUrl,
    browserBin,
    runs,
    summary,
  };

  if (options.json) {
    console.log(JSON.stringify(report, null, 2));
  } else {
    printReport(report);
  }

  if (
    options.strict &&
    (!summary.coldWithinBudget ||
      !summary.warmWithinBudget ||
      !summary.requestsSucceeded ||
      !summary.fontPolicyPassed)
  ) {
    process.exitCode = 1;
  }
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  main().catch((error) => {
    console.error(error?.stack || error?.message || String(error));
    process.exitCode = 1;
  });
}
