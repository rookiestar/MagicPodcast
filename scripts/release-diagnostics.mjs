#!/usr/bin/env node
// Emit only enumerated error classes and typed release facts, never raw logs.
import { spawnSync } from 'node:child_process';
import { mkdirSync, openSync, closeSync, readSync, fstatSync, statSync, writeFileSync, readdirSync, appendFileSync } from 'node:fs';
import path from 'node:path';

const phases = ['preflight', 'backend_build', 'frontend_build', 'stage_verification', 'switch', 'start', 'health', 'readiness', 'rollback', 'complete'];
const outcomes = ['failed', 'success', 'not_attempted'];
const [phase, outcome] = process.argv.slice(2);
const env = process.env;
const root = env.MAGICPODCAST_PROJECT_DIR;
const output = env.MAGICPODCAST_DIAGNOSTICS_DIR;
function tail(file) {
  let fd;
  try {
    if (!statSync(file).isFile()) return { available: false, error_classes: [] };
    fd = openSync(file, 'r');
    const size = fstatSync(fd).size;
    const buffer = Buffer.alloc(Math.min(size, 65536));
    readSync(fd, buffer, 0, buffer.length, Math.max(0, size - buffer.length));
    const text = buffer.toString('utf8');
    const classes = Object.entries({
      address_in_use: /EADDRINUSE|address already in use/i,
      permission_denied: /EACCES|permission denied/i,
      schema_mismatch: /schema.{0,60}(mismatch|expected|incompatible)/i,
      database_open_failed: /unable to open database|SQLITE_CANTOPEN/i,
      database_locked: /database is locked|SQLITE_BUSY/i,
      configuration_invalid: /invalid.{0,30}config|config.{0,30}(invalid|missing)/i,
      out_of_memory: /out of memory|heap limit/i,
      timeout: /timed out|timeout/i,
      start_failed: /start failed|start script failed/i,
    }).filter(([, pattern]) => pattern.test(text)).map(([name]) => name);
    return { available: true, truncated: size > buffer.length, error_classes: classes, classification: classes.length ? 'recognized' : 'unclassified' };
  } catch { return { available: false, error_classes: [] }; }
  finally { if (fd !== undefined) closeSync(fd); }
}
function pointer(file) {
  let fd;
  try {
    if (!statSync(file).isFile()) return {};
    fd = openSync(file, 'r');
    const buffer = Buffer.alloc(Math.min(fstatSync(fd).size, 8192));
    readSync(fd, buffer, 0, buffer.length, 0);
    return Object.fromEntries(buffer.toString('utf8').split('\n').map(line => { const i = line.indexOf('='); return [line.slice(0, i), line.slice(i + 1)]; }));
  } catch { return {}; }
  finally { if (fd !== undefined) closeSync(fd); }
}
function probe(command, args, timeout) {
  const r = spawnSync(command, args, { timeout, killSignal: 'SIGKILL', maxBuffer: 65536, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
  return { ok: r.status === 0, timed_out: r.error?.code === 'ETIMEDOUT', text: r.stdout ?? '' };
}
try {
  if (!phases.includes(phase) || !outcomes.includes(outcome) || !root || !output) throw new Error();
  mkdirSync(output, { recursive: true, mode: 0o700 });
  if (readdirSync(output).filter(name => name.endsWith('.json')).length >= 16) throw new Error();
  const releaseRoot = env.MAGICPODCAST_RELEASE_ROOT || path.join(root, '.magicpodcast-releases');
  const current = pointer(path.join(releaseRoot, 'current.env'));
  const stage = env.MAGICPODCAST_DIAGNOSTIC_STAGE;
  const manifest = stage ? pointer(path.join(stage, 'manifest.env')) : {};
  const sha = value => /^[a-f0-9]{40}$/.test(value ?? '') ? value : null;
  const release = value => /^\d{8}T\d{6}Z-[a-f0-9]{7,40}-\d+$/.test(value ?? '') ? value : null;
  const logNames = { release: env.MAGICPODCAST_RELEASE_LOG || path.join(root, 'logs/release.log'), backend: path.join(root, 'logs/backend.log'), frontend: path.join(root, 'logs/frontend.log') };
  if (stage) { logNames.backend_build = path.join(stage, 'backend-build.log'); logNames.frontend_build = path.join(stage, 'frontend-build.log'); }
  const endpoint = phase === 'readiness' ? '/ready' : '/health';
  const response = probe(env.MAGICPODCAST_CURL_BIN || 'curl', ['--silent', '--show-error', '--connect-timeout', '1', '--max-time', '4', '--write-out', '\n%{http_code}', `http://127.0.0.1:8080${endpoint}`], 4500);
  // Preserve structured 4xx/5xx error bodies; --fail would discard readiness evidence.
  const statusLine = response.text.match(/\n(\d{3})$/);
  const httpStatus = statusLine ? Number(statusLine[1]) : null;
  const body = statusLine ? response.text.slice(0, statusLine.index) : response.text;
  let health = {}; try { const value = JSON.parse(body); if (value && typeof value === 'object' && !Array.isArray(value)) health = value; } catch { /* unavailable */ }
  const expectedRelease = env.MAGICPODCAST_DIAGNOSTIC_RELEASE || manifest.release_id || current.release_id;
  const expectedBuild = env.MAGICPODCAST_DIAGNOSTIC_BUILD || manifest.frontend_build_id || current.frontend_build_id;
  const schema = env.MAGICPODCAST_RELEASE_SCHEMA_VERSION_OVERRIDE || manifest.schema_version || current.schema_version;
  const expectedSchema = /^\d+$/.test(schema ?? '') ? Number(schema) : null;
  const actualSchema = Number.isSafeInteger(health.schema_version) ? health.schema_version : null;
  const git = probe('git', ['-C', root, 'rev-parse', 'HEAD'], 500);
  const report = {
    phase, outcome, captured_at: new Date().toISOString(), target_commit: sha(env.DEPLOY_SHA || manifest.commit || git.text.trim()),
    expected_release: release(expectedRelease), current_release: release(current.release_id), actual_release: release(health.release_id),
    health: { endpoint, http_status: httpStatus, expected_schema: expectedSchema, actual_schema: actualSchema, schema_matches: expectedSchema === null || actualSchema === null ? null : actualSchema === expectedSchema, reachable: response.ok, timed_out: response.timed_out, status_ok: health.status == null ? null : health.status === 'ok', release_matches: expectedRelease && health.release_id != null ? health.release_id === expectedRelease : null, frontend_matches: expectedBuild && health.frontend_build_id != null ? health.frontend_build_id === expectedBuild : null, release_mode: health.build_mode == null ? null : health.build_mode === 'release', production_profile: health.data_profile == null ? null : health.data_profile === 'production' },
    listeners: Object.fromEntries([3000, 8080].map(port => { const r = probe('lsof', ['-tiTCP:' + port, '-sTCP:LISTEN'], 500); return [port, r.timed_out ? 'unknown' : r.ok ? 'listening' : 'unavailable_or_absent']; })),
    logs: Object.fromEntries(Object.entries(logNames).map(([key, file]) => [key, tail(file)])),
  };
  const json = JSON.stringify(report, null, 2);
  writeFileSync(path.join(output, `${phase}-${outcome}-${Date.now()}-${process.pid}.json`), json + '\n', { mode: 0o600, flag: 'wx' });
  console.log(`diagnostic phase=${phase} outcome=${outcome} captured=true`);
  if (env.GITHUB_STEP_SUMMARY) appendFileSync(env.GITHUB_STEP_SUMMARY, `### Release diagnostic\n\n\`\`\`json\n${json}\n\`\`\`\n`);
} catch {
  console.error('diagnostic_capture=unavailable');
  process.exitCode = 1;
}
