import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'

const fixture = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

async function overview(page: import('@playwright/test').Page, severity: 'warn' | 'error' = 'warn') {
  const data = structuredClone(fixture)
  const settings = data.sections.find((section: { id: string }) => section.id === 'settings')
  settings.findings = [{ source: 'lint', severity, code: 'LINT011', message: 'The session context exceeds its budget.' }]
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
  return settings
}

test('folds section warnings while Review and finding links reveal them', async ({ page }) => {
  await overview(page)
  await page.goto('/?view=settings&tab=preferences')
  const warnings = page.locator('.section-warnings')
  const finding = warnings.locator('.finding')
  await expect(warnings).not.toHaveAttribute('open', '')
  await expect(finding).toBeHidden()
  await expect(page.locator('.page-header .page-fact-findings')).toHaveText('1 warning')
  await page.getByRole('button', { name: 'Review 1 finding', exact: true }).click()
  await expect(finding).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Other findings', exact: true })).toBeFocused()
  const entry = await finding.getAttribute('data-entry')
  expect(entry).toBeTruthy()
  await page.goto(`/?view=settings&tab=preferences&entry=${encodeURIComponent(entry!)}`)
  await expect(finding).toBeVisible()
  await expect(finding).toBeFocused()
})

test('keeps section errors visible without opening a disclosure', async ({ page }) => {
  await overview(page, 'error')
  await page.goto('/?view=settings&tab=preferences')
  await expect(page.getByText('The session context exceeds its budget.', { exact: true })).toBeVisible()
  await expect(page.locator('.section-warnings')).toHaveCount(0)
  await expect(page.getByRole('alert')).toContainText('1 error')
  await expect(page.locator('.page-header .page-fact-findings')).toHaveText('1 error')
  expect(await page.evaluate(() => {
    const card = document.querySelector('.finding-error')
    const tabs = document.querySelector('[role="tablist"]')
    return Boolean(card && tabs && card.compareDocumentPosition(tabs) & Node.DOCUMENT_POSITION_FOLLOWING)
  })).toBe(true)
})

test('shows the effective configuration once beside its source file links', async ({ page }) => {
  const settings = await overview(page)
  const config = settings.items.find((item: { kind: string }) => item.kind === 'config')
  config.values = [{ key: 'requires', type: 'text', value: '>=0.80.0' }]
  settings.items.push({ ...config, name: 'agnostic-ai.local.yaml', label: 'agnostic-ai.local.yaml', path: 'agnostic-ai.local.yaml', openPath: '/work/project/agnostic-ai.local.yaml', layer: 'project-user' })
  await page.goto('/?view=settings&tab=configuration')
  const panel = page.getByRole('tabpanel', { name: /^Config files/ })
  await expect(panel.locator('.item-row')).toHaveCount(2)
  await expect(panel.locator('.item-row .value-row')).toHaveCount(0)
  await expect(panel.getByRole('heading', { name: 'Effective configuration', exact: true })).toBeVisible()
  await expect(panel.locator('.effective-config .value-row')).toHaveCount(1)
  await expect(panel.locator('.effective-config')).toContainText('>=0.80.0')
  await expect(panel.getByRole('button', { name: 'Copy path of agnostic-ai.local.yaml', exact: true })).toBeVisible()
})

test('fits all Settings tabs and their counts within narrow screens', async ({ page }) => {
  await overview(page)
  for (const width of [320, 390]) {
    await page.setViewportSize({ width, height: 844 })
    await page.goto('/?view=settings&tab=preferences')
    const tabs = page.getByRole('tablist', { name: 'Settings views' }).getByRole('tab')
    for (const tab of await tabs.all()) {
      const box = await tab.boundingBox()
      expect(box!.x).toBeGreaterThanOrEqual(0)
      expect(box!.x + box!.width).toBeLessThanOrEqual(width)
      if (await tab.locator('.section-tab-count').count() === 0) continue
      const count = await tab.locator('.section-tab-count').boundingBox()
      expect(count!.x + count!.width).toBeLessThanOrEqual(width)
    }
    await tabs.last().click()
    await expect(tabs.last()).toHaveAttribute('aria-selected', 'true')
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
  }
})

test('keeps warnings open and focus visible after dismissing a recheck result', async ({ page }) => {
  const settings = await overview(page)
  settings.findings.push({ source: 'lint', severity: 'warn', code: 'LINT012', message: 'Another warning still applies.' })
  await page.goto('/?view=settings&tab=preferences')
  await page.getByRole('button', { name: 'Review 2 findings', exact: true }).click()
  const finding = page.locator('.finding', { hasText: 'LINT011' })
  settings.findings.shift()
  await finding.getByRole('button', { name: 'Recheck', exact: true }).click()
  const result = page.locator('.finding-rechecked')
  await expect(result).toBeVisible()
  await result.getByRole('button', { name: 'Dismiss', exact: true }).click()
  await expect(page.locator('.section-warnings')).toHaveAttribute('open', '')
  await expect(page.getByRole('heading', { name: 'Other findings', exact: true })).toBeFocused()
  await expect(page.getByText('Another warning still applies.', { exact: true })).toBeVisible()
})

test('says once per page that a snapshot is read-only', async ({ page }) => {
  for (const view of ['health', 'authority', 'context', 'settings&tab=targets']) {
    await page.goto(`/?view=${view}&mock=1`)
    await expect(page.locator('.page > .banner')).toHaveCount(1)
    await expect(page.locator('.page > .banner-info')).toContainText('Opening files, rechecks and switches need agnostic-ai view.')
    await expect(page.getByText(/switches need agnostic-ai view/)).toHaveCount(1)
    await expect(page.getByText(/needs? a running agnostic-ai serve|only from a running agnostic-ai serve/)).toHaveCount(0)
  }
  await overview(page)
  await page.goto('/?view=settings')
  await expect(page.locator('.page > .banner-info')).toHaveCount(0)
  await expect(page.getByText(/switches need agnostic-ai view/)).toHaveCount(0)
})

test('explains a snapshot that does not load and retries it', async ({ page }) => {
  let requests = 0
  await page.route('**/nope/overview.json', (route) => {
    requests++
    return route.fulfill({ status: 404, contentType: 'text/plain', body: 'Not Found' })
  })
  await page.goto('/?view=settings&source=./nope')
  const alert = page.getByRole('alert')
  await expect(alert.locator('.banner-title')).toHaveText('Could not load the snapshot')
  await expect(alert.getByText(/nope\/overview\.json answered 404: Not Found/)).toBeHidden()
  await alert.getByText('Details', { exact: true }).click()
  await expect(alert.getByText(/nope\/overview\.json answered 404: Not Found/)).toBeVisible()
  await expect(alert.getByRole('button', { name: 'Copy details' })).toBeVisible()
  const before = requests
  await alert.getByRole('button', { name: 'Retry', exact: true }).click()
  await expect.poll(() => requests).toBeGreaterThan(before)
})

test('names a refused snapshot source once, without a retry', async ({ page }) => {
  await page.goto('/?view=settings&source=ftp://example.com/snap')
  const alert = page.getByRole('alert')
  await expect(alert).toHaveCount(1)
  await expect(alert.locator('.banner-title')).toHaveText('A snapshot source must be a relative directory or an http(s) URL.')
  await expect(alert.getByRole('button', { name: 'Retry' })).toHaveCount(0)
  await expect(page.getByText(/switches need agnostic-ai view/)).toHaveCount(0)
})
