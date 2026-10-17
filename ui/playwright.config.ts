import { defineConfig, devices } from '@playwright/test'

// E2E_BASE_URL points the tests at a running client: by default the Go
// server (`agnostic-ai serve`), which hosts the built files and /api.
// E2E_MOCK=1 appends ?mock=1 so the client reads the bundled fixtures.
// E2E_PREVIEW=1 builds the client and starts `vite preview` on 4173.
const preview = process.env.E2E_PREVIEW === '1'
const baseURL = process.env.E2E_BASE_URL ?? (preview ? 'http://127.0.0.1:4173' : 'http://127.0.0.1:4310')
// The preinstalled Chromium matches @playwright/test 1.56.1. Set
// E2E_CHROMIUM_PATH only if the default launch cannot find it, for
// example /opt/pw-browsers/chromium-1194/chrome-linux/chrome.
const executablePath = process.env.E2E_CHROMIUM_PATH

export default defineConfig({
  testDir: './e2e',
  timeout: 45_000,
  expect: { timeout: 8_000 },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [['list']],
  outputDir: 'test-results',
  use: {
    baseURL,
    viewport: { width: 1280, height: 900 },
    trace: 'retain-on-failure',
    launchOptions: executablePath ? { executablePath } : {},
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'], viewport: { width: 1280, height: 900 } } }],
  webServer: preview
    ? {
        command: 'npm run build && npm run preview -- --host 127.0.0.1 --port 4173 --strictPort',
        url: 'http://127.0.0.1:4173',
        reuseExistingServer: true,
        timeout: 120_000,
      }
    : undefined,
})
