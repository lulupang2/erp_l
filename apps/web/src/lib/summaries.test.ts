import { describe, expect, it, vi } from 'vitest';
import { inventorySummary, itemSummary, orderSummary } from './summaries';
import type { Inventory, ListQuery } from './types';

describe('read-only summary cards', () => {
  it('uses server item totals rather than the number of loaded rows', async () => {
    const items = vi.fn(async (query: ListQuery = {}) => ({ data: [], pagination: {
      page: 1, page_size: 1, total: query.kind === 'component' ? 201 : query.kind === 'finished_good' ? 42 : 243
    } }));
    expect(await itemSummary({ items })).toEqual({ total: 243, components: 201, finished: 42 });
    expect(items).toHaveBeenCalledTimes(3);
    expect(items.mock.calls.every(([query]) => query?.page_size === 1)).toBe(true);
  });

  it('uses each complete status total, not just the first order page', async () => {
    const counts: Record<string, number> = { '': 145, pending: 120, in_progress: 15, completed: 10 };
    const orders = vi.fn(async (query: ListQuery = {}) => ({ data: [], pagination: {
      page: 1, page_size: 1, total: counts[String(query.status ?? '')]
    } }));
    expect(await orderSummary({ orders })).toEqual({ total: 145, pending: 120, progress: 15, completed: 10 });
  });

  it('counts inventory items across all pages without summing dissimilar units', async () => {
    const rows: Inventory[] = Array.from({ length: 125 }, (_, i) => ({
      item_id: String(i), code: `P-${i}`, name: `Item ${i}`, kind: i < 100 ? 'component' : 'finished_good',
      unit: i % 2 ? 'EA' : 'm', quantity: i < 105 ? 20 : 0, updated_at: '2026-09-11T00:00:00Z'
    }));
    const allInventory = vi.fn(async () => rows);
    expect(await inventorySummary({ allInventory })).toEqual({ total: 125, stocked: 105, empty: 20, components: 100 });
    expect(allInventory).toHaveBeenCalledOnce();
  });

  it('zero is a valid count for a genuinely empty database', async () => {
    const items = vi.fn(async () => ({ data: [], pagination: { page: 1, page_size: 1, total: 0 } }));
    expect(await itemSummary({ items })).toEqual({ total: 0, components: 0, finished: 0 });
  });

  it.each([-1, 1.5, Number.NaN, Number.MAX_SAFE_INTEGER + 1])('rejects invalid totals instead of presenting %s as real data', async total => {
    const items = vi.fn(async () => ({ data: [], pagination: { page: 1, page_size: 1, total } }));
    await expect(itemSummary({ items })).rejects.toThrow('요약 수량');
  });

  it('keeps fetch failures as errors instead of replacing them with zero', async () => {
    const items = vi.fn(async () => { throw new Error('offline'); });
    await expect(itemSummary({ items })).rejects.toThrow('offline');
  });
});
