import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { parseEnv } from 'node:util';
import { loadEnvironment } from './environment.mjs';

const keys = ['DATABASE_URL', 'MIGRATION_DATABASE_URL', 'NEON_PROJECT_ID', 'NEON_BRANCH_ID', 'NEON_HOST', 'NEON_DATABASE'];

/** Explicit opt-in only. Never retarget the ordinary local test/reset commands. */
export function validateNeonProfile(profile) {
  for (const key of keys) if (typeof profile[key] !== 'string' || !profile[key]) throw new Error(`Neon profile requires ${key}.`);
  for (const key of Object.keys(profile)) if (!keys.includes(key)) throw new Error('Unexpected Neon profile setting.');
  if (!/^[a-z0-9-]+$/.test(profile.NEON_PROJECT_ID) || !/^br-[a-z0-9-]+$/.test(profile.NEON_BRANCH_ID)
    || !/^ep-[a-z0-9-]+\.(?:[a-z0-9-]+\.)*neon\.tech$/.test(profile.NEON_HOST)
    || profile.NEON_HOST.includes('-pooler.') || profile.NEON_DATABASE !== 'erp_demo') {
    throw new Error('An explicitly designated direct Neon ERP demo target is required.');
  }
  for (const key of ['DATABASE_URL', 'MIGRATION_DATABASE_URL']) {
    let url;
    try { url = new URL(profile[key]); } catch { throw new Error(`${key} is not a valid PostgreSQL URL.`); }
    if (!['postgres:', 'postgresql:'].includes(url.protocol) || url.hostname !== profile.NEON_HOST
      || url.pathname !== `/${profile.NEON_DATABASE}` || !['', '5432'].includes(url.port)
      || !url.username || !url.password || url.hash
      || url.searchParams.getAll('sslmode').length !== 1 || url.searchParams.get('sslmode') !== 'verify-full') {
      throw new Error('Neon connection target or certificate verification does not match the explicit profile.');
    }
    if ([...url.searchParams.keys()].some(option => option !== 'sslmode')) throw new Error('Unexpected Neon connection option.');
  }
  return { ...profile };
}

export function neonEnvironment(root, inherited = process.env) {
  let profile;
  try { profile = parseEnv(readFileSync(join(root, '.env.neon'), 'utf8')); }
  catch { throw new Error('Create the gitignored backend-only .env.neon profile before using Neon commands.'); }
  const validated = validateNeonProfile(profile);
  // File selection is explicit and takes precedence over an inherited local DSN.
  // The original local profile, test target, and Docker credentials are preserved.
  return { ...loadEnvironment(root, inherited), ...validated };
}
