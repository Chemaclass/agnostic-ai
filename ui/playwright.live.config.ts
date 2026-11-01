import { defineConfig, devices } from '@playwright/test'

const executablePath = process.env.E2E_CHROMIUM_PATH

export default defineConfig({
  testDir: './e2e',
  testMatch: ['**/live.spec.ts', '**/global-onboarding.spec.ts'],
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [['list']],
  outputDir: 'test-results/live',
  use: {
    viewport: { width: 1280, height: 900 },
    trace: 'retain-on-failure',
    launchOptions: executablePath ? { executablePath } : {},
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
})
