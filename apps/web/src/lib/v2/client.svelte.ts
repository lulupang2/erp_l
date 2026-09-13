import { localeTag, t } from '$lib/i18n.svelte';

export type Role = 'admin' | 'planner' | 'materials' | 'operator' | 'quality';
export type Row = Record<string, any>;
export type User = { id: string; username: string; roles: Role[]; active?: boolean };
const roleKeys: Record<Role, Parameters<typeof t>[0]> = { admin: 'role_admin', planner: 'role_planner', materials: 'role_materials', operator: 'role_operator', quality: 'role_quality' };
export const roleLabels = new Proxy({} as Record<Role, string>, { get: (_target, key: string) => t(roleKeys[key as Role] ?? 'dash'), ownKeys: () => Object.keys(roleKeys), getOwnPropertyDescriptor: () => ({ enumerable: true, configurable: true }) });
export const publicActor: User = { id: '00000000-0000-0000-0000-000000000002', username: 'portfolio', roles: ['admin', 'planner', 'materials', 'operator', 'quality'] };
export const session = $state<{ user: User; csrf: string; ready: boolean; expired: boolean }>({ user: publicActor, csrf: '', ready: true, expired: false });
export function can(...roles: Role[]) { return !!session.user?.roles.some(role => roles.includes(role)); }
export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string, public details?: unknown) { super(message); }
}
export async function request<T = Row>(path: string, method = 'GET', body?: unknown, key?: string): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' };
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (key) headers['Idempotency-Key'] = key;
  let response: Response;
  try { response = await fetch(`/api/v2${path}`, { method, credentials: 'same-origin', headers, body: body === undefined ? undefined : JSON.stringify(body) }); }
  catch { throw new ApiError(0, 'NETWORK_UNKNOWN', t('requestNetworkUnknown')); }
  let envelope: { data?: unknown; error?: { code?: string; message?: string; details?: unknown } };
  try { envelope = await response.json() as typeof envelope; }
  catch { throw new ApiError(response.ok ? 0 : response.status, 'RESPONSE_UNKNOWN', t('responseUnknown')); }
  if (!response.ok) {
    throw new ApiError(response.status, envelope.error?.code ?? 'REQUEST_FAILED', envelope.error?.message ?? t('requestFailed'), envelope.error?.details);
  }
  return envelope.data as T;
}
export function errorText(error: unknown) { return error instanceof ApiError ? `${error.message} (${error.code})` : error instanceof Error ? error.message : t('genericRequestFailed'); }

type PendingCommand = { path: string; method: string; body: unknown; bodyJSON: string; key: string; ownerId: string };
function cloneBody(body: unknown): unknown { return body === undefined ? undefined : JSON.parse(JSON.stringify(body)); }
function bodyJSON(body: unknown): string { return JSON.stringify(body === undefined ? null : body); }

export class Command {
  busy = $state(false);
  error = $state('');
  success = $state('');
  uncertain = $state(false);
  private pending: PendingCommand | null = null;
  get locked() { return this.uncertain && this.pending !== null; }

  private changedAccount(): boolean {
    if (!this.pending) return false;
    const ownerId = session.user.id;
    if (ownerId === this.pending.ownerId) return false;
    this.pending = null;
    this.uncertain = false;
    this.success = '';
    this.error = t('changedActorRequest');
    return true;
  }

  async run<T = Row>(path: string, body: unknown = {}, method = 'POST'): Promise<T | undefined> {
    if (this.busy) return;
    const ownerId = session.user.id;
    if (this.changedAccount()) return;

    const normalizedMethod = method.toUpperCase();
    const snapshot = cloneBody(body);
    const encoded = bodyJSON(snapshot);
    if (this.pending && (this.pending.path !== path || this.pending.method !== normalizedMethod || this.pending.bodyJSON !== encoded)) {
      this.error = t('pendingRequestConflict');
      this.success = '';
      this.uncertain = true;
      return;
    }

    this.busy = true; this.error = ''; this.success = '';
    const pending = this.pending ?? { path, method: normalizedMethod, body: snapshot, bodyJSON: encoded, key: crypto.randomUUID(), ownerId };
    this.pending = pending;
    try {
      const result = await request<T>(pending.path, pending.method, pending.body, pending.key);
      this.pending = null; this.uncertain = false; this.success = t('saved'); return result;
    } catch (error) {
      this.error = errorText(error);
      this.uncertain = !(error instanceof ApiError) || error.status === 0 || error.status === 408 || error.status >= 500;
      if (!this.uncertain) this.pending = null;
      return undefined;
    } finally { this.busy = false; }
  }

  async retry<T = Row>(): Promise<T | undefined> {
    if (!this.pending || this.busy) return;
    if (this.changedAccount() || !this.pending) return;
    return this.run<T>(this.pending.path, this.pending.body, this.pending.method);
  }
}
export class Resource<T> {
  value = $state<T | null>(null);
  loading = $state(false);
  error = $state('');
  private generation = 0;
  async load(loader: () => Promise<T>) {
    const generation = ++this.generation; this.loading = true; this.error = '';
    try { const value = await loader(); if (generation === this.generation) this.value = value; }
    catch (error) { if (generation === this.generation) this.error = errorText(error); }
    finally { if (generation === this.generation) this.loading = false; }
  }
}
const kindKeys: Record<string, Parameters<typeof t>[0]> = {
  component_receipt: 'kind_component_receipt', issue: 'kind_issue', return: 'kind_return', material_loss: 'kind_material_loss', production: 'kind_production', inspection: 'kind_inspection', disposition: 'kind_disposition', rework: 'kind_rework', goods_receipt: 'kind_goods_receipt'
};
export const kinds = new Proxy({} as Record<string, string>, { get: (_target, key: string) => kindKeys[key] ? t(kindKeys[key]) : undefined, ownKeys: () => Object.keys(kindKeys), getOwnPropertyDescriptor: (_target, key: string) => kindKeys[key] ? { enumerable: true, configurable: true } : undefined });
export const documentRoles: Record<string, Role[]> = { component_receipt: ['materials'], issue: ['materials'], return: ['materials'], material_loss: ['materials'], production: ['operator'], inspection: ['quality'], disposition: ['quality'], rework: ['operator'], goods_receipt: ['materials'] };
const stateKeys: Record<string, Parameters<typeof t>[0]> = {
  draft: 'state_draft', approved: 'state_approved', retired: 'state_retired', issued: 'state_issued', in_progress: 'state_in_progress', held: 'state_held', closed: 'state_closed', early_closed: 'state_early_closed', cancelled: 'state_cancelled', posted: 'state_posted', reversed: 'state_reversed', active: 'state_active', ended: 'state_ended', component: 'state_component', finished: 'state_finished', warehouse: 'state_warehouse', floor: 'state_floor', finished_goods: 'state_finished_goods', disposal: 'state_disposal', rework: 'state_rework', pass: 'state_pass', fail: 'state_fail'
};
export const states = new Proxy({} as Record<string, string>, { get: (_target, key: string) => stateKeys[key] ? t(stateKeys[key]) : undefined, ownKeys: () => Object.keys(stateKeys), getOwnPropertyDescriptor: (_target, key: string) => stateKeys[key] ? { enumerable: true, configurable: true } : undefined });
export function label(value: unknown): string { return value == null ? t('dash') : states[String(value)] ?? kinds[String(value)] ?? String(value); }
export function quantity(value: unknown): string { return value == null ? t('dash') : Number(value).toLocaleString(localeTag()); }
export function short(value: unknown): string { return value ? String(value).slice(0, 8) : t('dash'); }
export function name(rows: Row[] | undefined, id: unknown): string { const row = rows?.find(row => row.id === id); return row ? `${String(row.code ?? row.username ?? short(row.id))}${row.name ? ` · ${String(row.name)}` : ''}` : short(id); }
export function time(value: unknown): string { return value ? new Date(String(value)).toLocaleString(localeTag(), { timeZone: 'Asia/Seoul' }) : t('dash'); }
export type References = { items: Row[]; locations: Row[]; lots: Row[]; defect_reasons: Row[]; bom_revisions: Row[]; inspection_revisions: Row[] };
export function text(value: unknown): string { return typeof value === 'string' ? value : value == null ? '' : String(value); }
export function integer(value: unknown): number { const number = Number(value); return Number.isFinite(number) ? number : 0; }
