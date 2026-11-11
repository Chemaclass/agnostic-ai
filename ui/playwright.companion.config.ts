import { defineConfig, devices } from '@playwright/test'

// The companion's pages in Chromium, served by companion/dev/preview.mjs with
// fixture data. The app draws them in WKWebView on macOS, so these tests do not
// prove native focus, window placement or the global shortcut.
const port = Number(process.env.COMPANION_PREVIEW_PORT ?? 4321)
// The fixtures' time; panel.spec.ts fixes the page clock to the same.
const now = '2026-11-07T15:30:00Z'
const baseURL = `http://127.0.0.1:${port}`
const executablePath = process.env.E2E_CHROMIUM_PATH

export default defineConfig({
  testDir: './e2e/companion',
  timeout: 30_000,
  expect: { timeout: 5_000 },
  // One preview holds one fixture state.
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [['list']],
  outputDir: 'test-results/companion',
  use: {
    baseURL,
    viewport: { width: 432, height: 624 },
    contextOptions: { reducedMotion: 'reduce' },
    trace: 'retain-on-failure',
    launchOptions: executablePath ? { executablePath } : {},
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'], viewport: { width: 432, height: 624 } } }],
  webServer: {
    command: `node ../companion/dev/preview.mjs --port ${port} --now ${now}`,
    url: `${baseURL}/dev/`,
    reuseExistingServer: false,
    timeout: 30_000,
  },
})
