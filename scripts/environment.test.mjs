import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { browserEnvironment, initializeLocalEnvironment, loadEnvironment, testEnvironment } from './environment.mjs';

test('local setup is create-only, creates distinct databases and never changes a supplied environment', () => {
  const dir = mkdtempSync(join(tmpdir(), 'erp-env-'));
  try {
    assert.equal(initializeLocalEnvironment(dir), true);
    const before = readFileSync(join(dir, '.env'), 'utf8');
    assert.equal(initializeLocalEnvironment(dir), false);
    assert.equal(readFileSync(join(dir, '.env'), 'utf8'), before);
    const inherited = { API_ADDR: '127.0.0.1:18080' };
    const env = loadEnvironment(dir, inherited);
    assert.equal(env.API_ADDR, inherited.API_ADDR);
    assert.equal(inherited.DATABASE_URL, undefined);
    assert.equal(env.POSTGRES_PASSWORD.length, 64);
    assert.equal(new URL(env.DATABASE_URL).pathname, '/erp_demo');
    assert.equal(new URL(env.TEST_DATABASE_URL).pathname, '/erp_test');
    assert.notEqual(env.DATABASE_URL, env.TEST_DATABASE_URL);
    assert.equal(testEnvironment(env).DATABASE_URL, env.TEST_DATABASE_URL);
  } finally { rmSync(dir, { recursive: true, force: true }); }
});

test('dotenv cannot inject executable paths or unrelated environment keys', () => {
  const dir = mkdtempSync(join(tmpdir(), 'erp-env-'));
  try {
    writeFileSync(join(dir, '.env'), 'PATH=bad\nNODE_OPTIONS=bad\nDATABASE_URL="demo"\n');
    const env = loadEnvironment(dir, {});
    assert.deepEqual(env, { DATABASE_URL: 'demo' });
  } finally { rmSync(dir, { recursive: true, force: true }); }
});

test('integration environment fails closed for missing, remote, demo and redirected targets', () => {
  const bad = [undefined, '', 'not a url',
    'postgres://localhost:55433/erp_demo',
    'postgres://example.com:55433/erp_test',
    'postgres://localhost:5432/erp_test',
    'postgres://localhost:55433/erp_test?host=example.com',
    'postgres://localhost:55433/erp_test?options=other',
    'https://localhost:55433/erp_test'];
  for (const value of bad) assert.throws(() => testEnvironment({ TEST_DATABASE_URL: value }));
});

test('frontend tooling inherits no ERP database secrets', () => {
  const env = { PATH: 'tools', ERP_API_PROXY_TARGET: 'http://127.0.0.1:18080', DATABASE_URL: 'private',
    MIGRATION_DATABASE_URL: 'private', TEST_DATABASE_URL: 'private', POSTGRES_PASSWORD: 'private', API_ADDR: 'private' };
  assert.deepEqual(browserEnvironment(env), { PATH: 'tools', ERP_API_PROXY_TARGET: 'http://127.0.0.1:18080' });
  assert.equal(env.DATABASE_URL, 'private');
});
