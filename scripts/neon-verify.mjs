// Opt-in live verification. Adds uniquely named demo records through the real UI;
// never resets/truncates the cloud DB, and never changes the ordinary test target.
import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { browserEnvironment } from './environment.mjs';
import { neonEnvironment } from './neon-environment.mjs';
import { waitForIdleObservation } from './neon-idle.mjs';

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const windows = process.platform === 'win32';
const running = new Set();
const report = { started_at: new Date().toISOString(), status: 'running', checks: {} };
let env;
let stage = 'profile';
const reportPath = join(root, '.local/neon-verification.json');
const apiURL = 'http://127.0.0.1:18081';

function launch(command, args, childEnv) {
  if (command === 'pnpm') {
    const executable = process.env.npm_execpath ?? '';
    if (/\.[cm]?js$/i.test(executable)) { args = [executable, ...args]; command = process.execPath; }
    else command = /\.exe$/i.test(executable) ? executable : windows ? 'pnpm.exe' : 'pnpm';
  }
  const child = spawn(command, args, { cwd: root, env: childEnv, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] });
  running.add(child);
  child.output = '';
  child.done = new Promise((resolve, reject) => {
    child.once('error', () => reject(new Error('Process could not start.')));
    child.once('exit', code => { running.delete(child); resolve(code); });
  });
  // Retain output only in memory. Never persist raw command errors or environment.
  for (const stream of [child.stdout, child.stderr]) stream.on('data', chunk => {
    if (child.output.length < 2_000_000) child.output += chunk.toString();
  });
  return child;
}

function assertNoSecrets(text) {
  const urls = [env.DATABASE_URL, env.MIGRATION_DATABASE_URL];
  const secrets = [...urls, ...urls.map(value => decodeURIComponent(new URL(value).password))];
  assert.ok(secrets.every(value => !value || !text.includes(value)), 'Sensitive output was blocked.');
}

async function run(command, args, childEnv = env) {
  const child = launch(command, args, childEnv);
  const code = await child.done;
  assertNoSecrets(child.output);
  if (code !== 0) throw new Error('Verification subprocess failed.');
  return child.output;
}

async function stop(child) {
  if (!child || !running.has(child)) return;
  if (windows) spawnSync('taskkill', ['/PID', String(child.pid), '/T', '/F'], { stdio: 'ignore', windowsHide: true });
  else child.kill('SIGTERM');
  await Promise.race([child.done, new Promise(resolve => setTimeout(resolve, 5000))]);
}

async function ready(url, child) {
  for (let attempt = 0; attempt < 90; attempt++) {
    if (!running.has(child)) throw new Error('Verification server stopped during startup.');
    try { if ((await fetch(url, { signal: AbortSignal.timeout(2000) })).ok) return; } catch {}
    await new Promise(resolve => setTimeout(resolve, 1000));
  }
  throw new Error('Verification server was not ready.');
}

const save = () => writeFileSync(reportPath, JSON.stringify(report, null, 2) + '\n');
const digest = value => createHash('sha256').update(JSON.stringify(value)).digest('hex');

async function idleSignal(notBefore) {
  console.log(`NEON_IDLE_WAIT: no further database requests until resume; not before ${new Date(notBefore).toISOString()}.`);
  console.log('After actual control-plane idle is observed, write endpoint_id, current_state=idle and observed_at to .local/neon-idle-observation.json or send a JSON line on stdin.');
  return waitForIdleObservation({ file: join(root, '.local/neon-idle-observation.json'),
    endpoint: env.NEON_HOST.split('.')[0], notBefore });
}

for (const signal of ['SIGINT', 'SIGTERM']) process.once(signal, async () => {
  report.status = 'interrupted'; report.failed_stage = stage; save();
  for (const child of [...running]) await stop(child);
  process.exit(130);
});

try {
  mkdirSync(join(root, '.local'), { recursive: true });
  env = { ...neonEnvironment(root), GOTOOLCHAIN: 'go1.26.7', API_ADDR: '127.0.0.1:18081', ERP_API_PROXY_TARGET: apiURL };
  assert.equal(spawnSync('git', ['check-ignore', '--quiet', '.env.neon'], { cwd: root, windowsHide: true }).status, 0);
  assert.notEqual(spawnSync('git', ['ls-files', '--error-unmatch', '.env.neon'], { cwd: root, windowsHide: true, stdio: 'ignore' }).status, 0);
  report.project_id = env.NEON_PROJECT_ID;
  report.branch_id = env.NEON_BRANCH_ID;
  stage = 'build';
  const apiBinary = join(root, '.local', windows ? 'api-neon.exe' : 'api-neon');
  const probeBinary = join(root, '.local', windows ? 'dbprobe.exe' : 'dbprobe');
  await run('go', ['-C', 'apps/api', 'build', '-o', apiBinary, './cmd/server']);
  await run('go', ['-C', 'apps/api', 'build', '-o', probeBinary, './cmd/dbprobe']);
  const probe = async mode => JSON.parse((await run(probeBinary, mode ? [mode] : [])).trim());
  stage = 'tls-auth';
  report.checks.connection = await probe();
  assert.equal(report.checks.connection.migration_version, 1);
  report.checks.negative_tls = await probe('negative-tls');
  report.checks.negative_auth = await probe('negative-auth');
  report.checks.recovery_after_bad_auth = await probe();
  save();
  console.log('Actual Neon TLS chain, PostgreSQL 17, migration v1, rejected bad certificate/authentication and valid reconnection verified.');
  stage = 'browser';
  const api = launch(apiBinary, [], env);
  await ready(`${apiURL}/api/v1/health`, api);
  const frontendEnv = { ...browserEnvironment(env), ERP_NEON_VERIFY: 'explicit-demo-verification', ERP_EVIDENCE_PREFIX: 'neon' };
  const web = launch('pnpm', ['--filter', '@erp/web', 'dev', '--port', '5175'], frontendEnv);
  await ready('http://127.0.0.1:5175', web);
  await run('pnpm', ['exec', 'playwright', 'test', '--config', 'playwright.neon.config.ts'], frontendEnv);
  const browserReport = JSON.parse(readFileSync(join(root, '.local/neon-browser-results.json'), 'utf8'));
  assert.equal(browserReport.stats.unexpected, 0);
  assert.equal(browserReport.stats.skipped, 0);
  assert.equal(browserReport.stats.expected, 5);
  report.checks.browser = browserReport.stats;
  await stop(web);
  report.checks.after_browser = await probe();
  const baselineResponse = await fetch(`${apiURL}/api/v1/inventory?page_size=100`, { signal: AbortSignal.timeout(30_000) });
  assert.equal(baselineResponse.status, 200);
  const baseline = await baselineResponse.json();
  assert.ok(baseline.data.length > 0);
  const baselineHash = digest(baseline);
  report.checks.before_idle = { checked_at: new Date().toISOString(), inventory_sha256: baselineHash, api_pid: api.pid };
  report.status = 'waiting_for_observed_idle';
  save();
  console.log('Neon browser scenarios: 5 passed. Ledger mismatches: 0. API stays alive; no keep-alive queries are scheduled.');
  stage = 'idle-resume';
  report.checks.idle_observation = await idleSignal(Date.now() + 330_000);
  const firstStart = performance.now();
  const first = await fetch(`${apiURL}/api/v1/inventory?page_size=100`, { signal: AbortSignal.timeout(30_000) });
  const resumed = await first.json();
  report.checks.first_request_after_idle = { status: first.status, elapsed_ms: Math.round(performance.now() - firstStart),
    same_api_process: running.has(api), inventory_unchanged: digest(resumed) === baselineHash, checked_at: new Date().toISOString() };
  save();
  assert.equal(first.status, 200);
  assert.equal(digest(resumed), baselineHash);
  assert.ok(running.has(api));
  report.checks.after_idle = await probe();
  report.status = 'passed'; report.finished_at = new Date().toISOString(); save();
  console.log(JSON.stringify({ status: report.status, browser_passed: 5, first_request_after_idle: report.checks.first_request_after_idle }));
} catch {
  report.status = 'failed'; report.failed_stage = stage; report.finished_at = new Date().toISOString(); save();
  console.error(`Neon verification failed at ${stage}. No credentials or raw subprocess errors were printed.`);
  process.exitCode = 1;
} finally {
  for (const child of [...running]) await stop(child);
}
