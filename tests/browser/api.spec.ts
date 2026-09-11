import { randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import { all, api, assertLedger, balance, create, get, item, recipe, setBOM, suffix } from './helpers';

test('HTTP: PRD keyboard flow, partial production, defects, history and replay after completion', async ({ request }) => {
  const group = suffix();
  const make = (code: string, kind: string) => create(request, '/items', {
    code: ` ${group}_${code.toLowerCase()} `, name: code, kind, unit: '개',
  });
  const cases = await make('CASE', 'component');
  const switches = await make('SWITCH', 'component');
  const keyboard = await make('KEYBOARD', 'finished_good');
  expect(cases.code).toBe(`${group}_CASE`);
  expect(await balance(request, keyboard)).toBe(0);
  await create(request, '/stock-receipts', { item_id: cases.id, quantity: 10 });
  await create(request, '/stock-receipts', { item_id: switches.id, quantity: 800 });
  await setBOM(request, keyboard, [cases, { ...switches, quantity_per_unit: 80 }]);
  const order = await create(request, '/production-orders', { finished_item_id: keyboard.id, planned_quantity: 10 });
  const path = `/production-orders/${order.id}`;
  expect(await get(request, path)).toMatchObject({ good_quantity: 0, defective_quantity: 0, remaining_quantity: 10, status: 'pending' });
  await create(request, `${path}/results`, { good_quantity: 4, defective_quantity: 1, note: '1차' });
  expect(await get(request, path)).toMatchObject({ good_quantity: 4, defective_quantity: 1, remaining_quantity: 5, status: 'in_progress' });
  expect(await balance(request, cases)).toBe(5);
  expect(await balance(request, switches)).toBe(400);
  expect(await balance(request, keyboard)).toBe(4);
  const key = randomUUID();
  const finalInput = { good_quantity: 3, defective_quantity: 2, note: '2차' };
  const result = await create(request, `${path}/results`, finalInput, key);
  expect(await get(request, path)).toMatchObject({ good_quantity: 7, defective_quantity: 3, remaining_quantity: 0, status: 'completed' });
  expect(await balance(request, cases)).toBe(0);
  expect(await balance(request, switches)).toBe(0);
  expect(await balance(request, keyboard)).toBe(7);
  const replay = await api(request, 'POST', `${path}/results`, finalInput, key);
  expect(replay.status).toBe(200);
  expect(replay.body.data.id).toBe(result.id);
  const completed = await api(request, 'POST', `${path}/results`, { good_quantity: 1, defective_quantity: 0 }, randomUUID());
  expect(completed.status).toBe(409);
  const results = await all(request, `${path}/results`);
  expect(results).toHaveLength(2);
  expect(results.reduce((sum, r) => sum + r.good_quantity, 0)).toBe(7);
  expect(results.reduce((sum, r) => sum + r.defective_quantity, 0)).toBe(3);
  expect(await get(request, `/production-results/${result.id}`)).toMatchObject({ id: result.id, order_id: order.id });
  await assertLedger(request, [cases, switches, keyboard]);
  const paged = await api(request, 'GET', `/items?q=${group}&page=1&page_size=2`);
  expect(paged.body.pagination).toMatchObject({ page: 1, page_size: 2, total: 3 });
  expect(paged.body.data).toHaveLength(2);
  const inventory = await all(request, `/inventory?q=${group}`);
  expect(inventory.map(row => row.code)).toEqual(inventory.map(row => row.code).sort());
});

test('HTTP: strict inputs, required UUID keys, duplicate code and BOM/type constraints', async ({ request }) => {
  const component = await item(request, 'component', 'VALID');
  const finished = await item(request, 'finished_good', 'NOBOM');
  const duplicate = await api(request, 'POST', '/items', { code: component.code.toLowerCase(), name: '중복', kind: 'component', unit: '개' });
  expect(duplicate.status).toBe(409);
  for (const quantity of [-1, 0, 0.5, 1_000_001, '1', null]) {
    const result = await api(request, 'POST', '/stock-receipts', { item_id: component.id, quantity }, randomUUID());
    expect(result.status, `quantity=${quantity}`).toBe(400);
  }
  for (const key of [undefined, 'not-a-uuid']) {
    expect((await api(request, 'POST', '/stock-receipts', { item_id: component.id, quantity: 1 }, key)).status).toBe(400);
  }
  expect((await api(request, 'POST', '/stock-receipts', { item_id: finished.id, quantity: 1 }, randomUUID())).status).toBe(400);
  expect((await api(request, 'POST', '/production-orders', { finished_item_id: finished.id, planned_quantity: 1 }, randomUUID())).status).toBe(409);
  expect((await api(request, 'POST', '/production-orders', { finished_item_id: component.id, planned_quantity: 1 }, randomUUID())).status).toBe(400);
  const boms = [[], [{ item_id: component.id, quantity_per_unit: 0 }],
    [{ item_id: finished.id, quantity_per_unit: 1 }],
    [{ item_id: component.id, quantity_per_unit: 1 }, { item_id: component.id, quantity_per_unit: 2 }]];
  for (const components of boms) expect((await api(request, 'PUT', `/items/${finished.id}/bom`, { components })).status).toBe(400);
  await setBOM(request, finished, [component]);
  const order = await create(request, '/production-orders', { finished_item_id: finished.id, planned_quantity: 1 });
  for (const body of [{ good_quantity: 0, defective_quantity: 0 }, { good_quantity: -1, defective_quantity: 1 },
    { good_quantity: 0.5, defective_quantity: 0 }, { good_quantity: 1_000_001, defective_quantity: 0 }]) {
    expect((await api(request, 'POST', `/production-orders/${order.id}/results`, body, randomUUID())).status).toBe(400);
  }
  expect((await api(request, 'GET', '/items/not-a-uuid')).status).toBe(400);
  expect((await api(request, 'GET', `/items/${randomUUID()}`)).status).toBe(404);
  expect(await balance(request, component)).toBe(0);
});

test('HTTP: shortage and overproduction are atomic; a failed key is not committed', async ({ request }) => {
  const { component, finished } = await recipe(request, 10);
  const empty = await item(request, 'component', 'EMPTY');
  await setBOM(request, finished, [component, empty]);
  const order = await create(request, '/production-orders', { finished_item_id: finished.id, planned_quantity: 2 });
  const path = `/production-orders/${order.id}`;
  const key = randomUUID();
  const input = { good_quantity: 1, defective_quantity: 0 };
  const failure = await api(request, 'POST', `${path}/results`, input, key);
  expect(failure.status).toBe(409);
  expect(failure.body.error.details.shortages).toEqual(expect.arrayContaining([
    expect.objectContaining({ item_id: empty.id, required_quantity: 1, available_quantity: 0 }),
  ]));
  expect(await balance(request, component)).toBe(10);
  expect(await balance(request, finished)).toBe(0);
  expect(await all(request, `${path}/results`)).toHaveLength(0);
  expect(await get(request, path)).toMatchObject({ good_quantity: 0, defective_quantity: 0, status: 'pending' });
  await create(request, '/stock-receipts', { item_id: empty.id, quantity: 2 });
  await create(request, `${path}/results`, input, key);
  expect((await api(request, 'POST', `${path}/results`, { good_quantity: 2, defective_quantity: 0 }, randomUUID())).status).toBe(409);
  expect(await balance(request, component)).toBe(9);
  expect(await balance(request, empty)).toBe(1);
  expect(await all(request, `${path}/results`)).toHaveLength(1);
  await assertLedger(request, [component, empty, finished]);
});

test('HTTP: simultaneous duplicate receipts, orders and results commit once; keys bind to the route target', async ({ request }) => {
  const { component, finished } = await recipe(request);
  const once = async (path: string, body: unknown) => {
    const key = randomUUID();
    const replies = await Promise.all(Array.from({ length: 6 }, () => api(request, 'POST', path, body, key)));
    expect(replies.filter(r => r.status === 201)).toHaveLength(1);
    expect(replies.filter(r => r.status === 200)).toHaveLength(5);
    expect(new Set(replies.map(r => r.body.data.id)).size).toBe(1);
    return { entity: replies[0].body.data, key };
  };
  const receipt = await once('/stock-receipts', { item_id: component.id, quantity: 6 });
  expect(await balance(request, component)).toBe(6);
  expect((await api(request, 'POST', '/stock-receipts', { item_id: component.id, quantity: 7 }, receipt.key)).status).toBe(409);
  const order = await once('/production-orders', { finished_item_id: finished.id, planned_quantity: 1 });
  const body = { good_quantity: 1, defective_quantity: 0 };
  const result = await once(`/production-orders/${order.entity.id}/results`, body);
  expect(await all(request, `/production-orders/${order.entity.id}/results`)).toHaveLength(1);
  const other = await create(request, '/production-orders', { finished_item_id: finished.id, planned_quantity: 1 });
  expect((await api(request, 'POST', `/production-orders/${other.id}/results`, body, result.key)).status).toBe(409);
  expect(await all(request, `/production-orders/${other.id}/results`)).toHaveLength(0);
  expect(await balance(request, component)).toBe(5);
  expect(await balance(request, finished)).toBe(1);
  await assertLedger(request, [component, finished]);
});

test('HTTP: competing results cannot exceed one order or consume shared material twice', async ({ request }) => {
  const { component, finished } = await recipe(request, 2);
  const first = await create(request, '/production-orders', { finished_item_id: finished.id, planned_quantity: 1 });
  const submit = (id: string) => api(request, 'POST', `/production-orders/${id}/results`, { good_quantity: 1, defective_quantity: 0 }, randomUUID());
  const same = await Promise.all([submit(first.id), submit(first.id)]);
  expect(same.map(r => r.status).sort()).toEqual([201, 409]);
  expect(await balance(request, component)).toBe(1);
  const a = await create(request, '/production-orders', { finished_item_id: finished.id, planned_quantity: 1 });
  const b = await create(request, '/production-orders', { finished_item_id: finished.id, planned_quantity: 1 });
  const shared = await Promise.all([submit(a.id), submit(b.id)]);
  expect(shared.map(r => r.status).sort()).toEqual([201, 409]);
  expect(await balance(request, component)).toBe(0);
  expect(await balance(request, finished)).toBe(2);
  await assertLedger(request, [component, finished]);
});

test('HTTP: BOM snapshots survive replacement; all-defective results produce no finished-stock movement', async ({ request }) => {
  const { component, finished } = await recipe(request, 4, 1);
  const old = await create(request, '/production-orders', { finished_item_id: finished.id, planned_quantity: 1 });
  await setBOM(request, finished, [{ ...component, quantity_per_unit: 3 }]);
  const recent = await create(request, '/production-orders', { finished_item_id: finished.id, planned_quantity: 1 });
  expect((await get(request, `/production-orders/${old.id}`)).materials[0].quantity_per_unit).toBe(1);
  expect((await get(request, `/production-orders/${recent.id}`)).materials[0].quantity_per_unit).toBe(3);
  for (const order of [old, recent]) {
    await create(request, `/production-orders/${order.id}/results`, { good_quantity: 0, defective_quantity: 1 });
    expect(await get(request, `/production-orders/${order.id}`)).toMatchObject({ status: 'completed', good_quantity: 0, defective_quantity: 1 });
  }
  expect(await balance(request, component)).toBe(0);
  expect(await balance(request, finished)).toBe(0);
  expect(await all(request, `/stock-movements?item_id=${finished.id}`)).toHaveLength(0);
  await assertLedger(request, [component, finished]);
});
