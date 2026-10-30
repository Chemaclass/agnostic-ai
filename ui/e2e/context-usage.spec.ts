import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { selectTab } from './helpers'

const overview = JSON.parse(readFileSync(new URL('../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))
const contextView = JSON.parse(readFileSync(new URL('../src/fixtures/context.mock.json', import.meta.url), 'utf8'))

test.describe('context size', () => {
  test('shows the mock duplicate with its sources, review prompt, and refresh', async ({ page, context }) => {
    await page.goto('/?mock=1')
    await selectTab(page, 'Context size')
    const panel = page.locator('section.context-usage')
    await expect(panel.getByRole('heading', { name: 'Context size' })).toBeVisible()
    await expect(panel.getByLabel('Coding tool')).toHaveValue('claude')
    await expect(panel.locator('.context-usage-totals')).toContainText('184 words, 1212 bytes')
    const duplicate = panel.locator('.context-opportunity')
    await expect(duplicate).toHaveCount(1)
    await expect(duplicate).toContainText('Run the full test suite before you push.')
    await expect(duplicate).toContainText('Could drop 18 words, 94 bytes from startup content for claude.')
    await expect(duplicate.getByRole('list', { name: 'Sources' })).toContainText('.agnostic-ai/rules/testing.md')
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await duplicate.getByRole('button', { name: /^Copy review prompt/ }).click()
    await expect(duplicate.getByRole('button', { name: 'Copied', exact: true })).toBeVisible()
    const prompt = await page.evaluate(() => navigator.clipboard.readText())
    expect(prompt).toContain('Target: claude')
    expect(prompt).toContain('Do not apply any change until I agree.')
    await panel.getByRole('button', { name: 'Refresh' }).click()
    await expect(panel.locator('.context-usage-compare')).toContainText('Before refresh')
    await expect(panel.locator('.context-usage-compare')).toContainText('Now')
    await expect(panel).not.toContainText(/tokens? saved|faster/i)
  })

  test('refreshes the same target and shows the updated findings', async ({ page }) => {
    const after = structuredClone(contextView)
    after.report.startup = { words: 166, bytes: 1118 }
    after.report.opportunities = []
    const requests: string[] = []
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => {
      requests.push(new URL(route.request().url()).search)
      route.fulfill({ json: requests.length === 1 ? contextView : after })
    })
    await page.goto('/?view=context')
    await selectTab(page, 'Context size')
    const panel = page.locator('section.context-usage')
    await expect(panel.locator('.context-opportunity')).toHaveCount(1)
    await panel.getByRole('button', { name: 'Refresh' }).click()
    await expect(panel).toContainText('No duplicated passages found for claude.')
    await expect(panel.locator('.context-usage-compare')).toContainText('Before refresh184 words, 1212 bytes at startup, 1 duplicated passage.')
    await expect(panel.locator('.context-usage-compare')).toContainText('Now166 words, 1118 bytes at startup, 0 duplicated passages.')
    expect(requests).toEqual(['', '?target=claude'])
  })

  test('counts scoped rules for a named file and keeps the file on refresh', async ({ page }) => {
    const scoped = structuredClone(contextView)
    scoped.report.file = 'src/main.go'
    scoped.report.file_scope = { words: 46, bytes: 301 }
    scoped.report.on_demand = { words: 270, bytes: 1783 }
    scoped.report.contributions = scoped.report.contributions.map((c: { category: string; load: string }) => c.category === 'conditional rule body' ? { ...c, category: 'scoped rule', load: 'file-scope' } : c)
    const requests: string[] = []
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => {
      const search = new URL(route.request().url()).search
      requests.push(search)
      route.fulfill({ json: search.includes('file=') ? scoped : contextView })
    })
    await page.goto('/?view=context')
    await selectTab(page, 'Context size')
    const panel = page.locator('section.context-usage')
    await expect(panel).toContainText('Scoped rules count as on demand until you name a file.')
    await expect(panel.locator('.context-usage-totals')).not.toContainText('File-scoped')
    await panel.getByLabel('Working on file').fill('src/main.go')
    await panel.getByRole('button', { name: 'Refresh' }).click()
    await expect(panel.locator('.context-usage-totals')).toContainText('File-scoped, additional46 words, 301 bytes')
    await expect(panel).not.toContainText('Scoped rules count as on demand')
    await panel.getByText(/sources, largest first/).click()
    await expect(panel.getByRole('table', { name: /^File-scoped/ })).toContainText('scoped rule')
    await panel.getByRole('button', { name: 'Refresh' }).click()
    await expect.poll(() => requests.length).toBe(3)
    expect(requests.slice(1)).toEqual(['?target=claude&file=src%2Fmain.go', '?target=claude&file=src%2Fmain.go'])
  })

  test('hides the previous tool report when another tool fails', async ({ page }) => {
    const two = { ...structuredClone(contextView), targets: ['claude', 'gemini'] }
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => {
      if (new URL(route.request().url()).searchParams.get('target') === 'gemini') return route.fulfill({ status: 400, json: { error: 'explain --context: gemini is not a configured target' } })
      return route.fulfill({ json: two })
    })
    await page.goto('/?view=context')
    await selectTab(page, 'Context size')
    const panel = page.locator('section.context-usage')
    await expect(panel.locator('.context-opportunity')).toHaveCount(1)
    await panel.getByLabel('Coding tool').selectOption('gemini')
    await expect(panel.getByRole('alert')).toContainText('gemini is not a configured target')
    await expect(panel.locator('.context-opportunity')).toHaveCount(0)
    await expect(panel.locator('.context-usage-totals')).toHaveCount(0)
    await panel.getByLabel('Coding tool').selectOption('claude')
    await expect(panel.locator('.context-opportunity')).toHaveCount(1)
  })

  test('drops the file for a tool without file-scoped rules', async ({ page }) => {
    const two = { ...structuredClone(contextView), targets: ['claude', 'copilot'], fileTargets: ['claude'] }
    const requests: string[] = []
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => {
      const url = new URL(route.request().url())
      requests.push(url.search)
      return route.fulfill({ json: { ...two, target: url.searchParams.get('target') ?? 'claude' } })
    })
    await page.goto('/?view=context')
    await selectTab(page, 'Context size')
    const panel = page.locator('section.context-usage')
    await expect(panel.locator('.context-opportunity')).toHaveCount(1)
    await panel.getByLabel('Working on file').fill('src/main.go')
    await panel.getByRole('button', { name: 'Refresh' }).click()
    await expect.poll(() => requests.at(-1)).toBe('?target=claude&file=src%2Fmain.go')
    await panel.getByLabel('Coding tool').selectOption('copilot')
    await expect.poll(() => requests.at(-1)).toBe('?target=copilot')
    await expect(panel.getByLabel('Working on file')).toBeDisabled()
  })

  test('shows the reason for user configuration', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => route.fulfill({ json: { schemaVersion: '1', supported: false, reason: 'Context size is reported for a project. Open a project to see it.', targets: [], target: '' } }))
    await page.goto('/?view=context')
    await selectTab(page, 'Context size')
    const panel = page.locator('section.context-usage')
    await expect(panel).toContainText('Open a project to see it.')
    await expect(panel.getByLabel('Coding tool')).toHaveCount(0)
  })
})
