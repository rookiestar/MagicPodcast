import assert from "node:assert/strict";
import { once } from "node:events";
import http from "node:http";
import { spawn } from "node:child_process";
import test from "node:test";

const projectRoot = new URL("../..", import.meta.url);

function listen(server) {
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      server.off("error", reject);
      resolve(server.address().port);
    });
  });
}

function runStatus(primaryUrl, fallbackUrl) {
  const child = spawn("/bin/bash", ["scripts/access-path-status.sh"], {
    cwd: projectRoot,
    env: {
      ...process.env,
      MAGICPODCAST_PRIMARY_HEALTH_URL: primaryUrl,
      MAGICPODCAST_FALLBACK_HEALTH_URL: fallbackUrl,
      MAGICPODCAST_ACCESS_PATH_TIMEOUT: "2",
    },
    stdio: ["ignore", "pipe", "pipe"],
  });

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

  return once(child, "close").then(([code]) => ({ code, stdout, stderr }));
}

async function withFixture(
  t,
  { primaryStatus = 200, fallbackStatus = 302 } = {},
) {
  const primary = http.createServer((_request, response) => {
    response.statusCode = primaryStatus;
    response.setHeader("content-type", "application/json");
    response.end(
      JSON.stringify({
        build_mode: "release",
        release_id: "fixture-release",
        status: primaryStatus === 200 ? "ok" : "error",
      }),
    );
  });
  const fallback = http.createServer((_request, response) => {
    response.statusCode = fallbackStatus;
    if (fallbackStatus === 302) {
      response.setHeader(
        "location",
        "https://fixture.example/cdn-cgi/access/login",
      );
    }
    response.end();
  });
  const [primaryPort, fallbackPort] = await Promise.all([
    listen(primary),
    listen(fallback),
  ]);
  t.after(() => primary.close());
  t.after(() => fallback.close());

  return runStatus(
    `http://127.0.0.1:${primaryPort}/health`,
    `http://127.0.0.1:${fallbackPort}/health`,
  );
}

test("reports a reachable Access gate without overstating fallback usability", async (t) => {
  const result = await withFixture(t);

  assert.equal(result.code, 0, result.stderr);
  assert.match(result.stdout, /primary_status=healthy/);
  assert.match(result.stdout, /primary_release_id=fixture-release/);
  assert.match(result.stdout, /fallback_status=access_gate_reachable/);
  assert.match(result.stdout, /fallback_access_gate=present/);
  assert.match(result.stdout, /fallback_login_page=not_checked/);
  assert.match(result.stdout, /fallback_authenticated_app=not_checked/);
  assert.doesNotMatch(result.stdout, /standby_ready/);
});

test("fails when the primary relay is unhealthy", async (t) => {
  const result = await withFixture(t, { primaryStatus: 503 });

  assert.equal(result.code, 1);
  assert.match(result.stdout, /primary_status=unhealthy/);
  assert.match(result.stdout, /fallback_status=access_gate_reachable/);
});

test("fails closed when the public standby bypasses Access", async (t) => {
  const result = await withFixture(t, { fallbackStatus: 200 });

  assert.equal(result.code, 1);
  assert.match(result.stdout, /primary_status=healthy/);
  assert.match(result.stdout, /fallback_status=unavailable/);
  assert.match(result.stdout, /fallback_access_gate=missing/);
});
