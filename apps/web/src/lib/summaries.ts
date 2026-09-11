import { api, type ApiClient } from './api';

function count(value: number): number {
  if (!Number.isSafeInteger(value) || value < 0) throw new Error('요약 수량을 확인하지 못했습니다. 다시 조회해 주세요.');
  return value;
}

/** Counts come from server pagination totals, not the currently visible page. */
export async function itemSummary(client: Pick<ApiClient, 'items'> = api) {
  const [total, components, finished] = await Promise.all([
    client.items({ page_size: 1 }),
    client.items({ page_size: 1, kind: 'component' }),
    client.items({ page_size: 1, kind: 'finished_good' })
  ]);
  return { total: count(total.pagination.total), components: count(components.pagination.total), finished: count(finished.pagination.total) };
}

export async function orderSummary(client: Pick<ApiClient, 'orders'> = api) {
  const [total, pending, progress, completed] = await Promise.all([
    client.orders({ page_size: 1 }),
    client.orders({ page_size: 1, status: 'pending' }),
    client.orders({ page_size: 1, status: 'in_progress' }),
    client.orders({ page_size: 1, status: 'completed' })
  ]);
  return { total: count(total.pagination.total), pending: count(pending.pagination.total), progress: count(progress.pagination.total), completed: count(completed.pagination.total) };
}

/** Count item types, never add quantities measured in different units. */
export async function inventorySummary(client: Pick<ApiClient, 'allInventory'> = api) {
  const rows = await client.allInventory();
  for (const row of rows) count(row.quantity);
  return {
    total: rows.length,
    stocked: rows.filter(row => row.quantity > 0).length,
    empty: rows.filter(row => row.quantity === 0).length,
    components: rows.filter(row => row.kind === 'component').length
  };
}
