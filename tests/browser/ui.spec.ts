import { expect, test, type Page } from '@playwright/test';
import { all, assertLedger, balance, create, get, item, recipe, suffix, type Entity } from './helpers';

const evidencePrefix = process.env.ERP_EVIDENCE_PREFIX === 'neon' ? 'neon-' : '';

async function registerItem(page: Page, code: string, name: string, kind: 'component' | 'finished_good') {
  await page.goto('/items/new');
  await page.getByLabel(/^품목 코드/).fill(code);
  await page.getByLabel(/^품목명/).fill(name);
  await page.getByLabel(/^종류/).selectOption(kind);
  await page.getByLabel(/^단위/).fill('개');
  const response = page.waitForResponse(r => r.url().endsWith('/api/v1/items') && r.request().method() === 'POST');
  await page.getByRole('button', { name: '품목 등록', exact: true }).click();
  expect((await response).status()).toBe(201);
  await expect(page).toHaveURL(/\/items\/[0-9a-f-]{36}$/);
  await expect(page.getByRole('heading', { name, exact: true })).toBeVisible();
  return page.url().split('/').at(-1)!;
}

async function receive(page: Page, target: Entity, quantity: number) {
  await page.goto(`/inventory?item_id=${target.id}`);
  const form = page.getByRole('region', { name: '부품 입고', exact: true });
  await form.getByLabel(/^입고 부품/).selectOption(target.id);
  await form.getByLabel(/^입고 수량/).fill(String(quantity));
  const response = page.waitForResponse(r => r.url().endsWith('/api/v1/stock-receipts') && r.request().method() === 'POST');
  await form.getByRole('button', { name: '입고 등록', exact: true }).click();
  expect((await response).status()).toBe(201);
  await expect(form.getByRole('status').filter({ hasText: '입고가 저장되었습니다.' })).toBeVisible();
}

test('브라우저: 품목부터 부분 생산·불량 완료까지 전체 PRD 흐름과 원인 이력 조회', async ({ page, request }) => {
  const group = `${evidencePrefix ? 'NEON' : 'UI'}_${suffix()}`;
  const casesID = await registerItem(page, `${group}_CASE`, `${group} 케이스`, 'component');
  const switchesID = await registerItem(page, `${group}_SWITCH`, `${group} 스위치`, 'component');
  const keyboardID = await registerItem(page, `${group}_KEYBOARD`, `${group} 키보드`, 'finished_good');
  const cases = await get(request, `/items/${casesID}`);
  const switches = await get(request, `/items/${switchesID}`);
  const keyboard = await get(request, `/items/${keyboardID}`);
  await receive(page, cases, 10);
  await receive(page, switches, 800);

  await page.goto(`/bom?item_id=${keyboard.id}`);
  await page.getByLabel('부품 1', { exact: true }).selectOption(cases.id);
  await page.getByLabel('소요량 1', { exact: true }).fill('1');
  await page.getByRole('button', { name: '＋ 부품 추가', exact: true }).click();
  await page.getByLabel('부품 2', { exact: true }).selectOption(switches.id);
  await page.getByLabel('소요량 2', { exact: true }).fill('80');
  await page.getByRole('button', { name: 'BOM 전체 저장', exact: true }).click();
  await expect(page.getByRole('status').filter({ hasText: 'BOM이 저장되었습니다.' })).toBeVisible();
  const bom = await get(request, `/items/${keyboard.id}/bom`);
  expect(bom.components).toEqual(expect.arrayContaining([
    expect.objectContaining({ item_id: cases.id, quantity_per_unit: 1 }),
    expect.objectContaining({ item_id: switches.id, quantity_per_unit: 80 }),
  ]));

  await page.goto('/inventory');
  const stock = page.getByRole('region', { name: '현재고 목록', exact: true });
  await stock.getByLabel('코드 · 이름 검색', { exact: true }).fill(group);
  await stock.getByRole('button', { name: '조회', exact: true }).click();
  await expect(stock.getByRole('row').filter({ hasText: cases.code }).getByRole('cell').nth(2)).toHaveText('10');
  await expect(stock.getByRole('row').filter({ hasText: switches.code }).getByRole('cell').nth(2)).toHaveText('800');
  await expect(stock.getByRole('row').filter({ hasText: keyboard.code }).getByRole('cell').nth(2)).toHaveText('0');
  await expect(stock.getByLabel('페이지당 표시 수', { exact: true })).toHaveValue('20');
  await stock.getByLabel('페이지당 표시 수', { exact: true }).selectOption('50');
  await expect(stock.getByLabel('페이지당 표시 수', { exact: true })).toHaveValue('50');
  await expect(stock.getByRole('row').filter({ hasText: keyboard.code }).getByRole('cell').nth(2)).toHaveText('0');
  await stock.getByRole('link', { name: `${cases.name} 재고 이력`, exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/movements\\?item_id=${cases.id}$`));
  await expect(page.getByRole('heading', { name: '재고 이력', exact: true })).toBeVisible();
  const movements = await all(request, `/stock-movements?item_id=${cases.id}`);
  expect(movements).toHaveLength(1);
  const source = movements[0].receipt_id;
  await page.locator(`a[href="/receipts/${source}"]`).click();
  await expect(page).toHaveURL(new RegExp(`/receipts/${source}$`));
  await expect(page.getByText(cases.name, { exact: true }).first()).toBeVisible();
  await assertLedger(request, [cases, switches, keyboard]);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: `.local/${evidencePrefix}ui-receipt.png`, fullPage: true });

  await page.goto(`/orders/new?item_id=${keyboard.id}`);
  await page.getByLabel(/^계획 수량/).fill('10');
  await page.getByRole('button', { name: '생산 지시 생성', exact: true }).click();
  await expect(page).toHaveURL(/\/orders\/[0-9a-f-]{36}$/);
  const orderID = page.url().split('/').at(-1)!;
  await expect(page.getByTestId('planned-quantity')).toHaveText('10');
  const resultForm = page.getByRole('region', { name: '생산 실적 등록', exact: true });
  await resultForm.getByLabel(/^양품 수량/).fill('4');
  await resultForm.getByLabel(/^불량 수량/).fill('1');
  await resultForm.getByLabel('실적 메모', { exact: true }).fill('1차 생산');
  const consumption = resultForm.locator('[aria-label="예상 자재 소비"]');
  await expect(consumption.locator('.summary-line').filter({ hasText: cases.name }).locator('strong')).toHaveText('5 개');
  await expect(consumption.locator('.summary-line').filter({ hasText: switches.name }).locator('strong')).toHaveText('400 개');
  await resultForm.getByRole('button', { name: '실적 등록', exact: true }).click();
  await expect(page.getByTestId('good-quantity')).toHaveText('4');
  await expect(page.getByTestId('defective-quantity')).toHaveText('1');
  await expect(page.getByTestId('remaining-quantity')).toHaveText('5');
  expect(await balance(request, cases)).toBe(5);
  expect(await balance(request, switches)).toBe(400);
  expect(await balance(request, keyboard)).toBe(4);

  await resultForm.getByLabel(/^양품 수량/).fill('3');
  await resultForm.getByLabel(/^불량 수량/).fill('2');
  await resultForm.getByLabel('실적 메모', { exact: true }).fill('2차 생산');
  await resultForm.getByRole('button', { name: '실적 등록', exact: true }).click();
  await expect(page.getByTestId('good-quantity')).toHaveText('7');
  await expect(page.getByTestId('defective-quantity')).toHaveText('3');
  await expect(page.getByTestId('remaining-quantity')).toHaveText('0');
  await expect(page.locator('.badge.completed')).toHaveText('완료');
  await expect(resultForm.getByRole('button', { name: '실적 등록', exact: true })).toBeDisabled();
  expect(await balance(request, cases)).toBe(0);
  expect(await balance(request, switches)).toBe(0);
  expect(await balance(request, keyboard)).toBe(7);
  expect(await all(request, `/production-orders/${orderID}/results`)).toHaveLength(2);
  await expect(page.getByRole('region', { name: '생산 실적 이력', exact: true }).getByRole('row')).toHaveCount(3);
  await assertLedger(request, [cases, switches, keyboard]);
  await expect(page.getByRole('region', { name: '생산 실적 이력', exact: true }).getByLabel('페이지당 표시 수')).toHaveValue('20');
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: `.local/${evidencePrefix}ui-order-completed.png`, fullPage: true });
  await page.getByRole('link', { name: '실적 상세 보기 ↗', exact: true }).click();
  await expect(page.getByRole('heading', { name: '생산 실적 상세', exact: true })).toBeVisible();
});

test('브라우저: 서버 저장 후 응답 유실과 새로고침에도 원래 입고 키로 재확인', async ({ page, request }) => {
  const component = await item(request, 'component', 'RETRY');
  let lost = false;
  const keys: string[] = [];
  const bodies: string[] = [];
  await page.route('**/api/v1/stock-receipts', async route => {
    if (route.request().method() !== 'POST') return route.continue();
    keys.push(route.request().headers()['idempotency-key']);
    bodies.push(route.request().postData() ?? '');
    const response = await route.fetch();
    if (!lost) {
      lost = true;
      expect(response.status()).toBe(201);
      // The backend committed. Simulate loss between the proxy and browser only.
      await route.abort('failed');
    } else {
      expect(response.status()).toBe(200);
      await route.fulfill({ response });
    }
  });
  await page.goto(`/inventory?item_id=${component.id}`);
  let form = page.getByRole('region', { name: '부품 입고', exact: true });
  await form.getByLabel(/^입고 부품/).selectOption(component.id);
  await form.getByLabel(/^입고 수량/).fill('7');
  await form.getByLabel('입고 메모', { exact: true }).fill('응답 유실 검증');
  await form.getByRole('button', { name: '입고 등록', exact: true }).click();
  await expect(form.getByRole('button', { name: '동일 요청 다시 확인', exact: true })).toBeEnabled();
  await expect(form.getByLabel(/^입고 수량/)).toHaveValue('7');
  await expect(form.getByLabel(/^입고 수량/)).toBeDisabled();
  expect(await balance(request, component)).toBe(7);
  await page.reload();
  form = page.getByRole('region', { name: '부품 입고', exact: true });
  await expect(form.getByLabel(/^입고 수량/)).toHaveValue('7');
  await expect(form.getByLabel('입고 메모', { exact: true })).toHaveValue('응답 유실 검증');
  await form.getByRole('button', { name: '동일 요청 다시 확인', exact: true }).click();
  await expect(form.getByRole('status').filter({ hasText: '입고가 저장되었습니다.' })).toBeVisible();
  expect(keys).toHaveLength(2);
  expect(keys[0]).toMatch(/^[0-9a-f-]{36}$/i);
  expect(keys[1]).toBe(keys[0]);
  expect(bodies[1]).toBe(bodies[0]);
  expect(await balance(request, component)).toBe(7);
  expect(await all(request, `/stock-movements?item_id=${component.id}`)).toHaveLength(1);
});

test('브라우저: 중복 코드 오류에서 입력 유지, 조회 로딩·빈 결과·일시 장애 안내', async ({ page, request }) => {
  const component = await item(request, 'component', 'DUPLICATE');
  await page.goto('/items/new');
  await page.getByLabel(/^품목 코드/).fill(component.code.toLowerCase());
  await page.getByLabel(/^품목명/).fill('보존되어야 하는 이름');
  await page.getByRole('button', { name: '품목 등록', exact: true }).click();
  await expect(page.getByRole('alert')).toBeVisible();
  await expect(page.getByLabel(/^품목명/)).toHaveValue('보존되어야 하는 이름');
  await expect(page.getByLabel(/^품목 코드/)).toHaveValue(component.code.toLowerCase());

  let release!: () => void;
  const waiting = new Promise<void>(resolve => { release = resolve; });
  await page.route('**/api/v1/items?*', async route => {
    await waiting;
    await route.continue();
  });
  await page.goto('/items');
  await expect(page.getByRole('status').filter({ hasText: '서버에서 최신 정보를 불러오고 있습니다.' })).toBeVisible();
  release();
  await expect(page.getByRole('button', { name: '조회', exact: true })).toBeEnabled();
  await page.unroute('**/api/v1/items?*');
  await page.getByLabel('코드 · 이름 검색', { exact: true }).fill(`ABSENT_${suffix()}`);
  await page.getByRole('button', { name: '조회', exact: true }).click();
  await expect(page.getByText('등록된 품목이 없습니다. 부품 또는 완제품을 등록해 주세요.', { exact: true })).toBeVisible();

  // The error display is intentionally injected; the earlier requests use the real DB.
  await page.route('**/api/v1/items?*', route => route.fulfill({ status: 503, contentType: 'application/json',
    body: JSON.stringify({ error: { code: 'DB_UNAVAILABLE', message: '일시적으로 데이터베이스에 연결할 수 없습니다.' } }) }));
  await page.getByRole('button', { name: '조회', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('일시적으로 데이터베이스에 연결할 수 없습니다.');
  await expect(page.getByLabel('코드 · 이름 검색', { exact: true })).toHaveValue(/^ABSENT_/);
  await page.unroute('**/api/v1/items?*');
  await page.getByRole('button', { name: '다시 조회', exact: true }).click();
  await expect(page.getByRole('alert')).toHaveCount(0);
});

test('브라우저: 완료 실적 응답 유실 후 새로고침해도 같은 키로 한 번만 저장', async ({ page, request }) => {
  const { component, finished } = await recipe(request, 1);
  const order = await create(request, '/production-orders', { finished_item_id: finished.id, planned_quantity: 1 });
  let lost = false; const keys: string[] = []; const bodies: string[] = [];
  await page.route('**/api/v1/production-orders/*/results', async route => {
    if (route.request().method() !== 'POST') return route.continue();
    keys.push(route.request().headers()['idempotency-key']); bodies.push(route.request().postData() ?? '');
    const response = await route.fetch();
    if (!lost) { lost = true; expect(response.status()).toBe(201); await route.abort('failed'); }
    else { expect(response.status()).toBe(200); await route.fulfill({ response }); }
  });
  await page.goto(`/orders/${order.id}`);
  let form = page.getByRole('region', { name: '생산 실적 등록', exact: true });
  await form.getByLabel(/^양품 수량/).fill('1');
  await form.getByLabel(/^불량 수량/).fill('0');
  await form.getByRole('button', { name: '실적 등록', exact: true }).click();
  await expect(form.getByRole('button', { name: '동일 요청 다시 확인', exact: true })).toBeEnabled();
  expect((await get(request, `/production-orders/${order.id}`)).status).toBe('completed');
  await page.reload();
  form = page.getByRole('region', { name: '생산 실적 등록', exact: true });
  await expect(page.locator('.badge.completed')).toHaveText('완료');
  await expect(form.getByLabel(/^양품 수량/)).toHaveValue('1');
  await form.getByRole('button', { name: '동일 요청 다시 확인', exact: true }).click();
  await expect(page.getByRole('status').filter({ hasText: '실적이 저장되었습니다.' })).toBeVisible();
  expect(keys).toHaveLength(2); expect(keys[1]).toBe(keys[0]); expect(bodies[1]).toBe(bodies[0]);
  expect(await all(request, `/production-orders/${order.id}/results`)).toHaveLength(1);
  expect(await balance(request, component)).toBe(0); expect(await balance(request, finished)).toBe(1);
  await expect(form.getByRole('button', { name: '실적 등록', exact: true })).toBeDisabled();
});

test('브라우저: 계획 초과와 자재 부족에서 입력 보존, 입고 후 서버 기준으로 성공', async ({ page, request }) => {
  const { component, finished } = await recipe(request);
  const order = await create(request, '/production-orders', { finished_item_id: finished.id, planned_quantity: 3 });
  await page.goto(`/orders/${order.id}`);
  const form = page.getByRole('region', { name: '생산 실적 등록', exact: true });
  await form.getByLabel(/^양품 수량/).fill('4');
  await form.getByLabel(/^불량 수량/).fill('0');
  await form.getByRole('button', { name: '실적 등록', exact: true }).click();
  await expect(form.getByRole('alert')).toContainText('초과');
  await expect(form.getByLabel(/^양품 수량/)).toHaveValue('4');
  await form.getByLabel(/^양품 수량/).fill('2');
  await form.getByLabel(/^불량 수량/).fill('1');
  await form.getByLabel('실적 메모', { exact: true }).fill('보존할 입력');
  await form.getByRole('button', { name: '실적 등록', exact: true }).click();
  await expect(form.getByRole('alert')).toContainText(`${component.name} · 필요 3 / 현재 0`);
  await expect(form.getByLabel(/^양품 수량/)).toHaveValue('2');
  await expect(form.getByLabel(/^불량 수량/)).toHaveValue('1');
  await expect(form.getByLabel('실적 메모', { exact: true })).toHaveValue('보존할 입력');
  expect(await all(request, `/production-orders/${order.id}/results`)).toHaveLength(0);
  await create(request, '/stock-receipts', { item_id: component.id, quantity: 3 });
  await form.getByRole('button', { name: '실적 등록', exact: true }).click();
  await expect(page.getByTestId('good-quantity')).toHaveText('2');
  await expect(page.getByTestId('defective-quantity')).toHaveText('1');
  await expect(page.getByTestId('remaining-quantity')).toHaveText('0');
  expect(await balance(request, component)).toBe(0);
  expect(await balance(request, finished)).toBe(2);
});
