import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { saveScreenshot, selectTab } from './helpers'

const overview = JSON.parse(readFileSync(new URL('../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))
const contextView = JSON.parse(readFileSync(new URL('../src/fixtures/context.mock.json', import.meta.url), 'utf8'))
const rulePath = '/path/to/agnostic-ai/.agnostic-ai/rules/testing.md'

test.describe('row content', () => {
  test('shows a rule with its frontmatter, collapses, and reads it once', async ({ page, context }) => {
    const requests: string[] = []
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => route.fulfill({ json: contextView }))
    await page.route((url) => url.pathname === '/api/views/source', (route) => {
      const path = new URL(route.request().url()).searchParams.get('path') ?? ''
      requests.push(path)
      if (path !== rulePath) return route.fulfill({ status: 404, json: { error: 'path is not a file of the loaded configuration' } })
      return route.fulfill({ json: { path, kind: 'rule', frontmatter: [{ key: 'name', value: 'testing' }, { key: 'globs', value: '**/*.go, **/*.ts' }], body: '# Testing\n\nRun the tests before presenting work.\n', bytes: 98, truncated: false } })
    })
    await page.goto('/?view=context')
    await selectTab(page, 'Rules')
    const row = page.locator('.item-row').filter({ hasText: 'testing' })
    await expect(row.getByRole('link', { name: /^Open testing in / })).toBeVisible()
    await expect(row.getByRole('button', { name: 'Copy path of testing' })).toBeVisible()
    const toggle = row.getByText('Show content', { exact: true })
    await expect(row.locator('.source-text')).toBeHidden()
    await toggle.click()
    const chips = row.getByLabel('Frontmatter')
    await expect(chips).toContainText('name: testing')
    await expect(chips).toContainText('globs: **/*.go, **/*.ts')
    await expect(row.locator('.source-text')).toHaveText('# Testing\n\nRun the tests before presenting work.\n')
    await row.getByText('Hide content', { exact: true }).click()
    await expect(row.locator('.source-text')).toBeHidden()
    await toggle.click()
    await expect(row.locator('.source-text')).toBeVisible()
    expect(requests).toEqual([rulePath])

    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await row.getByRole('button', { name: 'Copy path of testing' }).click()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(rulePath)

    await selectTab(page, 'Agents')
    const agent = page.locator('.item-row').filter({ hasText: 'reviewer' })
    await agent.getByText('Show content', { exact: true }).click()
    await expect(agent.getByRole('alert')).toContainText('Could not show the file: the server no longer lists it.')
  })

  test('says why a withheld file shows no text', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => route.fulfill({ json: contextView }))
    await page.route((url) => url.pathname === '/api/views/source', (route) => {
      const path = new URL(route.request().url()).searchParams.get('path') ?? ''
      const large = path !== rulePath
      return route.fulfill({ json: { path, kind: large ? 'agent' : 'rule', frontmatter: [], body: '', bytes: large ? 400000 : 120, truncated: large, withheld: true } })
    })
    await page.goto('/?view=context')
    await selectTab(page, 'Rules')
    const row = page.locator('.item-row').filter({ has: page.getByRole('link', { name: /^Open testing in / }) })
    await row.getByText('Show content', { exact: true }).click()
    await expect(row).toContainText('The text is hidden because it looks like it holds a credential.')
    await expect(row).not.toContainText('The file has no text after its frontmatter.')

    await selectTab(page, 'Agents')
    const agent = page.locator('.item-row').filter({ has: page.getByRole('link', { name: /^Open reviewer in / }) })
    await agent.getByText('Show content', { exact: true }).click()
    await expect(agent).toContainText('The file is too large to check for credentials')
    await expect(agent).not.toContainText('Showing the first 256 KB')
  })

  test('zooms a hook from its summary to its key fields and its full spec', async ({ page, context }) => {
    const hookPath = '/path/to/agnostic-ai/.agnostic-ai/hooks/guard.yaml'
    const secretPath = '/path/to/agnostic-ai/.agnostic-ai/hooks/secret.yaml'
    const data = structuredClone(overview)
    const lifecycle = data.sections.find((section: { id: string }) => section.id === 'lifecycle')
    lifecycle.items = [
      { kind: 'hook', name: 'guard', label: 'guard', description: 'Blocks force pushes.', path: '.agnostic-ai/hooks/guard.yaml', openPath: hookPath, layer: 'project', group: 'before-tool', values: [], fields: [{ key: 'on', type: 'text', value: 'before-tool' }, { key: 'match', type: 'text', value: 'shell' }, { key: 'command', type: 'text', value: './guard.sh' }, { key: 'timeout', type: 'text', value: '10' }], findings: [] },
      { kind: 'hook', name: 'mine', label: 'mine', description: 'Waits longer on this machine.', path: '.agnostic-ai/local/hooks/mine.yaml', openPath: '/path/to/agnostic-ai/.agnostic-ai/local/hooks/mine.yaml', layer: 'project-user', group: 'before-tool', values: [], fields: [{ key: 'command', type: 'text', value: './mine.sh' }], findings: [] },
      { kind: 'hook', name: 'secret', label: 'secret', description: 'Calls the deploy API.', path: '.agnostic-ai/hooks/secret.yaml', openPath: secretPath, layer: 'project', group: 'before-tool', values: [], fields: [{ key: 'command', type: 'text', value: './deploy.sh' }], findings: [] },
      { kind: 'hook', name: 'memory-hook', label: 'memory-hook', description: '', path: 'builtin:memory', layer: 'builtin', group: 'before-tool', values: [], fields: [{ key: 'command', type: 'text', value: 'agnostic-ai hook memory' }], findings: [] },
    ]
    lifecycle.kinds[0].count = 4
    const requests: string[] = []
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.route((url) => url.pathname === '/api/views/source', (route) => {
      const path = new URL(route.request().url()).searchParams.get('path') ?? ''
      requests.push(path)
      if (path === secretPath) return route.fulfill({ json: { path, kind: 'hook', frontmatter: [], body: '', bytes: 90, truncated: false, withheld: true } })
      return route.fulfill({ json: { path, kind: 'hook', frontmatter: [{ key: 'name', value: 'guard' }, { key: 'targets', value: 'claude, codex' }, { key: 'command', value: './guard.sh' }, { key: 'timeout', value: '10' }], body: '', bytes: 120, truncated: false } })
    })
    await page.goto('/?view=lifecycle')
    await page.locator('details.stage').filter({ hasText: 'Before a tool runs' }).locator('summary').click()
    const row = page.locator('.item-row').filter({ hasText: 'Blocks force pushes.' })
    const zoom = row.getByRole('group', { name: 'Details of guard' })
    await expect(zoom.getByRole('button', { name: 'Summary' })).toHaveAttribute('aria-pressed', 'true')
    await expect(row.getByRole('link', { name: /^Open guard in / })).toBeVisible()
    await expect(row.getByRole('button', { name: 'Copy path of guard' })).toBeVisible()
    await expect(row.locator('.hook-fields')).toHaveCount(0)

    await zoom.getByRole('button', { name: 'Key fields' }).click()
    const keyFields = row.getByLabel('Key fields of guard')
    await expect(keyFields).toContainText('match')
    await expect(keyFields).toContainText('./guard.sh')
    expect(requests).toEqual([])

    await zoom.getByRole('button', { name: 'Full spec' }).click()
    const full = row.getByLabel('Full spec of guard')
    await expect(full).toContainText('claude, codex')
    await expect(full).toContainText('timeout')
    await saveScreenshot(page, 'hook-full-spec.png')

    await page.getByRole('button', { name: 'Collapse all', exact: true }).click()
    await page.getByRole('button', { name: 'Expand all', exact: true }).click()
    await expect(zoom.getByRole('button', { name: 'Full spec' })).toHaveAttribute('aria-pressed', 'true')
    await zoom.getByRole('button', { name: 'Summary' }).click()
    await expect(row.locator('.hook-fields')).toHaveCount(0)
    await zoom.getByRole('button', { name: 'Full spec' }).click()
    await expect(full).toBeVisible()
    expect(requests).toEqual([hookPath])

    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await row.getByRole('button', { name: 'Copy path of guard' }).click()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(hookPath)

    const builtin = page.locator('.item-row').filter({ hasText: 'builtin:memory' })
    const builtinZoom = builtin.getByRole('group', { name: 'Details of memory-hook' })
    await expect(builtinZoom.getByRole('button')).toHaveText(['Summary', 'Key fields'])
    await builtinZoom.getByRole('button', { name: 'Key fields' }).click()
    await expect(builtin.getByLabel('Key fields of memory-hook')).toContainText('agnostic-ai hook memory')

    const mine = page.locator('.item-row').filter({ hasText: 'Waits longer on this machine.' })
    const mineZoom = mine.getByRole('group', { name: 'Details of mine' })
    await expect(mineZoom.getByRole('button')).toHaveText(['Summary', 'Key fields', 'Fields this file sets'])
    await mineZoom.getByRole('button', { name: 'Fields this file sets' }).click()
    await expect(mine.getByLabel('Fields this file sets of mine')).toBeVisible()

    const secret = page.locator('.item-row').filter({ hasText: 'Calls the deploy API.' })
    await secret.getByRole('group', { name: 'Details of secret' }).getByRole('button', { name: 'Full spec' }).click()
    await expect(secret).toContainText('The text is hidden because it looks like it holds a credential.')
    await expect(secret).not.toContainText('does not read as YAML')
  })

  test('hides the toggle without a server', async ({ page }) => {
    await page.goto('/?mock=1&view=context')
    await selectTab(page, 'Rules')
    const row = page.locator('.item-row').filter({ hasText: 'testing' })
    await expect(row.locator('.item-path')).toHaveText('.agnostic-ai/rules/testing.md')
    await expect(row.getByText('Show content')).toHaveCount(0)
  })
})
