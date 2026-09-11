import { defineConfig, devices } from '@playwright/test';

if (process.env.ERP_NEON_VERIFY !== 'explicit-demo-verification') {
  throw new Error('Use pnpm neon:verify for the explicitly selected Neon demo. Ordinary tests remain local.');
}

export default defineConfig({
  testDir: './tests/browser', testMatch: 'ui.spec.ts',
  fullyParallel: false, workers: 1, retries: 0, timeout: 180_000,
  expect: { timeout: 25_000 },
  reporter: [['line'], ['json', { outputFile: '.local/neon-browser-results.json' }]],
  outputDir: 'test-results/neon',
  use: { baseURL: 'http://127.0.0.1:5175', trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  projects: [{ name: 'neon-chromium', use: { ...devices['Desktop Chrome'] } }],
});
