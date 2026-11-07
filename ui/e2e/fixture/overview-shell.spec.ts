import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { MOCK, openApp, saveScreenshot, selectTab } from '../helpers'

const fixture = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

test.describe('overview', () => {
  test('shows the brand once and keeps other project names as plain context', async ({ page }) => {
    const data = structuredClone(fixture)
    data.snapshot.project.name = 'agnostic-ai'
    await page.route('**/snapshot/overview.json', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=context&source=./snapshot')
    await expect(page.locator('.brand-name')).toHaveText('agnostic-ai')
    await expect(page.locator('.page > .banner-info', { hasText: 'Static snapshot' })).toBeVisible()
    await expect(page.locator('.project-name')).toHaveCount(0)
    data.snapshot.project.name = 'billing-service'
    data.snapshot.project.root = '/work/clients/billing-service'
    await page.reload()
    await expect(page.locator('.project-title')).toHaveText('billing-service')
    await expect(page.locator('.project-root')).toHaveText('/work/clients/billing-service')
    await expect(page.locator('.project-name')).not.toHaveClass(/chip/)
  })

  test('shows the project name from the snapshot when health fails', async ({ page }) => {
    await page.route('**/api/health', (route) => route.fulfill({ json: {} }))
    const data = structuredClone(fixture)
    data.snapshot.project.name = 'agnostic-ai'
    data.snapshot.project.root = '/work/agnostic-ai'
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=context')
    await expect(page.locator('.project-title')).toHaveText('agnostic-ai')
    await expect(page.locator('.project-root')).toHaveText('/work/agnostic-ai')
  })

  test('shows the project name and folder when served, even when the name matches the brand', async ({ page }) => {
    const root = '/work/agnostic-ai-header'
    await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root, global: false, ui: true } }))
    const data = structuredClone(fixture)
    data.snapshot.project.name = 'agnostic-ai'
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=context')
    await expect(page.locator('.project-title')).toContainText('agnostic-ai')
    await expect(page.locator('.project-title')).not.toContainText('agnostic-ai-header')
    await expect(page.locator('.project-title .chip')).toHaveText('Project')
    await expect(page.locator('.project-root')).toHaveText(root)
    data.snapshot.project.name = ''
    await page.reload()
    await expect(page.locator('.project-title')).toContainText('agnostic-ai-header')
    await expect(page.locator('.project-root')).toHaveText(root)
  })

  test('loads a logo for every registered target', async ({ page }) => {
    const data = structuredClone(fixture)
    const capabilities = readFileSync(new URL('../../../docs/site/data/capabilities.toml', import.meta.url), 'utf8')
    const targets = capabilities.split('[[targets]]').slice(1).map((block) => ({
      target: block.match(/id = "([^"]+)"/)?.[1],
      label: block.match(/name = "([^"]+)"/)?.[1],
      event: 'SessionStart',
    }))
    expect(targets.length).toBeGreaterThan(3)
    data.sections.find((section: { id: string }) => section.id === 'lifecycle').stages[0].targets = targets
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=lifecycle')
    const avatars = page.locator('.stage').first().locator('.tool-avatar')
    await expect(avatars).toHaveCount(targets.length)
    for (const target of targets) {
      const avatar = page.locator('.stage').first().locator(`.tool-avatar[data-target="${target.target}"]`)
      await expect(avatar).toHaveAttribute('title', `${target.label} - SessionStart`)
      const image = avatar.locator('img')
      await expect(image).toBeVisible()
      await expect.poll(() => image.evaluate((element: HTMLImageElement) => element.naturalWidth)).toBeGreaterThan(0)
    }
  })

  test('opens Context with Conversations, five sections, Setup health, and an icon theme toggle', async ({ page }) => {
    await openApp(page, '?view=context')
    await expect(page.getByRole('heading', { level: 1, name: 'Context' })).toBeVisible()
    await expect(page.locator('.page-verdict')).toHaveText('Agents follow 1 rule and can hand work to 3 agents.')
    for (const [tab, kind] of [['Guidance', 'Instructions'], ['Rules', 'Rules'], ['Agents', 'Agents'], ['Skills', 'Skills']] as const) {
      await selectTab(page, tab)
      await expect(page.getByRole('heading', { level: 2, name: new RegExp(`^${kind}`) })).toBeVisible()
    }
    await selectTab(page, 'Agents')
    const nav = page.getByRole('navigation', { name: 'Configuration sections' })
    await expect(nav.getByRole('link')).toHaveCount(7)
    await expect(nav.getByRole('link').first()).toHaveAccessibleName(/^Conversations/)
    for (const name of ['Conversations', 'Context', 'Authority', 'Tools', 'Lifecycle', 'Settings', 'Setup health']) await expect(nav.getByRole('link', { name: new RegExp(`^${name}`) })).toBeVisible()
    await expect(page.locator('.topbar .chip', { hasText: 'sync' })).toHaveCount(0)
    const theme = page.getByRole('button', { name: /^Switch to (light|dark) theme$/ })
    const before = await theme.getAttribute('aria-label')
    await theme.click()
    await expect(theme).not.toHaveAttribute('aria-label', before ?? '')
    await expect(page.locator('.topbar').getByRole('combobox')).toHaveCount(0)
    const open = page.locator('.item-row', { hasText: 'agents/' }).first().getByRole('link', { name: /^Open .+ in Cursor$/ })
    if (MOCK) await expect(open).toHaveCount(0)
    else await expect(open).toHaveAttribute('href', /^cursor:\/\/file\/.+\.md$/)
    await saveScreenshot(page, '01-context.png')
  })

  test('keeps the header and navigation in place while the content scrolls', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 500 })
    if (MOCK) await page.route('**/assets/overview.mock-*.js', async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 1000))
      await route.continue()
    })
    await openApp(page, '?view=lifecycle')
    await expect(page.locator('.footer')).toHaveCount(0)
    await expect.poll(() => page.evaluate(() => document.scrollingElement!.scrollHeight > window.innerHeight)).toBe(true)
    const navTop = await page.locator('.sidenav').evaluate((el) => el.getBoundingClientRect().top)
    await page.evaluate(() => window.scrollTo({ top: document.scrollingElement!.scrollHeight }))
    expect(await page.evaluate(() => window.scrollY)).toBeGreaterThan(0)
    expect(await page.locator('.rail').evaluate((el) => el.getBoundingClientRect().top)).toBeCloseTo(0, 0)
    expect(await page.locator('.sidenav').evaluate((el) => el.getBoundingClientRect().top)).toBeCloseTo(navTop, 0)
  })

  test('follows the system colour scheme until the person picks a theme', async ({ page }) => {
    await page.emulateMedia({ colorScheme: 'light' })
    await openApp(page)
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
    await page.emulateMedia({ colorScheme: 'dark' })
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    await page.getByRole('button', { name: 'Switch to light theme' }).click()
    await page.emulateMedia({ colorScheme: 'dark' })
    await page.reload()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  })

  test('shows an error panel, not stale data, when the first overview load fails', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ status: 500, json: { error: 'audit-simulated-overview-failure' } }))
    await page.goto('/?view=settings')
    await expect(page.getByRole('alert')).toContainText('audit-simulated-overview-failure')
    await expect(page.getByRole('alert')).not.toContainText('previous overview')
  })

  test('opens the JSON inspector on the overview document', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await openApp(page)
    const button = page.getByRole('button', { name: 'Overview JSON', exact: true })
    await button.click()
    const drawer = page.getByRole('dialog', { name: 'JSON inspector' })
    await expect(drawer.locator('.inspector-meta')).toContainText('overview/v1')
    await expect(button).toBeInViewport({ ratio: 1 })
    await expect(button).toHaveAttribute('aria-pressed', 'true')
    const railRight = await page.locator('.rail').evaluate((element) => element.getBoundingClientRect().right)
    expect(await drawer.evaluate((element) => element.getBoundingClientRect().left)).toBeGreaterThanOrEqual(railRight - 1)
    await drawer.getByRole('button', { name: 'Copy JSON' }).click()
    await expect(drawer.getByRole('button', { name: 'JSON copied' })).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(drawer).toHaveCount(0)
    await expect(button).toBeFocused()
  })
})
