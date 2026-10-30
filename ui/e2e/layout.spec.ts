import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'

const fixture = JSON.parse(readFileSync(new URL('../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

for (const width of [320, 390, 768, 900, 1024, 1440]) {
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
        if (width <= 900) {
          for (const sectionLink of await page.getByRole('navigation', { name: 'Configuration sections' }).getByRole('link').all()) {
            await expect(sectionLink).toBeInViewport({ ratio: 1 })
          }
        }
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

test('fits long project names and configuration switches on a phone', async ({ page }) => {
  const data = structuredClone(fixture)
  data.snapshot.project.name = 'a-project-with-a-long-unbroken-name-that-needs-to-wrap'
  data.snapshot.project.root = '/work/projects/a-project-with-a-long-unbroken-name-that-needs-to-wrap'
  await page.setViewportSize({ width: 320, height: 844 })
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
  await page.route('**/api/health', (route) => route.fulfill({ json: {
    ok: true, root: data.snapshot.project.root, global: true,
    projectRoot: data.snapshot.project.root,
  } }))
  await page.goto('/?view=context')
  const switchButton = page.getByRole('button', { name: /^Open project / })
  await expect(switchButton).toBeInViewport({ ratio: 1 })
  for (const name of ['Search', 'JSON', 'Switch to']) {
    await expect(page.locator('.topbar').getByRole('button', { name: new RegExp(`^${name}`) })).toBeInViewport({ ratio: 1 })
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

test('jumps to a source group without losing search or keyboard focus', async ({ page }) => {
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.goto('/?view=context&q=reviewer')
  const jump = page.getByRole('navigation', { name: 'On this page' }).getByRole('link', { name: /^Agents/ })
  await jump.focus()
  await page.keyboard.press('Enter')
  const heading = page.getByRole('heading', { level: 2, name: /^Agents/ })
  await expect(heading).toBeFocused()
  await expect(heading).toBeInViewport()
  expect(await heading.evaluate((element) => element.getBoundingClientRect().top)).toBeGreaterThanOrEqual(await page.locator('.topbar').evaluate((element) => element.getBoundingClientRect().bottom))
  await expect(page).toHaveURL(/q=reviewer/)
})

test('starts every section with its page header', async ({ page }) => {
  for (const view of ['context', 'authority', 'tools', 'lifecycle', 'settings']) {
    await page.goto(`/?view=${view}&mock=1`)
    await expect(page.locator('.page > :first-child')).toHaveClass('page-header')
  }
})

test('moves focus to the page title after keyboard navigation without drawing a ring', async ({ page }) => {
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.goto('/?view=context')
  await page.getByRole('navigation', { name: 'Configuration sections' }).getByRole('link', { name: /^Tools/ }).focus()
  await page.keyboard.press('Enter')
  const title = page.getByRole('heading', { level: 1, name: 'Tools', exact: true })
  await expect(title).toBeFocused()
  expect(await title.evaluate((element) => getComputedStyle(element).outlineStyle)).toBe('none')
})

for (const width of [1440, 390]) {
  test(`puts the search result count under the search field at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.goto('/?view=tools')
    const input = (await page.getByRole('searchbox', { name: 'Search tools' }).boundingBox())!
    const status = (await page.locator('.section-search-status').boundingBox())!
    expect(status.y).toBeGreaterThanOrEqual(input.y + input.height)
    expect(status.x).toBeCloseTo(input.x, 0)
  })
}

test('shows a permission key once in the narrow Authority layout', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.goto('/?view=authority')
  const row = page.locator('.field-row', { hasText: 'Allowed without asking' })
  await expect(row.getByText('permissions.allow', { exact: true })).toHaveCount(2)
  const visible = await row.locator('code').evaluateAll((codes) => codes.filter((code) => code.textContent === 'permissions.allow' && (code.closest('dt') ?? code).getBoundingClientRect().width > 1).length)
  expect(visible).toBe(1)
  await page.setViewportSize({ width: 1440, height: 900 })
  await expect(row.locator('.value-row dt').first()).toBeVisible()
})

test('keeps header buttons on the header colour in the light theme', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' })
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.goto('/?view=context')
  const header = await page.locator('.topbar').evaluate((element) => getComputedStyle(element).backgroundColor)
  for (const name of ['Search', 'JSON']) {
    const button = page.locator('.topbar').getByRole('button', { name, exact: true })
    expect(await button.evaluate((element) => getComputedStyle(element).backgroundColor)).toBe(header)
  }
  await page.locator('.topbar').getByRole('button', { name: 'JSON', exact: true }).click()
  const pressed = page.locator('.topbar').getByRole('button', { name: 'JSON', exact: true })
  await expect(pressed).toHaveAttribute('aria-pressed', 'true')
  expect(await pressed.evaluate((element) => getComputedStyle(element).backgroundColor)).not.toBe(header)
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
      // The sticky offsets are measured after the first paint, so wait for
      // the layout to settle before taking the reference positions.
      await expect.poll(async () => {
        await page.evaluate(() => window.scrollTo({ top: 0 }))
        const before = await headingTop()
        await page.waitForTimeout(100)
        return before === (await headingTop()) && await page.evaluate(() => window.scrollY === 0)
      }).toBe(true)
      const navTop = await sidenav.evaluate((element) => element.getBoundingClientRect().top)
      const start = await headingTop()

      const content = (await page.locator('.page').boundingBox())!
      await page.mouse.move(content.x + content.width / 2, 590)
      await page.mouse.wheel(0, 300)
      await expect.poll(headingTop).toBeLessThan(start - 100)
      expect(await sidenav.evaluate((element) => element.getBoundingClientRect().top)).toBeCloseTo(navTop, 0)
      await page.evaluate(() => window.scrollTo({ top: 0 }))
      // Sub-pixel rounding can move the heading by a pixel on the way back.
      await expect.poll(async () => Math.abs((await headingTop()) - start) <= 2).toBe(true)

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
