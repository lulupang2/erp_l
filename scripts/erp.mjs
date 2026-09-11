import { spawn } from 'node:child_process';
import { mkdirSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { browserEnvironment, initializeLocalEnvironment, loadEnvironment, requireValue, testEnvironment } from './environment.mjs';

const root = dirname(dirname(fileURLToPath(import.meta.url)));
let env = loadEnvironment(root);
const children = new Set();
const windows = process.platform === 'win32';

function launch(command, args, childEnv = env) {
  // Calling pnpm through its JS entry point avoids shell quoting and .cmd on Windows.
  if (command === 'pnpm' && /\.[cm]?js$/i.test(process.env.npm_execpath ?? '')) {
    args = [process.env.npm_execpath, ...args];
    command = process.execPath;
  } else if (command === 'pnpm' && /\.exe$/i.test(process.env.npm_execpath ?? '')) {
    command = process.env.npm_execpath;
  } else if (command === 'pnpm' && windows) command = 'pnpm.exe';
  const child = spawn(command, args, { cwd: root, env: childEnv, stdio: 'inherit', windowsHide: true });
  children.add(child);
  child.once('exit', () => children.delete(child));
  return child;
}

function run(command, args, childEnv = env) {
  return new Promise((resolve, reject) => {
    const child = launch(command, args, childEnv);
    child.once('error', () => reject(new Error(`${command} could not start. Check your local tool installation and PATH.`)));
    child.once('exit', (code, signal) => code === 0 ? resolve() : reject(new Error(`${command} failed (${signal ?? code}).`)));
  });
}

async function stopChildren() {
  const pending = [...children].map(child => new Promise(resolve => {
    if (!child.pid) return resolve();
    child.once('exit', resolve);
    if (windows) {
      // A fixed, owned PID only. No workspace paths or user input are shell-composed.
      const killer = spawn('taskkill', ['/PID', String(child.pid), '/T', '/F'], { windowsHide: true, stdio: 'ignore' });
      killer.once('error', resolve);
      killer.once('exit', resolve);
    } else { child.kill('SIGTERM'); setTimeout(resolve, 2000).unref(); }
  }));
  await Promise.all(pending);
}

for (const signal of ['SIGINT', 'SIGTERM']) process.once(signal, async () => {
  await stopChildren();
  process.exit(signal === 'SIGINT' ? 130 : 143);
});

const go = (...args) => run('go', ['-C', 'apps/api', ...args], { ...env, GOTOOLCHAIN: 'go1.26.7' });
const web = (...args) => run('pnpm', ['--filter', '@erp/web', ...args], browserEnvironment(env));
const action = process.argv[2];

try {
  switch (action) {
    case 'setup:local':
      console.log(initializeLocalEnvironment(root)
        ? 'Created ignored .env with random local-only credentials. Connection values were not printed.'
        : '.env already exists; it was left unchanged.');
      break;
    case 'db:up':
      requireValue(env, 'POSTGRES_PASSWORD');
      await run('docker', ['compose', 'up', '-d', '--wait']);
      break;
    case 'db:down':
      await run('docker', ['compose', 'down']); // Named data volumes are retained.
      break;
    case 'db:migrate':
    case 'db:status':
      requireValue(env, 'MIGRATION_DATABASE_URL');
      await go('run', './cmd/migrate', action === 'db:status' ? 'status' : 'up');
      break;
    case 'demo:seed':
      requireValue(env, 'DATABASE_URL');
      await go('run', './cmd/demo', 'seed');
      break;
    case 'demo:reset':
      if (process.argv.slice(3).join(' ') !== '--confirm ERP_DEMO_RESET') {
        throw new Error('Explicit confirmation required: pnpm demo:reset --confirm ERP_DEMO_RESET');
      }
      await go('run', './cmd/demo', 'reset', '--confirm', 'ERP_DEMO_RESET');
      break;
    case 'dev:api':
      requireValue(env, 'DATABASE_URL');
      await go('run', './cmd/server');
      break;
    case 'dev':
      requireValue(env, 'DATABASE_URL');
      await Promise.race([go('run', './cmd/server'), web('dev')]);
      await stopChildren();
      break;
    case 'check':
      await go('vet', './...');
      await web('check');
      break;
    case 'build':
      await go('build', './...');
      await web('build');
      break;
    case 'test:unit':
      env = { ...env, TEST_DATABASE_URL: '' };
      await go('test', '-count=1', './...');
      await web('test');
      await run(process.execPath, ['--test', 'scripts/environment.test.mjs', 'scripts/neon-environment.test.mjs', 'scripts/neon-idle.test.mjs']);
      break;
    case 'test:integration':
      env = testEnvironment(env);
      await go('run', './cmd/migrate', 'up');
      await go('test', '-count=1', './...');
      break;
    case 'check:invariants':
      env = testEnvironment(env);
      await run('docker', ['compose', 'exec', '-T', 'postgres-test', 'psql', '-U', 'erp', '-d', 'erp_test',
        '-v', 'ON_ERROR_STOP=1', '-c', readFileSync(join(root, 'scripts/assert-invariants.sql'), 'utf8')]);
      break;
    case 'test:e2e':
      env = testEnvironment(env);
      await go('run', './cmd/migrate', 'up');
      mkdirSync(join(root, '.local'), { recursive: true });
      await go('build', '-o', join(root, '.local', windows ? 'api-e2e.exe' : 'api-e2e'), './cmd/server');
      env = { ...env, API_ADDR: '127.0.0.1:18080', ERP_API_PROXY_TARGET: 'http://127.0.0.1:18080' };
      await run('pnpm', ['exec', 'playwright', 'test', ...process.argv.slice(3)]);
      break;
    case 'serve:test:api':
      env = testEnvironment(env);
      env.API_ADDR = '127.0.0.1:18080';
      await run(join(root, '.local', windows ? 'api-e2e.exe' : 'api-e2e'), []);
      break;
    case 'serve:test:web':
      env.ERP_API_PROXY_TARGET = 'http://127.0.0.1:18080';
      await web('dev', '--host', '127.0.0.1', '--port', '5174');
      break;
    default:
      throw new Error('Unknown ERP command. See package.json scripts and README.md.');
  }
} catch (error) {
  await stopChildren();
  // Only controlled messages are emitted. Never print environment or command arguments.
  console.error(error instanceof Error ? error.message : 'ERP command failed.');
  process.exitCode = 1;
}
