import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { selectTab } from './helpers'

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

  test('hides the toggle without a server', async ({ page }) => {
    await page.goto('/?mock=1&view=context')
    await selectTab(page, 'Rules')
    const row = page.locator('.item-row').filter({ hasText: 'testing' })
    await expect(row.locator('.item-path')).toHaveText('.agnostic-ai/rules/testing.md')
    await expect(row.getByText('Show content')).toHaveCount(0)
  })
})
