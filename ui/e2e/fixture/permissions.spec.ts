import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import type { PermissionEditRequest } from '../../src/contract'
import { normalizeOverviewView } from '../../src/model/normalize'

const shared = '.agnostic-ai/settings/permissions.yaml'
const personal = '.agnostic-ai/local/settings/permissions.yaml'

async function setup(page: Page, override = false) {
  const data = normalizeOverviewView(JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8')))
  const authority = data.sections.find((section) => section.id === 'authority')!
  authority.items = []
  authority.findings = []
  for (const field of authority.fields!) {
    field.set = []
    if (['permissions.allow', 'permissions.ask', 'permissions.deny'].includes(field.key)) {
      field.editable = { kind: 'permission-list', path: shared, localPath: personal }
      if (override && field.key === 'permissions.ask') field.set = [
        { spec: 'permissions', path: shared, values: [{ key: field.key, type: 'list', list: ['shell(git push *)'] }] },
        { spec: 'permissions', path: personal, local: true, values: [{ key: field.key, type: 'list', list: ['shell(git push origin main)'] }] },
      ]
    }
  }
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
  const requests: PermissionEditRequest[] = []
  await page.route('**/api/edit/permissions', (route) => {
    const body = route.request().postDataJSON() as PermissionEditRequest
    requests.push(body)
    const file = body.audience === 'personal' ? personal : shared
    if (body.action === 'apply') {
      const field = authority.fields!.find((field) => field.key === body.key)!
      field.set = field.set.filter((source) => source.path !== file)
      if (body.rules !== null) field.set.push({ spec: 'permissions', path: file, ...(body.audience === 'personal' ? { local: true } : {}), values: [{ key: body.key, type: 'list', list: body.rules }] })
    }
    return route.fulfill({ json: {
      schemaVersion: 'permission-edit/v1', file, key: body.key, audience: body.audience, before: body.before, rules: body.rules,
      diff: `--- ${file}\n+++ ${file}\n+permissions:\n+  ask: ${JSON.stringify(body.rules)}`, expected: 'preview-token', syncCommand: 'agnostic-ai sync', ...(body.action === 'apply' ? { ok: true } : {}),
    } })
  })
  await page.goto('/?view=authority')
  await expect(page.getByRole('heading', { name: 'Authority', exact: true })).toBeVisible()
  return { data, authority, requests }
}


const ask = (page: Page) => page.getByRole('group', { name: 'Ask first', exact: true })

async function addRule(page: Page, list = 'Ask first') {
  await page.getByRole('button', { name: 'Add a rule', exact: true }).click()
  await page.getByRole('menuitem', { name: list, exact: true }).click()
}

for (const width of [390, 1440]) {
  test(`adds permission rules with an explicit preview and source save at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    const { requests } = await setup(page)
    await expect(page.getByRole('heading', { name: 'No rules yet.' })).toBeVisible()
    await expect(page.getByText('permissions.allow')).toHaveCount(0)
    await addRule(page)
    const row = ask(page)
    const rules = row.getByRole('textbox', { name: 'Ask first rules', exact: true })
    await expect(rules).toBeFocused()
    await expect(rules).toHaveAttribute('rows', '4')
    await expect(rules).toHaveAttribute('placeholder', /shell\(git status\)\nread\(src\/\*\*\)/)
    await expect(row).toContainText(`Project file: ${shared}`)
    await rules.fill('shell(git push *)\n\nread(private/**)')
    await row.getByRole('button', { name: 'Preview', exact: true }).click()
    const preview = row.getByRole('region', { name: 'Ask first change preview' })
    await expect(preview).toContainText('Project (committed)')
    await expect(preview).toContainText(shared)
    await expect(preview.locator('.diff-added')).toHaveText(['+permissions:', '+  ask: ["shell(git push *)","read(private/**)"]'])
    await expect(preview.locator('.diff-meta')).toHaveCount(2)
    await expect(preview).toContainText('Apply saves this file. Then run agnostic-ai sync to update your tools.')
    await expect(preview.getByRole('button', { name: 'Edit', exact: true })).toBeVisible()
    await expect(preview.getByRole('button', { name: 'Cancel', exact: true })).toHaveCount(0)
    expect(requests).toEqual([{ action: 'preview', audience: 'shared', path: shared, key: 'permissions.ask', rules: ['shell(git push *)', 'read(private/**)'], before: null }])
    await preview.getByRole('button', { name: 'Apply', exact: true }).click()
    const saved = page.getByRole('status').filter({ hasText: /^Saved / })
    await expect(saved.locator('.banner-title')).toHaveText(`Saved ${shared}`)
    await expect(saved).toContainText('Run agnostic-ai sync to update your tools.')
    await expect(row.getByRole('list', { name: 'Saved Ask first rules' })).toBeFocused()
    expect(requests[1]).toMatchObject({ action: 'apply', expected: 'preview-token', before: null })
    await page.reload()
    await expect(row.getByRole('listitem')).toHaveText([/shell\(git push \*\)\??Ask first\s*Project/, /read\(private\/\*\*\)\??Ask first\s*Project/])
    await expect(page.locator('.section-search').getByRole('button', { name: 'Add a rule', exact: true })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })
}

test('lists every rule in one table, blocked first, with Edit and Remove on hover', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const { authority } = await setup(page)
  const list = (key: string, rules: string[]) => [{ spec: 'permissions', path: shared, values: [{ key, type: 'list' as const, list: rules }] }]
  authority.fields!.find((field) => field.key === 'permissions.allow')!.set = list('permissions.allow', ['shell(go test *)'])
  authority.fields!.find((field) => field.key === 'permissions.ask')!.set = list('permissions.ask', ['shell(git push *)'])
  authority.fields!.find((field) => field.key === 'permissions.deny')!.set = list('permissions.deny', ['edit(.env)', 'shell(rm *)'])
  authority.items = [{ kind: 'settings', name: 'permissions', label: 'permissions', description: '', path: shared, layer: 'project', findings: [], values: [
    { key: 'permissions.allow', type: 'list', list: ['shell(go test *)'] }, { key: 'permissions.ask', type: 'list', list: ['shell(git push *)'] }, { key: 'permissions.deny', type: 'list', list: ['edit(.env)', 'shell(rm *)'] },
  ] }]
  await page.reload()
  await expect(page.locator('.page-verdict')).toHaveText('2 actions are blocked and agents ask before 1.')
  await expect(page.locator('.page-facts')).toContainText('1 runs without asking')
  await expect(page.locator('.authority-rule')).toHaveText([/edit\(\.env\)\s*Blocked/, /shell\(rm \*\)\s*Blocked/, /shell\(git push \*\)\??Ask first/, /shell\(go test \*\)\s*Allowed/])
  const edit = page.getByRole('button', { name: 'Edit rule shell(rm *)' })
  await expect(edit).toHaveCSS('opacity', '0')
  await page.locator('.authority-rule').nth(1).hover()
  await expect(edit).toHaveCSS('opacity', '1')
  await page.getByRole('searchbox', { name: 'Search authority' }).fill('git push')
  await expect(page.locator('.authority-rule')).toHaveText([/shell\(git push \*\)/])
  await expect(page.getByRole('heading', { name: 'Also on this page' })).toHaveCount(0)
})

test('shows protected paths and the default mode as quiet rows under the table', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await setup(page)
  const also = page.locator('.authority-also-row')
  await expect(also).toHaveText([/Protected paths\s*Files agents must not edit without asking\.\s*None/, /Default permission mode\s*The mode a session starts in\. Claude Code only\.\s*Not set/])
})

test('shows personal replacement, compares values, and previews resetting one list', async ({ page }) => {
  const { requests } = await setup(page, true)
  const row = ask(page)
  await expect(row).toContainText('Ask first: local overrides this file’s project rules.')
  await expect(row.getByRole('button', { name: /^Edit rule / })).toHaveCount(0)
  await row.getByText('Compare project (1) and local (1)').click()
  await expect(row.locator('.permission-comparison').filter({ hasText: 'Local' })).toContainText('shell(git push origin main)')
  await row.getByRole('button', { name: 'View local settings' }).click()
  await expect(row.locator('.authority-rule')).toHaveText([/shell\(git push origin main\)\??Ask first\s*Local/])
  await row.getByRole('button', { name: 'Reset to the project rules', exact: true }).click()
  const preview = row.getByRole('region', { name: 'Ask first change preview' })
  await expect(preview).toContainText('Use the project rules')
  expect(requests).toEqual([{ action: 'preview', audience: 'personal', path: shared, key: 'permissions.ask', rules: null, before: ['shell(git push origin main)'] }])
  await preview.getByRole('button', { name: 'Apply', exact: true }).click()
  await expect(row.locator('.authority-rule')).toHaveText([/shell\(git push \*\)\??Ask first\s*Project/])
  await expect(row.getByRole('button', { name: 'Reset to the project rules' })).toHaveCount(0)
  await row.getByRole('button', { name: 'Edit rule shell(git push *)' }).click()
  await expect(row.getByRole('textbox', { name: 'Ask first rules' })).toHaveValue('shell(git push *)')
  await expect(row).toContainText('Starts with the project rules.')
  await row.getByRole('textbox', { name: 'Ask first rules' }).fill('shell(git push origin release)')
  await row.getByRole('button', { name: 'Preview', exact: true }).click()
  expect(requests.at(-1)).toMatchObject({ audience: 'personal', before: null, rules: ['shell(git push origin release)'] })
})

test('waits for the refreshed rules before offering the next change', async ({ page }) => {
  const { data } = await setup(page, true)
  const row = ask(page)
  await row.getByRole('button', { name: 'View local settings' }).click()
  let release = () => {}
  const held = new Promise<void>((resolve) => { release = resolve })
  await page.route('**/api/views/overview', async (route) => { await held; await route.fulfill({ json: data }) })
  await row.getByRole('button', { name: 'Edit rule shell(git push origin main)' }).click()
  await row.getByRole('textbox', { name: 'Ask first rules', exact: true }).fill('shell(git push origin release)')
  await row.getByRole('button', { name: 'Preview', exact: true }).click()
  await row.getByRole('button', { name: 'Apply', exact: true }).click()
  const reset = row.getByRole('button', { name: 'Reset to the project rules', exact: true })
  await expect(reset).toBeDisabled()
  await expect(row.getByRole('button', { name: 'Edit rule shell(git push origin main)' })).toBeDisabled()
  release()
  await expect(reset).toBeEnabled()
})

test('returns focus to the saved rules once the refreshed rules arrive after Apply', async ({ page }) => {
  const { data } = await setup(page)
  let release = () => {}
  const held = new Promise<void>((resolve) => { release = resolve })
  await page.route('**/api/views/overview', async (route) => { await held; await route.fulfill({ json: data }) })
  await addRule(page)
  const row = ask(page)
  await row.getByRole('textbox', { name: 'Ask first rules', exact: true }).fill('shell(git push *)')
  await row.getByRole('button', { name: 'Preview', exact: true }).click()
  await row.getByRole('button', { name: 'Apply', exact: true }).click()
  await expect(page.getByRole('list', { name: 'Saved Ask first rules' })).toHaveCount(0)
  release()
  await expect(page.getByRole('list', { name: 'Saved Ask first rules' })).toBeFocused()
})

test('keeps a draft through a conflict and discards previews when the scope changes', async ({ page }) => {
  const { authority, requests } = await setup(page)
  await addRule(page)
  const row = ask(page)
  await row.getByRole('textbox', { name: 'Ask first rules' }).fill('shell(git push *)')
  await page.route('**/api/edit/permissions', async (route) => {
    authority.fields!.find((field) => field.key === 'permissions.ask')!.set = [{ spec: 'permissions', path: shared, values: [{ key: 'permissions.ask', type: 'list', list: ['read(private/**)'] }] }]
    await route.fulfill({ status: 409, json: { error: 'Saved rules changed. Reload the saved values.' } })
  }, { times: 1 })
  await row.getByRole('button', { name: 'Preview', exact: true }).click()
  await expect(row.getByRole('alert')).toContainText('Saved rules changed')
  await row.getByRole('button', { name: 'Reload', exact: true }).click()
  await expect(row.getByRole('textbox', { name: 'Ask first rules' })).toHaveValue('shell(git push *)')
  await expect(row.getByRole('status')).toContainText('Your draft is kept')
  await row.getByRole('button', { name: 'Preview', exact: true }).click()
  expect(requests.at(-1)).toMatchObject({ before: ['read(private/**)'] })
  await expect(row.getByRole('region', { name: 'Ask first change preview' })).toBeVisible()
  await page.getByRole('button', { name: 'Local (not committed)', exact: true }).click()
  await expect(row.getByRole('region', { name: 'Ask first change preview' })).toHaveCount(0)
  await row.getByRole('button', { name: 'Edit rule read(private/**)' }).click()
  await expect(row.getByRole('textbox', { name: 'Ask first rules' })).toHaveValue('read(private/**)')
  expect(requests.every((request) => request.action !== 'apply')).toBe(true)
})

test('shows a rejected rule as a readable message under the editor', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await setup(page)
  const message = 'permissions.ask: rule 1 is invalid: unknown capability "test" for permissions.ask; use one of read, write, edit, delete, shell, web, shell(<pattern>), mcp:<server>, or a Claude Code tool name'
  await page.route('**/api/edit/permissions', (route) => route.fulfill({ status: 400, json: { error: message } }))
  await addRule(page)
  const row = ask(page)
  await row.getByRole('textbox', { name: 'Ask first rules', exact: true }).fill('test')
  await row.getByRole('button', { name: 'Preview', exact: true }).click()
  const alert = row.getByRole('alert')
  await expect(alert).toContainText('unknown capability "test" for permissions.ask; use one of')
  const box = await alert.boundingBox()
  expect(box!.width).toBeGreaterThan(520)
  expect(await alert.evaluate((element) => getComputedStyle(element).textAlign)).toMatch(/^(left|start)$/)
  await expect(alert.locator('.banner-title')).toHaveText('Could not preview the change')
  await alert.getByRole('button', { name: 'Dismiss', exact: true }).click()
  await expect(row.getByRole('alert')).toHaveCount(0)
  await expect(row.getByRole('textbox', { name: 'Ask first rules', exact: true })).toHaveValue('test')
  await expect(row.getByRole('textbox', { name: 'Ask first rules', exact: true })).toBeFocused()
})

test('returns focus to the saved rules when a failed rule removal is dismissed', async ({ page }) => {
  const { authority } = await setup(page)
  authority.fields!.find((field) => field.key === 'permissions.ask')!.set = [{ spec: 'permissions', path: shared, values: [{ key: 'permissions.ask', type: 'list', list: ['shell(git push *)', 'read(private/**)'] }] }]
  await page.reload()
  await page.route('**/api/edit/permissions', (route) => route.fulfill({ status: 409, json: { error: 'Saved rules changed. Reload the saved values.' } }))
  const row = ask(page)
  await row.getByRole('button', { name: 'Remove rule shell(git push *)' }).click()
  await expect(row.getByRole('alert').locator('.banner-title')).toHaveText('Could not preview the change')
  await expect(row.getByRole('button', { name: 'Reload', exact: true })).toBeVisible()
  await row.getByRole('alert').getByRole('button', { name: 'Dismiss', exact: true }).click()
  await expect(row.getByRole('list', { name: 'Saved Ask first rules' })).toBeFocused()
})

test('removes or edits one saved rule through the same preview and save', async ({ page }) => {
  const { authority, requests } = await setup(page)
  const field = authority.fields!.find((field) => field.key === 'permissions.ask')!
  field.set = [{ spec: 'permissions', path: shared, values: [{ key: 'permissions.ask', type: 'list', list: ['shell(git push *)', 'read(private/**)'] }] }]
  await page.reload()
  const row = ask(page)
  const saved = row.getByRole('list', { name: 'Saved Ask first rules' })
  await expect(saved.getByRole('listitem')).toHaveCount(2)
  await saved.getByRole('button', { name: 'Remove rule shell(git push *)' }).click()
  await expect(row.getByRole('region', { name: 'Ask first change preview' })).toBeVisible()
  expect(requests.at(-1)).toMatchObject({ action: 'preview', rules: ['read(private/**)'], before: ['shell(git push *)', 'read(private/**)'] })
  await expect(row.getByRole('region', { name: 'Ask first change preview' })).toBeFocused()
  expect(requests.every((request) => request.action !== 'apply')).toBe(true)
  await row.getByRole('region', { name: 'Ask first change preview' }).getByRole('button', { name: 'Cancel' }).click()
  await expect(saved).toBeFocused()
  await saved.getByRole('button', { name: 'Edit rule read(private/**)' }).click()
  const box = row.getByRole('textbox', { name: 'Ask first rules', exact: true })
  await expect(box).toBeFocused()
  expect(await box.evaluate((element: HTMLTextAreaElement) => element.value.slice(element.selectionStart, element.selectionEnd))).toBe('read(private/**)')
})

test('adds a rule on a new line and keeps a draft already open', async ({ page }) => {
  const { authority } = await setup(page)
  authority.fields!.find((field) => field.key === 'permissions.ask')!.set = [{ spec: 'permissions', path: shared, values: [{ key: 'permissions.ask', type: 'list', list: ['shell(git push *)'] }] }]
  await page.reload()
  await addRule(page)
  const box = ask(page).getByRole('textbox', { name: 'Ask first rules', exact: true })
  await expect(box).toBeFocused()
  await expect(box).toHaveValue('shell(git push *)\n')
  expect(await box.evaluate((element: HTMLTextAreaElement) => element.selectionStart)).toBe('shell(git push *)\n'.length)
  await box.fill('shell(git push *)\nread(private/**)')
  await addRule(page)
  await expect(box).toHaveValue('shell(git push *)\nread(private/**)\n')
  await expect(box).toBeFocused()
  await page.getByRole('button', { name: 'Add a rule', exact: true }).click()
  await expect(page.getByRole('menuitem', { name: 'Always blocked' })).toBeFocused()
  await page.keyboard.press('ArrowUp')
  await expect(page.getByRole('menuitem', { name: 'Allowed without asking' })).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(page.getByRole('menu', { name: 'Add a rule to' })).toBeHidden()
  await page.getByRole('button', { name: 'Add a rule', exact: true }).click()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('menu', { name: 'Add a rule to' })).toBeHidden()
  await expect(page.getByRole('button', { name: 'Add a rule', exact: true })).toBeFocused()
})

test('a link to an empty list says it has no rules yet', async ({ page }) => {
  await setup(page)
  await page.goto('/?view=authority&entry=field%3Apermissions.ask')
  await expect(ask(page)).toBeFocused()
  await expect(ask(page)).toContainText('Ask first: no rules yet.')
})

test('removing the last saved rule previews clearing the list, not an empty list', async ({ page }) => {
  const { authority, requests } = await setup(page)
  const field = authority.fields!.find((field) => field.key === 'permissions.ask')!
  field.set = [{ spec: 'permissions', path: shared, values: [{ key: 'permissions.ask', type: 'list', list: ['read(private/**)'] }] }]
  await page.reload()
  const row = ask(page)
  await row.getByRole('button', { name: 'Remove rule read(private/**)' }).click()
  const preview = row.getByRole('region', { name: 'Ask first change preview' })
  await expect(preview).toContainText('Remove the project list')
  expect(requests.at(-1)).toMatchObject({ action: 'preview', audience: 'shared', rules: null, before: ['read(private/**)'] })
})

test('removing the last personal rule clears the list for you, not the shared fallback', async ({ page }) => {
  const { requests } = await setup(page, true)
  const row = ask(page)
  const preview = row.getByRole('region', { name: 'Ask first change preview' })
  await row.getByRole('button', { name: 'View local settings' }).click()
  await row.getByRole('button', { name: 'Remove rule shell(git push origin main)' }).click()
  await expect(preview).toContainText('Clear the list for you')
  expect(requests.at(-1)).toMatchObject({ action: 'preview', audience: 'personal', rules: [], before: ['shell(git push origin main)'] })
})

test('keeps a personal null clear empty and offers a reset to shared rules', async ({ page }) => {
  const { authority, requests } = await setup(page, true)
  const field = authority.fields!.find((field) => field.key === 'permissions.ask')!
  field.set.find((source) => source.local)!.values = [{ key: field.key, type: 'text', value: 'null' }]
  await page.reload()
  const row = ask(page)
  await expect(row.getByRole('button', { name: /^Edit rule / })).toHaveCount(0)
  await row.getByRole('button', { name: 'View local settings' }).click()
  await expect(row.locator('.authority-rule')).toHaveCount(0)
  await addRule(page)
  await expect(row.getByRole('textbox', { name: 'Ask first rules' })).toHaveValue('')
  await row.getByRole('button', { name: 'Cancel', exact: true }).click()
  await row.getByRole('button', { name: 'Reset to the project rules', exact: true }).click()
  expect(requests.at(-1)).toMatchObject({ action: 'preview', audience: 'personal', before: null, rules: null })
  await row.getByRole('button', { name: 'Apply', exact: true }).click()
  await expect(row.locator('.authority-rule')).toHaveText([/shell\(git push \*\)\??Ask first\s*Project/])
})

test('requires selecting a source when more than one settings file is editable', async ({ page }) => {
  const { authority, requests } = await setup(page)
  authority.fields!.find((field) => field.key === 'permissions.ask')!.editable = { kind: 'permission-list', paths: [shared, '.agnostic-ai/settings/other.yaml'] }
  await page.reload()
  const row = ask(page)
  await expect(row).toContainText('Choose the file whose ask first rules you want to change.')
  await addRule(page)
  await expect(row.getByRole('textbox', { name: 'Ask first rules' })).toHaveCount(0)
  await row.getByLabel('Settings file for Ask first').selectOption('.agnostic-ai/settings/other.yaml')
  await addRule(page)
  await row.getByRole('textbox', { name: 'Ask first rules' }).fill('read(private/**)')
  await row.getByRole('button', { name: 'Preview', exact: true }).click()
  expect(requests.at(-1)).toMatchObject({ path: '.agnostic-ai/settings/other.yaml' })
})

test('edits a personal-only list in the selected spec when multiple shared files are available', async ({ page }) => {
  const { authority, requests } = await setup(page, true)
  const field = authority.fields!.find((field) => field.key === 'permissions.ask')!
  field.set = field.set.filter((source) => source.local)
  field.set[0].spec = 'declared-name'
  field.editable = {
    kind: 'permission-list', paths: [shared, '.agnostic-ai/settings/other.yaml'],
    sources: [{ path: shared, spec: 'declared-name', localPath: personal }, { path: '.agnostic-ai/settings/other.yaml', spec: 'other', localPath: '.agnostic-ai/local/settings/other.yaml' }],
  }
  await page.reload()
  const row = ask(page)
  await row.getByLabel('Settings file for Ask first').selectOption(shared)
  await expect(row.getByRole('button', { name: /^Edit rule / })).toHaveCount(0)
  await row.getByRole('button', { name: 'View local settings' }).click()
  await row.getByRole('button', { name: 'Edit rule shell(git push origin main)' }).click()
  await expect(row.getByRole('textbox', { name: 'Ask first rules' })).toHaveValue('shell(git push origin main)')
  await row.getByRole('textbox', { name: 'Ask first rules' }).fill('shell(git push origin release)')
  await row.getByRole('button', { name: 'Preview', exact: true }).click()
  expect(requests.at(-1)).toMatchObject({ path: shared, before: ['shell(git push origin main)'], audience: 'personal' })
})

test('shows a local protected: null as protection turned off and resets it to the project paths', async ({ page }) => {
  const { authority } = await setup(page)
  const field = authority.fields!.find((field) => field.key === 'protected')!
  field.editable = { kind: 'protected-list', path: shared, localPath: personal }
  field.set = [
    { spec: 'permissions', path: personal, local: true, values: [{ key: 'protected', type: 'text', value: 'null' }] },
    { spec: 'permissions', path: shared, values: [{ key: 'protected.paths', type: 'list', list: ['.github/**'] }, { key: 'protected.decision', type: 'text', value: 'deny' }] },
  ]
  const requests: Record<string, unknown>[] = []
  await page.route('**/api/edit/protected', (route) => {
    const body = route.request().postDataJSON() as Record<string, unknown>
    requests.push(body)
    if (body.action === 'apply') field.set = field.set.filter((source) => !source.local)
    return route.fulfill({ json: {
      schemaVersion: 'protected-edit/v1', file: personal, audience: 'personal', before: null, protected: null, cleared: true,
      diff: `--- ${personal}\n+++ ${personal}\n-protected: null`, expected: 'preview-token', syncCommand: 'agnostic-ai sync', ...(body.action === 'apply' ? { ok: true } : {}),
    } })
  })
  await page.reload()
  const row = page.locator('.authority-also-row').filter({ has: page.getByText('Protected paths', { exact: true }) })
  await expect(row).toContainText('turns these protected paths off for you')
  await expect(row.locator('.authority-also-value')).toHaveText('1 path, blocked')
  await row.getByRole('button', { name: 'View local settings' }).click()
  await expect(row).toContainText('so sync does not protect the project paths for you')
  await expect(row.locator('.authority-also-value')).toHaveText('Off for you')
  await row.getByRole('button', { name: 'Reset to project paths', exact: true }).click()
  expect(requests.at(-1)).toMatchObject({ action: 'preview', audience: 'personal', path: shared, protected: null, before: null })
  const preview = row.getByRole('region', { name: 'Protected paths change preview' })
  await expect(preview).toContainText('Use project paths')
  await expect(preview).toContainText('-protected: null')
  await preview.getByRole('button', { name: 'Apply', exact: true }).click()
  expect(requests.at(-1)).toMatchObject({ action: 'apply', expected: 'preview-token' })
  await expect(row).not.toContainText('so sync does not protect')
  await expect(row.getByRole('button', { name: 'Reset to project paths', exact: true })).toHaveCount(0)
})
