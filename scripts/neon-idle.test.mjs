import assert from 'node:assert/strict';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { Readable } from 'node:stream';
import { test } from 'node:test';
import { validateIdleObservation, waitForIdleObservation } from './neon-idle.mjs';

test('idle evidence must match the endpoint and current wait window', () => {
  const at = Date.now();
  const value = { endpoint_id: 'ep-test', current_state: 'idle', observed_at: new Date(at).toISOString(), untrusted: 'discard' };
  assert.deepEqual(validateIdleObservation(value, 'ep-test', at - 10, at), {
    endpoint_id: 'ep-test', current_state: 'idle', observed_at: value.observed_at,
  });
});

test('stale, future, active and mismatched endpoint observations fail closed', () => {
  const at = Date.now();
  const value = { endpoint_id: 'ep-test', current_state: 'idle', observed_at: new Date(at).toISOString() };
  for (const candidate of [null, {}, { ...value, current_state: 'active' }, { ...value, endpoint_id: 'ep-other' },
    { ...value, observed_at: new Date(at - 1000).toISOString() }, { ...value, observed_at: new Date(at + 60_000).toISOString() }]) {
    assert.throws(() => validateIdleObservation(candidate, 'ep-test', at - 10, at));
  }
});

test('closed stdin still accepts a new observation file; an old file is ignored', async () => {
  const dir = mkdtempSync(join(tmpdir(), 'erp-idle-'));
  const file = join(dir, 'observation.json');
  const at = Date.now();
  const value = { endpoint_id: 'ep-test', current_state: 'idle', observed_at: new Date(at - 60_000).toISOString() };
  let writer;
  try {
    writeFileSync(file, JSON.stringify(value));
    const pending = waitForIdleObservation({ file, endpoint: 'ep-test', notBefore: at, input: Readable.from([]), timeoutMs: 3000, pollMs: 10 });
    writer = setTimeout(() => writeFileSync(file, JSON.stringify({ ...value, observed_at: new Date().toISOString() })), 40);
    const result = await pending;
    assert.ok(Date.parse(result.observed_at) >= at);
  } finally { clearTimeout(writer); rmSync(dir, { recursive: true, force: true }); }
});

test('missing evidence times out instead of treating elapsed time as successful idle', async () => {
  const dir = mkdtempSync(join(tmpdir(), 'erp-idle-'));
  try {
    await assert.rejects(waitForIdleObservation({ file: join(dir, 'missing.json'), endpoint: 'ep-test', notBefore: 0,
      input: Readable.from([]), timeoutMs: 30, pollMs: 10 }), /timed out/);
  } finally { rmSync(dir, { recursive: true, force: true }); }
});
