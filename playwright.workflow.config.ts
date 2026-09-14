import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests/browser', testMatch: 'workflow.spec.ts', workers: 1, timeout: 30000,
  reporter: 'line', outputDir: '.local/workflow-tests',
  use: { baseURL: 'http://127.0.0.1:4187', ...devices['Desktop Chrome'], screenshot: 'only-on-failure' },
  webServer: { command: `"${process.execPath}" node_modules/vite/bin/vite.js --host 127.0.0.1 --port 4187`, cwd: './apps/web', url: 'http://127.0.0.1:4187', reuseExistingServer: false, timeout: 60000 }
});
