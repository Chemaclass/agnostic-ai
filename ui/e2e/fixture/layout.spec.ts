import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { openMenu } from '../helpers'

const fixture = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

// Each width sits in its own CSS breakpoint band. The theme changes colours, not layout, so dark runs once.
const usableLayouts = [
  { width: 320, colorSchemes: ['light', 'dark'] },
  { width: 900, colorSchemes: ['light'] },
  { width: 1024, colorSchemes: ['light'] },
  { width: 1440, colorSchemes: ['light'] },
] as const

for (const { width, colorSchemes } of usableLayouts) {
  test(`keeps all five pages usable at ${width}px in ${colorSchemes.length > 1 ? 'both themes' : 'the light theme'}`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    for (const colorScheme of colorSchemes) {
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

for (const width of [320, 390, 1440]) {
  test(`puts coding tool support below the tab panels at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.goto('/?view=context&tab=rules')
    await page.locator('details.support-block > summary').click()
    const support = page.getByRole('table', { name: 'Coding tool support' })
    await expect(support).toBeVisible()
    const panel = (await page.getByRole('tabpanel', { name: /^Rules/ }).boundingBox())!
    const table = (await support.boundingBox())!
    expect(table.y).toBeGreaterThanOrEqual(panel.y + panel.height)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })
}

test('names the serving release at the foot of the sidebar', async ({ page }) => {
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/tmp/agnostic-ai', global: false, ui: true, version: '0.91.0' } }))
  await page.goto('/?view=context')
  const badge = page.getByRole('link', { name: 'agnostic-ai v0.91.0 Release notes (opens in a new tab)' })
  await expect(badge).toHaveAttribute('href', 'https://github.com/Chemaclass/agnostic-ai/releases/tag/v0.91.0')
  await expect(badge).toBeInViewport({ ratio: 1 })
  await page.setViewportSize({ width: 900, height: 900 })
  await expect(badge).toBeHidden()
})

test('names the release the config requires beside the serving release', async ({ page }) => {
  let health: Record<string, unknown> = { ok: true, root: '/tmp/agnostic-ai', global: false, ui: true, version: '0.80.0', requires: '>=0.81.0', requiresMet: false }
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.route('**/api/health', (route) => route.fulfill({ json: health }))
  await page.goto('/?view=context')
  const badge = page.locator('.release-badge')
  const requires = badge.locator('.release-requires')
  await expect(badge.locator('.release-version')).toHaveText('agnostic-ai v0.80.0')
  await expect(requires).toHaveText('requires >=0.81.0, not met by this version')
  await expect(requires).toHaveAttribute('title', 'This project needs agnostic-ai >=0.81.0, and v0.80.0 is outside it. Run agnostic-ai sync to see the fix.')
  const muted = await badge.evaluate((element) => getComputedStyle(element).color)
  expect(await requires.evaluate((element) => getComputedStyle(element).color)).not.toBe(muted)
  expect(Math.abs((await requires.boundingBox())!.y - (await badge.locator('.release-version').boundingBox())!.y)).toBeLessThan(2)

  health = { ...health, version: '0.81.0', requiresMet: true }
  await page.reload()
  await expect(requires).toHaveText('requires >=0.81.0')
  expect(await requires.evaluate((element) => getComputedStyle(element).color)).toBe(muted)

  health = { ok: true, root: '/tmp/agnostic-ai', global: false, ui: true, version: '0.81.0' }
  await page.reload()
  await expect(badge.locator('.release-version')).toHaveText('agnostic-ai v0.81.0')
  await expect(requires).toHaveCount(0)
  await page.setViewportSize({ width: 900, height: 900 })
  await expect(badge).toBeHidden()
})

test('reads the required release again when the overview refreshes', async ({ page }) => {
  let health: Record<string, unknown> = { ok: true, root: '/tmp/agnostic-ai', global: false, ui: true, version: '0.81.0', requires: '>=0.81.0', requiresMet: true }
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.route('**/api/health', (route) => route.fulfill({ json: health }))
  await page.goto('/?view=authority')
  const requires = page.locator('.release-badge .release-requires')
  await expect(requires).toHaveText('requires >=0.81.0')
  health = { ...health, requires: '>=0.82.0', requiresMet: false }
  await page.locator('.finding', { hasText: 'LINT038' }).getByRole('button', { name: 'Recheck' }).click()
  await expect(requires).toHaveText('requires >=0.82.0, not met by this version')
})

test('keeps every rail row in place while the overview and server details load', async ({ page }) => {
  const data = { ...structuredClone(fixture), repository: { url: 'https://github.com/octo/demo-project', name: 'octo/demo-project' } }
  let release!: () => void
  const held = new Promise<void>((resolve) => { release = resolve })
  await page.route('**/api/views/overview', async (route) => { await held; await route.fulfill({ json: data }) })
  await page.route('**/api/health', async (route) => { await held; await route.fulfill({ json: { ok: true, root: '/tmp/demo-project', global: false, ui: true, version: '0.91.0', requires: '>=0.91.0', requiresMet: true } }) })
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.goto('/?view=health')
  const rows = page.locator('.rail .brand, .sidenav .nav-list > li, .sidenav-footer, .sidenav-footer > *')
  const boxes = () => rows.evaluateAll((elements) => elements.map((element) => {
    const { x, y, width, height } = element.getBoundingClientRect()
    return [x, y, width, height].map(Math.round).join(',')
  }))
  await expect(page.locator('.sidenav .skeleton').first()).toBeVisible()
  const loading = await boxes()
  release()
  await expect(page.locator('.release-badge .release-version')).toHaveText('agnostic-ai v0.91.0')
  await expect(page.locator('.sidenav .skeleton')).toHaveCount(0)
  await expect(page.locator('.rail .skeleton')).toHaveCount(0)
  expect(await boxes()).toEqual(loading)
})

test('keeps each rail summary on one line and its whole text in the link name', async ({ page }) => {
  const data = structuredClone(fixture)
  const context = data.sections.find((section: { id: string }) => section.id === 'context')!
  for (const kind of context.kinds) kind.count = 15
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/?view=context')
  const summaries = page.locator('.sidenav .nav-summary')
  await expect(summaries.first()).toBeVisible()
  for (const height of await summaries.evaluateAll((elements) => elements.map((element) => element.getBoundingClientRect().height))) expect(height).toBeLessThanOrEqual(16)
  const link = page.getByRole('navigation', { name: 'Configuration sections' }).getByRole('link', { name: /^Context: / })
  const summary = link.locator('.nav-summary')
  await expect(summary).toHaveAttribute('title', await summary.textContent() ?? '')
  expect(await summary.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(true)
})

test('links agnostic-ai and the project repository on GitHub', async ({ page }) => {
  const data = { ...structuredClone(fixture), repository: { url: 'https://github.com/octo/demo-project', name: 'octo/demo-project' } }
  let overview: Record<string, unknown> = data
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
  await page.goto('/?view=context')
  const product = page.locator('.sidenav').getByRole('link', { name: 'agnostic-ai on GitHub (opens in a new tab)' })
  await expect(product).toHaveAttribute('href', 'https://github.com/Chemaclass/agnostic-ai')
  await expect(product).toHaveAttribute('target', '_blank')
  const project = page.locator('.rail .brand').getByRole('link', { name: 'octo/demo-project on GitHub (opens in a new tab)' })
  await expect(project).toHaveAttribute('href', 'https://github.com/octo/demo-project')
  await expect(project).toHaveAttribute('target', '_blank')
  await expect(project).toBeInViewport({ ratio: 1 })
  await expect(product).toBeInViewport({ ratio: 1 })
  await page.setViewportSize({ width: 320, height: 900 })
  const menu = await openMenu(page)
  for (const name of ['octo/demo-project on GitHub (opens in a new tab)', 'agnostic-ai on GitHub (opens in a new tab)']) {
    await expect(menu.getByRole('link', { name })).toBeInViewport({ ratio: 1 })
  }
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.keyboard.press('Escape')
  await page.setViewportSize({ width: 1280, height: 900 })

  overview = fixture
  await page.reload()
  await expect(page.getByRole('heading', { level: 1, name: 'Context', exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: /^octo\/demo-project on GitHub/ })).toHaveCount(0)
  await expect(product).toBeVisible()
})

test('shows a checkout build without a release link', async ({ page }) => {
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/tmp/agnostic-ai', global: false, ui: true, version: 'main@b434c92e1a2b' } }))
  await page.goto('/?view=context')
  await expect(page.locator('.release-badge')).toContainText('main@b434c92e1a2b Development build')
  await expect(page.locator('.sidenav').getByRole('link', { name: /Release notes/ })).toHaveCount(0)
})

test('shows no version without a server that names one', async ({ page }) => {
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/tmp/agnostic-ai', global: false, ui: true } }))
  await page.goto('/?view=context')
  await expect(page.getByRole('heading', { level: 1, name: 'Context', exact: true })).toBeVisible()
  await expect(page.locator('.release-badge')).toHaveCount(0)
})

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
  for (const name of ['Search', 'Menu']) {
    await expect(page.locator('.topbar').getByRole('button', { name, exact: true })).toBeInViewport({ ratio: 1 })
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  const menu = await openMenu(page)
  await expect(menu.getByRole('button', { name: /^Open project / })).toBeInViewport({ ratio: 1 })
  await expect(menu.getByRole('button', { name: /^Switch to/ })).toBeInViewport({ ratio: 1 })
  await expect(menu.getByRole('button', { name: 'Overview JSON', exact: true })).toBeInViewport({ ratio: 1 })
  expect(await menu.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true)
})

test('switches a source tab without losing search or keyboard focus', async ({ page }) => {
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.goto('/?view=context&q=reviewer')
  const tab = page.getByRole('tab', { name: /^Agents/ })
  await tab.focus()
  await page.keyboard.press('Enter')
  const heading = page.getByRole('heading', { level: 2, name: /^Agents/ })
  await expect(tab).toBeFocused()
  await expect(tab).toHaveAttribute('aria-selected', 'true')
  await expect(heading).toBeInViewport()
  expect(await heading.evaluate((element) => element.getBoundingClientRect().top)).toBeGreaterThanOrEqual(await page.locator('.topbar').evaluate((element) => element.getBoundingClientRect().bottom))
  await expect(page).toHaveURL(/q=reviewer/)
})

test('starts every section with its page header', async ({ page }) => {
  for (const view of ['context', 'authority', 'tools', 'lifecycle', 'settings']) {
    await page.goto(`/?view=${view}&mock=1`)
    await expect(page.locator('.page > :first-child')).toHaveClass(/\bbanner-info\b/)
    await expect(page.locator('.page > :first-child + .page-header')).toHaveCount(1)
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
  test(`shows the search result count beside the search field only while filtering at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.goto('/?view=tools')
    const search = page.getByRole('searchbox', { name: 'Search tools' })
    await expect(page.locator('.section-search-status')).toBeHidden()
    await search.fill('notion')
    const input = (await search.boundingBox())!
    const status = (await page.locator('.section-search-status').boundingBox())!
    expect(status.x >= input.x + input.width || status.y >= input.y + input.height).toBe(true)
    expect(status.x + status.width).toBeLessThanOrEqual(width)
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

test('keeps the rail slate and its buttons unboxed in the light theme', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' })
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.goto('/?view=context')
  expect(await page.locator('.rail').evaluate((element) => getComputedStyle(element).backgroundColor)).toBe('rgb(34, 40, 42)')
  const search = page.locator('.sidenav').getByRole('button', { name: 'Search', exact: true })
  expect(await search.evaluate((element) => getComputedStyle(element).backgroundColor)).toBe('rgba(0, 0, 0, 0)')
})

test('keeps the phone top bar to Search and Menu, with the overview JSON in the rail', async ({ page }) => {
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.goto('/?view=context')
  const actions = page.locator('.topbar-actions').getByRole('button')
  await expect(actions).toHaveCount(0)
  const json = page.locator('.sidenav-footer').getByRole('button', { name: 'Overview JSON', exact: true })
  await json.click()
  await expect(json).toHaveAttribute('aria-pressed', 'true')
  const drawer = page.getByRole('dialog', { name: 'JSON inspector' })
  await expect(drawer.locator('.inspector-meta')).toContainText('agnostic-ai view --json')
  await page.keyboard.press('Escape')
  await expect(drawer).toHaveCount(0)
  await expect(json).toBeFocused()
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(actions).toHaveCount(2)
  await expect(actions.nth(0)).toHaveAccessibleName('Search')
  await expect(actions.nth(1)).toHaveAccessibleName('Menu')
  await expect(page.locator('.topbar').getByRole('button', { name: /JSON/ })).toHaveCount(0)
  const menu = await openMenu(page)
  const sheetJson = menu.getByRole('button', { name: 'Overview JSON', exact: true })
  await expect(sheetJson).toBeInViewport({ ratio: 1 })
  expect((await sheetJson.boundingBox())!.height).toBeGreaterThanOrEqual(44)
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
    // Wide screens stick the whole rail; phones stick the page-link row.
    const sidenav = page.locator(width > 900 ? '.rail' : '.sidenav')
    const views = ['Context', 'Authority', 'Tools', 'Lifecycle', 'Settings']
    for (const [index, name] of views.entries()) {
      await page.goto(`/?view=${name.toLowerCase()}${name === 'Context' ? '&tab=agents' : ''}`)
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

  const inspectorButton = page.getByRole('button', { name: 'Overview JSON', exact: true })
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

test('lands a linked row below the sticky phone link row', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.goto('/?view=tools')
  await expect(page.getByRole('heading', { level: 1, name: 'Tools', exact: true })).toBeVisible()
  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+K' : 'Control+K')
  const dialog = page.getByRole('dialog', { name: 'Search all configuration' })
  await dialog.getByRole('combobox', { name: 'Search all configuration' }).fill('railway')
  await dialog.getByRole('option').first().click()
  const linked = page.locator('.is-linked').first()
  await expect(linked).toBeFocused()
  await expect.poll(async () => {
    const row = (await page.locator('.sidenav').boundingBox())!
    return (await linked.boundingBox())!.y >= row.y + row.height
  }).toBe(true)
})

test('closes the phone Menu before search opens, and follows a result', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.goto('/?view=context')
  const menu = await openMenu(page)
  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+K' : 'Control+K')
  await expect(menu).toBeHidden()
  const dialog = page.getByRole('dialog', { name: 'Search all configuration' })
  const input = dialog.getByRole('combobox', { name: 'Search all configuration' })
  await expect(input).toBeFocused()
  await page.keyboard.type('github')
  await expect(dialog.getByRole('option').first()).toContainText('github')
  await page.keyboard.press('Enter')
  await expect(dialog).toHaveCount(0)
  await expect(page).toHaveURL(/view=tools&entry=/)
  await expect(page.locator('.item-row.is-linked')).toBeFocused()
})

test('scrolls only the Menu while it is open', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 600 })
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.goto('/?view=settings')
  await openMenu(page)
  expect(await page.evaluate(() => getComputedStyle(document.documentElement).overflow)).toBe('hidden')
  await page.keyboard.press('Escape')
  expect(await page.evaluate(() => getComputedStyle(document.documentElement).overflow)).not.toBe('hidden')
})

test('keeps Overview JSON on one line and the release arrow beside its text', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/tmp/agnostic-ai', global: false, ui: true, version: '0.80.0', notesUrl: 'https://example.com/notes' } }))
  await page.goto('/?view=context')
  const json = page.locator('.sidenav-footer .sidenav-json')
  const lineHeight = await json.evaluate((element) => parseFloat(getComputedStyle(element).lineHeight) || parseFloat(getComputedStyle(element).fontSize) * 1.6)
  expect((await json.boundingBox())!.height).toBeLessThan(lineHeight * 2)
  const badge = page.locator('.sidenav-footer .release-badge')
  await expect(badge.locator('.release-arrow')).toBeVisible()
  const gap = await badge.evaluate((element) => {
    const range = document.createRange()
    range.selectNodeContents(element.querySelector('.release-caption')!.firstChild!)
    const arrow = element.querySelector('.release-arrow')!.getBoundingClientRect()
    return arrow.left - range.getBoundingClientRect().right
  })
  expect(gap).toBeGreaterThanOrEqual(0)
  expect(gap).toBeLessThanOrEqual(8)
})

test('keeps focus inside the JSON inspector sheet on a phone and clear of the top bar at 761px', async ({ page }) => {
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  for (const width of [900, 800, 761]) {
    await page.setViewportSize({ width, height: 844 })
    await page.goto('/?view=settings')
    await page.getByRole('button', { name: 'Menu' }).click()
    await page.getByRole('dialog', { name: 'Menu' }).getByRole('button', { name: 'Overview JSON', exact: true }).click()
    const box = (await page.getByRole('dialog', { name: 'JSON inspector' }).boundingBox())!
    expect(box.x).toBe(0)
    expect(box.y).toBe(0)
  }
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/?view=settings')
  await page.getByRole('button', { name: 'Menu' }).click()
  await page.getByRole('dialog', { name: 'Menu' }).getByRole('button', { name: 'Overview JSON', exact: true }).click()
  const drawer = page.getByRole('dialog', { name: 'JSON inspector' })
  const close = drawer.getByRole('button', { name: /Close/ })
  await expect(close).toBeFocused()
  await page.keyboard.press('Shift+Tab')
  await expect(drawer.locator('.json')).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(close).toBeFocused()
})

test('keeps Tab inside search opened over the JSON inspector, and modal only as a sheet', async ({ page }) => {
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.setViewportSize({ width: 1440, height: 844 })
  await page.goto('/?view=settings')
  await page.getByRole('button', { name: 'Overview JSON', exact: true }).click()
  const drawer = page.getByRole('dialog', { name: 'JSON inspector' })
  await expect(drawer).toHaveAttribute('aria-modal', 'false')
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(drawer).toHaveAttribute('aria-modal', 'true')
  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+K' : 'Control+K')
  const search = page.getByRole('dialog', { name: 'Search all configuration' })
  await expect(search.getByRole('combobox')).toBeFocused()
  for (let i = 0; i < 4; i++) {
    await page.keyboard.press('Tab')
    await expect(search.locator(':focus')).toHaveCount(1)
  }
})
