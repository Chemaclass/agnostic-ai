import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'

const fixture = JSON.parse(readFileSync(new URL('../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

for (const width of [320, 390, 900, 1440]) {
  test(`keeps all five pages usable at ${width}px in both themes`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    for (const colorScheme of ['light', 'dark'] as const) {
      await page.emulateMedia({ colorScheme, reducedMotion: 'reduce' })
      await page.goto('/?view=context')
      for (const name of ['Context', 'Authority', 'Tools', 'Lifecycle', 'Settings']) {
        const link = page.getByRole('navigation', { name: 'Configuration sections' }).getByRole('link', { name: new RegExp(`^${name}`) })
        await link.click()
        await expect(link).toHaveAttribute('aria-current', 'page')
        await expect(page.getByRole('heading', { level: 1, name, exact: true })).toBeInViewport()
        const search = page.getByRole('searchbox', { name: `Search ${name.toLowerCase()}` })
        await expect(search).toBeInViewport()
        await search.fill('no-matching-configuration')
        await page.getByRole('button', { name: 'Clear search', exact: true }).click()
        await expect(search).toBeFocused()
        expect(await page.locator('#main').evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true)
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      }
    }
  })
}

test('starts every section with its page header and keeps generated files in the sidebar', async ({ page }) => {
  for (const view of ['context', 'authority', 'tools', 'lifecycle', 'settings']) {
    await page.goto(`/?view=${view}&mock=1`)
    await expect(page.locator('.page > :first-child')).toHaveClass('page-header')
    await expect(page.locator('.sidenav section.sync-status')).toHaveCount(1)
    await expect(page.locator('.page section.sync-status')).toHaveCount(0)
  }
})

for (const width of [320, 900]) {
  test(`opens sidebar tool states and files with the keyboard at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    let requests = 0
    await page.route('**/api/views/overview', (route) => {
      requests++
      return route.fulfill({ json: fixture })
    })
    await page.goto('/?view=lifecycle')
    const toggle = page.getByRole('button', { name: 'Configured tools', exact: true })
    const status = page.locator('.sidenav section.sync-status')
    await expect(toggle).toBeInViewport()
    expect(await page.locator('.section-nav').evaluate((nav) => {
      const current = nav.querySelector('[aria-current="page"]')!.getBoundingClientRect()
      const bounds = nav.getBoundingClientRect()
      return current.left >= bounds.left && current.right <= bounds.right
    })).toBe(true)
    await expect(toggle).toHaveAttribute('aria-expanded', 'false')
    await expect(status).not.toBeVisible()
    await toggle.focus()
    await page.keyboard.press('Enter')
    await expect(toggle).toHaveAttribute('aria-expanded', 'true')
    await expect(status.getByRole('button', { name: 'Refresh', exact: true })).toBeInViewport()
    const tool = status.locator('.sync-status-tool').filter({ hasText: 'Claude Code' })
    await expect(tool).toContainText('Needs sync')
    await tool.locator('summary').focus()
    await page.keyboard.press('Enter')
    await expect(tool.locator('.sync-status-files')).toContainText('.claude/settings.json')
    const beforeRefresh = requests
    await status.getByRole('button', { name: 'Refresh', exact: true }).click()
    await expect.poll(() => requests).toBe(beforeRefresh + 1)
    await expect(page.getByRole('heading', { level: 1, name: 'Lifecycle', exact: true })).toBeInViewport()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    expect(await page.locator('.nav-project').evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true)
    await toggle.click()
    await expect(status).not.toBeVisible()
  })
}
