import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  timeout: 35_000,
  expect: { timeout: 8000 },
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 2 : 3,
  reporter: 'list',
  use: { trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  webServer: [
    {
      command: 'npm run dev',
      url: 'http://127.0.0.1:5173',
      reuseExistingServer: !process.env.CI,
      env: { VITE_DATA_SOURCE: 'demo' },
    },
    {
      command: 'npm run dev -- --port 5174',
      url: 'http://127.0.0.1:5174',
      reuseExistingServer: !process.env.CI,
      env: {
        VITE_DATA_SOURCE: 'api',
        API_PROXY_TARGET: process.env.LIVE_API_URL || 'http://127.0.0.1:8080',
      },
    },
  ],
  projects: [
    {
      name: 'desktop',
      testMatch: 'demo.spec.ts',
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1440, height: 1000 },
        baseURL: 'http://127.0.0.1:5173',
      },
    },
    {
      name: 'mobile',
      testMatch: 'demo.spec.ts',
      use: { ...devices['Pixel 5'], baseURL: 'http://127.0.0.1:5173' },
    },
    {
      name: 'api',
      testMatch: 'api.spec.ts',
      use: { ...devices['Desktop Chrome'], baseURL: 'http://127.0.0.1:5174' },
    },
    {
      name: 'live',
      testMatch: 'live.spec.ts',
      use: { ...devices['Desktop Chrome'], baseURL: 'http://127.0.0.1:5174' },
    },
  ],
})
