export type Role = 'admin' | 'planner' | 'materials' | 'operator' | 'quality';
export type Row = Record<string, any>;
export type User = { id: string; username: string; roles: Role[]; active?: boolean };
export const roleLabels: Record<Role, string> = { admin: '관리자', planner: '생산관리', materials: '자재담당', operator: '작업자', quality: '품질담당' };
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
  catch { throw new ApiError(0, 'NETWORK_UNKNOWN', '서버 응답을 확인하지 못했습니다. 입력과 요청 키를 유지했습니다. 같은 요청을 다시 확인하세요.'); }
  let envelope: { data?: unknown; error?: { code?: string; message?: string; details?: unknown } };
  try { envelope = await response.json() as typeof envelope; }
  catch { throw new ApiError(response.ok ? 0 : response.status, 'RESPONSE_UNKNOWN', '서버 응답을 읽지 못했습니다. 확정 여부를 확인할 때 같은 요청을 사용하세요.'); }
  if (!response.ok) {
    throw new ApiError(response.status, envelope.error?.code ?? 'REQUEST_FAILED', envelope.error?.message ?? '요청을 처리하지 못했습니다.', envelope.error?.details);
  }
  return envelope.data as T;
}
export function errorText(error: unknown) { return error instanceof ApiError ? `${error.message} (${error.code})` : error instanceof Error ? error.message : '요청에 실패했습니다.'; }

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
    this.error = '데모 작업자 정보가 변경되어 이전의 미확정 요청은 재전송하지 않습니다. 새로 입력해 주세요.';
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
      this.error = '처리 결과를 확인하지 못한 이전 요청이 남아 있습니다. 다른 작업을 보내기 전에 “같은 요청 다시 확인”으로 기존 요청을 먼저 확인하세요.';
      this.success = '';
      this.uncertain = true;
      return;
    }

    this.busy = true; this.error = ''; this.success = '';
    const pending = this.pending ?? { path, method: normalizedMethod, body: snapshot, bodyJSON: encoded, key: crypto.randomUUID(), ownerId };
    this.pending = pending;
    try {
      const result = await request<T>(pending.path, pending.method, pending.body, pending.key);
      this.pending = null; this.uncertain = false; this.success = '저장했습니다.'; return result;
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
export const kinds: Record<string, string> = { component_receipt: '부품 입고', issue: '자재 불출', return: '미소비 자재 반납', material_loss: '현장 자재 손실', production: '신규 생산 보고', inspection: '검사 · 재검사', disposition: '부적합 처분', rework: '재작업 보고', goods_receipt: '합격품 입고' };
export const documentRoles: Record<string, Role[]> = { component_receipt: ['materials'], issue: ['materials'], return: ['materials'], material_loss: ['materials'], production: ['operator'], inspection: ['quality'], disposition: ['quality'], rework: ['operator'], goods_receipt: ['materials'] };
export const states: Record<string, string> = { draft: '초안', approved: '승인', retired: '사용 중지', issued: '발행', in_progress: '작업 중', held: '보류', closed: '마감', early_closed: '조기종결', cancelled: '취소', posted: '확정', reversed: '역분개 완료', active: '진행', ended: '종료', component: '부품', finished: '완제품', warehouse: '원자재 창고', floor: '현장', finished_goods: '완제품 창고', disposal: '폐기', rework: '재작업', pass: '합격', fail: '부적합' };
export function label(value: unknown): string { return value == null ? '—' : states[String(value)] ?? kinds[String(value)] ?? String(value); }
export function quantity(value: unknown): string { return value == null ? '—' : Number(value).toLocaleString('ko-KR'); }
export function short(value: unknown): string { return value ? String(value).slice(0, 8) : '—'; }
export function name(rows: Row[] | undefined, id: unknown): string { const row = rows?.find(row => row.id === id); return row ? `${String(row.code ?? row.username ?? short(row.id))}${row.name ? ` · ${String(row.name)}` : ''}` : short(id); }
export function time(value: unknown): string { return value ? new Date(String(value)).toLocaleString('ko-KR', { timeZone: 'Asia/Seoul' }) : '—'; }
export type References = { items: Row[]; locations: Row[]; lots: Row[]; defect_reasons: Row[]; bom_revisions: Row[]; inspection_revisions: Row[] };
export function text(value: unknown): string { return typeof value === 'string' ? value : value == null ? '' : String(value); }
export function integer(value: unknown): number { const number = Number(value); return Number.isFinite(number) ? number : 0; }
