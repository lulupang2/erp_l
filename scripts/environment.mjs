import { randomBytes } from 'node:crypto';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { parseEnv } from 'node:util';

const allowed = new Set([
  'POSTGRES_PASSWORD', 'DATABASE_URL', 'MIGRATION_DATABASE_URL',
  'TEST_DATABASE_URL', 'API_ADDR', 'ERP_API_PROXY_TARGET',
  'V2_DATABASE_URL', 'V2_MIGRATION_DATABASE_URL',
]);

/** Existing process variables take precedence; connection values are never logged. */
export function loadEnvironment(root, inherited = process.env) {
  const env = { ...inherited };
  const file = join(root, '.env');
  if (existsSync(file)) {
    const parsed = parseEnv(readFileSync(file, 'utf8'));
    for (const [key, value] of Object.entries(parsed)) {
      if (allowed.has(key) && env[key] === undefined) env[key] = value;
    }
  }
  return env;
}

/** The example contains no password; the real local .env is create-only and ignored. */
export function initializeLocalEnvironment(root) {
  const file = join(root, '.env');
  if (existsSync(file)) return false;
  const password = randomBytes(32).toString('hex');
  const localURL = (port, db) => `postgres://erp:${password}@127.0.0.1:${port}/${db}?sslmode=disable`;
  const lines = [
    '# Generated for local ERP only. Never commit or share this file.',
    `POSTGRES_PASSWORD=${password}`,
    `DATABASE_URL=${localURL(55432, 'erp_demo')}`,
    `MIGRATION_DATABASE_URL=${localURL(55432, 'erp_demo')}`,
    `TEST_DATABASE_URL=${localURL(55433, 'erp_test')}`,
    'API_ADDR=127.0.0.1:8080', '',
  ];
  writeFileSync(file, lines.join('\n'), { flag: 'wx', mode: 0o600 });
  return true;
}

export function requireValue(env, name) {
  if (!env[name]) throw new Error(`${name} is required. Run pnpm setup:local or set backend environment variables.`);
  return env[name];
}

/** UI tooling gets its proxy target, never the backend database credentials. */
export function browserEnvironment(env) {
  const publicEnv = { ...env };
  for (const key of ['POSTGRES_PASSWORD', 'DATABASE_URL', 'MIGRATION_DATABASE_URL', 'TEST_DATABASE_URL', 'API_ADDR',
    'V2_DATABASE_URL', 'V2_MIGRATION_DATABASE_URL', 'TEST_V2_DATABASE_URL', 'V2_ADMIN_PASSWORD', 'V2_DB_PASSWORD']) {
    delete publicEnv[key];
  }
  for (const key of Object.keys(publicEnv)) if (key.startsWith('NEON_')) delete publicEnv[key];
  return publicEnv;
}

/** Test commands must never silently target the demo DB or a remote database. */
export function testEnvironment(env) {
  const raw = requireValue(env, 'TEST_DATABASE_URL');
  let url;
  try { url = new URL(raw); } catch { throw new Error('TEST_DATABASE_URL is not a valid PostgreSQL URL.'); }
  if (!['postgres:', 'postgresql:'].includes(url.protocol)
    || !['localhost', '127.0.0.1', '[::1]'].includes(url.hostname)
    || url.pathname !== '/erp_test' || url.port !== '55433') {
    throw new Error('Tests require the isolated loopback erp_test database on port 55433.');
  }
  // Disallow libpq options that could redirect a URL or change the isolation target.
  for (const key of url.searchParams.keys()) {
    if (key !== 'sslmode') throw new Error('Unexpected TEST_DATABASE_URL connection option.');
  }
  return { ...env, DATABASE_URL: raw, MIGRATION_DATABASE_URL: raw, TEST_DATABASE_URL: raw,
    V2_DATABASE_URL: '', V2_MIGRATION_DATABASE_URL: raw, TEST_V2_DATABASE_URL: '' };
}
