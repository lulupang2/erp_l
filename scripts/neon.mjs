import { spawn } from 'node:child_process';
import { dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { neonEnvironment } from './neon-environment.mjs';

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const actions = { dev: 'dev', migrate: 'db:migrate', status: 'db:status', 'serve:api': 'dev:api' };
const action = process.argv[2];
try {
  const env = { ...neonEnvironment(root), GOTOOLCHAIN: 'go1.26.7' };
  const command = action === 'probe' ? 'go' : process.execPath;
  const args = action === 'probe' ? ['-C', 'apps/api', 'run', './cmd/dbprobe']
    : actions[action] ? ['scripts/erp.mjs', actions[action]] : null;
  if (!args) throw new Error('Unknown Neon command. No migration, seed or reset is run automatically.');
  const child = spawn(command, args, { cwd: root, env, stdio: 'inherit', windowsHide: true });
  child.on('error', () => { console.error('Neon command could not start.'); process.exitCode = 1; });
  child.on('exit', code => { process.exitCode = code ?? 1; });
  for (const signal of ['SIGINT', 'SIGTERM']) process.once(signal, () => {
    if (!child.pid) return;
    if (process.platform === 'win32') spawn('taskkill', ['/PID', String(child.pid), '/T', '/F'], { stdio: 'ignore', windowsHide: true });
    else child.kill(signal);
  });
} catch {
  console.error('Neon profile or command is invalid. Check .env.neon without printing its contents.');
  process.exitCode = 1;
}
