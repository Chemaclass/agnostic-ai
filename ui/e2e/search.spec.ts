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
      { kind: 'hook', name: 'record', label: 'Record result', description: '', path: '.agnostic-ai/hooks/record.yaml', layer: 'project', group: 'after-tool', values: [{ key: 'disabled', type: 'bool', value: 'true' }], findings: [] },
    ]
    section.kinds[0].count = section.items.length
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('./?view=lifecycle')
    const search = page.getByRole('searchbox', { name: 'Search lifecycle' })
    await search.fill('prepare')
    await expect(page.locator('.item-row')).toHaveCount(1)
    await expect(page.locator('.item-row')).toContainText('Prepare session')
    await expect(page.locator('.stage-title')).toHaveText(section.stages!.map((stage) => stage.label))
    await expect(page.locator('.stage-count')).toHaveText(section.stages!.map((stage) => stage.id === 'session-start' ? '1 hook' : 'No matching hooks'))
    await expect(page.locator('.stage[open]')).toHaveCount(1)
    await expect(page.locator('.stage[open] .stage-title')).toHaveText('Session start')
    await expect(page.locator('.stage-disabled-count')).toHaveCount(0)
    await expect(page.getByRole('heading', { level: 2, name: 'Hooks 2', exact: true })).toBeVisible()
    await search.fill('record')
    await expect(page.locator('.stage-count')).toHaveText(section.stages!.map((stage) => stage.id === 'after-tool' ? '1 hook' : 'No matching hooks'))
    await expect(page.locator('.stage[open]')).toHaveCount(1)
    await expect(page.locator('.stage[open] .stage-title')).toHaveText('After a tool runs')
    await expect(page.locator('.stage-disabled-count')).toHaveText('1 disabled')
    await search.fill('no-such-hook')
    await expect(page.locator('.item-row')).toHaveCount(0)
    await expect(page.locator('.stage-title')).toHaveText(section.stages!.map((stage) => stage.label))
    await expect(page.locator('.stage-empty')).toHaveText(section.stages!.map(() => 'No matching hooks.'))
    await expect(page.locator('.stage-count')).toHaveText(section.stages!.map(() => 'No matching hooks'))
    await expect(page.locator('.stage[open]')).toHaveCount(0)
    await expect(page.locator('.stage-disabled-count')).toHaveCount(0)
    await page.getByRole('button', { name: 'Clear search', exact: true }).click()
    await expect(page.locator('.item-row')).toHaveCount(2)
    await expect(page.locator('.stage-count')).toHaveText(section.stages!.map((stage) => ['session-start', 'after-tool'].includes(stage.id) ? '1 hook' : 'No hooks'))
    await expect(page.locator('.stage-disabled-count')).toHaveText('1 disabled')
    await expect(page.locator('.stage[open]')).toHaveCount(0)
  })

  test('keeps a retained hook finding available without counting it as a search match', async ({ page }) => {
    const data = structuredClone(fixture)
    const section = data.sections.find((entry) => entry.id === 'lifecycle')!
    section.items[0].findings = [{ source: 'lint', code: 'LINT001', severity: 'warning', message: 'Check the session hook.' }]
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('./?view=lifecycle')
    await page.getByRole('searchbox', { name: 'Search lifecycle' }).fill('no-such-hook')
    await expect(page.locator('.stage[open]')).toHaveCount(0)
    await expect(page.locator('.stage-count')).toHaveText(section.stages!.map(() => 'No matching hooks'))
    await expect(page.locator('.section-search-status')).toContainText('Items with findings stay visible.')
    await page.getByRole('button', { name: 'Expand all', exact: true }).click()
    await expect(page.locator('.stage .finding')).toBeVisible()
  })
})

test.describe('search across sections', () => {
  const modK = process.platform === 'darwin' ? 'Meta+K' : 'Control+K'

  test('opens a server from Ctrl+K, keeps its link across reload, and restores history', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.goto('./?view=context')
    await expect(page.locator('.item-row').first()).toBeVisible()
    await page.keyboard.press(modK)
    const dialog = page.getByRole('dialog', { name: 'Search all configuration' })
    const input = dialog.getByRole('combobox', { name: 'Search all configuration' })
    await expect(input).toBeFocused()
    await expect(dialog.locator('.search-dialog-status')).toContainText('Type to search every section')
    await input.fill('github')
    const first = dialog.getByRole('option').first()
    await expect(first).toHaveAttribute('aria-selected', 'true')
    await expect(first).toContainText('github')
    await expect(first).toContainText('Tools · MCP servers')
    await expect(first).toContainText('.agnostic-ai/mcps/github.yaml')
    await page.keyboard.press('Enter')
    await expect(dialog).toHaveCount(0)
    await expect(page).toHaveURL(/view=tools&entry=item%3Amcp%3Agithub%3A.agnostic-ai%2Fmcps%2Fgithub.yaml/)
    const linked = page.locator('.item-row.is-linked')
    await expect(linked).toHaveCount(1)
    await expect(linked.locator('.item-main strong')).toHaveText('github')
    await expect(linked).toBeFocused()
    await page.reload()
    await expect(page.locator('.item-row.is-linked .item-main strong')).toHaveText('github')
    await expect(page.locator('.item-row.is-linked')).toBeFocused()
    await page.goBack()
    await expect(page).toHaveURL(/view=context$/)
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Context')
    await expect(page.locator('.is-linked')).toHaveCount(0)
    await page.goForward()
    await expect(page.locator('.item-row.is-linked .item-main strong')).toHaveText('github')
  })

  test('links findings and unconfigured settings, and opens results with the mouse', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.goto('./?view=tools')
    await expect(page.locator('.item-row').first()).toBeVisible()
    await page.getByRole('button', { name: 'Search', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Search all configuration' })
    await dialog.getByRole('combobox').fill('lint038')
    await dialog.getByRole('option', { name: /bare read/ }).click()
    await expect(page).toHaveURL(/view=authority&entry=finding%3ALINT038%3A/)
    await expect(page.locator('.finding.is-linked')).toContainText('LINT038')
    await expect(page.locator('.finding.is-linked')).toBeFocused()
    await page.keyboard.press(modK)
    await dialog.getByRole('combobox').fill('default permission mode')
    await page.keyboard.press('Enter')
    await expect(page).toHaveURL(/entry=field%3Apermissions.default-mode/)
    const field = page.locator('.field-row.is-linked')
    await expect(field).toBeVisible()
    await expect(field).toContainText('Default permission mode')
    await expect(field).toBeFocused()
  })

  test('closes with Escape, restores focus, and shows an empty state', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.goto('./?view=context')
    const button = page.getByRole('button', { name: 'Search', exact: true })
    await button.click()
    const dialog = page.getByRole('dialog', { name: 'Search all configuration' })
    await dialog.getByRole('combobox').fill('no-such-entry-anywhere')
    await expect(dialog.getByRole('option')).toHaveCount(0)
    await expect(dialog.locator('.search-dialog-status')).toHaveText('No results for “no-such-entry-anywhere”.')
    await page.keyboard.press('Tab')
    await expect(dialog.getByRole('button', { name: 'Close' })).toBeFocused()
    await page.keyboard.press('Tab')
    await expect(dialog.getByRole('combobox')).toBeFocused()
    await page.keyboard.press('Escape')
    await expect(dialog).toHaveCount(0)
    await expect(button).toBeFocused()
    await expect(page).toHaveURL(/view=context$/)
  })

  test('keeps mock mode in result links', async ({ page }) => {
    await page.goto('./?mock=1')
    await expect(page.locator('.item-row').first()).toBeVisible()
    await page.keyboard.press(modK)
    await page.getByRole('combobox', { name: 'Search all configuration' }).fill('github')
    await page.keyboard.press('Enter')
    await expect(page).toHaveURL(/view=tools&entry=item%3Amcp%3Agithub%3A.*&mock=1$/)
    await page.reload()
    await expect(page.locator('.item-row.is-linked .item-main strong')).toHaveText('github')
  })

  test('explains a link whose entry is gone instead of selecting another', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.goto(`./?view=tools&entry=${encodeURIComponent('item:mcp:removed-server:.agnostic-ai/mcps/removed-server.yaml')}`)
    const notice = page.locator('.entry-missing')
    await expect(notice).toContainText('The linked item is not in this tools section.')
    await expect(page.locator('.is-linked')).toHaveCount(0)
    await notice.getByRole('button', { name: 'Dismiss' }).click()
    await expect(notice).toHaveCount(0)
    await expect(page).toHaveURL(/view=tools$/)
  })

  test('tells apart entries with the same name and links each one', async ({ page }) => {
    const data = structuredClone(fixture)
    const tools = data.sections.find((section) => section.id === 'tools')!
    const github = tools.items.find((item) => item.name === 'github')!
    tools.items.push({ ...github, path: '.agnostic-ai/mcps/team/github.yaml', layer: 'global' })
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('./?view=context')
    await page.keyboard.press(modK)
    const dialog = page.getByRole('dialog', { name: 'Search all configuration' })
    await dialog.getByRole('combobox').fill('github')
    await expect(dialog.getByRole('option', { name: /\.agnostic-ai\/mcps\/github\.yaml/ })).toHaveCount(1)
    await dialog.getByRole('option', { name: /mcps\/team\/github\.yaml/ }).click()
    await expect(page.locator('.item-row.is-linked .item-path')).toHaveText('.agnostic-ai/mcps/team/github.yaml')
  })

  test('limits a large result set and keeps the section search in the URL', async ({ page }) => {
    const data = structuredClone(fixture)
    const context = data.sections.find((section) => section.id === 'context')!
    for (let i = 0; i < 500; i++) {
      context.items.push({ kind: 'rule', name: `generated-${i}`, label: `generated-${i}`, description: 'Generated rule.', path: `.agnostic-ai/rules/generated-${i}.md`, layer: 'project', values: [], findings: [] })
    }
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('./?view=context')
    await page.keyboard.press(modK)
    const dialog = page.getByRole('dialog', { name: 'Search all configuration' })
    await dialog.getByRole('combobox').fill('generated')
    await expect(dialog.locator('.search-dialog-status')).toHaveText('Showing the first 50 of 500 results. Add words to narrow them.')
    await expect(dialog.getByRole('option')).toHaveCount(50)
    await page.keyboard.press('Escape')
    await page.getByRole('searchbox', { name: 'Search context' }).fill('generated-49')
    await expect(page).toHaveURL(/\?q=generated-49$/)
    await page.reload()
    await expect(page.getByRole('searchbox', { name: 'Search context' })).toHaveValue('generated-49')
    await expect(page.locator('.item-row')).toHaveCount(11)
  })
})
