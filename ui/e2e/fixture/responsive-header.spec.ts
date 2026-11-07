import { expect, test, type Locator } from '@playwright/test'
import { readFileSync } from 'node:fs'

const fixture = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))
// Reserve the width of a classic scrollbar on the narrowest viewport.
const narrowContent = '@media (max-width: 320px) { .app { max-width: 305px; } }'

async function sameLine(a: Locator, b: Locator) {
  const first = (await a.boundingBox())!
  const second = (await b.boundingBox())!
  return first.y < second.y + second.height && second.y < first.y + first.height
}

test('keeps the project path beside its name and the configuration switch in the sidebar', async ({ page }) => {
  const overview = structuredClone(fixture)
  overview.snapshot.project.name = 'agnostic-ai'
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/tmp/agnostic-ai', global: false, ui: true, version: '0.91.0' } }))
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/?view=context&tab=rules')
  const header = page.locator('.topbar')
  await expect(header.getByRole('button', { name: 'Open global settings' })).toHaveCount(0)
  await expect(header.locator('.project-root')).toHaveText('/tmp/agnostic-ai')
  expect(await sameLine(header.locator('.project-title'), header.locator('.project-root'))).toBe(true)
  expect((await header.boundingBox())!.height).toBeLessThanOrEqual(64)

  const footer = page.locator('.sidenav-footer')
  const configuration = footer.getByRole('button', { name: 'Open global settings' })
  await expect(configuration).toBeInViewport({ ratio: 1 })
  const badge = (await footer.locator('.release-badge').boundingBox())!
  expect((await configuration.boundingBox())!.y).toBeLessThan(badge.y)
})

test('keeps the configuration switch reachable under the phone navigation', async ({ page }) => {
  const overview = structuredClone(fixture)
  overview.snapshot.project.name = 'agnostic-ai'
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/tmp/agnostic-ai', global: false, ui: true } }))
  await page.setViewportSize({ width: 320, height: 844 })
  await page.goto('/?view=context&tab=rules')
  await page.addStyleTag({ content: narrowContent })
  const header = page.locator('.topbar')
  const configuration = page.locator('.sidenav').getByRole('button', { name: 'Open global settings' })
  await expect(configuration).toBeVisible()

  for (const width of [320, 390, 900]) {
    await page.setViewportSize({ width, height: 844 })
    await expect(configuration).toBeInViewport({ ratio: 1 })
    const navBox = (await page.locator('.section-nav').boundingBox())!
    expect((await configuration.boundingBox())!.y).toBeGreaterThanOrEqual(navBox.y + navBox.height - 1)
    expect((await header.boundingBox())!.height).toBeLessThanOrEqual(120)
    expect(await header.locator('.brand-name').evaluate((element) => {
      const range = document.createRange()
      range.selectNodeContents(element)
      return range.getClientRects().length
    })).toBe(1)
    if (width < 760) expect((await configuration.boundingBox())!.height).toBeGreaterThanOrEqual(44)
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
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
  const project = page.locator('.sidenav').getByRole('button', { name: 'Open project agnostic-ai' })
  await expect(project).toBeVisible()

  for (const width of [390, 320]) {
    await page.setViewportSize({ width, height: 844 })
    await expect(header.locator('.project-title .chip')).toBeInViewport({ ratio: 1 })
    await expect(header.locator('.project-title .chip')).toHaveText('Global')
    await expect(header.locator('.project-root')).toHaveText(root)
    expect(await sameLine(header.locator('.project-title'), header.locator('.project-root'))).toBe(true)
    expect((await header.locator('.project-root').boundingBox())!.width).toBeGreaterThan(40)
    await expect(project).toBeInViewport({ ratio: 1 })
    expect((await header.boundingBox())!.height).toBeLessThanOrEqual(120)
    expect((await project.boundingBox())!.height).toBeGreaterThanOrEqual(44)
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
  }
})

test('opens the global setup panel from the sidebar above the page and returns focus on close', async ({ page }) => {
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
