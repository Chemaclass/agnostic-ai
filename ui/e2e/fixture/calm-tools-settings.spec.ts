import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import type { OverviewView } from '../../src/contract'

const fixture: OverviewView = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

async function serve(page: Page, data: OverviewView = fixture) {
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: structuredClone(data) }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/tmp/agnostic-ai', global: false, ui: true } }))
  await page.emulateMedia({ reducedMotion: 'reduce' })
}

function tools(data: OverviewView) {
  return data.sections.find((section) => section.id === 'tools')!
}

test.describe('calm Tools page', () => {
  test('leads with tabs that hold something and keeps a found server to one row with one Import pill', async ({ page }) => {
    const data = structuredClone(fixture)
    const section = tools(data)
    section.items = section.items.filter((item) => item.kind !== 'mcp')
    section.kinds = section.kinds.map((kind) => (kind.kind === 'mcp' ? { ...kind, count: 0 } : kind))
    section.native = [{ name: 'paper', import: 'agnostic-ai import --global claude', holders: [
      { target: 'claude', label: 'Claude Code', path: '~/.claude.json', scope: 'user', transport: 'stdio' },
      { target: 'codex', label: 'Codex', path: '~/.codex/config.toml', scope: 'user', transport: 'stdio' },
    ] }]
    await serve(page, data)
    await page.goto('/?view=tools')
    await expect(page.locator('.page-facts')).toContainText('1 more server found in Claude Code and Codex')
    const tabs = page.getByRole('tab')
    await expect(tabs.last()).toHaveText('MCP servers')
    await expect(tabs.last().locator('.section-tab-count')).toHaveCount(0)
    await expect(page.getByRole('tab', { name: /^Found in tools/ })).toHaveAttribute('aria-selected', 'true')
    await expect(page.locator('.section-search').getByRole('group', { name: 'Save new servers for' })).toBeVisible()

    const row = page.getByRole('region', { name: /Found in tools/ }).locator('.item-row', { hasText: 'paper' })
    await expect(row.locator('.native-tools')).toHaveText('In Claude Code and Codex')
    await expect(row.getByRole('button')).toHaveCount(2)
    await expect(row.getByRole('button', { name: 'Import paper' })).toHaveClass(/button-primary/)
    await expect(row.locator('.native-details')).toBeHidden()
    await expect(row.getByText('stdio').first()).toBeHidden()

    const details = row.getByRole('button', { name: 'Details for paper' })
    await row.hover()
    await expect(details).toHaveCSS('opacity', '1')
    await details.click()
    await expect(details).toHaveAttribute('aria-expanded', 'true')
    const panel = row.locator('.native-details')
    await expect(panel.getByRole('combobox', { name: 'Source for paper' })).toBeVisible()
    await expect(panel).toContainText('~/.codex/config.toml')
    await expect(panel).toContainText('agnostic-ai import --global claude')
    await page.mouse.move(0, 0)
    await expect(details).toHaveCSS('opacity', '1')
  })
})

test.describe('calm Settings page', () => {
  test('reads built-ins as name, one line and a switch, with the config name under Details', async ({ page }) => {
    const data = structuredClone(fixture)
    const settings = data.sections.find((section) => section.id === 'settings')!
    for (const item of settings.items) if (item.name === 'handoff' || item.name === 'memory') item.enabled = true
    await serve(page, data)
    await page.goto('/?view=settings')
    await expect(page.locator('.page-verdict')).toHaveText('2 built-ins are on.')
    await expect(page.locator('.page-verdict-key')).toHaveText('2 built-ins')
    await expect(page.locator('.page-facts')).toContainText(/\d+ of \d+ tools on/)
    await expect(page.getByRole('heading', { level: 2, name: /^Built-ins/ })).toHaveCount(1)
    await expect(page.getByText('Ready-made features. Turning one on shows a preview before anything changes.')).toBeVisible()

    const row = page.locator('.item-row', { hasText: 'Handoff on start' })
    await expect(row.getByText('handoff-hook', { exact: true })).toBeHidden()
    const nameBox = await row.getByText('Handoff on start', { exact: true }).boundingBox()
    const lineBox = await row.locator('.item-description').boundingBox()
    expect(Math.abs(nameBox!.y - lineBox!.y)).toBeLessThan(6)
    await row.hover()
    await row.getByRole('button', { name: 'Details for Handoff on start' }).click()
    await expect(row.locator('.row-details')).toContainText('Name in config')
    await expect(row.getByText('handoff-hook', { exact: true })).toBeVisible()
  })

  test('puts Details behind the row menu on a phone', async ({ page }) => {
    await serve(page)
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/?view=settings')
    const row = page.locator('.item-row', { hasText: 'Handoff on start' })
    await row.getByRole('button', { name: 'More actions for Handoff on start' }).click()
    await row.getByRole('button', { name: 'Details for Handoff on start' }).click()
    await expect(row.getByText('handoff-hook', { exact: true })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })
})
