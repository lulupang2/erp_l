import { createHash } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { browserEnvironment } from './environment.mjs';

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const api = join(root, 'apps/api');
const generated = ['internal/db', 'internal/v2db', 'internal/v2authdb'].map(path => join(api, path));
const executable = process.platform === 'win32' ? join(root, '.tools/sqlc-1.31.1/sqlc.exe') : 'sqlc';
const env = browserEnvironment(process.env);
function fingerprint() {
  return generated.map(directory => {
    if (!existsSync(directory)) return `${directory}:missing`;
    return readdirSync(directory).filter(name => name.endsWith('.go')).sort().map(name =>
      `${directory}/${name}:${createHash('sha256').update(readFileSync(join(directory, name))).digest('hex')}`).join('\n');
  }).join('\n');
}
const version = spawnSync(executable, ['version'], { cwd: api, env, encoding: 'utf8', windowsHide: true });
if (version.status !== 0 || version.stdout.trim() !== 'v1.31.1') {
  console.error('sqlc v1.31.1 is required. On Windows run scripts/bootstrap-sqlc.ps1.');
  process.exit(1);
}
const before = fingerprint();
const generatedResult = spawnSync(executable, ['generate'], { cwd: api, env, stdio: 'inherit', windowsHide: true });
if (generatedResult.status !== 0) process.exit(1);
if (process.argv.includes('--check') && before !== fingerprint()) {
  console.error('Generated SQL bindings differed. Review and retain the regenerated files.');
  process.exit(1);
}
console.log(process.argv.includes('--check') ? 'sqlc regeneration verified: generated source unchanged.' : 'Generated PostgreSQL bindings with sqlc v1.31.1.');
