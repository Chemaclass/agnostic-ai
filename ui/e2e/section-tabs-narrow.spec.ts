import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'

const fixture = JSON.parse(readFileSync(new URL('../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

function withContextFindings() {
  const data = structuredClone(fixture)
  const context = data.sections.find((section: { id: string }) => section.id === 'context')
  const finding = { source: 'lint', code: 'LINT001', severity: 'warn', message: 'A finding', path: '.agnostic-ai/agents/reviewer.md' }
  for (const item of context.items) item.findings = [finding, finding, finding, finding, finding, finding, finding, finding, finding, finding, finding, finding]
  return data
}

for (const width of [320, 390]) {
  test(`keeps the Context tab row inside the page at ${width}px when items have findings`, async ({ page }) => {
    await page.setViewportSize({ width, height: 844 })
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: withContextFindings() }))
    await page.goto('/?view=context')
    const tablist = page.getByRole('tablist', { name: 'Context views' })
    await expect(tablist).toBeVisible()
    await expect(tablist.locator('.section-tab-findings').first()).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    for (const tab of await tablist.getByRole('tab').all()) {
      await tab.click()
      await expect(tab).toHaveAttribute('aria-selected', 'true')
      await expect(tab).toBeInViewport({ ratio: 1 })
      for (const part of await tab.locator('.section-tab-count, .section-tab-findings').all()) await expect(part).toBeInViewport({ ratio: 1 })
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    }
  })
}
