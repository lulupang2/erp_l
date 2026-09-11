import { expect, test } from '@playwright/test';
import { item } from './helpers';

test('디자인: 테마 전환 중 폼 유지, 새로고침 후 테마 복원', async ({ page }) => {
  await page.goto('/items/new');
  await page.getByLabel(/^품목명/).fill('입력 중인 부품');
  await page.getByRole('button', { name: '다크 모드로 전환', exact: true }).click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await expect(page.getByLabel(/^품목명/)).toHaveValue('입력 중인 부품');
  await page.reload();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await expect(page.getByRole('button', { name: '라이트 모드로 전환' })).toHaveAttribute('aria-pressed', 'true');
  await page.getByRole('button', { name: '라이트 모드로 전환' }).click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
});

test('디자인: 메뉴 접기 후 탐색과 설정 복원, 모바일 메뉴 접근', async ({ page }) => {
  await page.goto('/items');
  await page.getByRole('button', { name: '메뉴 접기', exact: true }).click();
  await expect(page.locator('.app-shell')).toHaveClass(/sidebar-collapsed/);
  await page.getByRole('navigation', { name: '주 메뉴' }).getByRole('link', { name: '생산 지시', exact: true }).click();
  await expect(page.getByRole('heading', { name: '생산 지시', exact: true })).toBeVisible();
  await page.reload();
  await expect(page.getByRole('button', { name: '메뉴 펼치기', exact: true })).toHaveAttribute('aria-expanded', 'false');
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole('navigation', { name: '주 메뉴' }).getByRole('link', { name: '재고 · 입고', exact: true }).click();
  await expect(page.getByRole('heading', { name: '재고 · 입고', exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

test('디자인: 상단 품목 검색이 실제 목록 검색으로 연결', async ({ page, request }) => {
  const target = await item(request, 'component', 'SEARCH');
  await page.goto('/orders');
  await page.getByLabel('전역 품목 검색', { exact: true }).fill(target.code);
  await page.getByRole('button', { name: '품목 검색 실행', exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/items\\?q=${target.code}$`));
  await expect(page.getByLabel('코드 · 이름 검색', { exact: true })).toHaveValue(target.code);
  await expect(page.getByRole('region', { name: '품목 목록' }).getByRole('row').filter({ hasText: target.code })).toHaveCount(1);
});

test('디자인: 요약은 서버 total 사용, 오류를 0으로 표시하지 않음', async ({ page }) => {
  let fail = false;
  // Isolated presentation test. Ordinary acceptance tests continue to use the real DB.
  await page.route('**/api/v1/items?*', async route => {
    const query = new URL(route.request().url()).searchParams;
    if (query.get('page_size') !== '1') return route.continue();
    if (fail) return route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: { code: 'DB_UNAVAILABLE', message: '요약 조회 실패' } }) });
    const total = query.get('kind') === 'component' ? 201 : query.get('kind') === 'finished_good' ? 42 : 243;
    return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ data: [], pagination: { page: 1, page_size: 1, total } }) });
  });
  await page.goto('/items');
  await expect(page.getByTestId('item-summary-total')).toHaveText('243');
  await expect(page.getByTestId('item-summary-components')).toHaveText('201');
  await expect(page.getByTestId('item-summary-finished')).toHaveText('42');
  fail = true;
  await page.reload();
  await expect(page.getByTestId('item-summary-total')).toHaveText('—');
  await expect(page.getByText('품목 요약을 불러오지 못했습니다.', { exact: false })).toBeVisible();
  fail = false;
  await page.getByRole('button', { name: '요약 다시 조회', exact: true }).click();
  await expect(page.getByTestId('item-summary-total')).toHaveText('243');
});

test('디자인: 주요 화면과 다크 모드, 모바일 레이아웃 기록', async ({ page }) => {
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.setViewportSize({ width: 1440, height: 1000 });
  for (const [path, heading, metric] of [
    ['/items', '품목 관리', 'item-summary-total'],
    ['/inventory', '재고 · 입고', 'stock-summary-total'],
    ['/orders', '생산 지시', 'order-summary-total']
  ]) {
    await page.goto(path);
    await expect(page.getByRole('heading', { name: heading, exact: true })).toBeVisible();
    await expect(page.getByTestId(metric)).toHaveText(/[0-9]/);
    await expect(page.locator('.spinner')).toHaveCount(0);
    await page.screenshot({ path: `.local/design-${path.slice(1)}-light.png`, fullPage: false });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  }
  await page.getByRole('button', { name: '다크 모드로 전환' }).click();
  await page.screenshot({ path: '.local/design-orders-dark.png', fullPage: false });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: '.local/design-mobile-dark.png', fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  expect(errors).toEqual([]);
});
