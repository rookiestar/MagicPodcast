import assert from "node:assert/strict";
import { once } from "node:events";
import { chmod, mkdir, mkdtemp, readFile, rm, utimes, writeFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { spawn } from "node:child_process";
import test from "node:test";

const projectRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../..",
);
const maintenanceScript = path.join(
  projectRoot,
  "scripts",
  "production-maintenance.sh",
);
const supervisorScript = path.join(
  projectRoot,
  "scripts",
  "service-supervisor.sh",
);
const startScript = path.join(projectRoot, "scripts", "start.sh");
const releaseScript = path.join(projectRoot, "scripts", "release.sh");
const productionDeployScript = path.join(
  projectRoot,
  "scripts",
  "production-deploy.sh",
);

function run(command, args = [], options = {}) {
  return new Promise((resolve) => {
    const child = spawn(command, args, {
      cwd: options.cwd ?? projectRoot,
      env: { ...process.env, ...(options.env ?? {}) },
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
    child.on("error", (error) => {
      resolve({ code: null, signal: null, stdout, stderr, error });
    });
    child.on("close", (code, signal) => {
      resolve({ code, signal, stdout, stderr });
    });
  });
}

async function writeExecutable(file, contents) {
  await writeFile(file, contents, { mode: 0o700 });
  await chmod(file, 0o700);
}

async function waitFor(check, timeoutMs = 3000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (check()) return;
    await new Promise((resolve) => setTimeout(resolve, 25));
  }
  assert.fail("timed out waiting for fixture state");
}

async function waitForExit(child) {
  if (child.exitCode !== null || child.signalCode !== null) return;
  await once(child, "close");
}

function lockEnv(lockDir) {
  return {
    MAGICPODCAST_DEPLOY_LOCK_DIR: lockDir,
    MAGICPODCAST_DEPLOY_LOCK_STALE_AFTER: "60",
    MAGICPODCAST_DEPLOY_LOCK_HEARTBEAT_INTERVAL: "1",
  };
}

test("supervisor records maintenance and skips recovery during an active publish", async (t) => {
  const root = await mkdtemp(path.join(tmpdir(), "magicpodcast-maintenance-"));
  const lockDir = path.join(root, "production.lock");
  const callsFile = path.join(root, "service-calls.log");
  const fakeCurl = path.join(root, "curl");
  const fakeStart = path.join(root, "start");
  const fakeStop = path.join(root, "stop");
  await writeExecutable(fakeCurl, "#!/bin/bash\nexit 1\n");
  await writeExecutable(
    fakeStart,
    [
      "#!/bin/bash",
      "printf 'start\\n' >> '" + callsFile + "'",
      "exit 0",
      "",
    ].join("\n"),
  );
  await writeExecutable(
    fakeStop,
    [
      "#!/bin/bash",
      "printf 'stop\\n' >> '" + callsFile + "'",
      "exit 0",
      "",
    ].join("\n"),
  );

  const owner = spawn(
    "/bin/bash",
    [
      "-c",
      [
        "source '" + maintenanceScript + "'",
        "production_maintenance_begin deploy",
        "trap 'exit 143' TERM INT",
        "trap 'production_maintenance_finish' EXIT",
        "exec /bin/sleep 30",
      ].join("\n"),
    ],
    {
      cwd: projectRoot,
      env: { ...process.env, ...lockEnv(lockDir) },
      stdio: ["ignore", "ignore", "pipe"],
    },
  );
  let ownerStderr = "";
  owner.stderr.setEncoding("utf8");
  owner.stderr.on("data", (chunk) => {
    ownerStderr += chunk;
  });

  let guardedStart = null;
  t.after(async () => {
    if (guardedStart && guardedStart.exitCode === null && guardedStart.signalCode === null) {
      guardedStart.kill("SIGKILL");
      await waitForExit(guardedStart);
    }
    if (owner.exitCode === null && owner.signalCode === null) {
      owner.kill("SIGKILL");
      await waitForExit(owner);
    }
    await rm(root, { recursive: true, force: true });
  });

  await waitFor(() => existsSync(path.join(lockDir, "owner.pid")));
  const result = await run("/bin/bash", [supervisorScript], {
    env: {
      ...lockEnv(lockDir),
      MAGICPODCAST_PROJECT_DIR: root,
      MAGICPODCAST_SUPERVISOR_LOG: path.join(root, "supervisor.log"),
      MAGICPODCAST_SUPERVISOR_STATUS_FILE: path.join(root, "supervisor.status"),
      MAGICPODCAST_SUPERVISOR_INTERVAL: "1",
      MAGICPODCAST_SUPERVISOR_MAX_BACKOFF: "1",
      MAGICPODCAST_SUPERVISOR_MAX_CYCLES: "1",
      MAGICPODCAST_SUPERVISOR_NO_BUILD: "true",
      MAGICPODCAST_CURL_BIN: fakeCurl,
      MAGICPODCAST_START_SCRIPT: fakeStart,
      MAGICPODCAST_STOP_SCRIPT: fakeStop,
    },
  });

  assert.equal(result.code, 0, result.stderr + "\n" + ownerStderr);
  const status = await readFile(path.join(root, "supervisor.status"), "utf8");
  assert.match(status, /^state=maintenance$/m);
  assert.match(status, /^maintenance_owner_pid=[1-9][0-9]*$/m);
  assert.match(status, /^maintenance_started_at=.+$/m);
  assert.match(status, /^maintenance_operation=deploy$/m);
  assert.equal(existsSync(callsFile), false, "supervisor attempted service recovery");

  const ownerPid = (await readFile(path.join(lockDir, "owner.pid"), "utf8")).trim();
  const ownerStart = (
    await readFile(path.join(lockDir, "owner.started_at"), "utf8")
  ).trim();
  const adopted = await run(
    "/bin/bash",
    [
      "-c",
      "source '" +
        maintenanceScript +
        "'; production_maintenance_enter deploy; [ \"$MAGICPODCAST_MAINTENANCE_OWNERSHIP\" = adopted ]",
    ],
    {
      env: {
        ...lockEnv(lockDir),
        MAGICPODCAST_MAINTENANCE_OWNER_PID: ownerPid,
        MAGICPODCAST_MAINTENANCE_OWNER_START: ownerStart,
      },
    },
  );
  assert.equal(adopted.code, 0, adopted.stderr);

  const contention = await run(
    "/bin/bash",
    ["-c", "source '" + maintenanceScript + "'; production_maintenance_begin rollback"],
    { env: lockEnv(lockDir) },
  );
  assert.equal(contention.code, 1, "active maintenance lock was stolen");
  assert.equal(existsSync(path.join(lockDir, "state")), true);

  guardedStart = spawn("/bin/bash", [startScript, "--prod"], {
    cwd: projectRoot,
    env: {
      ...lockEnv(lockDir),
      MAGICPODCAST_DEPLOY_LOCK_STALE_AFTER: "1",
      MAGICPODCAST_SUPERVISOR_STATUS_FILE: path.join(root, "supervisor.status"),
    },
    stdio: ["ignore", "pipe", "pipe"],
  });
  let guardedStdout = "";
  let guardedStderr = "";
  guardedStart.stdout.setEncoding("utf8");
  guardedStart.stderr.setEncoding("utf8");
  guardedStart.stdout.on("data", (chunk) => {
    guardedStdout += chunk;
  });
  guardedStart.stderr.on("data", (chunk) => {
    guardedStderr += chunk;
  });
  await new Promise((resolve) => setTimeout(resolve, 100));
  assert.equal(guardedStart.exitCode, null, "non-publisher start did not wait for maintenance");

  owner.kill("SIGKILL");
  await waitForExit(owner);
  await waitForExit(guardedStart);
  assert.equal(guardedStart.exitCode, 0, guardedStderr);
  assert.match(guardedStdout, /跳过非发布方启动/);
});

test("stale maintenance locks are reclaimed only after the owner is gone", async (t) => {
  const root = await mkdtemp(path.join(tmpdir(), "magicpodcast-stale-lock-"));
  const lockDir = path.join(root, "production.lock");
  await mkdir(lockDir, { recursive: true, mode: 0o700 });
  const deadOwner = spawn("/bin/sleep", ["30"]);
  deadOwner.kill("SIGKILL");
  await waitForExit(deadOwner);
  const oldEpoch = String(Math.floor(Date.now() / 1000) - 3600);
  await writeFile(path.join(lockDir, "owner.pid"), String(deadOwner.pid) + "\n");
  await writeFile(path.join(lockDir, "owner.started_at"), "not-the-same-process\n");
  await writeFile(path.join(lockDir, "started_at"), "2000-01-01T00:00:00Z\n");
  await writeFile(path.join(lockDir, "started_epoch"), oldEpoch + "\n");
  await writeFile(path.join(lockDir, "heartbeat_epoch"), oldEpoch + "\n");
  await writeFile(path.join(lockDir, "operation"), "deploy\n");
  await writeFile(path.join(lockDir, "state"), "maintenance\n");

  t.after(() => rm(root, { recursive: true, force: true }));

  const result = await run(
    "/bin/bash",
    [
      "-c",
      "source '" +
        maintenanceScript +
        "'; if production_maintenance_inspect; then exit 10; fi",
    ],
    { env: lockEnv(lockDir) },
  );

  assert.equal(result.code, 0, result.stderr);
  assert.equal(existsSync(lockDir), false, "stale lock was not reclaimed");

  const incompleteLockDir = path.join(root, "incomplete.lock");
  await mkdir(incompleteLockDir, { recursive: true, mode: 0o700 });
  const oldDate = new Date(Date.now() - 3600 * 1000);
  await utimes(incompleteLockDir, oldDate, oldDate);
  const incompleteResult = await run(
    "/bin/bash",
    [
      "-c",
      "source '" +
        maintenanceScript +
        "'; if production_maintenance_inspect; then exit 10; fi",
    ],
    { env: lockEnv(incompleteLockDir) },
  );
  assert.equal(incompleteResult.code, 0, incompleteResult.stderr);
  assert.equal(existsSync(incompleteLockDir), false, "incomplete stale lock was not reclaimed");
});

async function createReleaseFixture(t, { failFirstStart = false } = {}) {
  const root = await mkdtemp(path.join(tmpdir(), "magicpodcast-release-"));
  const backendDir = path.join(root, "backend");
  const frontendDir = path.join(root, "frontend");
  const releaseRoot = path.join(root, "releases");
  const binDir = path.join(root, "bin");
  const callsFile = path.join(root, "service-calls.log");
  const lockDir = path.join(root, "production.lock");
  await mkdir(path.join(backendDir, "cmd", "api"), { recursive: true });
  await mkdir(path.join(frontendDir, ".next"), { recursive: true });
  await mkdir(releaseRoot, { recursive: true });
  await mkdir(binDir, { recursive: true });
  await writeFile(path.join(frontendDir, "tsconfig.json"), "{}\n");
  await writeFile(path.join(frontendDir, "next-env.d.ts"), "// fixture\n");
  await writeFile(path.join(frontendDir, ".next", "BUILD_ID"), "old-frontend\n");
  await writeExecutable(
    path.join(backendDir, "api"),
    "#!/bin/bash\nexit 0\n",
  );
  await writeFile(path.join(root, "database.db"), "fixture\n");
  await writeFile(
    path.join(releaseRoot, "current.env"),
    [
      "release_id=old-release",
      "frontend_build_id=old-frontend",
      "backend_sha256=old-backend",
      "artifact_dir=old-artifacts",
      "schema_version=1",
      "",
    ].join("\n"),
  );
  await writeExecutable(
    path.join(binDir, "go"),
    [
      "#!/bin/bash",
      "set -euo pipefail",
      "output=\"\"",
      "while [ \"$#\" -gt 0 ]; do",
      "  if [ \"$1\" = \"-o\" ]; then output=\"$2\"; shift 2; else shift; fi",
      "done",
      "[ -n \"$output\" ]",
      "printf '#!/bin/bash\\nexit 0\\n' > \"$output\"",
      "chmod +x \"$output\"",
      "",
    ].join("\n"),
  );
  await writeExecutable(
    path.join(binDir, "npm"),
    [
      "#!/bin/bash",
      "set -euo pipefail",
      "dist=\"" + "$" + "{MAGICPODCAST_NEXT_DIST_DIR:?}\"",
      "mkdir -p \"$PWD/$dist\"",
      "printf 'new-frontend\\n' > \"$PWD/$dist/BUILD_ID\"",
      "",
    ].join("\n"),
  );
  await writeExecutable(path.join(binDir, "node"), "#!/bin/bash\nexit 0\n");
  await writeExecutable(
    path.join(binDir, "sqlite3"),
    "#!/bin/bash\nprintf '1\\n'\n",
  );
  await writeExecutable(
    path.join(root, "stop"),
    [
      "#!/bin/bash",
      "[ -f \"" + lockDir + "/state\" ] && [ \"$(cat \"" + lockDir + "/state\")\" = maintenance ] || exit 91",
      "printf 'stop\\n' >> '" + callsFile + "'",
      "",
    ].join("\n"),
  );
  await writeExecutable(
    path.join(root, "start"),
    [
      "#!/bin/bash",
      "set -euo pipefail",
      "[ -f \"" + lockDir + "/state\" ] && [ \"$(cat \"" + lockDir + "/state\")\" = maintenance ] || exit 91",
      "printf 'start=%s\\n' \"" +
        "$" +
        "{MAGICPODCAST_RELEASE_ID:-}\" >> '" +
        callsFile +
        "'",
      "count=\"$(grep -c '^start=' '" + callsFile + "' || true)\"",
      "if [ '" + String(failFirstStart) + "' = true ] && [ \"$count\" -eq 1 ]; then exit 1; fi",
      "",
    ].join("\n"),
  );

  t.after(() => rm(root, { recursive: true, force: true }));
  return {
    root,
    releaseRoot,
    lockDir,
    callsFile,
    env: {
      MAGICPODCAST_PROJECT_DIR: root,
      MAGICPODCAST_RELEASE_ROOT: releaseRoot,
      MAGICPODCAST_RELEASE_LOG: path.join(root, "release.log"),
      MAGICPODCAST_RELEASE_DATABASE_PATH: path.join(root, "database.db"),
      MAGICPODCAST_RELEASE_TEST_MODE: "true",
      MAGICPODCAST_GO_BIN: path.join(binDir, "go"),
      MAGICPODCAST_NPM_BIN: path.join(binDir, "npm"),
      MAGICPODCAST_NODE_BIN: path.join(binDir, "node"),
      MAGICPODCAST_START_SCRIPT: path.join(root, "start"),
      MAGICPODCAST_STOP_SCRIPT: path.join(root, "stop"),
      PATH: binDir + ":" + process.env.PATH,
      ...lockEnv(lockDir),
    },
  };
}

test("direct release acquires and releases maintenance around success and manual rollback", async (t) => {
  const fixture = await createReleaseFixture(t);
  const deploy = await run("/bin/bash", [releaseScript, "--prod"], {
    env: fixture.env,
  });
  assert.equal(deploy.code, 0, deploy.stderr);
  assert.equal(existsSync(fixture.lockDir), false);

  const rolledBack = await run("/bin/bash", [releaseScript, "--rollback"], {
    env: fixture.env,
  });
  assert.equal(rolledBack.code, 0, rolledBack.stderr);
  assert.equal(existsSync(fixture.lockDir), false);
  const current = await readFile(path.join(fixture.releaseRoot, "current.env"), "utf8");
  assert.match(current, /^release_id=old-release$/m);
  const calls = await readFile(fixture.callsFile, "utf8");
  assert.match(calls, /^stop$/m);
  assert.match(calls, /^start=/m);
});

test("failed direct release rolls back before releasing maintenance", async (t) => {
  const fixture = await createReleaseFixture(t, { failFirstStart: true });
  const result = await run("/bin/bash", [releaseScript, "--prod"], {
    env: fixture.env,
  });

  assert.equal(result.code, 1, result.stdout);
  assert.equal(existsSync(fixture.lockDir), false);
  const current = await readFile(path.join(fixture.releaseRoot, "current.env"), "utf8");
  assert.match(current, /^release_id=old-release$/m);
  const calls = await readFile(fixture.callsFile, "utf8");
  assert.equal((calls.match(/^start=/gm) ?? []).length, 2);
});

async function createManagedDeployFixture(t) {
  const root = await mkdtemp(path.join(tmpdir(), "magicpodcast-managed-deploy-"));
  const seed = path.join(root, "seed");
  const remote = path.join(root, "remote.git");
  const production = path.join(root, "production");
  const releaseRoot = path.join(root, "releases");
  const lockDir = path.join(root, "production.lock");
  const callsFile = path.join(root, "managed-calls.log");
  const curl = path.join(root, "curl");
  await mkdir(path.join(seed, "scripts"), { recursive: true });
  await mkdir(releaseRoot, { recursive: true });
  const lockState = "\"" + "$" + "{MAGICPODCAST_DEPLOY_LOCK_DIR}/state\"";
  await writeExecutable(
    path.join(seed, "scripts", "release.sh"),
    [
      "#!/bin/bash",
      "set -euo pipefail",
      "[ \"$(cat " + lockState + ")\" = maintenance ]",
      "printf 'release:%s\\n' \"$*\" >> '" + callsFile + "'",
      "",
    ].join("\n"),
  );
  await writeExecutable(
    path.join(seed, "scripts", "restart.sh"),
    [
      "#!/bin/bash",
      "set -euo pipefail",
      "[ \"$" + "{1:-}\" = --prod ]",
      "[ \"$(cat " + lockState + ")\" = maintenance ]",
      "mkdir -p '" + releaseRoot + "'",
      "printf 'restart\\n' >> '" + callsFile + "'",
      "cat > '" + path.join(releaseRoot, "current.env") + "' <<'EOF'",
      "release_id=managed-release",
      "frontend_build_id=managed-frontend",
      "backend_sha256=managed-backend",
      "schema_version=1",
      "EOF",
      "",
    ].join("\n"),
  );
  await writeFile(path.join(seed, "marker.txt"), "initial\n");
  await run("git", ["init", "--initial-branch=main"], { cwd: seed });
  await run("git", ["config", "user.email", "fixture@example.test"], { cwd: seed });
  await run("git", ["config", "user.name", "Fixture"], { cwd: seed });
  await run("git", ["add", "."], { cwd: seed });
  await run("git", ["commit", "-m", "fixture initial"], { cwd: seed });
  const initialSha = (
    await run("git", ["rev-parse", "HEAD"], { cwd: seed })
  ).stdout.trim();
  await writeFile(path.join(seed, "marker.txt"), "target\n");
  await run("git", ["add", "marker.txt"], { cwd: seed });
  await run("git", ["commit", "-m", "fixture target"], { cwd: seed });
  const targetSha = (
    await run("git", ["rev-parse", "HEAD"], { cwd: seed })
  ).stdout.trim();
  await run("git", ["init", "--bare", remote]);
  await run("git", ["remote", "add", "origin", remote], { cwd: seed });
  await run("git", ["push", "origin", "main"], { cwd: seed });
  await run("git", ["clone", remote, production], { cwd: root });
  await run("git", ["checkout", "--detach", initialSha], { cwd: production });
  await writeExecutable(
    curl,
    [
      "#!/bin/bash",
      "set -euo pipefail",
      "[ -f \"" +
        "$" +
        "{MAGICPODCAST_DEPLOY_LOCK_DIR}/state\" ] && [ \"$(cat \"" +
        "$" +
        "{MAGICPODCAST_DEPLOY_LOCK_DIR}/state\")\" = maintenance ] || exit 92",
      "url=\"" + "$" + "{@: -1}\"",
      "case \"$url\" in",
      "  */health)",
      "    printf '%s\\n' '{\"status\":\"ok\",\"release_id\":\"managed-release\",\"frontend_build_id\":\"managed-frontend\",\"build_mode\":\"release\",\"data_profile\":\"production\"}'",
      "    ;;",
      "  *) printf 'ok\\n' ;;",
      "esac",
      "",
    ].join("\n"),
  );

  t.after(() => rm(root, { recursive: true, force: true }));
  return {
    root,
    production,
    releaseRoot,
    lockDir,
    callsFile,
    initialSha,
    targetSha,
    curl,
  };
}

test("managed production workflow keeps the shared maintenance lock through release health verification", async (t) => {
  const fixture = await createManagedDeployFixture(t);
  const workflow = await readFile(
    path.join(projectRoot, ".github", "workflows", "production-deploy.yml"),
    "utf8",
  );
  const rollbackWorkflow = await readFile(
    path.join(projectRoot, ".github", "workflows", "production-rollback.yml"),
    "utf8",
  );
  assert.match(workflow, /production-deploy\.sh" deploy "\$DEPLOY_SHA"/);
  assert.match(workflow, /MAGICPODCAST_PRODUCTION_DIR/);
  assert.match(rollbackWorkflow, /production-deploy\.sh" rollback/);

  const result = await run(
    "/bin/bash",
    [productionDeployScript, "deploy", fixture.targetSha],
    {
      env: {
        MAGICPODCAST_PRODUCTION_DIR: fixture.production,
        MAGICPODCAST_RELEASE_ROOT: fixture.releaseRoot,
        MAGICPODCAST_DEPLOY_LOCK_DIR: fixture.lockDir,
        MAGICPODCAST_CURL_BIN: fixture.curl,
      },
    },
  );

  assert.equal(result.code, 0, result.stderr);
  assert.equal(existsSync(fixture.lockDir), false);
  assert.equal(
    (await run("git", ["rev-parse", "HEAD"], { cwd: fixture.production })).stdout.trim(),
    fixture.targetSha,
  );
  const calls = await readFile(fixture.callsFile, "utf8");
  assert.match(calls, /release:--dry-run/);
  assert.match(calls, /restart/);
  const sourceState = await readFile(
    path.join(fixture.releaseRoot, "source-state.env"),
    "utf8",
  );
  assert.match(sourceState, new RegExp("current_source_sha=" + fixture.targetSha));
});
