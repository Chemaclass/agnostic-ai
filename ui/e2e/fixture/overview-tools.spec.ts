import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { MOCK, openApp, saveScreenshot, selectTab } from '../helpers'
import type { SectionItem } from '../../src/contract'

const fixture = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

test.describe('overview', () => {
  test('says what agents can use and shows a turned-off server as off', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.goto('./?view=tools')
    await expect(page.locator('.page-verdict')).toHaveText('Agents can use 3 MCP servers.')
    await expect(page.locator('.page-verdict .page-verdict-key')).toHaveText('3 MCP servers')
    await expect(page.locator('.page-facts, .page-verdict + *').first()).toContainText('1 MCP server turned off')
    const row = page.getByRole('region', { name: /^MCP servers/ }).locator('.item-row', { hasText: 'railway' })
    await expect(row).toHaveClass(/is-off/)
    await expect(row.getByText('Turned off', { exact: true })).toBeVisible()
    await expect(page.getByText('Turned off', { exact: true })).toHaveCount(1)
  })

  test('lists each MCP server with its cached tools or how to list them', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.goto('./?view=tools')
    await expect(page.getByRole('heading', { level: 1, name: 'Tools' })).toBeVisible()
    const block = page.getByRole('region', { name: /^MCP servers/ })
    await expect(block.locator('.item-row').first()).toBeVisible()
    await expect(block.locator('.tools-unlisted')).toHaveText('Tools not listed yet. Run agnostic-ai mcp tools to list them.')
    await expect(block.locator('.item-row .tool-state')).toHaveCount(0)
  })

  test('says tools are not listed on each server only when others have a list', async ({ page }) => {
    const data = structuredClone(fixture)
    const servers: SectionItem[] = data.sections.find((section: { id: string }) => section.id === 'tools')!.items
    servers[0].tools = { state: 'listed', command: 'agnostic-ai mcp tools', tools: [{ name: 'query_metrics' }] }
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('./?view=tools')
    const block = page.getByRole('region', { name: /^MCP servers/ })
    await expect(block.locator('.tools-unlisted')).toHaveCount(0)
    await expect(block.locator('.item-row').first().locator('.tool-listing')).toContainText('query_metrics')
    await expect(block.locator('.item-row .tool-state')).toHaveCount(servers.length - 1)
  })

  test("lists MCP servers found only in a tool's own config, with the import hint", async ({ page }) => {
    test.skip(!MOCK, 'the mock fixture holds a native server')
    await openApp(page, '?view=tools&tab=native')
    const block = page.getByRole('region', { name: /Found in tools/ })
    const row = block.locator('.item-row', { hasText: 'notion' })
    await expect(row.locator('.native-tools')).toHaveText('In Claude Code')
    const details = row.locator('.native-details')
    await expect(details).toBeHidden()
    await row.hover()
    await row.getByRole('button', { name: 'Details for notion' }).click()
    await expect(details).toBeVisible()
    await expect(details).toContainText('~/.claude.json')
    await expect(details).toContainText('http')
    await expect(details).toContainText('agnostic-ai import --global claude')
    await expect(details.getByRole('button', { name: 'Copy import command for notion' })).toBeVisible()
    await expect(row.getByRole('button', { name: 'Import notion' })).toHaveCount(0)
    await page.getByRole('searchbox').fill('notion')
    await expect(block.locator('.kind-count')).toHaveText('1 of 1 match')
    await page.getByRole('searchbox').fill('github')
    await selectTab(page, 'Found in tools')
    await expect(block.locator('.kind-count')).toHaveText('0 of 1 match')
    await expect(block).toContainText('No matches.')
  })

  test('previews one native server and saves the reviewed spec', async ({ page }) => {
    const data = structuredClone(fixture)
    const tools = data.sections.find((section: { id: string }) => section.id === 'tools')!
    const native = tools.native![0]
    const calls: Record<string, unknown>[] = []
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.route('**/api/edit/mcp', async (route) => {
      const body = route.request().postDataJSON() as Record<string, unknown>
      calls.push(body)
      if (body.action === 'apply') tools.native = []
      await route.fulfill({ json: { file: '.agnostic-ai/mcps/notion.yaml', spec: 'name: notion\ntype: http\nurl: https://mcp.notion.com/mcp\n', variables: [], targets: ['claude', 'codex'], syncCommand: 'agnostic-ai sync', expected: 'reviewed-server' } })
    })
    await page.goto('./?view=tools&tab=native')
    const row = page.getByRole('region', { name: /Found in tools/ }).locator('.item-row', { hasText: native.name })
    // The audience switch sits in the toolbar beside the filter, one row above the tabs.
    const scope = page.locator('.section-search .edit-scope')
    await expect(scope.getByRole('group', { name: 'Save new servers for' })).toBeVisible()
    await expect(scope).toContainText('Project files, tracked by Git.')
    await expect(scope.getByRole('button', { name: 'Project (committed)', exact: true })).toHaveText('Project')
    const scopeBox = await scope.boundingBox()
    const searchBox = await page.locator('.section-search-input').boundingBox()
    expect(Math.abs(scopeBox!.y + scopeBox!.height / 2 - (searchBox!.y + searchBox!.height / 2))).toBeLessThan(4)
    await selectTab(page, 'MCP servers')
    await expect(page.getByRole('group', { name: 'Save new servers for' })).toHaveCount(0)
    await selectTab(page, 'Found in tools')
    await row.getByRole('button', { name: `Import ${native.name}` }).click()
    const preview = row.getByRole('region', { name: `${native.name} preview` })
    await expect(preview).toContainText('.agnostic-ai/mcps/notion.yaml')
    await expect(preview).toContainText('After sync, this server reaches')
    await preview.scrollIntoViewIfNeeded()
    await saveScreenshot(page, 'mcp-spec-preview-desktop.png')
    await page.setViewportSize({ width: 390, height: 844 })
    await preview.scrollIntoViewIfNeeded()
    await expect(preview.getByRole('button', { name: 'Apply', exact: true })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await saveScreenshot(page, 'mcp-spec-preview-mobile.png')
    await preview.getByRole('button', { name: 'Apply', exact: true }).click()
    const saved = page.getByRole('status').filter({ hasText: 'Saved .agnostic-ai/mcps/notion.yaml.' })
    await expect(saved).toContainText('Run agnostic-ai sync')
    await expect(saved.locator('.banner-success')).toBeVisible()
    await expect(row).toHaveCount(0)
    await expect(page.getByRole('group', { name: 'Save new servers for' })).toHaveCount(0)
    expect(calls).toEqual([
      { action: 'preview', audience: 'shared', name: native.name, target: native.holders[0].target, path: native.holders[0].path, scope: native.holders[0].scope },
      { action: 'apply', audience: 'shared', name: native.name, target: native.holders[0].target, path: native.holders[0].path, scope: native.holders[0].scope, expected: 'reviewed-server' },
    ])
    await page.reload()
    await expect(page.getByRole('status').filter({ hasText: 'Saved .agnostic-ai/mcps/notion.yaml.' })).toContainText('Run agnostic-ai sync')
    await page.getByRole('button', { name: 'Dismiss' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Saved .agnostic-ai/mcps/notion.yaml.' })).toHaveCount(0)
  })

  test('keeps a long saved file path inside the page at 390', async ({ page }) => {
    const name = 'a-server-with-a-very-long-name-that-has-no-break-points-at-all-so-the-path-is-long'
    const data = structuredClone(fixture)
    const tools = data.sections.find((section: { id: string }) => section.id === 'tools')!
    tools.native![0].name = name
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.route('**/api/edit/mcp', (route) => route.fulfill({ json: { file: `.agnostic-ai/mcps/${name}.yaml`, spec: `name: ${name}\n`, variables: [], targets: ['claude'], syncCommand: 'agnostic-ai sync', expected: 'reviewed-server' } }))
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('./?view=tools&tab=native')
    const row = page.getByRole('region', { name: /Found in tools/ }).locator('.item-row', { hasText: name })
    await row.getByRole('button', { name: /^Import / }).click()
    await row.getByRole('button', { name: 'Apply', exact: true }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Saved' })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })

  test('drops a native server preview when its audience changes and shows import refusals', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.route('**/api/edit/mcp', async (route) => {
      if (route.request().postDataJSON().audience === 'personal') {
        await route.fulfill({ status: 422, json: { error: 'args holds a credential import cannot rewrite; write a ${NAME} reference by hand' } })
        return
      }
      await route.fulfill({ json: { file: '.agnostic-ai/mcps/notion.yaml', spec: 'name: notion\n', variables: [], targets: ['claude'], syncCommand: 'agnostic-ai sync', expected: 'reviewed-server' } })
    })
    await page.goto('./?view=tools&tab=native')
    const row = page.getByRole('region', { name: /Found in tools/ }).locator('.item-row', { hasText: 'notion' })
    await row.getByRole('button', { name: /^Import / }).click()
    await expect(row.getByRole('region', { name: 'notion preview' })).toBeVisible()
    await page.getByRole('button', { name: 'Local (not committed)', exact: true }).click()
    await expect(row.getByRole('region', { name: 'notion preview' })).toHaveCount(0)
    await row.getByRole('button', { name: /^Import / }).click()
    await expect(row.getByRole('alert')).toContainText('args holds a credential import cannot rewrite')
    await expect(row.locator('.banner-error')).toBeVisible()
    await expect(row.getByRole('button', { name: 'Apply', exact: true })).toHaveCount(0)
  })
})
