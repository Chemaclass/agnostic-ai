// Shared helpers for the end-to-end tests.
//
//   E2E_BASE_URL        the running client (default http://127.0.0.1:4310, the Go server)
//   E2E_MOCK=1          append ?mock=1 so the client reads its bundled fixture
//   E2E_SCREENSHOT_DIR  where screenshots go (default docs/internal/assets/metaprogramming/authority)

import type { Page } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
export const MOCK = process.env.E2E_MOCK === '1'
export const SCREENSHOT_DIR = process.env.E2E_SCREENSHOT_DIR ?? resolve(here, '../../docs/internal/assets/metaprogramming/authority')

/** A relative app URL, so a base URL with a path prefix keeps working. */
export function appUrl(query = ''): string {
  const params = new URLSearchParams(query.replace(/^\?/, ''))
  if (MOCK) params.set('mock', '1')
  const search = params.toString()
  return search ? `./?${search}` : './'
}

export async function openApp(page: Page, query = ''): Promise<void> {
  await page.goto(appUrl(query))
}

export async function saveScreenshot(page: Page, name: string, options: { fullPage?: boolean } = {}): Promise<void> {
  mkdirSync(SCREENSHOT_DIR, { recursive: true })
  // Park the pointer in a corner so no hover state shows in the picture.
  await page.mouse.move(1, 1)
  await page.screenshot({ path: resolve(SCREENSHOT_DIR, name), animations: 'disabled', fullPage: options.fullPage === true })
}
