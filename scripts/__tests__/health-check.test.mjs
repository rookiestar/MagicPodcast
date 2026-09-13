import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import { tmpdir } from "node:os";
import http from "node:http";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { spawn } from "node:child_process";
import test from "node:test";

const exec = promisify(execFile);
const projectRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../..",
);
const healthCheck = path.join(projectRoot, "scripts", "health-check.sh");

async function run(command, args, options = {}) {
  try {
    return {
      ...(await exec(command, args, {
        timeout: 20_000,
        maxBuffer: 2 * 1024 * 1024,
        ...options,
      })),
      code: 0,
    };
  } catch (error) {
    return {
      stdout: error.stdout ?? "",
      stderr: error.stderr ?? "",
      code: typeof error.code === "number" ? error.code : 1,
    };
  }
}

function listen(server) {
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      server.off("error", reject);
      resolve(server.address().port);
    });
  });
}

async function startServices(
  t,
  { externalRedirect = false, sameOriginAuthRedirect = false } = {},
) {
  const backend = http.createServer((_request, response) => {
    response.setHeader("content-type", "application/json");
    response.end(JSON.stringify({ status: "ok" }));
  });
  const frontend = http.createServer((request, response) => {
    if (request.url === "/" && externalRedirect) {
      response.statusCode = 302;
      response.setHeader("location", "https://auth.example/cdn-cgi/access/login");
      response.end();
      return;
    }
    if (request.url === "/" && sameOriginAuthRedirect) {
      response.statusCode = 302;
      response.setHeader("location", "/cdn-cgi/access/login");
      response.end();
      return;
    }
    if (request.url === "/") {
      response.statusCode = 307;
      response.setHeader("location", "/discovery");
      response.end();
      return;
    }
    response.statusCode = request.url === "/discovery" ? 200 : 404;
    response.end("ok");
  });
  const [backendPort, frontendPort] = await Promise.all([
    listen(backend),
    listen(frontend),
  ]);
  t.after(() => backend.close());
  t.after(() => frontend.close());
  return {
    backendUrl: `http://127.0.0.1:${backendPort}/health`,
    frontendUrl: `http://127.0.0.1:${frontendPort}`,
  };
}

async function fixtureRoot(t) {
  const root = await mkdtemp(path.join(tmpdir(), "magicpodcast-health-"));
  const backupDir = path.join(root, "backups");
  await mkdir(backupDir);
  await writeFile(path.join(backupDir, "magicpodcast_fixture.db"), "backup");
  t.after(() => rm(root, { recursive: true, force: true }));
  return { root, backupDir };
}

async function runHealth(t, db, services, extraEnv = {}) {
  const fixture = await fixtureRoot(t);
  return run("bash", [healthCheck], {
    env: {
      ...process.env,
      MAGICPODCAST_PROJECT_DIR: fixture.root,
      MAGICPODCAST_DB_FILE: db,
      MAGICPODCAST_BACKUP_DIR: fixture.backupDir,
      MAGICPODCAST_BACKEND_HEALTH_URL: services.backendUrl,
      MAGICPODCAST_FRONTEND_URL: services.frontendUrl,
      MAGICPODCAST_HEALTH_HTTP_TIMEOUT: "2",
      MAGICPODCAST_HEALTH_MAX_REDIRECTS: "3",
      ...extraEnv,
    },
  });
}

async function createWalDatabase(t) {
  const dir = await mkdtemp(path.join(tmpdir(), "magicpodcast-health-db-"));
  const db = path.join(dir, "db ?#% space.db");
  t.after(() => rm(dir, { recursive: true, force: true }));
  const python = spawn(
    "python3",
    [
      "-c",
      [
        "import sqlite3,sys,time",
        "db=sys.argv[1]",
        "c=sqlite3.connect(db)",
        "c.execute('PRAGMA journal_mode=WAL')",
        "c.execute('PRAGMA wal_autocheckpoint=0')",
        "[c.execute('CREATE TABLE '+name+'(id INTEGER)') for name in ('podcasts','episodes','tags','workflows')]",
        "c.execute('INSERT INTO podcasts VALUES(42)')",
        "c.execute('INSERT INTO episodes VALUES(43)')",
        "c.execute('INSERT INTO tags VALUES(44)')",
        "c.execute('INSERT INTO workflows VALUES(45)')",
        "c.commit()",
        "print('ready',flush=True)",
        "time.sleep(60)",
      ].join("\n"),
      db,
    ],
    { stdio: ["ignore", "pipe", "pipe"] },
  );
  t.after(() => {
    if (python.exitCode === null) python.kill("SIGKILL");
  });
  await new Promise((resolve, reject) => {
    python.stdout.once("data", resolve);
    python.once("error", reject);
  });
  python.kill("SIGKILL");
  await new Promise((resolve) => python.once("close", resolve));
  assert.ok(existsSync(`${db}-wal`), "fixture must leave committed WAL pages");
  return db;
}

test("health check reads a stopped WAL database and follows only same-origin redirects", async (t) => {
  const db = await createWalDatabase(t);
  const services = await startServices(t);
  const result = await runHealth(t, db, services);

  assert.match(result.stdout, /前端首页正常: HTTP 200/);
  assert.doesNotMatch(result.stdout, /首页返回 HTTP 307/);
  assert.match(result.stdout, /SQLite integrity_check 通过/);
  assert.match(result.stdout, /播客数:\s+1/);
  assert.match(result.stdout, /单集数:\s+1/);
  assert.match(result.stdout, /标签数:\s+1/);
  assert.match(result.stdout, /工作流数:\s+1/);
  assert.match(result.stdout, /异机加密备份未配置/);
  assert.notEqual(result.code, 0, "offsite backup remains the known excluded failure");
});

test("health check does not follow an external authentication redirect", async (t) => {
  const db = await createWalDatabase(t);
  const services = await startServices(t, { externalRedirect: true });
  const result = await runHealth(t, db, services);

  assert.match(result.stdout, /前端端口存在，但首页返回 HTTP 302/);
  assert.doesNotMatch(result.stdout, /前端首页正常: HTTP 200/);
});

test("health check does not treat a same-origin authentication page as the app", async (t) => {
  const db = await createWalDatabase(t);
  const services = await startServices(t, { sameOriginAuthRedirect: true });
  const result = await runHealth(t, db, services);

  assert.match(result.stdout, /前端端口存在，但首页返回 HTTP 302/);
  assert.doesNotMatch(result.stdout, /前端首页正常: HTTP 200/);
});

test("health check fails for a missing or corrupt database", async (t) => {
  const services = await startServices(t);
  const missing = path.join((await fixtureRoot(t)).root, "missing.db");
  const missingResult = await runHealth(t, missing, services);
  assert.notEqual(missingResult.code, 0);
  assert.match(missingResult.stdout, /数据库文件不存在/);

  const corruptDir = await mkdtemp(path.join(tmpdir(), "magicpodcast-health-corrupt-"));
  const corrupt = path.join(corruptDir, "corrupt.db");
  t.after(() => rm(corruptDir, { recursive: true, force: true }));
  await writeFile(corrupt, "not a sqlite database");
  const corruptResult = await runHealth(t, corrupt, services);
  assert.notEqual(corruptResult.code, 0);
  assert.match(corruptResult.stdout, /SQLite integrity_check 失败/);
});
