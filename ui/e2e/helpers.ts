// Shared helpers for the end-to-end tests.
//
//   E2E_BASE_URL        the running client (default http://127.0.0.1:4310, the Go server)
//   E2E_MOCK=1          append ?mock=1 so the client reads its bundled fixture
//   E2E_SCREENSHOT_DIR  where screenshots go (default ui/test-results/screenshots, ignored by git)

import type { Locator, Page } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
export const MOCK = process.env.E2E_MOCK === '1'
export const SCREENSHOT_DIR = process.env.E2E_SCREENSHOT_DIR ?? resolve(here, '../test-results/screenshots')

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

export async function selectTab(page: Page, name: string): Promise<void> {
  await page.getByRole('tab', { name: new RegExp(`^${name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}(?:\\s|$)`) }).click()
}

export async function saveScreenshot(page: Page, name: string, options: { fullPage?: boolean } = {}): Promise<void> {
  mkdirSync(SCREENSHOT_DIR, { recursive: true })
  // Park the pointer in a corner so no hover state shows in the picture.
  await page.mouse.move(1, 1)
  await page.screenshot({ path: resolve(SCREENSHOT_DIR, name), animations: 'disabled', fullPage: options.fullPage === true })
}

/** Opens the phone Menu sheet and returns it. */
export async function openMenu(page: Page): Promise<Locator> {
  await page.locator('.topbar').getByRole('button', { name: 'Menu', exact: true }).click()
  const menu = page.getByRole('dialog', { name: 'Menu' })
  await menu.waitFor()
  return menu
}

/** The page header that a hidden section-name h1 labels; the verdict is what shows. */
export function pageHeader(page: Page, name: string) {
  return page.locator('.page-header').filter({ has: page.getByRole('heading', { level: 1, name, exact: true }) })
}

/** Opens the Coding tool support dialog from the facts line's Tool support link. */
export async function openToolSupport(page: Page) {
  await page.locator('.page-facts').getByRole('button', { name: 'Tool support', exact: true }).click()
  return page.getByRole('dialog', { name: 'Coding tool support' })
}

/** Opens a finding card's Details, where Open, Copy path and Recheck sit, and returns that panel. */
export async function findingDetails(finding: Locator): Promise<Locator> {
  const toggle = finding.getByRole('button', { name: 'Details', exact: true })
  if (await toggle.getAttribute('aria-expanded') !== 'true') await toggle.click()
  return finding.locator('.finding-details')
}
