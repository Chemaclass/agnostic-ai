import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { openMenu } from '../helpers'

const fixture = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))
// Reserve the width of a classic scrollbar on the narrowest viewport.
const narrowContent = '@media (max-width: 320px) { .app { max-width: 305px; } }'

test('keeps the project path under its name and the configuration switch in the rail', async ({ page }) => {
  const overview = structuredClone(fixture)
  overview.snapshot.project.name = 'agnostic-ai'
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/tmp/agnostic-ai', global: false, ui: true, version: '0.91.0' } }))
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/?view=context&tab=rules')
  const header = page.locator('.topbar')
  await expect(header.getByRole('button', { name: 'Open global settings' })).toHaveCount(0)
  await expect(header.locator('.project-root')).toHaveText('/tmp/agnostic-ai')
  const title = (await header.locator('.project-title').boundingBox())!
  const path = (await header.locator('.project-root').boundingBox())!
  expect(path.y).toBeGreaterThanOrEqual(title.y + title.height - 1)
  expect(path.x + path.width).toBeLessThanOrEqual((await page.locator('.rail').boundingBox())!.width)

  const footer = page.locator('.sidenav-footer')
  const configuration = footer.getByRole('button', { name: 'Open global settings' })
  await expect(configuration).toBeInViewport({ ratio: 1 })
  const badge = (await footer.locator('.release-badge').boundingBox())!
  expect((await configuration.boundingBox())!.y).toBeLessThan(badge.y)
})

test('keeps the configuration switch reachable from the phone Menu', async ({ page }) => {
  const overview = structuredClone(fixture)
  overview.snapshot.project.name = 'agnostic-ai'
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/tmp/agnostic-ai', global: false, ui: true } }))
  await page.setViewportSize({ width: 320, height: 844 })
  await page.goto('/?view=context&tab=rules')
  await page.addStyleTag({ content: narrowContent })
  const header = page.locator('.topbar')

  for (const width of [320, 390, 900]) {
    await page.setViewportSize({ width, height: 844 })
    expect((await header.boundingBox())!.height).toBeLessThanOrEqual(64)
    expect(await header.locator('.brand-name').evaluate((element) => {
      const range = document.createRange()
      range.selectNodeContents(element)
      return range.getClientRects().length
    })).toBe(1)
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
    const menu = await openMenu(page)
    const configuration = menu.getByRole('button', { name: 'Open global settings' })
    await expect(configuration).toBeInViewport({ ratio: 1 })
    expect((await configuration.boundingBox())!.height).toBeGreaterThanOrEqual(44)
    await page.keyboard.press('Escape')
    await expect(menu).toBeHidden()
    await expect(header.getByRole('button', { name: 'Menu', exact: true })).toBeFocused()
  }
})

test('keeps global configuration context and the project switch compact on phones', async ({ page }) => {
  const overview = structuredClone(fixture)
  const root = '/Users/chema/Code/Chemaclass/agnostic-ai-home'
  overview.snapshot.project.name = 'global (/Users/chema/.agnostic-ai)'
  overview.snapshot.project.root = root
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root, global: true, ui: true, projectRoot: '/Users/chema/Code/Chemaclass/agnostic-ai' } }))
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/?view=settings&tab=preferences')
  await page.addStyleTag({ content: narrowContent })
  const header = page.locator('.topbar')

  for (const width of [390, 320]) {
    await page.setViewportSize({ width, height: 844 })
    await expect(header.locator('.project-title .chip')).toBeInViewport({ ratio: 1 })
    await expect(header.locator('.project-title .chip')).toHaveText('Global')
    expect((await header.boundingBox())!.height).toBeLessThanOrEqual(64)
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
    const menu = await openMenu(page)
    await expect(menu.locator('.project-root')).toHaveText(root)
    expect((await menu.locator('.project-root').boundingBox())!.width).toBeGreaterThan(40)
    const project = menu.getByRole('button', { name: 'Open project agnostic-ai' })
    await expect(project).toBeInViewport({ ratio: 1 })
    expect((await project.boundingBox())!.height).toBeGreaterThanOrEqual(44)
    await page.keyboard.press('Escape')
  }
})

test('opens the global setup panel from the rail above the page and returns focus on close', async ({ page }) => {
  const root = '/tmp/missing-global-home'
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/tmp/agnostic-ai', global: false, ui: true } }))
  await page.route('**/api/switch', (route) => route.fulfill({ status: 409, json: { error: `no global setup at ${root}`, missing: true, root } }))
  await page.setViewportSize({ width: 1280, height: 500 })
  await page.goto('/?view=settings')
  const configuration = page.locator('.sidenav').getByRole('button', { name: 'Open global settings' })
  const panel = page.getByRole('dialog', { name: 'No global setup yet' })

  for (const close of ['Escape', 'Cancel'] as const) {
    await configuration.click()
    await expect(panel).toBeVisible()
    for (const part of [panel.getByRole('heading', { name: 'No global setup yet' }), panel.getByRole('button', { name: 'Create global setup' })]) {
      expect(await part.evaluate((element) => {
        const box = element.getBoundingClientRect()
        const top = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2)
        return top !== null && element.contains(top)
      })).toBe(true)
    }
    if (close === 'Escape') await page.keyboard.press('Escape')
    else await panel.getByRole('button', { name: 'Cancel' }).click()
    await expect(panel).toHaveCount(0)
    await expect(configuration).toBeFocused()
  }
})

test('closes only the global setup panel on Escape inside the phone Menu', async ({ page }) => {
  const root = '/tmp/missing-global-home'
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/tmp/agnostic-ai', global: false, ui: true } }))
  await page.route('**/api/switch', (route) => route.fulfill({ status: 409, json: { error: `no global setup at ${root}`, missing: true, root } }))
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/?view=settings')
  const menu = await openMenu(page)
  const configuration = menu.getByRole('button', { name: 'Open global settings' })
  await configuration.click()
  const panel = page.getByRole('dialog', { name: 'No global setup yet' })
  await expect(panel).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(panel).toHaveCount(0)
  await expect(menu).toBeVisible()
  await expect(configuration).toBeFocused()
})
