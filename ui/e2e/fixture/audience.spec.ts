import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { normalizeOverviewView } from '../../src/model/normalize'
import { selectTab } from '../helpers'

for (const width of [320, 390, 1440]) {
  test(`changes the visible built-in, model, and native values with the selected audience at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    const data = normalizeOverviewView(JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8')))
    const settings = data.sections.find((section) => section.id === 'settings')!
    for (const item of settings.items) {
      if (item.name === 'caveman') Object.assign(item, { enabled: true, local: true })
      if (item.name === 'memory') Object.assign(item, { enabled: false, local: true })
    }
    settings.kinds.find((kind) => kind.kind === 'builtin')!.local = true
    const shared = '.agnostic-ai/settings/defaults.yaml'
    const personal = '.agnostic-ai/local/settings/defaults.yaml'
    Object.assign(settings.fields!.find((field) => field.key === 'model')!, {
      set: [
        { spec: 'defaults', path: personal, local: true, values: [{ key: 'model', type: 'text', value: 'opus' }] },
        { spec: 'defaults', path: shared, values: [{ key: 'model', type: 'text', value: 'sonnet' }] },
      ],
      editable: { kind: 'text', path: shared, localPath: personal },
    })
    Object.assign(settings.fields!.find((field) => field.key === 'x-')!, {
      set: [
        { spec: 'defaults', path: personal, local: true, values: [{ key: 'x-claude.verbose', type: 'bool', value: 'true' }] },
        { spec: 'defaults', path: shared, values: [{ key: 'x-claude.verbose', type: 'bool', value: 'false' }, { key: 'x-claude.theme', type: 'text', value: 'light' }] },
      ],
    })
    settings.items.push({ kind: 'settings', name: 'defaults', label: 'defaults', description: 'Personal defaults.', path: personal, layer: 'project-user', values: [], findings: [], localKeys: ['model', 'x-claude.verbose'] })
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: { schemaVersion: 'builtin-change/v1', status: 'idle', enabled: false, supported: false, undoAvailable: false, targets: [], changes: [], findings: [] } }))
    const edits: unknown[] = []
    await page.route('**/api/edit/**', (route) => {
      edits.push(route.request().postDataJSON())
      return route.fulfill({ json: { ok: true } })
    })
    await page.goto('/?view=settings')
    const caveman = page.getByRole('switch', { name: 'caveman built-in' })
    const memory = page.getByRole('switch', { name: 'memory built-in' })
    const verbose = page.getByRole('switch', { name: 'x-claude.verbose', exact: true })
    const model = page.getByRole('textbox', { name: 'Default model value', exact: true })
    const builtins = page.getByRole('region', { name: /^Built-ins/ })
    const cavemanRow = page.locator('.item-row').filter({ has: caveman })
    await expect(caveman).toHaveAttribute('aria-checked', 'false')
    await expect(memory).toHaveAttribute('aria-checked', 'true')
    await expect(caveman).toBeDisabled()
    await expect(cavemanRow.locator('.override-comparison')).toContainText('Project: Off')
    await expect(cavemanRow.locator('.override-comparison')).toContainText('Local: On')
    await expect(builtins.getByRole('button', { name: /^Reset/ })).toHaveCount(0)
    await selectTab(page, 'Preferences')
    await expect(verbose).toHaveAttribute('aria-checked', 'false')
    await expect(verbose).toBeDisabled()
    await expect(model).toHaveValue('sonnet')
    await expect(model).toBeDisabled()
    await selectTab(page, 'Built-ins')
    await builtins.getByRole('button', { name: 'View local settings' }).click()
    await expect(caveman).toHaveAttribute('aria-checked', 'true')
    await expect(memory).toHaveAttribute('aria-checked', 'false')
    await expect(caveman).toBeEnabled()
    const name = page.locator('.item-row').filter({ has: caveman }).locator('.item-main > strong')
    expect(await name.evaluate((element) => {
      const range = document.createRange()
      range.selectNodeContents(element)
      return range.getClientRects().length
    })).toBe(1)
    await selectTab(page, 'Preferences')
    await expect(verbose).toHaveAttribute('aria-checked', 'true')
    await expect(verbose).toBeEnabled()
    await expect(model).toHaveValue('opus')
    await expect(model).toBeEnabled()
    await expect(page.locator('.edit-scope-hint')).toContainText('Local files in .agnostic-ai/local, ignored by Git. Local values win.')
    const modelRow = page.locator('.field-row').filter({ has: model })
    const modelComparison = modelRow.locator('.field-edit').first().locator('.override-comparison')
    await expect(modelComparison).toContainText('Project: sonnet')
    await expect(modelComparison).toContainText('Local: opus')
    const verboseRow = page.locator('.value-row').filter({ has: verbose })
    await expect(verboseRow.locator('.override-comparison')).toContainText('Project: Off')
    await expect(verboseRow.locator('.override-comparison')).toContainText('Local: On')
    await expect(page.locator('.field-row').filter({ has: page.getByText('Native keys', { exact: true }) })).toContainText('light')
    await selectTab(page, 'Built-ins')
    await expect(builtins.locator('p .badge', { hasText: 'local' })).toBeVisible()
    await expect(builtins).toContainText('Your local config replaces the project list.')
    await selectTab(page, 'Preferences')
    expect(edits).toEqual([])

    await page.reload()
    await expect(page.getByRole('button', { name: 'Local (not committed)', exact: true })).toHaveAttribute('aria-pressed', 'true')
    await selectTab(page, 'Built-ins')
    await expect(caveman).toHaveAttribute('aria-checked', 'true')
    await selectTab(page, 'Preferences')
    await verbose.click()
    expect(edits).toEqual([{ audience: 'personal', path: personal, key: 'x-claude.verbose', value: false, expected: true }])
  })
}

test('shows one scope selector beside the filter row for every block on the page', async ({ page }) => {
  const data = normalizeOverviewView(JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8')))
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
  await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: { schemaVersion: 'builtin-change/v1', status: 'idle', enabled: false, supported: false, undoAvailable: false, targets: [], changes: [], findings: [] } }))
  await page.addInitScript(() => window.localStorage.setItem('agnostic-ai.edit-scope', 'shared'))
  for (const width of [390, 1440]) {
    await page.setViewportSize({ width, height: 900 })
    for (const [view, tab, label] of [['authority', '', 'Authority'], ['settings', 'builtins', 'Settings'], ['settings', 'targets', 'Settings'], ['settings', 'preferences', 'Settings']]) {
      await page.goto(`/?view=${view}${tab ? `&tab=${tab}` : ''}`)
      const filterRow = page.getByRole('search', { name: `Search ${label}` })
      const scope = filterRow.locator('.edit-scope')
      await expect(scope.getByRole('group', { name: 'View and edit' })).toBeVisible()
      await expect(page.locator('.edit-scope')).toHaveCount(1)
      await expect(page.locator('.block .edit-scope')).toHaveCount(0)
      const project = scope.getByRole('button', { name: 'Project (committed)', exact: true })
      await expect(project).toHaveText('Project')
      await expect(scope.getByRole('button', { name: 'Local (not committed)', exact: true })).toHaveText('Local')
      await filterRow.getByRole('searchbox').focus()
      await page.keyboard.press('Tab')
      await expect(project).toBeFocused()
      const search = await filterRow.getByRole('searchbox').boundingBox()
      const control = await scope.boundingBox()
      expect(control!.x).toBeGreaterThanOrEqual(0)
      expect(control!.x + control!.width).toBeLessThanOrEqual(width)
      if (width === 1440) expect(control!.x).toBeGreaterThan(search!.x + search!.width)
      else expect(control!.y).toBeGreaterThanOrEqual(search!.y + search!.height)
    }
    await page.goto('/?view=settings&tab=configuration')
    await expect(page.locator('.edit-scope')).toHaveCount(0)
  }
})

test('one scope choice changes every editor on the page', async ({ page }) => {
  const data = normalizeOverviewView(JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8')))
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
  await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: { schemaVersion: 'builtin-change/v1', status: 'idle', enabled: false, supported: false, undoAvailable: false, targets: [], changes: [], findings: [] } }))
  await page.goto('/?view=settings&tab=builtins')
  await page.getByRole('button', { name: 'Local (not committed)', exact: true }).click()
  await expect(page.getByText('Inherited from project. Changing a built-in creates your local list.')).toBeVisible()
  for (const tab of ['Coding tools', 'Preferences']) {
    await selectTab(page, tab)
    await expect(page.getByRole('button', { name: 'Local (not committed)', exact: true })).toHaveAttribute('aria-pressed', 'true')
    await expect(page.locator('.edit-scope-hint')).toContainText('Local files in .agnostic-ai/local')
  }
  await selectTab(page, 'Coding tools')
  await expect(page.locator('.coding-tools-hint')).toContainText('Changes save to agnostic-ai.local.yaml.')
  await page.goto('/?view=authority')
  await expect(page.getByRole('button', { name: 'Local (not committed)', exact: true })).toHaveAttribute('aria-pressed', 'true')
})

test('names the global level and its local layer in the global view', async ({ page }) => {
  const data = normalizeOverviewView(JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8')))
  data.global = true
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
  await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: { schemaVersion: 'builtin-change/v1', status: 'idle', enabled: false, supported: false, undoAvailable: false, targets: [], changes: [], findings: [] } }))
  await page.addInitScript(() => window.localStorage.setItem('agnostic-ai.edit-scope', 'shared'))
  await page.goto('/?view=settings&tab=builtins')
  const scope = page.locator('.edit-scope')
  await expect(scope.getByRole('button', { name: 'Global (every project)', exact: true })).toHaveText('Global')
  await expect(scope.getByRole('button', { name: 'Local (not committed)', exact: true })).toHaveText('Local')
  await expect(scope.locator('.edit-scope-hint')).toHaveText('Global files in ~/.agnostic-ai, used by every project. Local values win for you.')
  await scope.getByRole('button', { name: 'Local (not committed)', exact: true }).click()
  await expect(scope.locator('.edit-scope-hint')).toHaveText('Local files in ~/.agnostic-ai/local, ignored by Git. Local values win.')
  await expect(scope).not.toContainText(/\b(shared|personal)\b/i)
})
