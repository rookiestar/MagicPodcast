import assert from "node:assert/strict";
import { createHash } from "node:crypto";
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
const migrateScript = path.join(projectRoot, "scripts", "migrate-db.sh");
const restoreScript = path.join(projectRoot, "scripts", "restore-db.sh");

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

  const wrongAdoption = await run(
    "/bin/bash",
    [
      "-c",
      "source '" + maintenanceScript + "'; production_maintenance_enter migration",
    ],
    {
      env: {
        ...lockEnv(lockDir),
        MAGICPODCAST_MAINTENANCE_OWNER_PID: ownerPid,
        MAGICPODCAST_MAINTENANCE_OWNER_START: ownerStart,
      },
    },
  );
  assert.equal(wrongAdoption.code, 1, "migration adopted a deploy window");

  for (const operation of ["rollback", "migration", "recovery"]) {
    const contention = await run(
      "/bin/bash",
      ["-c", "source '" + maintenanceScript + "'; production_maintenance_begin " + operation],
      { env: lockEnv(lockDir) },
    );
    assert.equal(contention.code, 1, operation + " stole an active maintenance lock");
  }
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

test("supervisor rechecks maintenance after a probe before recovery", async (t) => {
  const root = await mkdtemp(path.join(tmpdir(), "magicpodcast-maintenance-race-"));
  const lockDir = path.join(root, "production.lock");
  const callsFile = path.join(root, "service-calls.log");
  const fakeCurl = path.join(root, "curl");
  const fakeStart = path.join(root, "start");
  const fakeStop = path.join(root, "stop");
  const armedFile = path.join(root, "curl-armed");

  await writeExecutable(
    fakeCurl,
    [
      "#!/bin/bash",
      "if [ ! -e '" + armedFile + "' ]; then",
      "  touch '" + armedFile + "'",
      "  mkdir '" + lockDir + "'",
      "  printf 'maintenance\\n' > '" + lockDir + "/state'",
      "fi",
      "exit 1",
      "",
    ].join("\n"),
  );
  await writeExecutable(
    fakeStart,
    "#!/bin/bash\nprintf 'start\\n' >> '" + callsFile + "'\nexit 0\n",
  );
  await writeExecutable(
    fakeStop,
    "#!/bin/bash\nprintf 'stop\\n' >> '" + callsFile + "'\nexit 0\n",
  );

  t.after(async () => {
    await rm(root, { recursive: true, force: true });
  });

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

  assert.equal(result.code, 0, result.stderr);
  const status = await readFile(path.join(root, "supervisor.status"), "utf8");
  assert.match(status, /^state=maintenance$/m);
  assert.equal(existsSync(callsFile), false, "supervisor recovered after the publisher claimed the lock");
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

test("critical migration lock is never reclaimed without explicit recovery", async (t) => {
  const root = await mkdtemp(path.join(tmpdir(), "magicpodcast-critical-lock-"));
  const lockDir = path.join(root, "production.lock");
  await mkdir(lockDir, { recursive: true, mode: 0o700 });
  const oldEpoch = String(Math.floor(Date.now() / 1000) - 3600);
  for (const [name, value] of Object.entries({
    "owner.pid": "99999999\n",
    "owner.started_at": "dead-owner\n",
    started_at: "2000-01-01T00:00:00Z\n",
    started_epoch: oldEpoch + "\n",
    heartbeat_epoch: oldEpoch + "\n",
    operation: "migration\n",
    state: "critical\n",
  })) {
    await writeFile(path.join(lockDir, name), value);
  }
  t.after(() => rm(root, { recursive: true, force: true }));

  const inspected = await run(
    "/bin/bash",
    ["-c", "source '" + maintenanceScript + "'; production_maintenance_inspect"],
    { env: lockEnv(lockDir) },
  );
  assert.equal(inspected.code, 0, inspected.stderr);
  assert.match(inspected.stdout, /^state=recovery_required$/m);
  assert.equal(existsSync(lockDir), true, "critical lock was reclaimed");

  const deploy = await run(
    "/bin/bash",
    ["-c", "source '" + maintenanceScript + "'; production_maintenance_begin deploy"],
    { env: lockEnv(lockDir) },
  );
  assert.equal(deploy.code, 1, "deploy stole a critical migration lock");

  const invalidBackup = path.join(root, "invalid-backup.db");
  await writeFile(invalidBackup, "invalid\n");
  const failedRecovery = await run(
    "/bin/bash",
    [restoreScript, invalidBackup, "--no-safety-backup"],
    { env: { ...lockEnv(lockDir), DB_PATH: path.join(root, "target.db") } },
  );
  assert.notEqual(failedRecovery.code, 0);
  assert.equal(
    (await readFile(path.join(lockDir, "state"), "utf8")).trim(),
    "recovery_required",
    "failed recovery released an ambiguous database",
  );

  const recovery = await run(
    "/bin/bash",
    [
      "-c",
      "source '" + maintenanceScript + "'; production_maintenance_begin recovery; production_maintenance_finish",
    ],
    { env: lockEnv(lockDir) },
  );
  assert.equal(recovery.code, 0, recovery.stderr);
  assert.equal(existsSync(lockDir), false, "explicit recovery did not release the lock");
});

test("restore cannot overlap an active migration window", async (t) => {
  const root = await mkdtemp(path.join(tmpdir(), "magicpodcast-restore-lock-"));
  const lockDir = path.join(root, "production.lock");
  const backup = path.join(root, "backup.db");
  await writeFile(backup, "not-read-because-lock-fails\n");
  const owner = spawn(
    "/bin/bash",
    [
      "-c",
      "source '" + maintenanceScript + "'; production_maintenance_begin migration; exec /bin/sleep 30",
    ],
    { env: { ...process.env, ...lockEnv(lockDir) }, stdio: "ignore" },
  );
  t.after(async () => {
    if (owner.exitCode === null && owner.signalCode === null) {
      owner.kill("SIGKILL");
      await waitForExit(owner);
    }
    await rm(root, { recursive: true, force: true });
  });
  await waitFor(() => existsSync(path.join(lockDir, "owner.pid")));

  const restored = await run(
    "/bin/bash",
    [restoreScript, backup, "--no-safety-backup"],
    { env: { ...lockEnv(lockDir), DB_PATH: path.join(root, "target.db") } },
  );
  assert.equal(restored.code, 1);
  assert.match(restored.stderr, /another production maintenance window is active/);
});

test("successful restore remains held until a paired release is accepted", async (t) => {
  const root = await mkdtemp(path.join(tmpdir(), "magicpodcast-restore-hold-"));
  const lockDir = path.join(root, "production.lock");
  const backup = path.join(root, "backup.db");
  const target = path.join(root, "target.db");
  const fakeLsof = path.join(root, "lsof");
  await writeExecutable(fakeLsof, "#!/bin/bash\nexit 1\n");
  const schema = [
    "podcasts",
    "episodes",
    "tags",
    "podcasts_tags",
    "workflows",
    "jobs",
    "job_executions",
    "reports",
    "sync_configs",
  ]
    .map((table) => "CREATE TABLE " + table + " (id INTEGER PRIMARY KEY);")
    .join(" ");
  const created = await run("sqlite3", [backup, schema]);
  assert.equal(created.code, 0, created.stderr);
  t.after(() => rm(root, { recursive: true, force: true }));

  const forced = await run("/bin/bash", [restoreScript, backup, "--force"], {
    env: { ...lockEnv(lockDir), DB_PATH: target, MAGICPODCAST_LSOF_BIN: fakeLsof },
  });
  assert.notEqual(forced.code, 0, "restore still accepted --force");

  const restored = await run(
    "/bin/bash",
    [restoreScript, backup, "--no-safety-backup"],
    { env: { ...lockEnv(lockDir), DB_PATH: target, MAGICPODCAST_LSOF_BIN: fakeLsof } },
  );
  assert.equal(restored.code, 0, restored.stderr);
  assert.match(restored.stdout, /Recovery lock retained/);
  assert.equal(
    (await readFile(path.join(lockDir, "state"), "utf8")).trim(),
    "recovery_required",
  );
});

async function createMigrationApplyFixture(
  t,
  { goFails = false, startFails = false, healthFails = false } = {},
) {
  const root = await mkdtemp(path.join(tmpdir(), "magicpodcast-migration-apply-"));
  const lockDir = path.join(root, "production.lock");
  const binDir = path.join(root, "bin");
  const callsFile = path.join(root, "calls.log");
  const database = path.join(root, "source.db");
  const backup = path.join(root, "backup.db.gz");
  const report = path.join(root, "migration-report.json");
  const config = path.join(root, "config.yaml");
  const releaseRoot = path.join(root, "releases");
  const artifactDir = path.join(releaseRoot, "fixture-release");
  const backendArtifact = path.join(artifactDir, "backend.api");
  const frontendBuildIDFile = path.join(artifactDir, "frontend.next", "BUILD_ID");
  const targetCommitResult = await run("git", ["rev-parse", "HEAD"]);
  assert.equal(targetCommitResult.code, 0, targetCommitResult.stderr);
  const targetCommit = targetCommitResult.stdout.trim();
  await mkdir(binDir, { recursive: true });
  await mkdir(path.dirname(frontendBuildIDFile), { recursive: true });
  await writeFile(database, "fixture\n");
  await writeFile(backup, "fixture-backup\n");
  await writeFile(backup + ".sha256", "fixture-sha  backup.db.gz\n");
  await writeFile(backup + ".meta", "source_kind=magicpodcast_sqlite\n");
  await writeFile(config, "database:\n  path: fixture\n");
  const backendContents = "fixture-backend-artifact\n";
  await writeExecutable(backendArtifact, "#!/bin/bash\n" + backendContents);
  const backendSHA = createHash("sha256")
    .update("#!/bin/bash\n" + backendContents)
    .digest("hex");
  await writeFile(frontendBuildIDFile, "fixture-frontend\n");
  await writeFile(
    path.join(releaseRoot, "current.env"),
    [
      "release_id=fixture-release",
      "frontend_build_id=fixture-frontend",
      "backend_sha256=" + backendSHA,
      "artifact_dir=" + artifactDir,
      "schema_version=24",
      "",
    ].join("\n"),
  );
  await writeFile(
    path.join(artifactDir, "manifest.env"),
    [
      "release_id=fixture-release",
      "frontend_build_id=fixture-frontend",
      "backend_sha256=" + backendSHA,
      "commit=" + targetCommit,
      "worktree_clean=true",
      "schema_version=24",
      "",
    ].join("\n"),
  );
  await writeFile(
    report,
    JSON.stringify(
      {
        report_version: "migration_report.v1",
        plan_id: "fixture-plan",
        backup_sha256: "fixture-backup-sha",
        result: { status: "passed", apply_eligible: true },
      },
      null,
      2,
    ) + "\n",
  );
  await writeExecutable(
    path.join(binDir, "stop"),
    [
      "#!/bin/bash",
      "set -euo pipefail",
      "[ \"$(cat '" + lockDir + "/operation')\" = migration ]",
      "printf 'stop\\n' >> '" + callsFile + "'",
      "",
    ].join("\n"),
  );
  await writeExecutable(
    path.join(binDir, "release"),
    [
      "#!/bin/bash",
      "set -euo pipefail",
      "[ \"$(cat '" + lockDir + "/operation')\" = migration ]",
      "[ \"$(cat '" + lockDir + "/state')\" = critical ]",
      "[ \"$MAGICPODCAST_RELEASE_MAINTENANCE_OPERATION\" = migration ]",
      "[ \"$MAGICPODCAST_RELEASE_SCHEMA_VERSION_OVERRIDE\" = 25 ]",
      "[ \"$1\" = --activate-prepared ]",
      "[ \"$2\" = '" + artifactDir + "' ]",
      "printf 'activate\\n' >> '" + callsFile + "'",
      startFails ? "exit 1" : "exit 0",
      "",
    ].join("\n"),
  );
  await writeExecutable(path.join(binDir, "lsof"), "#!/bin/bash\nexit 1\n");
  await writeExecutable(
    path.join(binDir, "go"),
    [
      "#!/bin/bash",
      "[ \"$(cat '" + lockDir + "/state')\" = critical ]",
      "printf 'go:%s\\n' \"$*\" >> '" + callsFile + "'",
      goFails ? "exit 1" : "exit 0",
      "",
    ].join("\n"),
  );
  await writeExecutable(
    path.join(binDir, "sqlite3"),
    [
      "#!/bin/bash",
      "query=\"${@: -1}\"",
      "case \"$query\" in",
      "  *'MAX(version)'*) printf '25\\n' ;;",
      "  *'episode_triage_decisions'*) printf 'focus=3\\ninbox=4\\nsomeday=3\\ndone=3\\n' ;;",
      "  *'episode_processing_runs'*) printf 'completed=1\\n' ;;",
      "  *) exit 1 ;;",
      "esac",
      "",
    ].join("\n"),
  );
  await writeExecutable(
    path.join(binDir, "curl"),
    healthFails
      ? "#!/bin/bash\nexit 1\n"
      : "#!/bin/bash\n[ \"${@: -1}\" = http://127.0.0.1:8080/ready ]\nprintf '%s\\n' '{\"status\":\"ok\",\"schema_version\":25,\"release_id\":\"fixture-release\",\"frontend_build_id\":\"fixture-frontend\",\"build_mode\":\"release\",\"data_profile\":\"production\"}'\n",
  );
  t.after(() => rm(root, { recursive: true, force: true }));
  return {
    root,
    lockDir,
    callsFile,
    env: {
      CONFIG_PATH: config,
      DB_PATH: database,
      MAGICPODCAST_MIGRATION_BACKUP: backup,
      MAGICPODCAST_MIGRATION_REPORT: report,
      MAGICPODCAST_RELEASE_ROOT: releaseRoot,
      MAGICPODCAST_MIGRATION_RELEASE_STAGE: artifactDir,
      MAGICPODCAST_MIGRATION_RELEASE_SCRIPT: path.join(binDir, "release"),
      MAGICPODCAST_TARGET_COMMIT: targetCommit,
      MAGICPODCAST_MIGRATION_CONFIRM: "I_UNDERSTAND_THIS_WRITES_DATA",
      MAGICPODCAST_MIGRATION_RELEASE_CONFIRM: "I_UNDERSTAND_THIS_SWITCHES_RELEASE",
      MAGICPODCAST_MIGRATION_STOP_SCRIPT: path.join(binDir, "stop"),
      MAGICPODCAST_GO_BIN: path.join(binDir, "go"),
      MAGICPODCAST_SQLITE_BIN: path.join(binDir, "sqlite3"),
      MAGICPODCAST_LSOF_BIN: path.join(binDir, "lsof"),
      MAGICPODCAST_CURL_BIN: path.join(binDir, "curl"),
      MAGICPODCAST_MIGRATION_HEALTH_ATTEMPTS: "1",
      MAGICPODCAST_MIGRATION_HEALTH_INTERVAL: "1",
      ...lockEnv(lockDir),
    },
  };
}

test("migration apply owns the shared window through post-start verification", async (t) => {
  const fixture = await createMigrationApplyFixture(t);
  const result = await run("/bin/bash", [migrateScript, "--apply"], { env: fixture.env });
  assert.equal(result.code, 0, result.stderr);
  assert.equal(existsSync(fixture.lockDir), false, "successful migration left the lock behind");
  assert.match(result.stdout, /^migration_post_start_schema=25$/m);
  assert.match(result.stdout, /^migration_post_start_release=fixture-release$/m);
  assert.match(result.stdout, /^migration_post_start_frontend_build=fixture-frontend$/m);
  assert.match(result.stdout, /^migration_post_start_queue_counts=.*focus=3.*inbox=4/m);
  assert.match(result.stdout, /^migration_post_start_processing_counts=completed=1/m);
  assert.match(result.stdout, /^migration_service_state=running$/m);
  const calls = await readFile(fixture.callsFile, "utf8");
  assert.match(calls, /^stop$/m);
  assert.match(calls, /^go:run \.\/cmd\/migrate --apply$/m);
  assert.match(calls, /^activate$/m);
  assert.ok(calls.indexOf("stop") < calls.indexOf("go:"));
  assert.ok(calls.indexOf("go:") < calls.indexOf("activate"));
});

test("migration apply rejects a release that is not bound to target commit", async (t) => {
  const fixture = await createMigrationApplyFixture(t);
  const manifest = path.join(
    fixture.env.MAGICPODCAST_RELEASE_ROOT,
    "fixture-release",
    "manifest.env",
  );
  const content = await readFile(manifest, "utf8");
  await writeFile(
    manifest,
    content.replace(/^commit=.*$/m, "commit=0000000000000000000000000000000000000000"),
  );
  const result = await run("/bin/bash", [migrateScript, "--apply"], { env: fixture.env });
  assert.equal(result.code, 1);
  assert.match(result.stderr, /目标 release 与目标 commit 未配对/);
  assert.equal(existsSync(fixture.callsFile), false, "migration stopped services before release validation");
});

test("migration apply requires independent release-switch confirmation", async (t) => {
  const fixture = await createMigrationApplyFixture(t);
  const env = { ...fixture.env };
  delete env.MAGICPODCAST_MIGRATION_RELEASE_CONFIRM;
  const result = await run("/bin/bash", [migrateScript, "--apply"], { env });
  assert.equal(result.code, 1);
  assert.match(result.stderr, /单独确认目标 release 切换/);
  assert.equal(existsSync(fixture.callsFile), false);
});

test("failed migration keeps services stopped and supervisor suppressed until recovery", async (t) => {
  const fixture = await createMigrationApplyFixture(t, { goFails: true });
  const result = await run("/bin/bash", [migrateScript, "--apply"], { env: fixture.env });
  assert.equal(result.code, 1);
  assert.match(result.stderr, /^migration_recovery_required=true$/m);
  assert.match(result.stderr, /^migration_service_state=stopped$/m);
  assert.equal(existsSync(fixture.lockDir), true);
  assert.equal((await readFile(path.join(fixture.lockDir, "state"), "utf8")).trim(), "recovery_required");
  assert.equal((await readFile(path.join(fixture.lockDir, "operation"), "utf8")).trim(), "migration");
  const calls = await readFile(fixture.callsFile, "utf8");
  assert.match(calls, /^stop$/m);
  assert.equal(calls.includes("activate"), false);

  const fakeCurl = path.join(fixture.root, "supervisor-curl");
  const fakeStart = path.join(fixture.root, "supervisor-start");
  const fakeStop = path.join(fixture.root, "supervisor-stop");
  const supervisorCalls = path.join(fixture.root, "supervisor-calls.log");
  await writeExecutable(fakeCurl, "#!/bin/bash\nexit 1\n");
  await writeExecutable(fakeStart, "#!/bin/bash\nprintf 'start\\n' >> '" + supervisorCalls + "'\n");
  await writeExecutable(fakeStop, "#!/bin/bash\nprintf 'stop\\n' >> '" + supervisorCalls + "'\n");
  const supervised = await run("/bin/bash", [supervisorScript], {
    env: {
      ...lockEnv(fixture.lockDir),
      MAGICPODCAST_PROJECT_DIR: fixture.root,
      MAGICPODCAST_SUPERVISOR_LOG: path.join(fixture.root, "supervisor.log"),
      MAGICPODCAST_SUPERVISOR_STATUS_FILE: path.join(fixture.root, "supervisor.status"),
      MAGICPODCAST_SUPERVISOR_INTERVAL: "1",
      MAGICPODCAST_SUPERVISOR_MAX_BACKOFF: "1",
      MAGICPODCAST_SUPERVISOR_MAX_CYCLES: "1",
      MAGICPODCAST_CURL_BIN: fakeCurl,
      MAGICPODCAST_START_SCRIPT: fakeStart,
      MAGICPODCAST_STOP_SCRIPT: fakeStop,
    },
  });
  assert.equal(supervised.code, 0, supervised.stderr);
  assert.equal(existsSync(supervisorCalls), false, "supervisor restarted services during migration recovery hold");
  const status = await readFile(path.join(fixture.root, "supervisor.status"), "utf8");
  assert.match(status, /^state=maintenance$/m);
  assert.match(status, /^maintenance_operation=migration$/m);

  const recovered = await run(
    "/bin/bash",
    [
      "-c",
      "source '" + maintenanceScript + "'; production_maintenance_enter recovery; [ \"$(cat '" + fixture.lockDir + "/operation')\" = recovery ]; production_maintenance_finish",
    ],
    { env: lockEnv(fixture.lockDir) },
  );
  assert.equal(recovered.code, 0, recovered.stderr);
  assert.equal(existsSync(fixture.lockDir), false);
});

test("post-commit start failure keeps the migration window in recovery state", async (t) => {
  const fixture = await createMigrationApplyFixture(t, { startFails: true });
  const result = await run("/bin/bash", [migrateScript, "--apply"], { env: fixture.env });
  assert.equal(result.code, 1);
  assert.match(result.stderr, /^migration_recovery_required=true$/m);
  assert.match(result.stderr, /^migration_service_state=stopped$/m);
  assert.equal((await readFile(path.join(fixture.lockDir, "state"), "utf8")).trim(), "recovery_required");
  const calls = await readFile(fixture.callsFile, "utf8");
  assert.match(calls, /^go:run \.\/cmd\/migrate --apply$/m);
  assert.match(calls, /^activate$/m);
});

test("post-start acceptance failure keeps the migration window in recovery state", async (t) => {
  const fixture = await createMigrationApplyFixture(t, { healthFails: true });
  const result = await run("/bin/bash", [migrateScript, "--apply"], { env: fixture.env });
  assert.equal(result.code, 1);
  assert.match(result.stderr, /迁移后服务验收失败/);
  assert.match(result.stderr, /^migration_recovery_required=true$/m);
  assert.equal((await readFile(path.join(fixture.lockDir, "state"), "utf8")).trim(), "recovery_required");
  const calls = await readFile(fixture.callsFile, "utf8");
  assert.match(calls, /^activate$/m);
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
      "state=\"$(cat '" + lockDir + "/state')\"",
      "[ \"$state\" = maintenance ] || [ \"$state\" = critical ] || exit 91",
      "printf 'stop\\n' >> '" + callsFile + "'",
      "",
    ].join("\n"),
  );
  await writeExecutable(
    path.join(root, "start"),
    [
      "#!/bin/bash",
      "set -euo pipefail",
      "state=\"$(cat '" + lockDir + "/state')\"",
      "[ \"$state\" = maintenance ] || [ \"$state\" = critical ] || exit 91",
      "printf 'start=%s\\n' \"" +
        "$" +
        "{MAGICPODCAST_RELEASE_ID:-}\" >> '" +
        callsFile +
        "'",
      "printf 'start_state=%s\\n' \"$state\" >> '" + callsFile + "'",
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

test("prepared release activates inside an inherited critical migration window", async (t) => {
  const fixture = await createReleaseFixture(t);
  const prepared = await run("/bin/bash", [releaseScript, "--prepare"], {
    env: fixture.env,
  });
  assert.equal(prepared.code, 0, prepared.stderr);
  const match = prepared.stdout.match(/^prepared_stage=(.+)$/m);
  assert.ok(match, prepared.stdout);
  const stage = match[1].trim();
  const releaseID = path.basename(stage);
  const manifest = path.join(stage, "manifest.env");
  const manifestBody = await readFile(manifest, "utf8");
  await writeFile(
    manifest,
    manifestBody.replace(/^worktree_clean=.*$/m, "worktree_clean=true"),
  );

  const activated = await run(
    "/bin/bash",
    [
      "-c",
      [
        "source '" + maintenanceScript + "'",
        "production_maintenance_begin migration",
        "production_maintenance_mark_critical",
        "MAGICPODCAST_RELEASE_MAINTENANCE_OPERATION=migration " +
          "MAGICPODCAST_RELEASE_SCHEMA_VERSION_OVERRIDE=2 " +
          "'" + releaseScript + "' --activate-prepared '" + stage + "'",
        "production_maintenance_finish",
      ].join("\n"),
    ],
    { env: fixture.env },
  );
  assert.equal(activated.code, 0, activated.stderr);
  assert.equal(existsSync(fixture.lockDir), false);
  const current = await readFile(path.join(fixture.releaseRoot, "current.env"), "utf8");
  assert.match(current, new RegExp("^release_id=" + releaseID + "$", "m"));
  assert.match(current, /^schema_version=2$/m);
  const calls = await readFile(fixture.callsFile, "utf8");
  assert.match(calls, new RegExp("^start=" + releaseID + "$", "m"));
  assert.match(calls, /^start_state=critical$/m);
});

test("prepared release atomically takes over a held recovery window", async (t) => {
  const fixture = await createReleaseFixture(t);
  const prepared = await run("/bin/bash", [releaseScript, "--prepare"], {
    env: fixture.env,
  });
  assert.equal(prepared.code, 0, prepared.stderr);
  const match = prepared.stdout.match(/^prepared_stage=(.+)$/m);
  assert.ok(match, prepared.stdout);
  const stage = match[1].trim();
  const releaseID = path.basename(stage);
  const manifest = path.join(stage, "manifest.env");
  await writeFile(
    manifest,
    (await readFile(manifest, "utf8")).replace(
      /^worktree_clean=.*$/m,
      "worktree_clean=true",
    ),
  );
  const held = await run(
    "/bin/bash",
    [
      "-c",
      [
        "source '" + maintenanceScript + "'",
        "production_maintenance_begin recovery",
        "production_maintenance_mark_critical",
        "production_maintenance_hold_for_recovery",
      ].join("\n"),
    ],
    { env: fixture.env },
  );
  assert.equal(held.code, 0, held.stderr);
  assert.equal(
    (await readFile(path.join(fixture.lockDir, "state"), "utf8")).trim(),
    "recovery_required",
  );

  const activated = await run(
    "/bin/bash",
    [releaseScript, "--activate-prepared", stage],
    {
      env: {
        ...fixture.env,
        MAGICPODCAST_RELEASE_MAINTENANCE_OPERATION: "recovery",
        MAGICPODCAST_RELEASE_SCHEMA_VERSION_OVERRIDE: "2",
      },
    },
  );
  assert.equal(activated.code, 0, activated.stderr);
  assert.equal(existsSync(fixture.lockDir), false);
  const current = await readFile(path.join(fixture.releaseRoot, "current.env"), "utf8");
  assert.match(current, new RegExp("^release_id=" + releaseID + "$", "m"));
  assert.match(current, /^schema_version=2$/m);
  const calls = await readFile(fixture.callsFile, "utf8");
  assert.match(calls, /^start_state=critical$/m);

  const heldAgain = await run(
    "/bin/bash",
    [
      "-c",
      [
        "source '" + maintenanceScript + "'",
        "production_maintenance_begin recovery",
        "production_maintenance_mark_critical",
        "production_maintenance_hold_for_recovery",
      ].join("\n"),
    ],
    { env: fixture.env },
  );
  assert.equal(heldAgain.code, 0, heldAgain.stderr);
  const failedReady = await run(
    "/bin/bash",
    [releaseScript, "--activate-prepared", stage],
    {
      env: {
        ...fixture.env,
        MAGICPODCAST_RELEASE_MAINTENANCE_OPERATION: "recovery",
        MAGICPODCAST_RELEASE_SCHEMA_VERSION_OVERRIDE: "2",
        MAGICPODCAST_RELEASE_TEST_READY_FAIL: "true",
      },
    },
  );
  assert.equal(failedReady.code, 1);
  assert.equal(
    (await readFile(path.join(fixture.lockDir, "state"), "utf8")).trim(),
    "recovery_required",
  );
  const failedCalls = await readFile(fixture.callsFile, "utf8");
  assert.match(failedCalls, /^stop$/m);
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
  assert.doesNotMatch(workflow, /migrate-db|--apply/);
  assert.doesNotMatch(rollbackWorkflow, /migrate-db|--apply/);

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
