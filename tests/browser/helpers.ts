import { randomUUID } from 'node:crypto';
import { expect, type APIRequestContext } from '@playwright/test';

export type Entity = Record<string, any>;

export async function api(request: APIRequestContext, method: string, path: string, body?: unknown, key?: string) {
  const response = await request.fetch(`/api/v1${path}`, {
    method,
    ...(body === undefined ? {} : { data: body }),
    headers: key ? { 'Idempotency-Key': key } : {},
  });
  return { status: response.status(), body: await response.json() as Entity };
}

export async function create(request: APIRequestContext, path: string, body: unknown, key = randomUUID()) {
  const result = await api(request, 'POST', path, body, key);
  expect(result.status, JSON.stringify(result.body)).toBe(201);
  return result.body.data as Entity;
}

export async function get(request: APIRequestContext, path: string) {
  const result = await api(request, 'GET', path);
  expect(result.status, JSON.stringify(result.body)).toBe(200);
  return result.body.data as Entity;
}

export async function all(request: APIRequestContext, path: string) {
  const rows: Entity[] = [];
  const separator = path.includes('?') ? '&' : '?';
  for (let page = 1; page <= 1000; page++) {
    const result = await api(request, 'GET', `${path}${separator}page=${page}&page_size=100`);
    expect(result.status, JSON.stringify(result.body)).toBe(200);
    expect(Array.isArray(result.body.data)).toBe(true);
    expect(result.body.pagination.page).toBe(page);
    rows.push(...result.body.data);
    if (rows.length >= result.body.pagination.total) return rows;
    expect(result.body.data.length).toBeGreaterThan(0);
  }
  throw new Error('Pagination did not terminate.');
}

export const suffix = () => randomUUID().slice(0, 8).toUpperCase();

export async function item(request: APIRequestContext, kind: 'component' | 'finished_good', prefix: string) {
  const code = `${prefix}_${suffix()}`;
  return create(request, '/items', { code, name: code, kind, unit: '개' });
}

export async function setBOM(request: APIRequestContext, finished: Entity, components: Entity[]) {
  const result = await api(request, 'PUT', `/items/${finished.id}/bom`, {
    components: components.map(c => ({ item_id: c.id, quantity_per_unit: c.quantity_per_unit ?? 1 })),
  });
  expect(result.status, JSON.stringify(result.body)).toBe(200);
  return result.body.data;
}

export async function recipe(request: APIRequestContext, stock = 0, quantityPerUnit = 1) {
  const component = await item(request, 'component', 'PART');
  const finished = await item(request, 'finished_good', 'PRODUCT');
  await setBOM(request, finished, [{ ...component, quantity_per_unit: quantityPerUnit }]);
  if (stock > 0) await create(request, '/stock-receipts', { item_id: component.id, quantity: stock });
  return { component, finished };
}

export async function balance(request: APIRequestContext, target: Entity) {
  const rows = await all(request, `/inventory?q=${encodeURIComponent(target.code)}`);
  const found = rows.find(row => row.item_id === target.id);
  expect(found).toBeTruthy();
  return found!.quantity as number;
}

export async function assertLedger(request: APIRequestContext, items: Entity[]) {
  for (const target of items) {
    const movements = await all(request, `/stock-movements?item_id=${target.id}`);
    expect(movements.reduce((sum, row) => sum + row.delta, 0)).toBe(await balance(request, target));
    for (const row of movements) {
      expect(Number.isSafeInteger(row.delta)).toBe(true);
      expect(row.delta).not.toBe(0);
      if (row.movement_type === 'manual_receipt') {
        expect(row.receipt_id).toBeTruthy();
        expect(row.result_id).toBeNull();
      } else {
        expect(row.result_id).toBeTruthy();
        expect(row.receipt_id).toBeNull();
      }
    }
  }
}
