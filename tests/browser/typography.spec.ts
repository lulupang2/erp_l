import { writeFileSync } from 'node:fs';
import { expect, test, type Page } from '@playwright/test';

// Inspect rendered styles, not only token declarations. This is a focused text
// contrast/readability regression check, not a full accessibility certification.
async function auditText(page: Page) {
  return page.evaluate(() => {
    type RGBA = [number, number, number, number];
    const rgba = (value: string): RGBA => {
      const channels = value.match(/[\d.]+/g)?.map(Number) ?? [0, 0, 0, 0];
      return [channels[0], channels[1], channels[2], channels[3] ?? 1];
    };
    const blend = (front: RGBA, back: RGBA): RGBA => [
      ...front.slice(0, 3).map((channel, i) => channel * front[3] + back[i] * (1 - front[3])), 1
    ] as RGBA;
    const background = (element: Element): RGBA => {
      const ancestors: Element[] = [];
      for (let node: Element | null = element; node; node = node.parentElement) ancestors.push(node);
      return ancestors.reverse().reduce((color, node) => blend(rgba(getComputedStyle(node).backgroundColor), color), [255, 255, 255, 1] as RGBA);
    };
    const luminance = (color: RGBA) => color.slice(0, 3).reduce((total, channel, i) => {
      const c = channel / 255;
      return total + (c <= .04045 ? c / 12.92 : ((c + .055) / 1.055) ** 2.4) * [.2126, .7152, .0722][i];
    }, 0);
    const samples: { selector: string; size: number; weight: string; contrast: number }[] = [];
    for (const element of document.querySelectorAll<HTMLElement>('body *')) {
      if (element.closest('svg, script, style, option, .sr-only, [aria-hidden="true"], .brand-wordmark')) continue;
      const isInput = element instanceof HTMLInputElement || element instanceof HTMLSelectElement || element instanceof HTMLTextAreaElement;
      if (!isInput && ![...element.childNodes].some(node => node.nodeType === Node.TEXT_NODE && node.textContent?.trim())) continue;
      const bounds = element.getBoundingClientRect();
      const style = getComputedStyle(element);
      if (!bounds.width || !bounds.height || style.visibility === 'hidden' || style.opacity === '0') continue;
      const bg = background(element);
      const contrast = (foreground: string) => {
        const a = luminance(blend(rgba(foreground), bg));
        const b = luminance(bg);
        return (Math.max(a, b) + .05) / (Math.min(a, b) + .05);
      };
      const selector = `${element.tagName.toLowerCase()}${element.id ? `#${element.id}` : ''}.${[...element.classList].join('.')}`;
      samples.push({ selector, size: parseFloat(style.fontSize), weight: style.fontWeight, contrast: contrast(style.color) });
      if ((element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) && element.placeholder) {
        samples.push({ selector: `${selector}::placeholder`, size: parseFloat(style.fontSize), weight: style.fontWeight, contrast: contrast(getComputedStyle(element, '::placeholder').color) });
      }
    }
    return {
      checked: samples.length,
      minimumContrast: Math.min(...samples.map(sample => sample.contrast)),
      minimumTextSize: Math.min(...samples.map(sample => sample.size)),
      failures: samples.filter(sample => sample.contrast < 4.5 || sample.size < 13 || !['400', '700'].includes(sample.weight))
    };
  });
}

for (const theme of ['light', 'dark'] as const) {
  test(`타이포그래피: ${theme} 실제 LINE Seed KR 렌더링·본문 크기·대비`, async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.goto('/items');
    if (theme === 'dark') await page.locator('.theme-toggle').click();
    const measurements = [];
    for (const path of ['/items', '/inventory', '/orders', '/items/new']) {
      await page.goto(path);
      await expect(page.locator('h1')).toBeVisible();
      await expect(page.locator('.spinner')).toHaveCount(0);
      await expect(page.locator('.metric-card[aria-busy="true"]')).toHaveCount(0);
      await page.evaluate(() => document.fonts.ready);
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
      await expect(page.locator('h1')).toHaveCSS('font-size', '30px');
      await expect(page.locator('body')).toHaveCSS('font-size', '15px');
      const fonts = await page.evaluate(() => [...document.fonts].filter(font => font.family.replaceAll('"', '') === 'LINE Seed KR').map(font => ({ weight: font.weight, status: font.status })));
      expect(fonts).toEqual(expect.arrayContaining([{ weight: '400', status: 'loaded' }, { weight: '700', status: 'loaded' }]));
      const result = await auditText(page);
      expect(result.checked).toBeGreaterThan(15);
      expect(result.failures).toEqual([]);
      measurements.push({ path, ...result });
      if (path === '/items/new') {
        await expect(page.locator('#name')).toHaveCSS('font-size', '15px');
        await expect(page.locator('label[for="name"]')).toHaveCSS('font-size', '14px');
        const backButton = page.getByRole('link', { name: '목록으로', exact: true });
        // Chromium serializes the computed value of legacy inline-flex as flex.
        await expect(backButton).toHaveCSS('display', 'flex');
        await expect(backButton).toHaveCSS('align-items', 'center');
        await expect(backButton).toHaveCSS('padding-top', '2px');
      } else {
        await expect(page.locator('th').first()).toHaveCSS('font-size', '14px');
        await expect(page.locator('.metric-value strong').first()).toHaveCSS('font-size', '32px');
      }
      if (path === '/items' || path === '/inventory' || path === '/orders') await page.screenshot({ path: `.local/typography-${path.slice(1)}-${theme}.png` });
    }
    // A font-family declaration alone could silently use a fallback. Chromium's
    // platform-font report confirms the actual Regular/Bold glyph provider.
    const cdp = await page.context().newCDPSession(page);
    await cdp.send('DOM.enable'); await cdp.send('CSS.enable');
    const document = await cdp.send('DOM.getDocument');
    for (const selector of ['h1', '.page-heading p:not(.eyebrow)']) {
      const { nodeId } = await cdp.send('DOM.querySelector', { nodeId: document.root.nodeId, selector });
      const { fonts } = await cdp.send('CSS.getPlatformFontsForNode', { nodeId });
      expect(fonts.length).toBeGreaterThan(0);
      expect(fonts.every((font: { isCustomFont: boolean; familyName: string }) => font.isCustomFont && font.familyName.startsWith('LINE Seed'))).toBe(true);
    }
    await cdp.detach();
    writeFileSync(`.local/typography-${theme}.json`, JSON.stringify({ theme, measurements }, null, 2));
  });
}

test('타이포그래피: 모바일과 200% 글자 확대에서도 폼·테마·메뉴 유지', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/items/new');
  await page.locator('#name').fill('글자 확대 중인 부품');
  await expect(page.locator('#name')).toHaveCSS('font-size', '16px');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  // Inline root size overrides :root (a bare html rule has lower specificity).
  await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
  await expect(page.locator('#name')).toHaveCSS('font-size', '32px');
  await page.locator('.theme-toggle').click();
  await expect(page.locator('#name')).toHaveValue('글자 확대 중인 부품');
  await expect(page.getByRole('button', { name: '품목 등록', exact: true })).toBeEnabled();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  const layout = await page.evaluate(() => {
    const location = document.querySelector('.breadcrumb')!.getBoundingClientRect();
    const controls = document.querySelector('.topbar-actions')!.getBoundingClientRect();
    const skip = document.querySelector('.skip-link')!.getBoundingClientRect();
    return { locationRight: location.right, controlsLeft: controls.left, hiddenSkipBottom: skip.bottom };
  });
  expect(layout.locationRight).toBeLessThanOrEqual(layout.controlsLeft);
  expect(layout.hiddenSkipBottom).toBeLessThanOrEqual(0);
  await page.screenshot({ path: '.local/typography-mobile-text-200.png', fullPage: true });
});
