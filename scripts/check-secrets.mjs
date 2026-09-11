import { spawnSync } from 'node:child_process';
import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseEnv } from 'node:util';
import { loadEnvironment } from './environment.mjs';

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const env = loadEnvironment(root);
const profiles = ['.env', '.env.neon'];
const environments = [env, ...profiles.filter(file => existsSync(join(root, file)))
  .map(file => parseEnv(readFileSync(join(root, file), 'utf8')))];
const values = [...new Set(environments.flatMap(profile =>
  ['POSTGRES_PASSWORD', 'DATABASE_URL', 'MIGRATION_DATABASE_URL', 'TEST_DATABASE_URL'].flatMap(key => {
    const value = profile[key];
    if (!value) return [];
    try { return [value, decodeURIComponent(new URL(value).password)]; } catch { return [value]; }
  })).filter(value => value && value.length >= 8))];

const listed = spawnSync('git', ['ls-files', '-co', '--exclude-standard', '-z'], { cwd: root, encoding: 'utf8', windowsHide: true });
if (listed.status !== 0) throw new Error('Cannot enumerate source files for secret verification.');
const files = new Set(listed.stdout.split('\0').filter(Boolean).map(path => join(root, path)));
for (const base of ['apps/web/build', 'apps/web/.svelte-kit/output']) {
  const visit = path => {
    if (!existsSync(path)) return;
    for (const entry of readdirSync(path, { withFileTypes: true })) {
      const child = join(path, entry.name);
      if (entry.isDirectory()) visit(child);
      else if (entry.isFile()) files.add(child);
    }
  };
  visit(join(root, base));
}

const failures = [];
if (existsSync(join(root, '.local'))) {
  for (const file of readdirSync(join(root, '.local'))) {
    if (/\.(?:log|json|txt)$/.test(file)) files.add(join(root, '.local', file));
  }
}
for (const file of profiles.filter(file => existsSync(join(root, file)))) {
  const ignored = spawnSync('git', ['check-ignore', '--quiet', file], { cwd: root, windowsHide: true });
  const tracked = spawnSync('git', ['ls-files', '--error-unmatch', file], { cwd: root, stdio: 'ignore', windowsHide: true });
  if (ignored.status !== 0 || tracked.status === 0) failures.push(`${file} is not safely excluded from Git`);
}
for (const path of files) {
  const contents = readFileSync(path);
  if (values.some(value => contents.includes(Buffer.from(value)))) failures.push(relative(root, path));
}
if (failures.length) {
  console.error('Concrete local credential values found, or environment exclusion failed:');
  for (const path of failures) console.error(path);
  process.exitCode = 1;
} else {
  console.log(`Credential-value scan: ${files.size} source/build/report files checked; no matches. Local/Neon profile exclusion verified where present.`);
  if (!values.length) console.log('No local credential values supplied; concrete-value matching was not exercised.');
}
