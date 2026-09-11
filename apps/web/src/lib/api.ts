import type {
  Bom, BomInput, Data, Inventory, Item, ItemInput, ListQuery, Movement,
  Order, OrderDetail, OrderInput, Page, ProductionResult, Receipt, ReceiptInput, ResultInput, Shortage
} from './types';

type Fetcher = typeof fetch;
type JsonObject = Record<string, unknown>;
const object = (value: unknown): value is JsonObject => typeof value === 'object' && value !== null;
const isUuid = (value: unknown): value is string => typeof value === 'string'
  && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value);

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly details?: unknown,
    public readonly definitive = false
  ) { super(message); this.name = 'ApiError'; }

  get shortages(): Shortage[] {
    if (!object(this.details) || !Array.isArray(this.details.shortages)) return [];
    return this.details.shortages.filter((row): row is Shortage => object(row)
      && typeof row.item_id === 'string'
      && Number.isSafeInteger(row.required_quantity) && Number.isSafeInteger(row.available_quantity));
  }
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : '요청을 처리하지 못했습니다. 다시 확인해 주세요.';
}

export class ApiClient {
  constructor(private readonly fetcher: Fetcher = (...args) => fetch(...args)) {}

  async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 35_000);
    try {
      const response = await this.fetcher(`/api/v1${path}`, {
        ...options, signal: controller.signal, credentials: 'same-origin', cache: 'no-store',
        headers: { Accept: 'application/json', ...(options.body ? { 'Content-Type': 'application/json' } : {}), ...options.headers }
      });
      let payload: unknown;
      try { payload = await response.json(); }
      catch { throw new ApiError(response.status, 'INVALID_RESPONSE', '서버 응답을 확인할 수 없습니다. 저장 요청은 같은 요청으로 다시 확인해 주세요.'); }
      if (!response.ok) {
        const error = object(payload) && object(payload.error) ? payload.error : undefined;
        const valid = error && typeof error.code === 'string' && typeof error.message === 'string';
        throw new ApiError(response.status, valid ? String(error.code) : 'HTTP_ERROR',
          valid ? String(error.message) : '서버 요청에 실패했습니다. 잠시 후 다시 확인해 주세요.',
          error?.details, Boolean(valid && [400, 404, 409].includes(response.status)));
      }
      if (!object(payload) || !('data' in payload)) {
        throw new ApiError(response.status, 'INVALID_RESPONSE', '서버 응답 형식이 올바르지 않습니다. 같은 요청으로 다시 확인해 주세요.');
      }
      return payload as T;
    } catch (error) {
      if (error instanceof ApiError) throw error;
      throw new ApiError(0, 'NETWORK_ERROR', '서버에 연결하지 못했거나 응답 시간이 초과되었습니다. 저장 여부가 불확실하므로 같은 요청을 다시 확인해 주세요.');
    } finally { clearTimeout(timer); }
  }

  list<T>(path: string, query: ListQuery = {}): Promise<Page<T>> {
    const search = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== '') search.set(key, String(value));
    }
    return this.request<Page<T>>(`${path}${search.size ? `?${search}` : ''}`);
  }

  // Selectors must never silently truncate the server's paginated data.
  async all<T>(path: string, query: ListQuery = {}): Promise<T[]> {
    const rows: T[] = [];
    let total: number | undefined;
    let size: number | undefined;
    for (let page = 1; page <= 10_000; page++) {
      const response = await this.list<T>(path, { ...query, page, page_size: 100 });
      if (!Array.isArray(response.data) || !response.pagination
        || response.pagination.page !== page || !Number.isSafeInteger(response.pagination.page_size)
        || response.pagination.page_size < 1 || response.pagination.page_size > 100
        || !Number.isSafeInteger(response.pagination.total) || response.pagination.total < 0) {
        throw new ApiError(200, 'INVALID_PAGINATION', '전체 목록을 불러오지 못했습니다. 목록 응답을 확인해 주세요.');
      }
      total ??= response.pagination.total;
      size ??= response.pagination.page_size;
      if (total !== response.pagination.total || size !== response.pagination.page_size
        || response.data.length !== Math.min(size, Math.max(0, total - rows.length))) {
        throw new ApiError(200, 'INCOMPLETE_LIST', '목록이 변경되었거나 일부 항목이 누락되었습니다. 전체 목록을 다시 조회해 주세요.');
      }
      rows.push(...response.data);
      if (rows.length === total) return rows;
    }
    throw new ApiError(200, 'PAGE_LIMIT', '목록 페이지 한도를 초과했습니다.');
  }

  async one<T>(path: string): Promise<T> { return (await this.request<Data<T>>(path)).data; }
  async write<T>(path: string, method: 'POST' | 'PUT', body: object): Promise<T> {
    return (await this.request<Data<T>>(path, { method, body: JSON.stringify(body) })).data;
  }

  items = (query: ListQuery = {}) => this.list<Item>('/items', query);
  allItems = (query: ListQuery = {}) => this.all<Item>('/items', query);
  item = (id: string) => this.one<Item>(`/items/${encodeURIComponent(id)}`);
  createItem = (body: ItemInput) => this.write<Item>('/items', 'POST', body);
  bom = (id: string) => this.one<Bom>(`/items/${encodeURIComponent(id)}/bom`);
  replaceBom = (id: string, body: BomInput) => this.write<Bom>(`/items/${encodeURIComponent(id)}/bom`, 'PUT', body);
  inventory = (query: ListQuery = {}) => this.list<Inventory>('/inventory', query);
  allInventory = () => this.all<Inventory>('/inventory');
  movements = (query: ListQuery = {}) => this.list<Movement>('/stock-movements', query);
  receipt = (id: string) => this.one<Receipt>(`/stock-receipts/${encodeURIComponent(id)}`);
  orders = (query: ListQuery = {}) => this.list<Order>('/production-orders', query);
  order = (id: string) => this.one<OrderDetail>(`/production-orders/${encodeURIComponent(id)}`);
  results = (id: string, query: ListQuery = {}) => this.list<ProductionResult>(`/production-orders/${encodeURIComponent(id)}/results`, query);
  result = (id: string) => this.one<ProductionResult>(`/production-results/${encodeURIComponent(id)}`);

  async itemMap(ids: string[] = []): Promise<Map<string, Item>> {
    const map = new Map((await this.allItems()).map((item) => [item.id, item]));
    await Promise.all([...new Set(ids)].filter((id) => !map.has(id)).map(async (id) => map.set(id, await this.item(id))));
    return map;
  }
}

export const api = new ApiClient();
export type PendingRequest = Readonly<{ key: string; path: string; body: string }>;
export type PendingStorage = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;

function browserStorage(): PendingStorage | undefined {
  try { return typeof window === 'undefined' ? undefined : window.sessionStorage; }
  catch { return undefined; }
}

/** Keeps both UUID and serialized bytes until a definitive outcome or explicit reset. */
export class IdempotentOperation<Body extends object, Result> {
  private current: PendingRequest | null = null;
  private running = false;
  private readonly storageKey: string;

  constructor(
    private readonly path: string,
    private readonly client = api,
    private readonly storage = browserStorage(),
    private readonly uuid: () => string = () => crypto.randomUUID()
  ) {
    this.storageKey = `erp.pending.v1:${path}`;
    try {
      const raw = this.storage?.getItem(this.storageKey);
      const saved: unknown = raw ? JSON.parse(raw) : null;
      if (object(saved) && saved.path === path && isUuid(saved.key)
        && typeof saved.body === 'string' && object(JSON.parse(saved.body))) {
        this.current = Object.freeze({ key: saved.key, path, body: saved.body });
      }
    } catch { /* Storage may be unavailable; the live instance still preserves retries. */ }
  }

  get pending(): PendingRequest | null { return this.current; }
  get draft(): Body | null { return this.current ? JSON.parse(this.current.body) as Body : null; }

  reset(): void {
    if (this.running) throw new Error('처리 중인 요청은 초기화할 수 없습니다.');
    this.current = null;
    try { this.storage?.removeItem(this.storageKey); } catch { /* No credentials are persisted. */ }
  }

  async submit(body: Body): Promise<Result> {
    if (this.running) throw new Error('동일한 요청을 이미 처리 중입니다.');
    if (!this.current) {
      this.current = Object.freeze({ key: this.uuid(), path: this.path, body: JSON.stringify(body) });
      try { this.storage?.setItem(this.storageKey, JSON.stringify(this.current)); } catch { /* Retain in memory. */ }
    }
    const pending = this.current;
    this.running = true;
    try {
      const result = await this.client.request<Data<Result>>(pending.path, {
        method: 'POST', body: pending.body, headers: { 'Idempotency-Key': pending.key }
      });
      // A 2xx status alone cannot prove a committed resource was received.
      // Keep the exact key/body when a proxy or broken response hides the ID.
      if (!object(result.data) || !isUuid(result.data.id)) {
        throw new ApiError(200, 'INVALID_RESPONSE', '저장 결과의 문서 번호를 확인할 수 없습니다. 같은 요청으로 다시 확인해 주세요.');
      }
      this.running = false;
      this.reset();
      return result.data;
    } catch (error) {
      this.running = false;
      if (error instanceof ApiError && error.definitive) this.reset();
      throw error;
    }
  }
}

export const receiptOperation = () => new IdempotentOperation<ReceiptInput, Receipt>('/stock-receipts');
export const orderOperation = () => new IdempotentOperation<OrderInput, OrderDetail>('/production-orders');
export const resultOperation = (id: string) => new IdempotentOperation<ResultInput, ProductionResult>(`/production-orders/${encodeURIComponent(id)}/results`);
