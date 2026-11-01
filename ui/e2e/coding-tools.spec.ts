import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'

const fixture = JSON.parse(readFileSync(new URL('../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))
const total: number = fixture.tools.items.length
const on: number = fixture.tools.items.filter((tool: { enabled: boolean }) => tool.enabled).length

async function settings(page: Page, options: { source?: 'default' | 'shared' | 'personal'; status?: number; file?: string; delay?: number; detected?: string[]; query?: string } = {}) {
  const data = structuredClone(fixture)
  if (options.source) data.tools.source = options.source
  for (const tool of data.tools.items) if (options.detected?.includes(tool.target)) tool.detected = true
  const sent: unknown[] = []
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
  await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: { schemaVersion: 'builtin-change/v1', status: 'idle', enabled: false, supported: false, undoAvailable: false, targets: [], changes: [], findings: [] } }))
  await page.route('**/api/edit/targets', async (route) => {
    sent.push(route.request().postDataJSON())
    if (options.delay) await new Promise((resolve) => setTimeout(resolve, options.delay))
    if (options.status && options.status >= 400) return route.fulfill({ status: options.status, json: { error: 'The list changed.' } })
    return route.fulfill({ json: { ok: true, file: options.file ?? 'agnostic-ai.yaml' } })
  })
  await page.addInitScript(() => window.localStorage.setItem('agnostic-ai.edit-scope', 'shared'))
  await page.goto(options.query ?? '/?view=settings&tab=targets')
  return sent
}

test.describe('coding tools', () => {
  test('lists every tool with a docs link and a count', async ({ page }) => {
    await settings(page)
    const tab = page.getByRole('tab', { name: /^Coding tools/ })
    await expect(tab).toHaveAttribute('aria-selected', 'true')
    await expect(tab).toContainText(`${on} of ${total} on`)
    await expect(page.locator('.coding-tool')).toHaveCount(total)
    const claude = page.locator('.coding-tool[data-target="claude"]')
    await expect(claude.getByRole('switch', { name: 'Claude Code coding tool' })).toHaveAttribute('aria-checked', 'true')
    const docs = claude.getByRole('link', { name: 'Claude Code docs' })
    await expect(docs).toHaveAttribute('href', 'https://agnostic-ai.org/docs/targets/claude/')
    await expect(docs).toHaveAttribute('target', '_blank')
    await expect(docs).toHaveAttribute('rel', /noopener/)
    await expect(page.locator('.coding-tool[data-target="amp"]').getByRole('switch')).toHaveAttribute('aria-checked', 'false')
    await expect(claude.locator('.tool-avatar img')).toBeVisible()
  })

  test('filters by name and by on state', async ({ page }) => {
    await settings(page)
    await page.getByLabel('Filter tools').fill('curs')
    await expect(page.locator('.coding-tool')).toHaveCount(1)
    await expect(page.locator('.coding-tool')).toHaveAttribute('data-target', 'cursor')
    await page.getByLabel('Filter tools').fill('')
    await page.getByLabel('On only').check()
    await expect(page.locator('.coding-tool')).toHaveCount(on)
    await expect(page.getByRole('status').filter({ hasText: 'tools shown' })).toContainText(`${on} of ${total} tools shown`)
  })

  test('turns a tool on with one request and asks before turning one off', async ({ page }) => {
    const sent = await settings(page)
    await page.locator('.coding-tool[data-target="amp"]').getByRole('switch').click()
    await expect.poll(() => sent.length).toBe(1)
    expect(sent[0]).toEqual({ audience: 'shared', name: 'amp', enabled: true, expected: false })
    const status = page.getByRole('status').filter({ hasText: 'Saved' })
    await expect(status).toContainText('agnostic-ai.yaml')
    await expect(status).toContainText('agnostic-ai sync')
    await page.locator('.coding-tool[data-target="cursor"]').getByRole('switch').click()
    const confirm = page.getByRole('group', { name: 'Turn off Cursor' })
    await expect(confirm).toContainText('removes the files it wrote for Cursor')
    await expect(page.getByRole('button', { name: 'Turn off', exact: true })).toBeFocused()
    await page.getByRole('button', { name: 'Keep on' }).click()
    await expect(confirm).toHaveCount(0)
    await expect(page.locator('.coding-tool[data-target="cursor"]').getByRole('switch')).toBeFocused()
    await page.keyboard.press('Enter')
    await expect(confirm).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(confirm).toHaveCount(0)
    await expect(page.locator('.coding-tool[data-target="cursor"]').getByRole('switch')).toBeFocused()
    expect(sent.length).toBe(1)
    await page.locator('.coding-tool[data-target="cursor"]').getByRole('switch').click()
    await page.getByRole('button', { name: 'Turn off', exact: true }).click()
    await expect.poll(() => sent.length).toBe(2)
    expect(sent[1]).toEqual({ audience: 'shared', name: 'cursor', enabled: false, expected: true })
    await expect(page.locator('.coding-tool[data-target="cursor"]').getByRole('switch')).toBeFocused()
  })

  test('offers reload when the save is stale', async ({ page }) => {
    await settings(page, { status: 409 })
    const toggle = page.locator('.coding-tool[data-target="amp"]').getByRole('switch')
    await toggle.click()
    await expect(page.getByRole('alert')).toContainText('The list changed.')
    await expect(page.getByRole('button', { name: 'Reload saved values' })).toBeFocused()
    await expect(page.getByRole('button', { name: 'Reload saved values' })).toBeVisible()
    await expect(toggle).toHaveAttribute('aria-checked', 'true')
    await expect(toggle).toBeDisabled()
  })

  test('explains the default list, blocks shared edits behind a personal list, and stays read-only in mock mode', async ({ page }) => {
    await settings(page, { source: 'default' })
    await expect(page.getByText('the default list applies')).toBeVisible()
    await page.unroute('**/api/views/overview')
    const data = structuredClone(fixture)
    data.tools.source = 'personal'
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=settings&tab=targets')
    await expect(page.getByText('Reset that list before changing the shared one.')).toBeVisible()
    await expect(page.locator('.coding-tool[data-target="claude"]').getByRole('switch')).toHaveCount(0)
    await page.goto('/?view=settings&tab=targets&mock=1')
    await expect(page.locator('.coding-tool')).toHaveCount(total)
    await expect(page.getByRole('switch')).toHaveCount(0)
    await expect(page.getByText('Switches need a running')).toBeVisible()
  })

  test('fits a phone viewport', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await settings(page)
    await expect(page.getByRole('tab', { name: /^Coding tools/ })).toBeInViewport({ ratio: 1 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.locator('.coding-tool[data-target="cursor"]').getByRole('switch').click()
    await expect(page.getByRole('group', { name: 'Turn off Cursor' })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })

  test('shows found in project only on tools that are off', async ({ page }) => {
    await settings(page, { detected: ['claude', 'amp'] })
    await expect(page.locator('.coding-tool[data-target="amp"]')).toContainText('found in project')
    await expect(page.locator('.coding-tool[data-target="claude"]')).not.toContainText('found in project')
  })

  test('opens from page search and keeps the shown count after a save', async ({ page }) => {
    await settings(page, { query: '/?view=settings&q=windsurf' })
    await expect(page.getByRole('tab', { name: /^Coding tools/ })).toHaveAttribute('aria-selected', 'true')
    await expect(page.locator('.coding-tool')).toHaveCount(1)
    await expect(page.locator('.section-search-status')).toContainText(/^1 of \d+ items, fields, and tools match\./)
    await page.locator('.coding-tool[data-target="windsurf"]').getByRole('switch').click()
    await expect(page.getByRole('status').filter({ hasText: 'Saved' })).toBeVisible()
    await page.getByLabel('Filter tools').fill('c')
    await expect(page.getByRole('status').filter({ hasText: 'tools shown' })).toContainText(`of ${total} tools shown`)
  })

  for (const width of [1280, 390]) test(`keeps the list in place while a switch saves at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    await settings(page, { delay: 400 })
    const place = (selector: string) => page.locator(selector).evaluate((element) => {
      const box = element.getBoundingClientRect()
      return { x: box.x, y: box.y + window.scrollY, width: box.width, height: box.height }
    })
    const before = { list: await place('.coding-tools-list'), last: await place('.coding-tool:last-child') }
    await page.locator('.coding-tool[data-target="amp"]').getByRole('switch').click()
    await expect(page.locator('.coding-tool[data-target="amp"]').getByRole('switch')).toHaveAttribute('aria-busy', 'true')
    expect(await place('.coding-tools-list')).toEqual(before.list)
    await expect(page.getByRole('status').filter({ hasText: 'Saved' })).toBeVisible()
    expect(await place('.coding-tools-list')).toEqual(before.list)
    expect(await place('.coding-tool:last-child')).toEqual(before.last)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })
})
