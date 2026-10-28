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

test('starts every section with its page header', async ({ page }) => {
  for (const view of ['context', 'authority', 'tools', 'lifecycle', 'settings']) {
    await page.goto(`/?view=${view}&mock=1`)
    await expect(page.locator('.page > :first-child')).toHaveClass('page-header')
  }
})

for (const width of [320, 900]) {
  test(`keeps the current section in view in the narrow navigation at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.goto('/?view=lifecycle')
    await expect(page.getByRole('heading', { level: 1, name: 'Lifecycle', exact: true })).toBeInViewport()
    expect(await page.locator('.section-nav').evaluate((nav) => {
      const current = nav.querySelector('[aria-current="page"]')!.getBoundingClientRect()
      const bounds = nav.getBoundingClientRect()
      return current.left >= bounds.left && current.right <= bounds.right
    })).toBe(true)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })
}
