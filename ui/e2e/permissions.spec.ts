import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import type { PermissionEditRequest } from '../src/contract'
import { normalizeOverviewView } from '../src/model/normalize'

const shared = '.agnostic-ai/settings/permissions.yaml'
const personal = '.agnostic-ai/local/settings/permissions.yaml'

async function setup(page: Page, override = false) {
  const data = normalizeOverviewView(JSON.parse(readFileSync(new URL('../src/fixtures/overview.mock.json', import.meta.url), 'utf8')))
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

for (const width of [390, 1440]) {
  test(`adds permission rules with an explicit preview and source save at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    const { requests } = await setup(page)
    const row = page.locator('.field-row').filter({ has: page.getByText('Ask first', { exact: true }) })
    await row.getByRole('button', { name: 'Add rules', exact: true }).click()
    const rules = row.getByRole('textbox', { name: 'Ask first rules', exact: true })
    await expect(rules).toHaveAttribute('rows', '4')
    await expect(rules).toHaveAttribute('placeholder', /shell\(git status\)\nread\(src\/\*\*\)/)
    await rules.fill('shell(git push *)\n\nread(private/**)')
    await row.getByRole('button', { name: 'Preview changes' }).click()
    const preview = row.getByRole('region', { name: 'Ask first change preview' })
    await expect(preview).toContainText('Shared (committed)')
    await expect(preview).toContainText(shared)
    await expect(preview).toContainText('After saving, run agnostic-ai sync')
    expect(requests).toEqual([{ action: 'preview', audience: 'shared', path: shared, key: 'permissions.ask', rules: ['shell(git push *)', 'read(private/**)'], before: null }])
    await preview.getByRole('button', { name: 'Save rules', exact: true }).click()
    await expect(page.getByRole('status').filter({ hasText: /^Saved / })).toContainText(`Saved ${shared}. Run agnostic-ai sync`)
    await expect(row.getByRole('button', { name: 'Edit rules', exact: true })).toBeVisible()
    expect(requests[1]).toMatchObject({ action: 'apply', expected: 'preview-token', before: null })
    await page.reload()
    await expect(row).toContainText('shell(git push *)')
    await expect(row).toContainText('read(private/**)')
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })
}

test('shows personal replacement, compares values, and previews resetting one list', async ({ page }) => {
  const { requests } = await setup(page, true)
  const row = page.locator('.field-row').filter({ has: page.getByText('Ask first', { exact: true }) })
  await expect(row).toContainText('Personal overrides this file’s shared rules.')
  await expect(row.getByRole('button', { name: 'Edit rules', exact: true })).toHaveCount(0)
  await row.getByText('Compare shared (1) and personal (1)').click()
  await expect(row.locator('.permission-comparison').filter({ hasText: 'Personal' })).toContainText('shell(git push origin main)')
  await row.getByRole('button', { name: 'View personal settings' }).click()
  await expect(row.locator('.field-source')).toContainText('shell(git push origin main)')
  await row.getByRole('button', { name: 'Reset to shared rules', exact: true }).click()
  const preview = row.getByRole('region', { name: 'Ask first change preview' })
  await expect(preview).toContainText('Use shared rules')
  expect(requests).toEqual([{ action: 'preview', audience: 'personal', path: shared, key: 'permissions.ask', rules: null, before: ['shell(git push origin main)'] }])
  await preview.getByRole('button', { name: 'Save rules', exact: true }).click()
  await expect(row).toContainText('Inherited from shared')
  await expect(row.getByRole('button', { name: 'Reset to shared rules' })).toHaveCount(0)
  await row.getByRole('button', { name: 'Edit rules', exact: true }).click()
  await expect(row.getByRole('textbox', { name: 'Ask first rules' })).toHaveValue('shell(git push *)')
  await row.getByRole('textbox', { name: 'Ask first rules' }).fill('shell(git push origin release)')
  await row.getByRole('button', { name: 'Preview changes' }).click()
  expect(requests.at(-1)).toMatchObject({ audience: 'personal', before: null, rules: ['shell(git push origin release)'] })
})

test('keeps a draft through a conflict and discards previews when the scope changes', async ({ page }) => {
  const { authority, requests } = await setup(page)
  const row = page.locator('.field-row').filter({ has: page.getByText('Ask first', { exact: true }) })
  await row.getByRole('button', { name: 'Add rules', exact: true }).click()
  await row.getByRole('textbox', { name: 'Ask first rules' }).fill('shell(git push *)')
  await page.route('**/api/edit/permissions', async (route) => {
    authority.fields!.find((field) => field.key === 'permissions.ask')!.set = [{ spec: 'permissions', path: shared, values: [{ key: 'permissions.ask', type: 'list', list: ['read(private/**)'] }] }]
    await route.fulfill({ status: 409, json: { error: 'Saved rules changed. Reload the saved values.' } })
  }, { times: 1 })
  await row.getByRole('button', { name: 'Preview changes' }).click()
  await expect(row.getByRole('alert')).toContainText('Saved rules changed')
  await row.getByRole('button', { name: 'Reload saved values' }).click()
  await expect(row.getByRole('textbox', { name: 'Ask first rules' })).toHaveValue('shell(git push *)')
  await expect(row.getByRole('status')).toContainText('Your draft is kept')
  await row.getByRole('button', { name: 'Preview changes' }).click()
  expect(requests.at(-1)).toMatchObject({ before: ['read(private/**)'] })
  await expect(row.getByRole('region', { name: 'Ask first change preview' })).toBeVisible()
  await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
  await expect(row.getByRole('region', { name: 'Ask first change preview' })).toHaveCount(0)
  await row.getByRole('button', { name: 'Edit rules', exact: true }).click()
  await expect(row.getByRole('textbox', { name: 'Ask first rules' })).toHaveValue('read(private/**)')
  expect(requests.every((request) => request.action !== 'apply')).toBe(true)
})

test('shows a rejected rule as a readable message under the editor', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await setup(page)
  const message = 'permissions.ask: rule 1 is invalid: unknown capability "test" for permissions.ask; use one of read, write, edit, delete, shell, web, shell(<pattern>), mcp:<server>, or a Claude Code tool name'
  await page.route('**/api/edit/permissions', (route) => route.fulfill({ status: 400, json: { error: message } }))
  const row = page.locator('.field-row').filter({ has: page.getByText('Ask first', { exact: true }) })
  await row.getByRole('button', { name: 'Add rules', exact: true }).click()
  await row.getByRole('textbox', { name: 'Ask first rules', exact: true }).fill('test')
  await row.getByRole('button', { name: 'Preview changes' }).click()
  const alert = row.getByRole('alert')
  await expect(alert).toContainText('unknown capability "test" for permissions.ask; use one of')
  const box = await alert.boundingBox()
  expect(box!.width).toBeGreaterThan(520)
  expect(await alert.evaluate((element) => getComputedStyle(element).textAlign)).toMatch(/^(left|start)$/)
})

test('keeps a personal null clear empty and offers a reset to shared rules', async ({ page }) => {
  const { authority, requests } = await setup(page, true)
  const field = authority.fields!.find((field) => field.key === 'permissions.ask')!
  field.set.find((source) => source.local)!.values = [{ key: field.key, type: 'text', value: 'null' }]
  await page.reload()
  const row = page.locator('.field-row').filter({ has: page.getByText('Ask first', { exact: true }) })
  await expect(row.getByRole('button', { name: 'Edit rules', exact: true })).toHaveCount(0)
  await row.getByRole('button', { name: 'View personal settings' }).click()
  await row.getByRole('button', { name: 'Add rules', exact: true }).click()
  await expect(row.getByRole('textbox', { name: 'Ask first rules' })).toHaveValue('')
  await row.getByRole('button', { name: 'Cancel', exact: true }).click()
  await row.getByRole('button', { name: 'Reset to shared rules', exact: true }).click()
  expect(requests.at(-1)).toMatchObject({ action: 'preview', audience: 'personal', before: null, rules: null })
  await row.getByRole('button', { name: 'Save rules', exact: true }).click()
  await expect(row).toContainText('Inherited from shared')
  await expect(row.locator('.field-source')).toContainText('shell(git push *)')
})

test('requires selecting a source when more than one settings file is editable', async ({ page }) => {
  const { authority, requests } = await setup(page)
  authority.fields!.find((field) => field.key === 'permissions.ask')!.editable = { kind: 'permission-list', paths: [shared, '.agnostic-ai/settings/other.yaml'] }
  await page.reload()
  const row = page.locator('.field-row').filter({ has: page.getByText('Ask first', { exact: true }) })
  await expect(row.getByRole('button', { name: 'Add rules', exact: true })).toBeDisabled()
  await row.getByLabel('Settings file for Ask first').selectOption('.agnostic-ai/settings/other.yaml')
  await row.getByRole('button', { name: 'Add rules', exact: true }).click()
  await row.getByRole('textbox', { name: 'Ask first rules' }).fill('read(private/**)')
  await row.getByRole('button', { name: 'Preview changes' }).click()
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
  const row = page.locator('.field-row').filter({ has: page.getByText('Ask first', { exact: true }) })
  await row.getByLabel('Settings file for Ask first').selectOption(shared)
  await expect(row.getByRole('button', { name: 'Add rules', exact: true })).toHaveCount(0)
  await row.getByRole('button', { name: 'View personal settings' }).click()
  await row.getByRole('button', { name: 'Edit rules', exact: true }).click()
  await expect(row.getByRole('textbox', { name: 'Ask first rules' })).toHaveValue('shell(git push origin main)')
  await row.getByRole('textbox', { name: 'Ask first rules' }).fill('shell(git push origin release)')
  await row.getByRole('button', { name: 'Preview changes' }).click()
  expect(requests.at(-1)).toMatchObject({ path: shared, before: ['shell(git push origin main)'], audience: 'personal' })
})
