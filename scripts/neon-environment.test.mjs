import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { browserEnvironment, initializeLocalEnvironment, loadEnvironment, testEnvironment } from './environment.mjs';
import { neonEnvironment, validateNeonProfile } from './neon-environment.mjs';

function fixture() {
  const url = 'postgres://example:example-only@ep-example.ap-southeast-1.aws.neon.tech/erp_demo?sslmode=verify-full';
  return { DATABASE_URL: url, MIGRATION_DATABASE_URL: url, NEON_PROJECT_ID: 'example-project',
    NEON_BRANCH_ID: 'br-example', NEON_HOST: 'ep-example.ap-southeast-1.aws.neon.tech', NEON_DATABASE: 'erp_demo' };
}

test('Neon is opt-in; local configuration and destructive test target stay local', () => {
  const root = mkdtempSync(join(tmpdir(), 'erp-neon-'));
  try {
    initializeLocalEnvironment(root);
    const original = readFileSync(join(root, '.env'), 'utf8');
    writeFileSync(join(root, '.env.neon'), Object.entries(fixture()).map(([key, value]) => `${key}=${value}`).join('\n'));
    const env = neonEnvironment(root, { PATH: 'tools', DATABASE_URL: 'inherited-local' });
    assert.equal(env.DATABASE_URL, fixture().DATABASE_URL);
    assert.equal(testEnvironment(env).DATABASE_URL, loadEnvironment(root, {}).TEST_DATABASE_URL);
    assert.equal(readFileSync(join(root, '.env'), 'utf8'), original);
    const safe = browserEnvironment(env);
    assert.equal(safe.PATH, 'tools');
    assert.ok(Object.keys(safe).every(key => !key.startsWith('NEON_') && !key.endsWith('DATABASE_URL')));
  } finally { rmSync(root, { recursive: true, force: true }); }
});

test('Neon refuses missing profiles without falling back to the local DB', () => {
  const root = mkdtempSync(join(tmpdir(), 'erp-neon-'));
  try { initializeLocalEnvironment(root); assert.throws(() => neonEnvironment(root, {})); }
  finally { rmSync(root, { recursive: true, force: true }); }
});

test('Neon rejects mismatched targets, poolers, weaker TLS, overrides and duplicates', () => {
  const profile = fixture();
  assert.deepEqual(validateNeonProfile(profile), profile);
  const bad = [profile.DATABASE_URL.replace('verify-full', 'require'),
    profile.DATABASE_URL + '&sslmode=disable', profile.DATABASE_URL + '&host=other',
    profile.DATABASE_URL + '&options=-csearch_path=other',
    profile.DATABASE_URL.replace('/erp_demo', '/other'),
    profile.DATABASE_URL.replace('ep-example.', 'ep-example-pooler.'),
    profile.DATABASE_URL.replace('postgres:', 'https:')];
  for (const url of bad) assert.throws(() => validateNeonProfile({ ...profile, DATABASE_URL: url }));
  assert.throws(() => validateNeonProfile({ ...profile, PATH: 'bad' }));
  assert.throws(() => validateNeonProfile({ ...profile, MIGRATION_DATABASE_URL: bad[0] }));
});
