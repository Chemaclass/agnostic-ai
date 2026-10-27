import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import type { BuiltinChange } from '../src/builtin-change'

const fixture = JSON.parse(readFileSync(new URL('../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))
type State = BuiltinChange & { schemaVersion: string }

async function session(page: Page) {
  let state: State = { schemaVersion: 'builtin-change/v1', status: 'idle', enabled: false, supported: true, undoAvailable: false, targets: ['claude'], changes: [], findings: [] }
  let enabled: Record<string, boolean> = {}
  let before: Record<string, boolean> = {}
  let appliedName = ''
  let nextId = 0
  let failApply = false
  let failCancel = false
  let settled = state
  let statusReads = 0
  let sourceEdits = 0
  const previews: unknown[] = []
  const cancels: string[] = []
  await page.route('**/api/views/overview', (route) => {
    const view = structuredClone(fixture)
    for (const item of view.sections.find((section: { id: string }) => section.id === 'settings').items) {
      if (item.kind === 'builtin') item.enabled = enabled[item.name] ?? false
    }
    return route.fulfill({ json: view })
  })
  await page.route('**/api/changes/builtin', (route) => {
    statusReads++
    return route.fulfill({ json: state })
  })
  await page.route('**/api/edit/builtin', (route) => {
    sourceEdits++
    return route.fulfill({ status: 500, json: { error: 'Project changes must use the transaction.' } })
  })
  await page.route('**/api/changes/builtin/preview', (route) => {
    const request = route.request().postDataJSON() as { name: string; enabled: boolean }
    previews.push(request)
    if (state.status !== 'proposed') settled = state
    state = { ...state, ...request, status: 'proposed', id: `proposal-${++nextId}`, config: 'agnostic-ai.yaml' }
    return route.fulfill({ json: state })
  })
  await page.route('**/api/changes/builtin/apply', (route) => {
    expect(route.request().postDataJSON()).toEqual({ id: state.id })
    if (failApply) return route.fulfill({ status: 422, json: { error: 'The write failed and the previous files were restored.' } })
    before = { ...enabled }
    appliedName = state.name!
    enabled[appliedName] = state.enabled
    state = { ...state, status: 'applied', undoAvailable: true, undoId: state.id, undoName: appliedName }
    return route.fulfill({ json: state })
  })
  await page.route('**/api/changes/builtin/cancel', (route) => {
    const { id } = route.request().postDataJSON() as { id: string }
    cancels.push(id)
    if (failCancel) return route.fulfill({ status: 500, json: { error: 'The server stopped answering.' } })
    if (state.status !== 'proposed' || id !== state.id) return route.fulfill({ status: 409, json: { error: 'This preview was already replaced or closed; reload the change state.' } })
    state = settled
    return route.fulfill({ json: state })
  })
  await page.route('**/api/changes/builtin/undo', (route) => {
    expect(route.request().postDataJSON()).toEqual({ id: state.undoId })
    enabled = before
    state = { ...state, name: appliedName, status: 'undone', enabled: enabled[appliedName] ?? false, undoAvailable: false, undoId: undefined, undoName: undefined }
    return route.fulfill({ json: state })
  })
  await page.goto('/?view=settings')
  return {
    previews, cancels, reads: () => statusReads, edits: () => sourceEdits, failApply: () => { failApply = true }, failCancel: (fail: boolean) => { failCancel = fail },
    otherTabPreview: () => { state = { ...state, status: 'proposed', id: `proposal-${++nextId}` } },
    otherTabCancel: () => { state = settled },
    otherTabApply: () => {
      before = { ...enabled }
      appliedName = state.name!
      enabled[appliedName] = state.enabled
      state = { ...state, status: 'applied', undoAvailable: true, undoId: state.id, undoName: appliedName }
    },
  }
}

function toggle(page: Page, name: string) {
  return page.getByRole('switch', { name: `${name} built-in`, exact: true })
}

async function apply(page: Page, name: string) {
  await toggle(page, name).click()
  await page.getByRole('button', { name: 'Apply change', exact: true }).click()
  await expect(toggle(page, name)).toHaveAttribute('aria-checked', 'true')
}

test('shares a proposal and keeps undo through filtering, cancellation, and reload', async ({ page }) => {
  const server = await session(page)
  await apply(page, 'handoff')
  await expect(page.getByRole('button', { name: 'Undo last change' })).toHaveCount(1)
  await expect(page.getByText('Last applied: handoff.')).toBeVisible()
  await toggle(page, 'caveman').evaluate((button: HTMLButtonElement) => { button.click(); button.click() })
  await expect(page.getByRole('region', { name: 'Caveman change preview' })).toBeVisible()
  expect(server.previews).toEqual([{ audience: 'shared', name: 'handoff', enabled: true }, { audience: 'shared', name: 'caveman', enabled: true }])
  await expect(toggle(page, 'handoff')).toBeDisabled()
  await expect(page.getByRole('region', { name: 'Handoff change preview' })).toHaveCount(0)
  await page.getByRole('searchbox', { name: 'Search settings', exact: true }).fill('handoff')
  await expect(page.getByRole('region', { name: 'Caveman change preview' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Undo last change' })).toBeEnabled()
  await page.getByRole('button', { name: 'Clear search', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Caveman change preview' })).toBeVisible()
  expect(server.reads()).toBe(1)
  await page.reload()
  await expect(page.getByRole('region', { name: 'Caveman change preview' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Handoff change preview' })).toHaveCount(0)
  await page.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(toggle(page, 'handoff')).toBeEnabled()
  await page.reload()
  await expect(toggle(page, 'handoff')).toBeEnabled()
  await expect(page.getByRole('region', { name: 'Caveman change preview' })).toHaveCount(0)
  await expect(page.getByText('Last applied: handoff.')).toBeVisible()
  await page.getByRole('button', { name: 'Undo last change' }).click()
  await expect(toggle(page, 'handoff')).toHaveAttribute('aria-checked', 'false')
  await expect(toggle(page, 'caveman')).toHaveAttribute('aria-checked', 'false')
  await expect(page.getByRole('button', { name: 'Undo last change' })).toHaveCount(0)
  expect(server.edits()).toBe(0)
})

test('a failed cancel keeps the proposal and says it was not discarded', async ({ page }) => {
  const server = await session(page)
  await toggle(page, 'caveman').click()
  const preview = page.getByRole('region', { name: 'Caveman change preview' })
  server.failCancel(true)
  await preview.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Preview could not be cancelled.')
  await expect(page.getByRole('alert')).toContainText('The server stopped answering.')
  await expect(page.getByText('Preview discarded.')).toHaveCount(0)
  server.failCancel(false)
  await page.getByRole('button', { name: 'Reload change state' }).click()
  await expect(preview).toBeVisible()
  await preview.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(preview).toHaveCount(0)
  await expect(page.getByText('Preview discarded.')).toBeVisible()
  await expect(toggle(page, 'handoff')).toBeEnabled()
  expect(server.edits()).toBe(0)
})

test('cancel from a stale tab leaves a newer preview and says so', async ({ page }) => {
  const server = await session(page)
  await toggle(page, 'caveman').click()
  const preview = page.getByRole('region', { name: 'Caveman change preview' })
  server.otherTabPreview()
  await preview.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Another tab opened a newer preview.')
  expect(server.cancels).toEqual(['proposal-1'])
  await page.getByRole('button', { name: 'Reload change state' }).click()
  await expect(preview).toBeVisible()
  await preview.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(preview).toHaveCount(0)
  expect(server.cancels).toEqual(['proposal-1', 'proposal-2'])
})

test('cancel of a preview another tab applied shows the applied change', async ({ page }) => {
  const server = await session(page)
  await toggle(page, 'caveman').click()
  server.otherTabApply()
  await page.getByRole('region', { name: 'Caveman change preview' }).getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(page.getByText('Another tab applied this preview.')).toBeVisible()
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(toggle(page, 'caveman')).toHaveAttribute('aria-checked', 'true')
  await expect(page.getByRole('button', { name: 'Undo last change' })).toBeEnabled()
})

test('cancel of a preview another tab already closed just closes it', async ({ page }) => {
  const server = await session(page)
  await toggle(page, 'caveman').click()
  const preview = page.getByRole('region', { name: 'Caveman change preview' })
  server.otherTabCancel()
  await preview.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(preview).toHaveCount(0)
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(page.getByText('Preview discarded.')).toBeVisible()
  await expect(toggle(page, 'handoff')).toBeEnabled()
})

test('replaces the prior undo only after another built-in applies', async ({ page }) => {
  const server = await session(page)
  await apply(page, 'handoff')
  await apply(page, 'caveman')
  await expect(page.getByRole('button', { name: 'Undo last change' })).toHaveCount(1)
  await expect(page.getByText('Last applied: caveman.')).toBeVisible()
  await expect(page.getByText('Last applied: handoff.')).toHaveCount(0)
  await page.getByRole('button', { name: 'Undo last change' }).click()
  await expect(toggle(page, 'caveman')).toHaveAttribute('aria-checked', 'false')
  await expect(toggle(page, 'handoff')).toHaveAttribute('aria-checked', 'true')
  expect(server.edits()).toBe(0)
})

test('recovers retained undo after a failed apply without a source-only fallback', async ({ page }) => {
  const server = await session(page)
  await apply(page, 'handoff')
  server.failApply()
  await toggle(page, 'caveman').click()
  await page.getByRole('button', { name: 'Apply change', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveCount(1)
  await expect(page.getByRole('alert')).toContainText('The write failed')
  await expect(toggle(page, 'caveman')).toHaveAttribute('aria-checked', 'false')
  await expect(toggle(page, 'handoff')).toBeDisabled()
  await page.getByRole('searchbox', { name: 'Search settings', exact: true }).fill('no matches')
  await page.getByRole('button', { name: 'Reload change state' }).click()
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Undo last change' })).toBeVisible()
  await page.getByRole('button', { name: 'Undo last change' }).click()
  await page.getByRole('button', { name: 'Clear search', exact: true }).click()
  await expect(toggle(page, 'handoff')).toHaveAttribute('aria-checked', 'false')
  await expect(toggle(page, 'caveman')).toBeEnabled()
  expect(server.edits()).toBe(0)
})

test('drops a proposal when the save audience changes and resets the whole personal list', async ({ page }) => {
  const server = await session(page)
  await toggle(page, 'caveman').click()
  await expect(page.getByRole('region', { name: 'Caveman change preview' })).toContainText('for the team')
  await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Apply change', exact: true })).toHaveCount(0)
  await expect(toggle(page, 'caveman')).toBeEnabled()
  await expect.poll(() => server.cancels).toEqual(['proposal-1'])
  await page.reload()
  await expect(toggle(page, 'caveman')).toBeEnabled()
  await expect(page.getByRole('button', { name: 'Apply change', exact: true })).toHaveCount(0)
  await toggle(page, 'caveman').click()
  await expect(page.getByRole('region', { name: 'Caveman change preview' })).toContainText('for you only')
  expect(server.previews.at(-1)).toEqual({ audience: 'personal', name: 'caveman', enabled: true })
})

test('a failed audience-switch cancellation can be retried', async ({ page }) => {
  const server = await session(page)
  await toggle(page, 'caveman').click()
  server.failCancel(true)
  await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Preview could not be cancelled.')
  await expect(page.getByRole('alert')).toContainText('The server stopped answering.')
  await expect(toggle(page, 'caveman')).toBeDisabled()
  server.failCancel(false)
  await page.getByRole('button', { name: 'Retry cancellation' }).click()
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(toggle(page, 'caveman')).toBeEnabled()
  await page.reload()
  await page.getByRole('button', { name: 'Shared (committed)', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Caveman change preview' })).toHaveCount(0)
  expect(server.cancels).toEqual(['proposal-1', 'proposal-1'])
})

test('switching audience during apply does not cancel the applied preview', async ({ page }) => {
  const server = await session(page)
  let answer: () => void = () => {}
  const answered = new Promise<void>((resolve) => { answer = resolve })
  await page.route('**/api/changes/builtin/apply', async (route) => {
    server.otherTabApply()
    await answered
    await route.fulfill({ json: { schemaVersion: 'builtin-change/v1', status: 'applied', id: 'proposal-1', name: 'caveman', enabled: true, audience: 'shared', supported: true, undoAvailable: true, undoId: 'proposal-1', undoName: 'caveman', config: 'agnostic-ai.yaml', targets: ['claude'], changes: [], findings: [] } })
  })
  await toggle(page, 'caveman').click()
  await page.getByRole('button', { name: 'Apply change', exact: true }).click()
  await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
  await page.waitForTimeout(200)
  expect(server.cancels).toEqual([])
  answer()
  await expect(page.getByRole('button', { name: 'Undo last change' })).toBeEnabled()
  await expect(page.getByRole('alert')).toHaveCount(0)
  expect(server.cancels).toEqual([])
})

test('resets a personal built-in list that matches the team list', async ({ page }) => {
  const previews: unknown[] = []
  await page.route('**/api/views/overview', (route) => {
    const view = structuredClone(fixture)
    const settings = view.sections.find((section: { id: string }) => section.id === 'settings')
    for (const kind of settings.kinds) if (kind.kind === 'builtin') kind.local = true
    return route.fulfill({ json: view })
  })
  await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: { schemaVersion: 'builtin-change/v1', status: 'idle', enabled: false, supported: true, undoAvailable: false, targets: ['claude'], changes: [], findings: [] } }))
  await page.route('**/api/changes/builtin/preview', (route) => {
    previews.push(route.request().postDataJSON())
    return route.fulfill({ json: { schemaVersion: 'builtin-change/v1', status: 'proposed', id: 'reset-1', enabled: false, reset: true, audience: 'personal', supported: true, undoAvailable: false, config: 'agnostic-ai.local.yaml', targets: ['claude'], changes: [], findings: [] } })
  })
  await page.goto('/?view=settings')
  const block = page.locator('section.block').filter({ has: page.getByRole('heading', { name: /Built-ins/ }) })
  await expect(block.locator('.badge', { hasText: 'local' })).toBeVisible()
  await block.getByRole('button', { name: 'Reset built-ins to the shared list' }).click()
  await expect.poll(() => previews.at(-1)).toEqual({ audience: 'personal', reset: true })
  await expect(page.getByRole('region', { name: 'Built-in list reset preview' })).toContainText('agnostic-ai.local.yaml')
})

test('discards a preview that answers after the save audience changed', async ({ page }) => {
  const cancels: unknown[] = []
  let answer: () => void = () => {}
  const answered = new Promise<void>((resolve) => { answer = resolve })
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: structuredClone(fixture) }))
  await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: { schemaVersion: 'builtin-change/v1', status: 'idle', enabled: false, supported: true, undoAvailable: false, targets: ['claude'], changes: [], findings: [] } }))
  await page.route('**/api/changes/builtin/preview', async (route) => {
    await answered
    await route.fulfill({ json: { schemaVersion: 'builtin-change/v1', status: 'proposed', id: 'late', name: 'caveman', enabled: true, audience: 'shared', supported: true, undoAvailable: false, config: 'agnostic-ai.yaml', targets: ['claude'], changes: [], findings: [] } })
  })
  await page.route('**/api/changes/builtin/cancel', (route) => {
    cancels.push(route.request().postDataJSON())
    return route.fulfill({ json: { schemaVersion: 'builtin-change/v1', status: 'idle', enabled: false, supported: true, undoAvailable: false, targets: ['claude'], changes: [], findings: [] } })
  })
  await page.goto('/?view=settings')
  await page.getByRole('button', { name: 'Shared (committed)', exact: true }).click()
  await toggle(page, 'caveman').click()
  await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
  answer()
  await expect(toggle(page, 'caveman')).toBeEnabled()
  await expect(page.getByRole('button', { name: 'Apply change', exact: true })).toHaveCount(0)
  await expect.poll(() => cancels).toEqual([{ id: 'late' }])
})

test('a failed cancellation of a late preview can be retried', async ({ page }) => {
  let answer: () => void = () => {}
  const answered = new Promise<void>((resolve) => { answer = resolve })
  const idle = { schemaVersion: 'builtin-change/v1', status: 'idle', enabled: false, supported: true, undoAvailable: false, targets: ['claude'], changes: [], findings: [] }
  let attempts = 0
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: structuredClone(fixture) }))
  await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: idle }))
  await page.route('**/api/changes/builtin/preview', async (route) => {
    await answered
    await route.fulfill({ json: { ...idle, status: 'proposed', id: 'late', name: 'caveman', enabled: true, audience: 'shared', config: 'agnostic-ai.yaml' } })
  })
  await page.route('**/api/changes/builtin/cancel', (route) => {
    expect(route.request().postDataJSON()).toEqual({ id: 'late' })
    attempts++
    return attempts === 1 ? route.fulfill({ status: 500, json: { error: 'Cancellation failed.' } }) : route.fulfill({ json: idle })
  })
  await page.goto('/?view=settings')
  await toggle(page, 'caveman').click()
  await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
  answer()
  await expect(page.getByRole('alert')).toContainText('Cancellation failed.')
  await expect(toggle(page, 'caveman')).toBeDisabled()
  await page.getByRole('button', { name: 'Retry cancellation' }).click()
  await expect(toggle(page, 'caveman')).toBeEnabled()
  expect(attempts).toBe(2)
})

test('drops a restored proposal saved for another audience', async ({ page }) => {
  await page.addInitScript(() => window.localStorage.setItem('agnostic-ai.edit-scope', 'local'))
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: structuredClone(fixture) }))
  await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: { schemaVersion: 'builtin-change/v1', status: 'proposed', id: 'team-1', name: 'caveman', enabled: true, audience: 'shared', supported: true, undoAvailable: false, config: 'agnostic-ai.yaml', targets: ['claude'], changes: [], findings: [] } }))
  const cancels: unknown[] = []
  await page.route('**/api/changes/builtin/cancel', (route) => {
    cancels.push(route.request().postDataJSON())
    return route.fulfill({ status: 409, json: { error: 'unexpected' } })
  })
  await page.goto('/?view=settings')
  await expect(page.getByRole('button', { name: 'Personal (not committed)', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(toggle(page, 'caveman')).toBeEnabled()
  await expect(page.getByRole('button', { name: 'Apply change', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Shared (committed)', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Shared (committed)', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await page.waitForTimeout(300)
  expect(cancels).toEqual([])
})

test('shows why a linked worktree needs the CLI for a personal built-in', async ({ page }) => {
  const message = '.claude/skills/caveman/SKILL.md: built-ins only you turn on stay untracked in a linked worktree; use agnostic-ai sync in the CLI'
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: structuredClone(fixture) }))
  await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: { schemaVersion: 'builtin-change/v1', status: 'idle', enabled: false, supported: true, undoAvailable: false, targets: ['claude'], changes: [], findings: [] } }))
  await page.route('**/api/changes/builtin/preview', (route) => route.fulfill({ status: 400, json: { error: message } }))
  await page.goto('/?view=settings')
  await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
  await toggle(page, 'caveman').click()
  await expect(page.getByRole('alert')).toContainText(message)
  await page.getByRole('button', { name: 'Reload change state' }).click()
  await expect(toggle(page, 'caveman')).toBeEnabled()
  await expect(toggle(page, 'caveman')).toHaveAttribute('aria-checked', 'false')
})

test('previews an effort change with pending source changes and shows a stale value at the field', async ({ page }) => {
  const view = structuredClone(fixture)
  const team = '.agnostic-ai/settings/defaults.yaml'
  const effortField = view.sections.find((section: { id: string }) => section.id === 'settings').fields.find((field: { key: string }) => field.key === 'effort')
  Object.assign(effortField, { set: [{ spec: 'defaults', path: team, values: [{ key: 'effort', type: 'text', value: 'high' }] }], editable: { kind: 'text', path: team, paths: [team], syncCommand: 'agnostic-ai sync' } })
  const idle = { schemaVersion: 'builtin-change/v1', status: 'idle', enabled: false, supported: true, undoAvailable: false, targets: [], changes: [], findings: [] }
  const previews: unknown[] = []
  let stale = false
  await page.route('**/api/views/overview', (route) => {
    const now = structuredClone(view)
    now.sections.find((section: { id: string }) => section.id === 'settings').fields.find((field: { key: string }) => field.key === 'effort').set[0].values[0].value = stale ? 'medium' : 'high'
    return route.fulfill({ json: now })
  })
  await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: idle }))
  await page.route('**/api/changes/builtin/cancel', (route) => route.fulfill({ json: idle }))
  await page.route('**/api/changes/builtin/preview', (route) => {
    previews.push(route.request().postDataJSON())
    if (stale && (route.request().postDataJSON() as { setting: { expected: string } }).setting.expected === 'high') return route.fulfill({ status: 409, json: { error: `${team}: effort changed after this page loaded and is now medium. Reload to review it, then save again` } })
    return route.fulfill({ json: {
      ...idle, status: 'proposed', id: 'p1', name: 'effort', key: 'effort', title: 'effort set to low', audience: 'shared', config: team, targets: ['claude', 'codex'],
      changes: [{ path: team, action: 'update', diff: '-effort: high\n+effort: low' }, { path: '.claude/agents/reviewer.md', action: 'update', diff: '+Review one change.' }],
      findings: ['This preview also applies pending source changes.'],
    } })
  })
  await page.goto('/?view=settings')
  const effort = page.locator('.field-row').filter({ has: page.getByText('Reasoning effort', { exact: true }) })
  await effort.getByRole('textbox', { name: 'Reasoning effort value' }).fill('low')
  await effort.getByRole('button', { name: 'Preview and apply' }).click()
  const preview = effort.getByRole('region', { name: 'Reasoning effort change preview' })
  await expect(preview.getByRole('heading', { name: 'Effort set to low' })).toBeVisible()
  await expect(preview.getByRole('note')).toContainText('This preview also applies pending source changes.')
  await expect(preview).toContainText('.claude/agents/reviewer.md')
  expect(previews.at(-1)).toEqual({ audience: 'shared', setting: { path: team, key: 'effort', value: 'low', expected: 'high' } })

  await preview.getByRole('button', { name: 'Cancel', exact: true }).click()
  stale = true
  await effort.getByRole('button', { name: 'Preview and apply' }).click()
  await expect(effort.getByRole('alert')).toContainText('effort changed after this page loaded and is now medium')
  await effort.getByRole('button', { name: 'Reload saved values' }).click()
  await expect(effort).toContainText('The saved value changed after you started editing and is now medium.')
  await expect(effort.getByRole('textbox', { name: 'Reasoning effort value' })).toHaveValue('low')
  await effort.getByRole('button', { name: 'Preview and apply' }).click()
  await expect(preview).toBeVisible()
  expect(previews.at(-1)).toEqual({ audience: 'shared', setting: { path: team, key: 'effort', value: 'low', expected: 'medium' } })
  await expect(effort.getByRole('textbox', { name: 'Reasoning effort value' })).toBeDisabled()
  await expect(page.getByRole('heading', { name: /^Built-ins/ }).locator('..').getByRole('alert')).toHaveCount(0)
})
