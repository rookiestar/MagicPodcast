import assert from 'node:assert/strict';
import test from 'node:test';
import { execFile, spawn } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdtemp, mkdir, writeFile, readFile, rm, chmod, readdir } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
const exec = promisify(execFile);
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const scripts = path.join(root, 'scripts');
async function run(cmd, args, options = {}) {
  try { return { ...(await exec(cmd, args, { timeout: 20000, ...options })), code: 0 }; }
  catch (e) { return { stdout: e.stdout ?? '', stderr: e.stderr ?? '', code: e.code, signal: e.signal }; }
}
async function temp(t) { const p = await mkdtemp(path.join(tmpdir(), 'release-reliability-')); t.after(() => rm(p, { recursive: true, force: true })); return p; }
async function until(fn) { for (let i = 0; i < 200; i++) { if (await fn()) return; await new Promise(r => setTimeout(r, 25)); } throw new Error('fixture did not reach expected state'); }
const now = '2026-09-13T00:00:00Z';
function heartbeat() {
  return { runs: { workflow_runs: [{ id: 1, status: 'completed', conclusion: 'success', created_at: '2026-09-12T23:40:00Z', updated_at: '2026-09-12T23:41:00Z' }] },
    jobs: { jobs: [{ run_id: 1, runner_id: 7, conclusion: 'success', completed_at: '2026-09-12T23:41:00Z', labels: ['magicpodcast-production'] }] },
    runners: { runners: [{ id: 7, status: 'online', busy: false, labels: [{ name: 'magicpodcast-production' }] }] } };
}
for (const [name, change, classification, code] of [
  ['fresh execution', () => {}, 'recent_execution', 0],
  ['offline despite old success', d => { d.runners.runners[0].status = 'offline'; }, 'offline', 1],
  ['queued heartbeat', d => { d.runs.workflow_runs[0].status = 'queued'; }, 'queued', 0],
  ['executing heartbeat', d => { d.runs.workflow_runs[0].status = 'in_progress'; }, 'executing', 0],
  ['completed heartbeat failure', d => { d.runs.workflow_runs[0].conclusion = 'failure'; }, 'heartbeat_failed', 1],
  ['Actions API unavailable', d => { d.runs = null; }, 'unknown', 1],
  ['runner permission denied with recent job', d => { d.runners = null; }, 'recent_execution', 0],
  ['different runner success', d => { d.jobs.jobs[0].runner_id = 8; }, 'unknown', 1],
  ['wrong run identity', d => { d.jobs.jobs[0].run_id = 2; d.runners = null; }, 'unknown', 1],
  ['future timestamps', d => { d.runs.workflow_runs[0].created_at = '2099-01-01'; }, 'unknown', 1],
  ['missing jobs with online runner', d => { d.jobs = null; }, 'unknown', 1],
  ['wrong job with online runner', d => { d.jobs.jobs[0].run_id = 99; }, 'unknown', 1],
  ['unknown workflow status', d => { d.runs.workflow_runs[0].status = 'unexpected'; }, 'unknown', 1],
  ['contradictory times', d => { d.runs.workflow_runs[0].updated_at = '2026-09-12T20:00:00Z'; }, 'unknown', 1],
  ['no heartbeat', d => { d.runs.workflow_runs = []; }, 'unknown', 1],
  ['stale but directly online', d => { d.jobs.jobs[0].completed_at = '2026-09-12T13:27:19Z'; d.runs.workflow_runs[0].created_at = '2026-09-12T13:26:56Z'; }, 'scheduling_delay', 0],
  ['stale and no runner permission', d => { d.jobs.jobs[0].completed_at = '2026-09-12T13:27:19Z'; d.runners = null; }, 'unknown', 1],
  ['stale queued job', d => { d.runs.workflow_runs[0].created_at = '2026-09-12T13:26:56Z'; d.runs.workflow_runs[0].status = 'queued'; }, 'queued', 1],
]) {
  test(`runner evidence: ${name}`, async t => {
    const dir = await temp(t), data = heartbeat(); change(data);
    await writeFile(path.join(dir, 'input.json'), JSON.stringify(data));
    const r = await run(process.execPath, [path.join(scripts, 'production-runner-health.mjs'), '--input', path.join(dir, 'input.json'), '--now', now]);
    assert.equal(r.code, code, r.stderr); const out = JSON.parse(r.stdout);
    assert.equal(out.classification, classification); assert.equal(out.service_health, 'not_checked');
  });
}

test('SQLite read-only preserves stopped WAL, special paths, errors and write rejection', async t => {
  const dir = await temp(t), db = path.join(dir, 'db ?#% space.db');
  const python = spawn('python3', ['-c', `import sqlite3,sys,time\nc=sqlite3.connect(sys.argv[1]);c.execute('PRAGMA journal_mode=WAL');c.execute('PRAGMA wal_autocheckpoint=0');c.execute('CREATE TABLE facts(value)');c.execute('INSERT INTO facts VALUES(42)');c.commit();print('ready',flush=True);time.sleep(60)`, db], { stdio: ['ignore', 'pipe', 'pipe'] });
  const ended = new Promise(resolve => python.on('close', resolve));
  t.after(() => { if (python.exitCode === null) python.kill('SIGKILL'); });
  await new Promise((resolve, reject) => { python.stdout.once('data', resolve); python.once('error', reject); });
  python.kill('SIGKILL'); await ended;
  const original = await readFile(db), wal = await readFile(db + '-wal'); assert.ok(wal.length > 0);
  const query = sql => run('bash', [path.join(scripts, 'sqlite-readonly.sh'), db, sql]);
  let r = await query('SELECT value FROM facts;'); assert.equal(r.code, 0, r.stderr); assert.equal(r.stdout.trim(), '42');
  r = await query('INSERT INTO facts VALUES(99);'); assert.notEqual(r.code, 0); assert.match(r.stderr, /readonly/i);
  assert.deepEqual(await readFile(db), original); assert.deepEqual(await readFile(db + '-wal'), wal);
  const missing = path.join(dir, 'missing.db');
  r = await run('bash', [path.join(scripts, 'sqlite-readonly.sh'), missing, 'SELECT 1;']); assert.notEqual(r.code, 0); assert.equal(existsSync(missing), false);
  const broken = path.join(dir, 'broken.db'); await writeFile(broken, 'not a database');
  r = await run('bash', [path.join(scripts, 'sqlite-readonly.sh'), broken, 'PRAGMA integrity_check;']); assert.notEqual(r.code, 0);
  if (process.getuid?.() !== 0) {
    await chmod(db, 0); r = await query('SELECT value FROM facts;'); await chmod(db, 0o600); assert.notEqual(r.code, 0);
  } else t.diagnostic('Permission case unavailable under root; run this test as an unprivileged user.');
});
async function backupFixture(t) {
  const dir = await temp(t), db = path.join(dir, 'input.db'), backups = path.join(dir, 'backups');
  const tables = ['podcasts', 'episodes', 'tags', 'podcasts_tags', 'workflows', 'jobs', 'job_executions', 'reports', 'sync_configs'];
  const schema = tables.map(n => `CREATE TABLE ${n}(id INTEGER);`).join('') + 'CREATE TABLE schema_migrations(version INTEGER);INSERT INTO schema_migrations VALUES(32);';
  const r = await run('sqlite3', [db, schema]); assert.equal(r.code, 0, r.stderr);
  return { dir, db, backups, env: { ...process.env, DB_PATH: db, BACKUP_DIR: backups, COMPRESS: 'false', KEEP: '2' } };
}
test('backup exposes verified completion and never starts a backup from status query', async t => {
  const f = await backupFixture(t);
  let r = await run('bash', [path.join(scripts, 'backup-db.sh')], { env: f.env }); assert.equal(r.code, 0, r.stderr);
  const artifact = path.join(f.backups, (await readdir(f.backups)).find(n => n.endsWith('.db')));
  const before = await readdir(f.backups);
  r = await run('bash', [path.join(scripts, 'backup-db.sh'), '--status', artifact], { env: f.env }); assert.equal(r.code, 0, r.stderr); assert.match(r.stdout, /state=completed/);
  assert.deepEqual(await readdir(f.backups), before);
  await writeFile(artifact, 'corruption');
  r = await run('bash', [path.join(scripts, 'backup-db.sh'), '--status', artifact], { env: f.env }); assert.notEqual(r.code, 0); assert.match(r.stdout, /state=unknown/);
});
test('slow backup remains queryable and compressor failure is not completion', async t => {
  const f = await backupFixture(t), bin = path.join(f.dir, 'bin'), marker = path.join(f.dir, 'continue'); await mkdir(bin);
  await writeFile(path.join(bin, 'gzip'), `#!/bin/bash\nwhile [ ! -f '${marker}' ]; do sleep 0.1; done\nexit 9\n`, { mode: 0o755 });
  const env = { ...f.env, COMPRESS: 'true', PATH: bin + ':' + process.env.PATH };
  const child = spawn('bash', [path.join(scripts, 'backup-db.sh')], { env, stdio: ['ignore', 'ignore', 'pipe'] });
  const ended = new Promise(resolve => child.on('close', resolve)); t.after(() => { if (child.exitCode === null) child.kill(); });
  let status;
  await until(async () => { const files = await readdir(f.backups).catch(() => []); status = files.find(n => n.endsWith('.status')); return status && (await readFile(path.join(f.backups, status), 'utf8')).includes('phase=compressing'); });
  const artifact = path.join(f.backups, status.replace(/\.status$/, ''));
  let r = await run('bash', [path.join(scripts, 'backup-db.sh'), '--status', artifact], { env }); assert.equal(r.code, 0); assert.match(r.stdout, /state=running/);
  assert.equal((await readdir(f.backups)).filter(n => n.endsWith('.status')).length, 1);
  await writeFile(marker, ''); assert.equal(await ended, 9);
  r = await run('bash', [path.join(scripts, 'backup-db.sh'), '--status', artifact], { env }); assert.notEqual(r.code, 0); assert.match(r.stdout, /state=failed/); assert.equal(existsSync(artifact + '.meta'), false);
});

test('diagnostic artifact contains only allowed facts even with secrets and oversized logs', async t => {
  const dir = await temp(t), output = path.join(dir, 'diagnostics'); await mkdir(path.join(dir, 'logs'));
  const secret = 'SECRET_AUTH_TOKEN_private_body_signed_url';
  await writeFile(path.join(dir, 'logs/backend.log'), 'x'.repeat(100000) + '\nEADDRINUSE ' + secret);
  const curl = path.join(dir, 'curl');
  await writeFile(curl, `#!/bin/bash\nprintf '%s' '{"status":"ok","release_id":"${secret}","body":"${secret}"}'\n`, { mode: 0o755 });
  const r = await run(process.execPath, [path.join(scripts, 'release-diagnostics.mjs'), 'start', 'failed'], { env: { ...process.env, MAGICPODCAST_PROJECT_DIR: dir, MAGICPODCAST_DIAGNOSTICS_DIR: output, MAGICPODCAST_CURL_BIN: curl } });
  assert.equal(r.code, 0, r.stderr);
  const text = await readFile(path.join(output, (await readdir(output))[0]), 'utf8');
  assert.equal(text.includes(secret), false); assert.ok(text.length < 8192);
  const report = JSON.parse(text); assert.equal(report.actual_release, null); assert.deepEqual(report.logs.backend.error_classes, ['address_in_use']); assert.equal(report.logs.backend.truncated, true); assert.equal(report.logs.frontend.available, false);
});
test('diagnostic hanging probe is bounded and produces a timeout fact', async t => {
  const dir = await temp(t), output = path.join(dir, 'diagnostics'), curl = path.join(dir, 'curl');
  await writeFile(curl, '#!/bin/bash\nexec sleep 20\n', { mode: 0o755 });
  const start = Date.now();
  const r = await run(process.execPath, [path.join(scripts, 'release-diagnostics.mjs'), 'health', 'failed'], { env: { ...process.env, MAGICPODCAST_PROJECT_DIR: dir, MAGICPODCAST_DIAGNOSTICS_DIR: output, MAGICPODCAST_CURL_BIN: curl } });
  assert.equal(r.code, 0, r.stderr); assert.ok(Date.now() - start < 10000);
  const report = JSON.parse(await readFile(path.join(output, (await readdir(output))[0]), 'utf8')); assert.equal(report.health.timed_out, true);
});

test('compressed backups require complete metadata and keep the existing count policy', async t => {
  const f = await backupFixture(t), bin = path.join(f.dir, 'bin'); await mkdir(bin);
  const date = (await run('which', ['date'])).stdout.trim();
  await writeFile(path.join(bin, 'date'), `#!/bin/bash\nif [ "$1" = '+%Y%m%d_%H%M%S' ]; then printf '%s\\n' "$FIXTURE_TIMESTAMP"; else exec '${date}' "$@"; fi\n`, { mode: 0o755 });
  for (const time of ['20260913_010001', '20260913_010002', '20260913_010003']) {
    const r = await run('bash', [path.join(scripts, 'backup-db.sh')], { env: { ...f.env, COMPRESS: 'true', FIXTURE_TIMESTAMP: time, PATH: bin + ':' + process.env.PATH } });
    assert.equal(r.code, 0, r.stderr);
  }
  const files = await readdir(f.backups);
  assert.equal(files.filter(n => n.endsWith('.db.gz')).length, 2);
  assert.equal(files.some(n => n.includes('010001')), false);
  const artifact = path.join(f.backups, files.find(n => n.endsWith('.db.gz')));
  let r = await run('bash', [path.join(scripts, 'backup-db.sh'), '--status', artifact]); assert.equal(r.code, 0, r.stderr);
  await rm(artifact + '.meta');
  r = await run('bash', [path.join(scripts, 'backup-db.sh'), '--status', artifact]); assert.notEqual(r.code, 0); assert.match(r.stdout, /state=unknown/);
});

test('lost backup owner is unknown rather than success or automatic retry', async t => {
  const f = await backupFixture(t), bin = path.join(f.dir, 'bin'); await mkdir(bin);
  await writeFile(path.join(bin, 'gzip'), '#!/bin/bash\nexec sleep 60\n', { mode: 0o755 });
  const child = spawn('bash', [path.join(scripts, 'backup-db.sh')], {
    detached: true, stdio: 'ignore', env: { ...f.env, COMPRESS: 'true', PATH: bin + ':' + process.env.PATH },
  });
  const ended = new Promise(resolve => child.on('close', resolve));
  t.after(() => { try { process.kill(-child.pid, 'SIGKILL'); } catch {} });
  let status;
  await until(async () => { const files = await readdir(f.backups).catch(() => []); status = files.find(n => n.endsWith('.status')); return status && (await readFile(path.join(f.backups, status), 'utf8')).includes('phase=compressing'); });
  child.kill('SIGKILL'); await ended;
  const before = await readdir(f.backups);
  const r = await run('bash', [path.join(scripts, 'backup-db.sh'), '--status', path.join(f.backups, status.replace(/\.status$/, ''))]);
  assert.notEqual(r.code, 0); assert.match(r.stdout, /state=unknown/); assert.doesNotMatch(r.stdout, /state=completed/);
  assert.deepEqual(await readdir(f.backups), before);
});

test('diagnostic deadline kills a probe that ignores SIGTERM', async t => {
  const dir = await temp(t), output = path.join(dir, 'diagnostics'), curl = path.join(dir, 'curl');
  await writeFile(curl, `#!/bin/bash\nexec '${process.execPath}' -e 'process.on("SIGTERM",()=>{});setTimeout(()=>process.exit(0),12000)'\n`, { mode: 0o755 });
  const start = Date.now();
  const r = await run(process.execPath, [path.join(scripts, 'release-diagnostics.mjs'), 'health', 'failed'], {
    env: { ...process.env, MAGICPODCAST_PROJECT_DIR: dir, MAGICPODCAST_DIAGNOSTICS_DIR: output, MAGICPODCAST_CURL_BIN: curl },
  });
  assert.equal(r.code, 0, r.stderr); assert.ok(Date.now() - start < 7500, 'ignored SIGTERM delayed recovery');
  const report = JSON.parse(await readFile(path.join(output, (await readdir(output))[0]), 'utf8')); assert.equal(report.health.timed_out, true);
});

test('readiness diagnostic records the failed schema check instead of healthy liveness', async t => {
  const dir = await temp(t), output = path.join(dir, 'diagnostics'), curl = path.join(dir, 'curl');
  await writeFile(curl, `#!/bin/bash\nfor arg in "$@"; do [ "$arg" != --fail ] || exit 22; done\ncase "\${@: -1}" in\n */ready) printf '%s\\n503' '{"status":"error","schema_version":31}' ;;\n *) printf '%s' '{"status":"ok"}' ;;\nesac\n`, { mode: 0o755 });
  const r = await run(process.execPath, [path.join(scripts, 'release-diagnostics.mjs'), 'readiness', 'failed'], {
    env: { ...process.env, MAGICPODCAST_PROJECT_DIR: dir, MAGICPODCAST_DIAGNOSTICS_DIR: output, MAGICPODCAST_CURL_BIN: curl, MAGICPODCAST_RELEASE_SCHEMA_VERSION_OVERRIDE: '32' },
  });
  assert.equal(r.code, 0, r.stderr);
  const report = JSON.parse(await readFile(path.join(output, (await readdir(output))[0]), 'utf8'));
  assert.equal(report.health.status_ok, false); assert.equal(report.health.http_status, 503); assert.equal(report.health.schema_matches, false); assert.equal(report.health.actual_schema, 31);
});

test('managed outer health diagnosis compares against its actual release root', async t => {
  const dir = await temp(t), releaseRoot = path.join(dir, 'custom-releases'), output = path.join(dir, 'diagnostics'), curl = path.join(dir, 'curl');
  await mkdir(releaseRoot);
  await writeFile(path.join(releaseRoot, 'current.env'), 'release_id=20260913T010000Z-abcdef0-123\nfrontend_build_id=expected\n');
  await writeFile(curl, `#!/bin/bash\nprintf '%s' '{"status":"ok","release_id":"20260913T010000Z-abcdef0-123","frontend_build_id":"wrong"}'\n`, { mode: 0o755 });
  const r = await run(process.execPath, [path.join(scripts, 'release-diagnostics.mjs'), 'health', 'failed'], {
    env: { ...process.env, MAGICPODCAST_PROJECT_DIR: dir, MAGICPODCAST_RELEASE_ROOT: releaseRoot, MAGICPODCAST_DIAGNOSTICS_DIR: output, MAGICPODCAST_CURL_BIN: curl },
  });
  assert.equal(r.code, 0, r.stderr);
  const report = JSON.parse(await readFile(path.join(output, (await readdir(output))[0]), 'utf8'));
  assert.equal(report.health.release_matches, true); assert.equal(report.health.frontend_matches, false);
});

test('diagnostic unreachable service leaves version and profile comparisons unknown', async t => {
  const dir = await temp(t), output = path.join(dir, 'diagnostics'), curl = path.join(dir, 'curl');
  await writeFile(curl, '#!/bin/bash\nexit 7\n', { mode: 0o755 });
  const r = await run(process.execPath, [path.join(scripts, 'release-diagnostics.mjs'), 'health', 'failed'], {
    env: { ...process.env, MAGICPODCAST_PROJECT_DIR: dir, MAGICPODCAST_DIAGNOSTICS_DIR: output, MAGICPODCAST_CURL_BIN: curl, MAGICPODCAST_DIAGNOSTIC_RELEASE: '20260913T010000Z-abcdef0-123' },
  });
  assert.equal(r.code, 0, r.stderr);
  const report = JSON.parse(await readFile(path.join(output, (await readdir(output))[0]), 'utf8'));
  assert.equal(report.health.reachable, false);
  for (const key of ['release_matches', 'frontend_matches', 'production_profile', 'schema_matches', 'status_ok']) assert.equal(report.health[key], null);
});
