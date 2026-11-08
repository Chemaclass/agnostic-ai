import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { openToolSupport, selectTab } from '../helpers'

const fixture = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

test.describe('overview', () => {
  test('uses named tool logos in every support table and configured target list', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    for (const view of ['context', 'authority', 'tools', 'lifecycle', 'settings']) {
      await page.goto(`/?view=${view}`)
      await openToolSupport(page)
      const tools = page.locator('.support-table .tool-avatar')
      await expect(tools.first()).toBeVisible()
      for (const avatar of await tools.all()) {
        await expect(avatar).toHaveAttribute('title', /Claude Code|Codex|Cursor/)
        await expect(avatar.locator('img')).toBeVisible()
        await expect(avatar.locator('xpath=following-sibling::*[1]')).toHaveText(await avatar.getAttribute('title') ?? '')
      }
      await page.keyboard.press('Escape')
    }
    await selectTab(page, 'Config files')
    const targets = page.locator('.value-row', { hasText: 'targets' }).locator('.tool-avatar')
    await expect(targets).toHaveCount(fixture.targets.length)
    await expect(targets.first()).toHaveAttribute('title', 'Claude Code')
  })

  test('opens coding tool support from the facts line, with a summary, and closes back to the link', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    for (const view of ['context', 'authority', 'tools', 'lifecycle', 'settings']) {
      await page.goto(`/?view=${view}`)
      const link = page.locator('.page-facts').getByRole('button', { name: 'Tool support', exact: true })
      await expect(link).toHaveAttribute('aria-haspopup', 'dialog')
      await expect(page.getByRole('table', { name: 'Coding tool support' })).toBeHidden()
      const dialog = await openToolSupport(page)
      await expect(dialog).toBeVisible()
      await expect(dialog.locator('.support-line')).toHaveText(/^3 of 3 tools support /)
      await expect(dialog.getByRole('table', { name: 'Coding tool support' })).toBeVisible()
      await page.keyboard.press('Escape')
      await expect(dialog).toBeHidden()
      await expect(link).toBeFocused()
    }
    const dialog = await openToolSupport(page)
    await expect(dialog.locator('.support-line')).toHaveText('3 of 3 tools support preferences')
    await dialog.getByRole('button', { name: 'Close' }).click()
    await expect(dialog).toBeHidden()
    await expect(page.locator('details.support-block')).toHaveCount(0)
  })

  test('keeps the tabs in view on a phone', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/?view=context')
    await expect(page.getByRole('tablist', { name: 'Context views' })).toBeInViewport()
  })

  test('stacks mobile tool support groups without losing table labels', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/?view=tools')
    const table = (await openToolSupport(page)).getByRole('table', { name: 'Coding tool support' })
    await expect(table).toBeVisible()
    await expect(table.getByRole('columnheader', { name: 'Supported on', exact: true })).toHaveCount(1)
    const status = table.locator('tbody td').first()
    await expect(status.locator('.support-status-label')).toBeVisible()
    expect((await status.boundingBox())?.width).toBeGreaterThan(300)
    expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  })
})
