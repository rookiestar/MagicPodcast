#!/usr/bin/env node
// Classify evidence from the existing heartbeat, not application health.
import { execFileSync } from 'node:child_process';
import { readFileSync, appendFileSync } from 'node:fs';

const ageLimit = 75 * 60_000;
function api(endpoint) {
  try {
    return JSON.parse(execFileSync('gh', ['api', endpoint], {
      encoding: 'utf8', timeout: 5000, killSignal: 'SIGKILL', maxBuffer: 2 * 1024 * 1024,
      stdio: ['ignore', 'pipe', 'pipe'],
    }));
  } catch { return null; } // Never print API errors that may contain credentials.
}
function collect() {
  const repo = process.env.GITHUB_REPOSITORY;
  if (!/^[\w.-]+\/[\w.-]+$/.test(repo ?? '')) return {};
  const base = `repos/${repo}/actions`;
  const runs = api(`${base}/workflows/production-runner-heartbeat.yml/runs?per_page=50`);
  const ordered = [...(runs?.workflow_runs ?? [])].sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at));
  const success = ordered.find(r => r.conclusion === 'success');
  const jobs = success && Number.isSafeInteger(success.id) ? api(`${base}/runs/${success.id}/jobs`) : null;
  return { runs, jobs, runners: api(`${base}/runners?per_page=100`) };
}
function classify(data, now) {
  const unknown = reason => ({ classification: 'unknown', reason, exit: 1 });
  if (!Array.isArray(data.runs?.workflow_runs)) return unknown('actions_unavailable');
  const runs = [...data.runs.workflow_runs];
  if (runs.some(r => !Number.isSafeInteger(r.id) || !Number.isFinite(Date.parse(r.created_at)) || Date.parse(r.created_at) > now ||
      (r.updated_at && (!Number.isFinite(Date.parse(r.updated_at)) || Date.parse(r.updated_at) > now || Date.parse(r.updated_at) < Date.parse(r.created_at))))) return unknown('invalid_timestamps');
  runs.sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at));
  const latest = runs[0];
  const statuses = ['queued', 'waiting', 'pending', 'requested', 'in_progress', 'completed'];
  if (runs.some(r => !statuses.includes(r.status))) return unknown('invalid_workflow_status');
  const candidates = data.runners?.runners?.filter(r => r.labels?.some(l => l.name === 'magicpodcast-production'));
  if (candidates && candidates.length !== 1) return unknown('runner_identity_ambiguous');
  const runner = candidates?.[0];
  if (runner?.status === 'offline') return { classification: 'offline', reason: 'runner_api_offline', exit: 1 };
  const success = runs.find(r => r.status === 'completed' && r.conclusion === 'success');
  const job = data.jobs?.jobs?.find(j => j.conclusion === 'success' && j.labels?.includes('magicpodcast-production'));
  if (job && (!Number.isFinite(Date.parse(job.completed_at)) || Date.parse(job.completed_at) > now)) return unknown('invalid_job_timestamp');
  const sameRunner = job && Number.isSafeInteger(job.runner_id) && job.runner_id > 0 && job.run_id === success?.id &&
    (!runner || runner.id === job.runner_id);
  if (job && runner && runner.id !== job.runner_id) return unknown('runner_identity_changed');
  if (!latest) return unknown('heartbeat_missing');
  if (latest.status === 'completed' && latest.conclusion !== 'success') return { classification: 'heartbeat_failed', reason: 'latest_heartbeat_failed', exit: 1 };
  const latestAge = now - Date.parse(latest.created_at);
  if (['queued', 'waiting', 'pending', 'requested'].includes(latest.status)) {
    return { classification: 'queued', reason: latestAge <= ageLimit ? 'heartbeat_awaiting_execution' : 'queue_stale', exit: latestAge <= ageLimit ? 0 : 1 };
  }
  if (latest.status === 'in_progress') return { classification: 'executing', reason: 'heartbeat_running', exit: latestAge <= ageLimit ? 0 : 1 };
  if (sameRunner && Number.isFinite(Date.parse(job.completed_at)) && Date.parse(job.completed_at) <= now && now - Date.parse(job.completed_at) <= ageLimit) {
    return { classification: 'recent_execution', reason: 'heartbeat_job_completed', exit: 0 };
  }
  if (latest.status === 'completed' && latest.conclusion === 'success' && !sameRunner) return unknown('heartbeat_job_evidence_missing_or_conflicting');
  if (runner?.status === 'online') return { classification: 'scheduling_delay', reason: runner.busy ? 'runner_online_busy' : 'runner_online_no_recent_heartbeat', exit: 0 };
  return unknown('stale_heartbeat_runner_status_unavailable');
}

try {
  const args = process.argv.slice(2);
  let data, now = Date.now();
  if (args.length) {
    if (args[0] !== '--input' || !args[1] || (args.length !== 2 && args.length !== 4) || (args.length === 4 && args[2] !== '--now')) throw new Error();
    data = JSON.parse(readFileSync(args[1], 'utf8'));
    if (args[3]) now = Date.parse(args[3]);
  } else { data = collect(); now = Date.now(); }
  if (!Number.isFinite(now)) throw new Error();
  const result = classify(data, now);
  const latest = data.runs?.workflow_runs?.reduce((a, b) => Date.parse(a.created_at) > Date.parse(b.created_at) ? a : b, {}) ?? {};
  const safeDate = value => Number.isFinite(Date.parse(value)) ? new Date(value).toISOString() : null;
  const evidence = {
    ...result, observed_at: new Date(now).toISOString(), latest_created_at: safeDate(latest.created_at),
    latest_updated_at: safeDate(latest.updated_at), heartbeat_limit_minutes: 75,
    runner_status_available: Array.isArray(data.runners?.runners), service_health: 'not_checked',
  };
  console.log(JSON.stringify(evidence));
  if (process.env.GITHUB_STEP_SUMMARY) appendFileSync(process.env.GITHUB_STEP_SUMMARY, `### Production runner evidence\n\n\`\`\`json\n${JSON.stringify(evidence, null, 2)}\n\`\`\`\n`);
  process.exitCode = result.exit;
} catch {
  console.log(JSON.stringify({ classification: 'unknown', reason: 'invalid_input_or_summary_unavailable', service_health: 'not_checked' }));
  process.exitCode = 1;
}
