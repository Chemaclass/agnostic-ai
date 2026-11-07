import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { selectTab } from '../helpers'

const fixture = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

test.describe('overview', () => {
  test('uses named tool logos in every support table and configured target list', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    for (const view of ['context', 'authority', 'tools', 'lifecycle', 'settings']) {
      await page.goto(`/?view=${view}`)
      await page.locator('details.support-block > summary').click()
      const tools = page.locator('.support-table .tool-avatar')
      await expect(tools.first()).toBeVisible()
      for (const avatar of await tools.all()) {
        await expect(avatar).toHaveAttribute('title', /Claude Code|Codex|Cursor/)
        await expect(avatar.locator('img')).toBeVisible()
        await expect(avatar.locator('xpath=following-sibling::*[1]')).toHaveText(await avatar.getAttribute('title') ?? '')
      }
    }
    await selectTab(page, 'Config files')
    const targets = page.locator('.value-row', { hasText: 'targets' }).locator('.tool-avatar')
    await expect(targets).toHaveCount(fixture.targets.length)
    await expect(targets.first()).toHaveAttribute('title', 'Claude Code')
  })

  test('folds coding tool support closed below the content with a summary, and remembers opening per page', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    for (const view of ['context', 'authority', 'tools', 'lifecycle', 'settings']) {
      await page.goto(`/?view=${view}`)
      const support = page.locator('details.support-block')
      await expect(support).not.toHaveAttribute('open', '')
      await expect(support.getByRole('table', { name: 'Coding tool support' })).toBeHidden()
      await expect(support.locator('.support-line')).toHaveText(/^3 of 3 tools support /)
      const order = await support.evaluate((element) => {
        const follows = (a: Element | null, b: Element | null) => Boolean(a && b && a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING)
        const panels = document.querySelectorAll('.section-panel')
        return { afterPanels: follows(panels[panels.length - 1], element), afterSearch: follows(document.querySelector('.section-search'), element), outsidePanel: element.closest('[role="tabpanel"]') === null }
      })
      expect(order).toEqual({ afterPanels: true, afterSearch: true, outsidePanel: true })
    }
    await expect(page.locator('details.support-block .support-line')).toHaveText('3 of 3 tools support preferences')
    await page.goto('/?view=tools')
    await page.locator('details.support-block > summary').click()
    await expect(page.getByRole('table', { name: 'Coding tool support' })).toBeVisible()
    await expect.poll(() => page.evaluate(() => localStorage.getItem('agnostic-ai.support-open:tools'))).toBe('1')
    await page.reload()
    await expect(page.locator('details.support-block')).toHaveAttribute('open', '')
    await page.goto('/?view=context')
    await expect(page.locator('details.support-block')).not.toHaveAttribute('open', '')
    await page.goto('/?view=tools')
    await page.locator('details.support-block > summary').click()
    await expect.poll(() => page.evaluate(() => localStorage.getItem('agnostic-ai.support-open:tools'))).toBe('0')
    await page.reload()
    await expect(page.locator('details.support-block')).not.toHaveAttribute('open', '')
  })

  test('keeps the tabs in view on a phone with support folded', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/?view=context')
    await expect(page.locator('details.support-block')).not.toHaveAttribute('open', '')
    await expect(page.getByRole('tablist', { name: 'Context views' })).toBeInViewport()
  })

  test('stacks mobile tool support groups without losing table labels', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/?view=tools')
    await page.locator('details.support-block > summary').click()
    const table = page.getByRole('table', { name: 'Coding tool support' })
    await expect(table).toBeVisible()
    await expect(table.getByRole('columnheader', { name: 'Supported on', exact: true })).toHaveCount(1)
    const status = table.locator('tbody td').first()
    await expect(status.locator('.support-status-label')).toBeVisible()
    expect((await status.boundingBox())?.width).toBeGreaterThan(300)
    expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  })
})
