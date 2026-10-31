import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { itemEntryId } from '../src/model/identity'
import { normalizeOverviewView } from '../src/model/normalize'
import { selectTab } from './helpers'

function tabFixture() {
  const data = normalizeOverviewView(JSON.parse(readFileSync(new URL('../src/fixtures/overview.mock.json', import.meta.url), 'utf8')))
  const tools = data.sections.find((section) => section.id === 'tools')!
  tools.native = [{ name: 'notion', holders: [{ target: 'claude', label: 'Claude Code', path: '~/.claude.json', scope: 'user', transport: 'http' }] }]
  tools.kinds.push({ kind: 'environment', label: 'Environments', count: 1 })
  tools.items.push({ kind: 'environment', name: 'environment-bootstrap', label: 'environment-bootstrap', description: 'Prepare the project environment.', path: '.agnostic-ai/environments/dev.yaml', layer: 'project', values: [], findings: [] })
  const context = data.sections.find((section) => section.id === 'context')!
  context.kinds.find((kind) => kind.kind === 'instructions')!.count = 1
  context.items.push({ kind: 'instructions', name: 'AGNOSTIC_AI.md', label: 'AGNOSTIC_AI.md', description: 'Project instructions.', path: '.agnostic-ai/AGNOSTIC_AI.md', layer: 'project', values: [], findings: [] })
  const settings = data.sections.find((section) => section.id === 'settings')!
  Object.assign(settings.fields!.find((field) => field.key === 'model')!, {
    set: [{ spec: 'defaults', path: '.agnostic-ai/settings/defaults.yaml', values: [{ key: 'model', type: 'text', value: 'sonnet' }] }],
    editable: { kind: 'text', path: '.agnostic-ai/settings/defaults.yaml', localPath: '.agnostic-ai/local/settings/defaults.yaml' },
  })
  return data
}

async function loadTabs(page: Page) {
  const data = tabFixture()
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true } }))
  await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: { schemaVersion: 'builtin-change/v1', status: 'idle', enabled: false, supported: false, undoAvailable: false, targets: [], changes: [], findings: [] } }))
  return data
}

test('switches between configured servers and native servers without mixing their content', async ({ page }) => {
  await loadTabs(page)
  await page.goto('/?view=tools')
  const configured = page.getByRole('tab', { name: /^Configured servers/ })
  const native = page.getByRole('tab', { name: /^Found in tools/ })
  await expect(configured).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('region', { name: /^MCP servers/ }).getByText('github', { exact: true })).toBeVisible()
  await expect(page.getByRole('region', { name: /Found in a tool's own config/ })).toHaveCount(0)
  await native.click()
  await expect(native).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('region', { name: /Found in a tool's own config/ }).getByText('notion', { exact: true })).toBeVisible()
  await expect(page.getByRole('region', { name: /^MCP servers/ })).toHaveCount(0)
  await expect(page.getByRole('tabpanel', { includeHidden: true })).toHaveCount(3)
  await expect(page.locator('[role="tabpanel"][hidden]')).toHaveCount(2)
  await configured.click()
  await expect(page.getByRole('region', { name: /^MCP servers/ }).getByText('github', { exact: true })).toBeVisible()
})

test('keeps the tab in reload and browser history and resets it when changing pages', async ({ page }) => {
  await loadTabs(page)
  await page.goto('/?view=tools')
  await selectTab(page, 'Found in tools')
  await expect(page).toHaveURL(/tab=native/)
  await selectTab(page, 'Environments')
  await expect(page).toHaveURL(/tab=environment/)
  await page.reload()
  await expect(page.getByRole('tab', { name: /^Environments/ })).toHaveAttribute('aria-selected', 'true')
  await page.goBack()
  await expect(page.getByRole('tab', { name: /^Found in tools/ })).toHaveAttribute('aria-selected', 'true')
  await page.goBack()
  await expect(page.getByRole('tab', { name: /^Configured servers/ })).toHaveAttribute('aria-selected', 'true')
  await selectTab(page, 'Found in tools')
  await page.getByRole('navigation', { name: 'Configuration sections' }).getByRole('link', { name: /^Context/ }).click()
  expect(new URL(page.url()).searchParams.has('tab')).toBe(false)
  await expect(page.getByRole('tab', { name: /^Guidance/ })).toHaveAttribute('aria-selected', 'true')
  await page.goto('/?view=tools&tab=unknown-panel')
  await expect(page.getByRole('tab', { name: /^Configured servers/ })).toHaveAttribute('aria-selected', 'true')
})

test('activates tabs with arrow, Home and End keys and connects each tab to its panel', async ({ page }) => {
  await loadTabs(page)
  await page.goto('/?view=tools')
  const configured = page.getByRole('tab', { name: /^Configured servers/ })
  const native = page.getByRole('tab', { name: /^Found in tools/ })
  const environment = page.getByRole('tab', { name: /^Environments/ })
  await configured.focus()
  await page.keyboard.press('ArrowRight')
  await expect(native).toBeFocused()
  await expect(native).toHaveAttribute('aria-selected', 'true')
  await expect(native).toHaveAttribute('tabindex', '0')
  await expect(configured).toHaveAttribute('tabindex', '-1')
  const panel = page.getByRole('tabpanel', { name: /^Found in tools/ })
  await expect(panel).toHaveAttribute('id', (await native.getAttribute('aria-controls'))!)
  await expect(panel).toHaveAttribute('aria-labelledby', (await native.getAttribute('id'))!)
  await page.keyboard.press('End')
  await expect(environment).toBeFocused()
  await expect(environment).toHaveAttribute('aria-selected', 'true')
  await page.keyboard.press('Home')
  await expect(configured).toBeFocused()
  await expect(configured).toHaveAttribute('aria-selected', 'true')
  await page.keyboard.press('ArrowLeft')
  await expect(environment).toBeFocused()
  await expect(environment).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('tabpanel')).toHaveCount(1)
})

test('search selects matching tabs and clearing search keeps the current panel', async ({ page }) => {
  await loadTabs(page)
  await page.goto('/?view=tools&q=notion')
  await expect(page.getByRole('tab', { name: /^Found in tools/ })).toHaveAttribute('aria-selected', 'true')
  await selectTab(page, 'Configured servers')
  await expect(page.getByRole('searchbox', { name: 'Search tools' })).toHaveValue('notion')
  await expect(page.getByRole('tab', { name: /^Configured servers/ })).toHaveAttribute('aria-selected', 'true')
  await page.getByRole('searchbox', { name: 'Search tools' }).fill('environment-bootstrap')
  expect(new URL(page.url()).searchParams.has('tab')).toBe(false)
  await expect(page.getByRole('tab', { name: /^Environments/ })).toHaveAttribute('aria-selected', 'true')
  await page.getByRole('button', { name: 'Clear search', exact: true }).click()
  await expect(page.getByRole('tab', { name: /^Environments/ })).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('searchbox', { name: 'Search tools' })).toBeFocused()
  await expect(page.getByRole('searchbox', { name: 'Search tools' })).toHaveValue('')
})

test('keeps a native server visible when search matches the holder scope', async ({ page }) => {
  await loadTabs(page)
  await page.goto('/?view=tools&q=user')
  const tab = page.getByRole('tab', { name: /^Found in tools/ })
  await expect(tab).toHaveAttribute('aria-selected', 'true')
  await expect(tab.locator('.section-tab-count')).toHaveText('1 match')
  const native = page.getByRole('region', { name: /Found in a tool's own config/ })
  await expect(native.locator('.kind-count')).toHaveText('1 of 1 match')
  await expect(native.getByText('notion', { exact: true })).toBeVisible()
})

test('opens the owning tab for a deep entry and clears that entry on manual tab selection', async ({ page }) => {
  const data = await loadTabs(page)
  const github = data.sections.find((section) => section.id === 'tools')!.items.find((item) => item.name === 'github')!
  await page.goto(`/?view=tools&tab=native&q=notion&entry=${encodeURIComponent(itemEntryId(github))}`)
  await expect(page.getByRole('tab', { name: /^Configured servers/ })).toHaveAttribute('aria-selected', 'true')
  await expect(page.locator('.item-row.is-linked')).toBeFocused()
  await expect(page.locator('.item-row.is-linked')).toContainText('github')
  await selectTab(page, 'Found in tools')
  expect(new URL(page.url()).searchParams.has('entry')).toBe(false)
  await expect(page.getByRole('searchbox', { name: 'Search tools' })).toHaveValue('notion')
  await page.getByRole('button', { name: 'Search', exact: true }).click()
  await page.getByRole('combobox', { name: 'Search all configuration' }).fill('default model')
  await page.keyboard.press('Enter')
  await expect(page.getByRole('tab', { name: /^Preferences/ })).toHaveAttribute('aria-selected', 'true')
  await expect(page.locator('.field-row.is-linked')).toBeFocused()
  await expect(page.locator('.field-row.is-linked')).toContainText('Default model')
})

test('retains a personal model draft and audience while changing Settings tabs', async ({ page }) => {
  await loadTabs(page)
  const edits: unknown[] = []
  await page.route('**/api/edit/**', (route) => {
    edits.push(route.request().postDataJSON())
    return route.fulfill({ json: { ok: true } })
  })
  await page.goto('/?view=settings&tab=preferences')
  const personal = page.getByRole('button', { name: 'Personal (not committed)', exact: true })
  await personal.click()
  const model = page.getByRole('textbox', { name: 'Default model value', exact: true })
  await model.fill('opus')
  await selectTab(page, 'Built-ins')
  await expect(personal).toHaveAttribute('aria-pressed', 'true')
  await selectTab(page, 'Config files')
  await selectTab(page, 'Preferences')
  await expect(personal).toHaveAttribute('aria-pressed', 'true')
  await expect(model).toHaveValue('opus')
  await expect(page.getByRole('tabpanel', { includeHidden: true })).toHaveCount(3)
  expect(edits).toEqual([])
})

test('keeps a built-in proposal mounted without cancellation when changing tabs', async ({ page }) => {
  await loadTabs(page)
  let reads = 0
  const cancelled: unknown[] = []
  let state = { schemaVersion: 'builtin-change/v1', status: 'idle', enabled: false, supported: true, undoAvailable: false, targets: ['claude'], changes: [], findings: [] } as Record<string, unknown>
  await page.route('**/api/changes/builtin', (route) => { reads++; return route.fulfill({ json: state }) })
  await page.route('**/api/changes/builtin/preview', (route) => {
    state = { ...state, ...route.request().postDataJSON(), status: 'proposed', id: 'retained-preview', config: 'agnostic-ai.yaml' }
    return route.fulfill({ json: state })
  })
  await page.route('**/api/changes/builtin/cancel', (route) => { cancelled.push(route.request().postDataJSON()); return route.fulfill({ json: { ...state, status: 'idle' } }) })
  await page.goto('/?view=settings')
  await page.getByRole('switch', { name: 'caveman built-in', exact: true }).click()
  const preview = page.getByRole('region', { name: 'Caveman change preview', includeHidden: true })
  await expect(preview).toBeVisible()
  await selectTab(page, 'Preferences')
  await expect(preview).toHaveCount(1)
  await expect(preview).not.toBeVisible()
  await selectTab(page, 'Built-ins')
  await expect(preview).toBeVisible()
  await expect(preview.getByRole('button', { name: 'Apply change', exact: true })).toBeEnabled()
  expect(reads).toBe(1)
  expect(cancelled).toEqual([])
})

test('keeps support and section findings available outside panels and leaves Authority and Lifecycle continuous', async ({ page }) => {
  const data = await loadTabs(page)
  data.sections.find((section) => section.id === 'tools')!.findings = [{ source: 'doctor', severity: 'warn', message: 'Tools configuration warning.' }]
  await page.goto('/?view=tools')
  for (const tab of ['Configured servers', 'Found in tools', 'Environments']) {
    await selectTab(page, tab)
    await page.getByRole('button', { name: 'Review 1 finding', exact: true }).click()
    const finding = page.getByText('Tools configuration warning.', { exact: true })
    await expect(finding).toBeVisible()
    expect(await finding.evaluate((element) => element.closest('[role="tabpanel"]') === null)).toBe(true)
    const support = page.getByRole('table', { name: 'Coding tool support' })
    await expect(support).toBeVisible()
    expect(await support.evaluate((element) => element.closest('[role="tabpanel"]') === null)).toBe(true)
  }
  for (const view of ['Authority', 'Lifecycle']) {
    await page.getByRole('navigation', { name: 'Configuration sections' }).getByRole('link', { name: new RegExp(`^${view}`) }).click()
    await expect(page.getByRole('tab')).toHaveCount(0)
    await expect(page.getByRole('heading', { level: 1, name: view, exact: true })).toBeVisible()
  }
})

test('keeps every selected panel within a phone viewport', async ({ page }) => {
  await loadTabs(page)
  await page.setViewportSize({ width: 390, height: 844 })
  for (const [view, tabs] of [
    ['context', ['Guidance', 'Rules', 'Agents', 'Skills', 'Context size']],
    ['tools', ['Configured servers', 'Found in tools', 'Environments']],
    ['settings', ['Built-ins', 'Preferences', 'Config files']],
  ] as const) {
    await page.goto(`/?view=${view}`)
    for (const tab of tabs) {
      await selectTab(page, tab)
      await expect(page.getByRole('tab', { name: new RegExp(`^${tab}`) })).toBeInViewport({ ratio: 1 })
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      expect(await page.locator('#main').evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true)
    }
  }
})

test('keeps a directly linked Context size tab fully visible on a phone without moving focus', async ({ page }) => {
  await loadTabs(page)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/?view=context&tab=usage')
  const tab = page.getByRole('tab', { name: /^Context size/ })
  await expect(tab).toHaveAttribute('aria-selected', 'true')
  await expect(tab).toBeInViewport({ ratio: 1 })
  await expect(page.locator('body')).toBeFocused()
  await expect(page.getByRole('heading', { level: 1, name: 'Context', exact: true })).toBeInViewport()
})

test('reveals the active desktop tab after resizing to a phone without moving focus', async ({ page }) => {
  await loadTabs(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/?view=context&tab=usage')
  const tab = page.getByRole('tab', { name: /^Context size/ })
  await expect(tab).toHaveAttribute('aria-selected', 'true')
  await expect(tab).toBeInViewport({ ratio: 1 })
  await expect(page.locator('body')).toBeFocused()
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(tab).toBeInViewport({ ratio: 1 })
  await expect(page.locator('body')).toBeFocused()
  await expect(page.getByRole('heading', { level: 1, name: 'Context', exact: true })).toBeInViewport()
})

test('reveals the Skills tab on a phone when global search focuses a skill', async ({ page }) => {
  const data = await loadTabs(page)
  const context = data.sections.find((section) => section.id === 'context')!
  context.kinds.find((kind) => kind.kind === 'skill')!.count = 1
  context.items.push({ kind: 'skill', name: 'release-notes', label: 'release-notes', description: 'Prepare release notes.', path: '.agnostic-ai/skills/release-notes/SKILL.md', layer: 'project', values: [], findings: [] })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/?view=context&tab=guidance')
  await expect(page.getByRole('tab', { name: /^Guidance/ })).toHaveAttribute('aria-selected', 'true')
  await page.getByRole('button', { name: 'Search', exact: true }).click()
  await page.getByRole('combobox', { name: 'Search all configuration' }).fill('release-notes')
  await page.keyboard.press('Enter')
  const tab = page.getByRole('tab', { name: /^Skills/ })
  await expect(tab).toHaveAttribute('aria-selected', 'true')
  await expect(tab).toBeInViewport({ ratio: 1 })
  const skill = page.locator('.item-row.is-linked')
  await expect(skill).toContainText('release-notes')
  await expect(skill).toBeFocused()
  await expect(skill).toBeInViewport()
})
