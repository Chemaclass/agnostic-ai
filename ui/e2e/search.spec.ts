import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import type { OverviewView } from '../src/contract'

const fixture: OverviewView = JSON.parse(readFileSync(new URL('../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

test.describe('section search', () => {
  test('searches item names, labels, paths, descriptions, and values, then clears on navigation', async ({ page }) => {
    const data = structuredClone(fixture)
    const section = data.sections.find((entry) => entry.id === 'context')!
    Object.assign(section.items[0], {
      name: 'quality-gate', label: 'Quality checks', path: '.agnostic-ai/rules/policies.md',
      description: 'Run checks before sharing changes.',
      values: [
        { key: 'release-channel', type: 'text', value: 'preview' },
        { key: 'branches', type: 'list', list: ['mainline'] },
      ],
    })
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('./?view=context')
    const search = page.getByRole('searchbox', { name: 'Search context' })
    await expect(page.locator('.item-row')).toHaveCount(section.items.length)
    for (const query of ['quality-gate', 'quality checks', 'rules/policies.md', 'SHARING CHANGES', 'release-channel', 'preview', 'mainline']) {
      await search.fill(query)
      await expect(page.locator('.item-row')).toHaveCount(1)
      await expect(page.locator('.item-row .item-main strong')).toHaveText('Quality checks')
      await expect(page.locator('.section-search-status')).toHaveText(`1 of ${section.items.length} items match.`)
    }
    await search.fill('no-such-item')
    await expect(page.locator('.item-row')).toHaveCount(0)
    await expect(page.locator('.section-search-status')).toHaveText(`0 of ${section.items.length} items match.`)
    await page.getByRole('button', { name: 'Clear search', exact: true }).click()
    await expect(search).toBeFocused()
    await expect(search).toHaveValue('')
    await expect(page.locator('.item-row')).toHaveCount(section.items.length)
    await search.fill('quality-gate')
    const nav = page.getByRole('navigation', { name: 'Configuration sections' })
    await nav.getByRole('link', { name: /^Tools/ }).click()
    await expect(page.getByRole('searchbox', { name: 'Search tools' })).toHaveValue('')
    await nav.getByRole('link', { name: /^Context/ }).click()
    await expect(search).toHaveValue('')
    await expect(page.locator('.item-row')).toHaveCount(section.items.length)
  })

  test('searches settings fields and reveals matching unconfigured fields', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.goto('./?view=authority')
    const search = page.getByRole('searchbox', { name: 'Search authority' })
    await search.fill('default permission mode')
    await expect(page.locator('.field-row')).toHaveCount(1)
    await expect(page.locator('.field-row')).toBeVisible()
    await expect(page.locator('.field-row')).toContainText('Not set')
    await search.fill('shell(go test:*)')
    await expect(page.locator('.field-row')).toHaveCount(1)
    await expect(page.locator('.field-row')).toContainText('Allowed without asking')
    await search.fill('settings/protected.yaml')
    await expect(page.locator('.field-row')).toHaveCount(1)
    await expect(page.locator('.field-row')).toContainText('Protected paths')
    await page.getByRole('button', { name: 'Clear search', exact: true }).click()
    const fields = fixture.sections.find((section) => section.id === 'authority')!.fields!
    await expect(page.locator('.field-row')).toHaveCount(fields.length)
    await expect(page.locator('.field-row', { hasText: 'Default permission mode' })).not.toBeVisible()
  })

  test('finds servers by their cached tool names, titles, and descriptions', async ({ page }) => {
    const data = structuredClone(fixture)
    const server = data.sections.find((section) => section.id === 'tools')!.items[0]
    server.tools = {
      state: 'listed', command: 'agnostic-ai mcp tools',
      tools: [{ name: 'query_metrics', title: 'Usage totals', description: 'Fetch weekly activity.' }],
    }
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('./?view=tools')
    const search = page.getByRole('searchbox', { name: 'Search tools' })
    for (const query of ['query_metrics', 'usage totals', 'weekly activity']) {
      await search.fill(query)
      await expect(page.locator('.item-row')).toHaveCount(1)
      await expect(page.locator('.item-row .item-main strong')).toHaveText(server.label)
    }
  })

  test('keeps item and section findings visible when no items match', async ({ page }) => {
    const data = structuredClone(fixture)
    data.sections.find((section) => section.id === 'authority')!.findings = [{
      source: 'doctor', severity: 'error', message: 'A configured tool could not be checked.',
    }]
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('./?view=authority')
    await page.getByRole('searchbox', { name: 'Search authority' }).fill('no-such-setting')
    await expect(page.locator('.field-row')).toHaveCount(0)
    await expect(page.locator('.item-row')).toHaveCount(1)
    await expect(page.locator('.item-row .finding')).toContainText('LINT038')
    await expect(page.getByText('A configured tool could not be checked.', { exact: true })).toBeVisible()
    await expect(page.locator('.section-search-status')).toContainText('0 of 7 items and fields match. Items with findings stay visible.')
    await page.getByRole('button', { name: 'Review 1 finding', exact: true }).click()
    await expect(page.getByRole('heading', { name: 'Other findings', exact: true })).toBeFocused()
  })

  test('filters hooks while keeping every lifecycle stage in order', async ({ page }) => {
    const data = structuredClone(fixture)
    const section = data.sections.find((entry) => entry.id === 'lifecycle')!
    section.items = [
      { kind: 'hook', name: 'prepare', label: 'Prepare session', description: '', path: '.agnostic-ai/hooks/prepare.yaml', layer: 'project', group: 'session-start', values: [], findings: [] },
      { kind: 'hook', name: 'record', label: 'Record result', description: '', path: '.agnostic-ai/hooks/record.yaml', layer: 'project', group: 'after-tool', values: [], findings: [] },
    ]
    section.kinds[0].count = section.items.length
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('./?view=lifecycle')
    const search = page.getByRole('searchbox', { name: 'Search lifecycle' })
    await search.fill('prepare')
    await expect(page.locator('.item-row')).toHaveCount(1)
    await expect(page.locator('.item-row')).toContainText('Prepare session')
    await expect(page.locator('.stage-title')).toHaveText(section.stages!.map((stage) => stage.label))
    await search.fill('no-such-hook')
    await expect(page.locator('.item-row')).toHaveCount(0)
    await expect(page.locator('.stage-title')).toHaveText(section.stages!.map((stage) => stage.label))
    await expect(page.locator('.stage-empty')).toHaveText(section.stages!.map(() => 'No matching hooks.'))
    await page.getByRole('button', { name: 'Clear search', exact: true }).click()
    await expect(page.locator('.item-row')).toHaveCount(2)
  })
})
