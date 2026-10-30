import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { normalizeOverviewView } from '../src/model/normalize'

for (const width of [320, 390, 1440]) {
  test(`changes the visible built-in, model, and native values with the selected audience at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    const data = normalizeOverviewView(JSON.parse(readFileSync(new URL('../src/fixtures/overview.mock.json', import.meta.url), 'utf8')))
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
    await expect(verbose).toHaveAttribute('aria-checked', 'false')
    await expect(verbose).toBeDisabled()
    await expect(model).toHaveValue('sonnet')
    await expect(model).toBeDisabled()
    await expect(cavemanRow.locator('.override-comparison')).toContainText('Shared: Off')
    await expect(cavemanRow.locator('.override-comparison')).toContainText('Personal: On')
    await expect(builtins.getByRole('button', { name: /^Reset/ })).toHaveCount(0)

    await builtins.getByRole('button', { name: 'View personal settings' }).click()
    await expect(caveman).toHaveAttribute('aria-checked', 'true')
    await expect(memory).toHaveAttribute('aria-checked', 'false')
    await expect(caveman).toBeEnabled()
    const name = page.locator('.item-row').filter({ has: caveman }).locator('.item-main > strong')
    expect(await name.evaluate((element) => {
      const range = document.createRange()
      range.selectNodeContents(element)
      return range.getClientRects().length
    })).toBe(1)
    await expect(verbose).toHaveAttribute('aria-checked', 'true')
    await expect(verbose).toBeEnabled()
    await expect(model).toHaveValue('opus')
    await expect(model).toBeEnabled()
    await expect(page.locator('.edit-scope-hint')).toContainText('personal overrides take priority over Shared')
    const modelRow = page.locator('.field-row').filter({ has: model })
    const modelComparison = modelRow.locator('.field-edit').first().locator('.override-comparison')
    await expect(modelComparison).toContainText('Shared: sonnet')
    await expect(modelComparison).toContainText('Personal: opus')
    const verboseRow = page.locator('.value-row').filter({ has: verbose })
    await expect(verboseRow.locator('.override-comparison')).toContainText('Shared: Off')
    await expect(verboseRow.locator('.override-comparison')).toContainText('Personal: On')
    await expect(page.locator('.field-row').filter({ has: page.getByText('Native keys', { exact: true }) })).toContainText('light')
    await expect(builtins.getByRole('heading', { level: 2 })).toContainText('personal')
    expect(edits).toEqual([])

    await page.reload()
    await expect(page.getByRole('button', { name: 'Personal (not committed)', exact: true })).toHaveAttribute('aria-pressed', 'true')
    await expect(caveman).toHaveAttribute('aria-checked', 'true')
    await verbose.click()
    expect(edits).toEqual([{ audience: 'personal', path: personal, key: 'x-claude.verbose', value: false, expected: true }])
  })
}
