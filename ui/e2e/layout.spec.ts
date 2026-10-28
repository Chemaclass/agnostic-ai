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

for (const width of [1440, 390]) {
  test(`scrolls every page from the sidebar and the page body at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 600 })
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.emulateMedia({ reducedMotion: 'reduce' })
    const nav = page.getByRole('navigation', { name: 'Configuration sections' })
    const sidenav = page.locator('.sidenav')
    const views = ['Context', 'Authority', 'Tools', 'Lifecycle', 'Settings']
    for (const [index, name] of views.entries()) {
      await page.goto(`/?view=${name.toLowerCase()}`)
      const heading = page.getByRole('heading', { level: 1, name, exact: true })
      await expect(heading).toBeInViewport()
      const headingTop = () => heading.evaluate((element) => element.getBoundingClientRect().top)
      const navTop = await sidenav.evaluate((element) => element.getBoundingClientRect().top)
      const start = await headingTop()

      const content = (await page.locator('.page').boundingBox())!
      await page.mouse.move(content.x + content.width / 2, 590)
      await page.mouse.wheel(0, 300)
      await expect.poll(headingTop).toBeLessThan(start - 100)
      expect(await sidenav.evaluate((element) => element.getBoundingClientRect().top)).toBeCloseTo(navTop, 0)
      await page.evaluate(() => window.scrollTo({ top: 0 }))
      await expect.poll(headingTop).toBe(start)

      const navBox = (await sidenav.boundingBox())!
      await page.mouse.move(navBox.x + navBox.width / 2, navBox.y + Math.min(navBox.height / 2, 20))
      // A sidebar taller than the window scrolls itself first, then the page.
      await page.mouse.wheel(0, 600)
      await page.mouse.wheel(0, 300)
      await expect.poll(headingTop).toBeLessThan(start - 100)
      expect(await sidenav.evaluate((element) => element.getBoundingClientRect().top)).toBeCloseTo(navTop, 0)
      const next = views[(index + 1) % views.length]!
      const link = nav.getByRole('link', { name: new RegExp(`^${next}`) })
      await link.click()
      await expect(link).toHaveAttribute('aria-current', 'page')
      await expect(page.getByRole('heading', { level: 1, name: next, exact: true })).toBeInViewport()
    }
  })
}

test('keeps the JSON inspector and search results scrolling their own content', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 600 })
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.goto('/?view=settings')
  await expect(page.getByRole('heading', { level: 1, name: 'Settings', exact: true })).toBeInViewport()
  const pageScroll = () => page.evaluate(() => window.scrollY)

  const inspectorButton = page.getByRole('button', { name: 'JSON', exact: true })
  await inspectorButton.click()
  const json = page.getByRole('dialog', { name: 'JSON inspector' }).locator('.json')
  await expect(json).toBeVisible()
  const jsonBox = (await json.boundingBox())!
  await page.mouse.move(jsonBox.x + jsonBox.width / 2, jsonBox.y + jsonBox.height / 2)
  await page.mouse.wheel(0, 400)
  await expect.poll(() => json.evaluate((element) => element.scrollTop)).toBeGreaterThan(0)
  expect(await pageScroll()).toBe(0)
  await page.keyboard.press('Escape')
  await expect(json).toHaveCount(0)
  await expect(inspectorButton).toBeFocused()

  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+K' : 'Control+K')
  const dialog = page.getByRole('dialog', { name: 'Search all configuration' })
  await expect(dialog.getByRole('combobox')).toBeFocused()
  await dialog.getByRole('combobox').fill('e')
  const results = dialog.locator('.search-results')
  await expect.poll(() => results.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(true)
  const resultsBox = (await results.boundingBox())!
  await page.mouse.move(resultsBox.x + resultsBox.width / 2, resultsBox.y + resultsBox.height / 2)
  await page.mouse.wheel(0, 400)
  await expect.poll(() => results.evaluate((element) => element.scrollTop)).toBeGreaterThan(0)
  await page.mouse.move(8, 590)
  await page.mouse.wheel(0, 400)
  await page.waitForTimeout(200)
  expect(await pageScroll()).toBe(0)
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  await expect(inspectorButton).toBeFocused()
})
