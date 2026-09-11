import { readFileSync } from 'node:fs';
import { createInterface } from 'node:readline';

export function validateIdleObservation(value, endpoint, notBefore, now = Date.now()) {
  const observed = Date.parse(value?.observed_at);
  if (!value || value.endpoint_id !== endpoint || value.current_state !== 'idle'
    || !Number.isFinite(observed) || now < notBefore || observed < notBefore || observed > now + 10_000) {
    throw new Error('A fresh matching idle observation is required.');
  }
  // Keep only the explicitly validated metadata, not arbitrary operator input.
  return { endpoint_id: endpoint, current_state: 'idle', observed_at: value.observed_at };
}

/** File polling is local only; it never queries or wakes the database. */
export function waitForIdleObservation({ file, endpoint, notBefore, input = process.stdin, timeoutMs = 12 * 60_000, pollMs = 1000 }) {
  const reader = createInterface({ input });
  return new Promise((resolve, reject) => {
    let timeout, interval;
    const cleanup = () => { clearTimeout(timeout); clearInterval(interval); reader.close(); };
    const accept = raw => {
      try {
        const observation = validateIdleObservation(JSON.parse(raw), endpoint, notBefore);
        cleanup(); resolve(observation);
      } catch { /* Missing, stale, incomplete or mismatched observations are not evidence. */ }
    };
    timeout = setTimeout(() => { cleanup(); reject(new Error('Idle observation timed out; it is not a pass.')); }, timeoutMs);
    interval = setInterval(() => {
      try { accept(readFileSync(file, 'utf8')); } catch { /* Wait for an explicit control-plane observation. */ }
    }, pollMs);
    reader.on('line', accept);
    // A noninteractive terminal may close stdin. The file path remains usable.
  });
}
