import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import type { Conversation } from '../../src/contract'

const overview = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))
const fixture = JSON.parse(readFileSync(new URL('../../src/fixtures/conversations.mock.json', import.meta.url), 'utf8'))
const base = (over: Partial<Conversation> = {}): Conversation => ({ tool: 'claude', app: 'cli', id: 'internal', title: 'Parser', updatedAt: '2026-11-07T12:00:00Z', state: 'active', open: { kind: 'terminal', label: 'Open in Terminal', command: 'claude --resume internal' }, ...over })
const row = (page: Page, name: string) => page.locator('li.conversation').filter({ has: page.getByRole('heading', { level: 3, name, exact: true, includeHidden: true }) })
async function menu(page: Page, title = 'Parser') { await row(page, title).getByRole('button', { name: `More actions for ${title}` }).click(); return page.getByRole('menu', { name: `Actions for ${title}` }) }
async function setup(page: Page, initial: Conversation[]) {
  const state = { rows: initial, posts: [] as { action: string; body: Record<string, unknown>; session?: string; origin?: string }[], views: [] as string[], fail: '', failId: '', freedBytes: undefined as number | undefined, beforeRead: undefined as (() => void) | undefined }
  await page.addInitScript(() => { sessionStorage.setItem('agnostic-ai.session', 'tab-key'); Reflect.deleteProperty(window, 'IntersectionObserver') })
  await page.route('**/api/**', (route) => route.fulfill({ status: 404, json: { error: 'Not mocked.' } }))
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/repo', global: false, ui: true } }))
  await page.route('**/api/views/conversations**', (route) => { state.views.push(new URL(route.request().url()).search); state.beforeRead?.(); return route.fulfill({ json: { ...fixture, conversations: state.rows } }) })
  await page.route('**/api/conversations/*', async (route) => {
    const action = new URL(route.request().url()).pathname.split('/').at(-1)!
    const body = route.request().postDataJSON()
    state.posts.push({ action, body, session: route.request().headers()['x-agnostic-ai-session'], origin: route.request().headers().origin })
    if (state.fail && (!state.failId || state.failId === body.id)) return route.fulfill({ status: 500, json: { error: state.fail, ...(action === 'delete' ? { reason: 'failed', removed: ['old.json'], remaining: ['transcript.jsonl'] } : {}) } })
    const at = state.rows.findIndex((row) => row.id === body.id)
    if (action === 'mark' && at >= 0) {
      if (body.as === 'hidden') state.rows.splice(at, 1)
      else if (body.as === 'pinned' || body.as === 'unpinned') state.rows[at] = { ...state.rows[at], pinnedAt: body.as === 'pinned' ? '2026-11-08T12:00:00Z' : undefined }
      else state.rows[at] = { ...state.rows[at], state: body.as === 'completed' ? 'completed' : 'active', unread: false, completedAt: body.as === 'completed' ? 'now' : undefined }
    }
    if (action === 'rename' && at >= 0) state.rows[at].title = body.title
    if (action === 'delete') state.rows.splice(at, 1)
    return action === 'delete' ? route.fulfill({ json: { deleted: true, ...(state.freedBytes ? { freedBytes: state.freedBytes } : {}) } }) : route.fulfill({ status: 204 })
  })
  return state
}

test('pins by keyboard, keeps the unread state on refresh, and unpins into the state group', async ({ page }) => {
  const state = await setup(page, [
    base({ state: 'unread' }),
    base({ id: 'old', title: 'Previous release', state: 'completed', pinnedAt: '2026-11-07T12:00:00Z' }),
    base({ id: 'nested', title: 'Child review', parent: { tool: 'claude', id: 'old' } }),
  ])
  await page.goto('/')
  const pinned = page.getByRole('region', { name: /^Pinned/ })
  await expect(pinned.getByRole('heading', { level: 3 })).toHaveText(['Previous release', 'Child review'])
  await expect((await menu(page, 'Child review')).getByRole('menuitem', { name: 'Pin to top' })).toHaveCount(0)
  await page.keyboard.press('Escape')
  await row(page, 'Parser').getByRole('button', { name: 'More actions for Parser' }).focus()
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('ArrowDown')
  await expect(page.getByRole('menuitem', { name: 'Pin to top' })).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(pinned.getByRole('heading', { level: 3 })).toHaveText(['Previous release', 'Child review', 'Parser'])
  await expect(row(page, 'Parser').getByRole('status')).toHaveText('Pinned.')
  await page.reload()
  await expect(pinned.getByRole('heading', { level: 3 })).toHaveText(['Previous release', 'Child review', 'Parser'])
  const actions = await menu(page)
  await expect(actions.getByRole('menuitem', { name: 'Mark as read' })).toBeVisible()
  await actions.getByRole('menuitem', { name: 'Unpin' }).click()
  await expect(page.getByRole('region', { name: /^Needs you/ }).getByRole('heading', { level: 3 })).toHaveText(['Parser'])
  expect(state.posts.map(({ body }) => body)).toEqual([{ tool: 'claude', id: 'internal', as: 'pinned' }, { tool: 'claude', id: 'internal', as: 'unpinned' }])
  await page.goto('/?place=cloud')
  await expect(pinned).toHaveCount(0)
})

test('keeps the original state and actions on pinned chats', async ({ page }) => {
  const pinnedAt = '2026-11-07T12:00:00Z'
  await setup(page, [
    base({ id: 'building', title: 'Build the package', state: 'working', live: true, activity: 'working', pinnedAt }),
    base({ id: 'reviewed', title: 'Review the release', state: 'completed', completedAt: '2026-11-06T12:00:00Z', pinnedAt }),
    base({ id: 'read', title: 'Read the report', state: 'active', seenAt: '2026-11-06T12:00:00Z', pinnedAt }),
    base({ id: 'child', title: 'Check build inputs', state: undefined, live: true, activity: 'waiting', parent: { tool: 'claude', id: 'building' } }),
  ])
  await page.goto('/')
  const pinned = page.getByRole('region', { name: /^Pinned/ })
  await expect(pinned.getByRole('heading', { level: 3 })).toHaveCount(4)
  await expect(row(page, 'Build the package').getByText('Working', { exact: true })).toBeVisible()
  await expect(row(page, 'Review the release').getByText('Archived', { exact: true })).toBeVisible()
  await expect(row(page, 'Read the report').locator('.conversation-state [aria-hidden="true"]')).toHaveText('Active')
  await expect(row(page, 'Check build inputs').getByText('Waiting for you', { exact: true })).toBeVisible()
  await expect(row(page, 'Check build inputs').getByText('Active', { exact: true })).toHaveCount(0)
  await expect((await menu(page, 'Review the release')).getByRole('menuitem', { name: 'Undo complete' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect((await menu(page, 'Read the report')).getByRole('menuitem', { name: 'Mark as read' })).toHaveCount(0)
})

test('row menu keyboard marks, regroups, undoes and hides using the protected internal ID', async ({ page }) => {
  const state = await setup(page, [base({ state: 'unread' })])
  await page.goto('/?place=local')
  const more = row(page, 'Parser').getByRole('button', { name: 'More actions for Parser' })
  await expect(more).toHaveAttribute('aria-haspopup', 'menu')
  await expect(more).toHaveAttribute('aria-expanded', 'false')
  await more.focus(); await page.keyboard.press('ArrowDown')
  await expect(more).toHaveAttribute('aria-expanded', 'true')
  await expect(page.getByRole('menuitem', { name: 'Mark as read', exact: true })).toBeFocused()
  await page.keyboard.press('ArrowDown')
  await expect(page.getByRole('menuitem', { name: 'Rename…', exact: true })).toBeFocused()
  await page.keyboard.press('ArrowUp')
  await expect(page.getByRole('menuitem', { name: 'Mark as read', exact: true })).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(more).toBeFocused()
  await expect(more).toHaveAttribute('aria-expanded', 'false')
  await expect(page.getByRole('menu')).toHaveCount(0)
  expect(state.posts).toHaveLength(0)
  await (await menu(page)).getByRole('menuitem', { name: 'Mark as read', exact: true }).click()
  await expect(page.getByRole('region', { name: /^Active/ }).getByRole('heading', { level: 3 })).toHaveText(['Parser'])
  await (await menu(page)).getByRole('menuitem', { name: 'Mark complete', exact: true }).click()
  const archived = page.getByRole('region', { name: /^Archived/ })
  await expect(archived.locator('details')).not.toHaveAttribute('open')
  await archived.locator('summary').click()
  await (await menu(page)).getByRole('menuitem', { name: 'Undo complete', exact: true }).click()
  await expect(page.getByRole('region', { name: /^Active/ })).toBeVisible()
  expect(state.posts.map(({ body }) => body)).toEqual([{ tool: 'claude', id: 'internal', as: 'seen' }, { tool: 'claude', id: 'internal', as: 'completed' }, { tool: 'claude', id: 'internal', as: 'open' }])
  for (const request of state.posts) { expect(request.session).toBe('tab-key'); expect(request.origin).toBe(new URL(page.url()).origin) }
})

test('a refused mark shows the server sentence by the row and keeps it in place', async ({ page }) => {
  const state = await setup(page, [base({ state: 'unread' })])
  await page.goto('/')
  state.fail = 'That conversation is no longer listed.'
  await (await menu(page)).getByRole('menuitem', { name: 'Mark as read', exact: true }).click()
  await expect(row(page, 'Parser').getByRole('status')).toHaveText('That conversation is no longer listed.')
  await expect(page.getByRole('region', { name: /^Needs you/ }).getByRole('heading', { level: 3 })).toHaveText(['Parser'])
  expect(state.posts.map(({ body }) => body)).toEqual([{ tool: 'claude', id: 'internal', as: 'seen' }])
})

test('rename sends a new name on Enter, keeps a blank or refused name in the field, and Escape cancels', async ({ page }) => {
  const state = await setup(page, [base()])
  await page.goto('/')
  const field = row(page, 'Parser').getByRole('textbox', { name: 'New name for Parser' })
  await (await menu(page)).getByRole('menuitem', { name: 'Rename…', exact: true }).click()
  await expect(field).toBeFocused()
  await field.press('Enter')
  await expect(field).toHaveCount(0)
  await expect(row(page, 'Parser').getByRole('button', { name: 'More actions for Parser' })).toBeFocused()
  await (await menu(page)).getByRole('menuitem', { name: 'Rename…', exact: true }).click()
  await field.fill('   ')
  await field.press('Enter')
  await expect(row(page, 'Parser').getByRole('alert')).toHaveText('Type a name, or press Escape to keep the old one.')
  await expect(field).toBeFocused()
  await field.press('Escape')
  await expect(field).toHaveCount(0)
  await expect(row(page, 'Parser').getByRole('button', { name: 'More actions for Parser' })).toBeFocused()
  expect(state.posts).toHaveLength(0)
  state.fail = 'Pick a shorter name.'
  await (await menu(page)).getByRole('menuitem', { name: 'Rename…', exact: true }).click()
  await field.fill('Parser refused')
  await field.press('Enter')
  await expect(row(page, 'Parser').getByRole('alert')).toHaveText('Pick a shorter name.')
  await expect(field).toBeVisible()
  await expect(field).toHaveValue('Parser refused')
  state.fail = ''
  await field.fill('  Parser   release  ')
  await field.press('Enter')
  await expect(row(page, 'Parser release')).toBeVisible()
  await expect(row(page, 'Parser release').getByRole('textbox')).toHaveCount(0)
  expect(state.posts.map(({ action, body }) => [action, body])).toEqual([['rename', { tool: 'claude', id: 'internal', title: 'Parser refused' }], ['rename', { tool: 'claude', id: 'internal', title: 'Parser release' }]])
})

test('Copy session ID copies each row its own id, and a Claude cloud row copies its resume command', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  const cloud = (id: string, title: string) => base({ app: 'cloud', id, title, cloudResumeCommand: `claude --teleport ${id}`, open: { kind: 'link', label: 'Open on claude.ai' } })
  await setup(page, [cloud('session_first', 'First'), cloud('session_second', 'Second')])
  await page.goto('/')
  await (await menu(page, 'Second')).getByRole('menuitem', { name: 'Copy session ID', exact: true }).click()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('session_second')
  await (await menu(page, 'First')).getByRole('menuitem', { name: 'Copy session ID', exact: true }).click()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('session_first')
  await (await menu(page, 'Second')).getByRole('menuitem', { name: 'Copy resume command', exact: true }).click()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('claude --teleport session_second')
})

test('Archived always offers Hide and separates Cancel-first deletion, refusals and partial failure feedback', async ({ page }) => {
  const state = await setup(page, [base({ state: 'completed', completedAt: 'now' }), base({ id: 'other', title: 'Other', state: 'dormant' })])
  await page.goto('/')
  await page.getByRole('region', { name: /^Archived/ }).locator('summary').click()
  let actions = await menu(page)
  await expect(actions.getByRole('menuitem', { name: 'Hide from agnostic-ai', exact: true })).toBeVisible()
  await expect(actions.getByRole('group', { name: 'Danger zone' }).getByRole('menuitem', { name: 'Delete permanently…' })).toBeVisible()
  await expect(actions.locator('[role="separator"] + [role="group"]')).toHaveAttribute('aria-label', 'Danger zone')
  await actions.getByRole('menuitem', { name: 'Delete permanently…' }).click()
  let dialog = page.getByRole('dialog', { name: /Delete.*permanently/ })
  await expect(dialog.getByRole('button', { name: 'Cancel', exact: true })).toBeFocused()
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).click()
  expect(state.posts).toHaveLength(0)
  await (await menu(page)).getByRole('menuitem', { name: 'Delete permanently…' }).click()
  await page.keyboard.press('Escape')
  expect(state.posts).toHaveLength(0)
  state.fail = 'Could not delete every file.'
  await (await menu(page)).getByRole('menuitem', { name: 'Delete permanently…' }).click()
  dialog = page.getByRole('dialog', { name: /Delete.*permanently/ })
  await dialog.getByRole('button', { name: 'Delete permanently', exact: true }).click()
  await expect(row(page, 'Parser').getByRole('status')).toContainText('Remaining: transcript.jsonl.')
  await expect(row(page, 'Parser').getByRole('status')).toBeFocused()
  state.fail = ''
  await (await menu(page)).getByRole('menuitem', { name: 'Hide from agnostic-ai', exact: true }).click()
  await expect(row(page, 'Parser')).toHaveCount(0)
  await expect(page.getByText('Hidden from agnostic-ai. The transcript stays.', { exact: true })).toBeVisible()
  expect(state.posts.at(-1)?.body).toEqual({ tool: 'claude', id: 'internal', as: 'hidden' })
  expect(state.posts[0].body).toEqual({ tool: 'claude', id: 'internal' })
  state.freedBytes = 4200
  await (await menu(page, 'Other')).getByRole('menuitem', { name: 'Delete permanently…' }).click()
  await page.getByRole('dialog', { name: /Delete.*permanently/ }).getByRole('button', { name: 'Delete permanently', exact: true }).click()
  await expect(row(page, 'Other')).toHaveCount(0)
  expect(state.posts.at(-1)?.body).toEqual({ tool: 'claude', id: 'other' })
  await expect(page.getByText('Deleted. Freed 4.2 KB.', { exact: true })).toBeVisible()

})

test('Mark all complete stays on folded Active, covers only filtered rows and skips a row that changed', async ({ page }) => {
  const state = await setup(page, [base({ id: 'one', title: 'One', updatedAt: '2026-11-07T12:03:00Z' }), base({ id: 'two', title: 'Two', updatedAt: '2026-11-07T12:02:00Z' }), base({ id: 'three', title: 'Three', updatedAt: '2026-11-07T12:01:00Z' }), base({ id: 'other', title: 'Other', tool: 'codex' })])
  await page.goto('/?place=local')
  await page.getByRole('group', { name: 'Filter by tool' }).getByRole('button', { name: 'Claude Code', exact: true }).click()
  const active = page.getByRole('region', { name: /^Active/ })
  await active.locator('summary').click()
  await expect(active.locator('details')).not.toHaveAttribute('open')
  state.beforeRead = () => { if (state.posts.length === 1) state.rows[1] = { ...state.rows[1], updatedAt: '2026-11-07T12:05:00Z' } }
  await active.getByRole('button', { name: 'Mark all complete' }).click()
  await expect(page.getByText('2 conversations marked complete. 1 changed and skipped.', { exact: true })).toBeVisible()
  expect(state.posts.map(({ body }) => body)).toEqual([{ tool: 'claude', id: 'one', as: 'completed' }, { tool: 'claude', id: 'three', as: 'completed' }])
  await expect(page.getByRole('group', { name: 'Filter by place' }).getByRole('button', { name: 'Local' })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByRole('group', { name: 'Filter by tool' }).getByRole('button', { name: 'Claude Code', exact: true })).toHaveAttribute('aria-pressed', 'true')
})

test('Mark all complete stops at the first refused write, shows it by the row and reloads', async ({ page }) => {
  const state = await setup(page, [base({ id: 'one', title: 'One', updatedAt: '2026-11-07T12:03:00Z' }), base({ id: 'two', title: 'Two', updatedAt: '2026-11-07T12:02:00Z' }), base({ id: 'three', title: 'Three', updatedAt: '2026-11-07T12:01:00Z' })])
  await page.goto('/')
  state.fail = 'Write refused.'
  state.failId = 'two'
  const reads = state.views.length
  await page.getByRole('button', { name: 'Mark all complete' }).click()
  await expect(page.getByText('1 conversation marked complete. Two: Write refused.', { exact: true })).toBeVisible()
  expect(state.posts.map(({ body }) => body.id)).toEqual(['one', 'two'])
  await expect(row(page, 'Two').getByRole('status')).toHaveText('Write refused.')
  await expect(page.getByRole('region', { name: /^Active/ }).getByRole('heading', { level: 3 })).toHaveText(['Two', 'Three'])
  expect(state.views.length).toBeGreaterThan(reads + 2)
})

test('server refusal and conflict precedence keep destructive actions unavailable without hiding valuable transcripts', async ({ page }) => {
  const conflict = { id: 'c', worktree: '/repo', path: 'app.ts', requester: { tool: 'claude' as const, id: 'conflict' } }
  const state = await setup(page, [base({ id: 'refused', title: 'Refused', app: 'cloud', state: 'dormant', deleteUnsupported: 'Stored in the cloud. Delete it in its own app.' }), base({ id: 'conflict', title: 'Conflict', state: 'dormant', claimConflicts: [conflict] })])
  await page.goto('/')
  const actions = await menu(page, 'Conflict')
  await expect(actions.getByRole('menuitem', { name: 'Hide from agnostic-ai' })).toHaveCount(0)
  await expect(actions.getByRole('menuitem', { name: 'Delete permanently…' })).toHaveCount(0)
  await expect(actions.getByRole('menuitem', { name: 'Mark as read' })).toHaveCount(0)
  await actions.getByRole('menuitem', { name: 'Mark complete', exact: true }).click()
  await expect(page.getByRole('region', { name: /^Needs you/ }).getByRole('heading', { level: 3 })).toHaveText(['Conflict'])
  await page.getByRole('region', { name: /^Archived/ }).locator('summary').click()
  await (await menu(page, 'Refused')).getByRole('menuitem', { name: 'Delete permanently…' }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(row(page, 'Refused').getByRole('status')).toHaveText('Stored in the cloud. Delete it in its own app.')
  expect(state.posts.map(({ action }) => action)).toEqual(['mark'])
  await (await menu(page, 'Refused')).getByRole('menuitem', { name: 'Hide from agnostic-ai' }).click()
  await expect(row(page, 'Refused')).toHaveCount(0)
})

test('a write reloads a fresh first page and keeps the search, filters and an expanded Archived group', async ({ page }) => {
  const state = await setup(page, [base({ id: 'first', title: 'Parser first' }), base({ id: 'hidden', title: 'Parser hidden', state: 'completed', completedAt: 'now' }), base({ id: 'kept', title: 'Parser kept', state: 'dormant' })])
  const before = '2026-10-31T12:00:00Z'
  await page.route('**/api/views/conversations**', (route) => {
    const params = new URL(route.request().url()).searchParams
    state.views.push(params.toString())
    const older = params.has('before')
    return route.fulfill({ json: { ...fixture, conversations: state.rows.filter((row) => older ? row.id === 'kept' : row.id !== 'kept'), matches: state.rows.length, next: older ? undefined : before } })
  })
  await page.goto('/?q=Parser&place=local')
  await page.getByRole('group', { name: 'Filter by tool' }).getByRole('button', { name: 'Claude Code', exact: true }).click()
  await page.getByRole('button', { name: 'Load the week before' }).click()
  const archived = page.getByRole('region', { name: /^Archived/ })
  await archived.locator('summary').click()
  await expect(row(page, 'Parser kept')).toBeVisible()
  const reads = state.views.length
  await (await menu(page, 'Parser hidden')).getByRole('menuitem', { name: 'Hide from agnostic-ai' }).click()
  await expect(row(page, 'Parser hidden')).toHaveCount(0)
  await expect(row(page, 'Parser kept')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Load the week before' })).toBeVisible()
  expect(state.views.slice(reads).map((request) => new URLSearchParams(request).has('before'))).toEqual([false])
  for (const request of state.views) expect(new URLSearchParams(request).get('q')).toBe('Parser')
  await expect(page.getByRole('group', { name: 'Filter by tool' }).getByRole('button', { name: 'Claude Code', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(page).toHaveURL(/q=Parser&place=local/)
  await expect(page.getByText('Hidden from agnostic-ai. The transcript stays.', { exact: true })).toBeFocused()
  await page.getByRole('button', { name: 'Load the week before' }).click()
  await expect(row(page, 'Parser kept')).toBeVisible()
  await expect(archived.locator('details')).toHaveAttribute('open')
})

test('mock actions explain the running-server requirement without sending a mutation', async ({ page }) => {
  const state = await setup(page, [])
  await page.goto('/?mock=1')
  const title = 'Plan the sync --scope user dry run'
  await (await menu(page, title)).getByRole('menuitem', { name: 'Mark as read', exact: true }).click()
  await expect(row(page, title).getByRole('status')).toHaveText('Changing conversations needs a running agnostic-ai server.')
  await (await menu(page, title)).getByRole('menuitem', { name: 'Rename…', exact: true }).click()
  await expect(row(page, title).getByRole('textbox')).toHaveCount(0)
  await expect(row(page, title).getByRole('status')).toHaveText('Changing conversations needs a running agnostic-ai server.')
  await page.getByRole('region', { name: /^Archived/ }).locator('summary').click()
  const archived = 'Nightly dependency audit'
  await (await menu(page, archived)).getByRole('menuitem', { name: 'Delete permanently…', exact: true }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(row(page, archived).getByRole('status')).toHaveText('Changing conversations needs a running agnostic-ai server.')
  await page.getByRole('button', { name: 'Mark all complete' }).click()
  await expect(page.getByText('Changing conversations needs a running agnostic-ai server.', { exact: true }).first()).toBeVisible()
  expect(state.posts).toHaveLength(0)
})


test('bulk read exists only on expanded Needs you and affects eligible loaded filtered rows', async ({ page }) => {
  const conflict = { id: 'c', worktree: '/repo', path: 'app.ts', requester: { tool: 'claude' as const, id: 'conflict' } }
  const state = await setup(page, [base({ id: 'read', state: 'blocked' }), base({ id: 'other-tool', title: 'Other tool', tool: 'codex', state: 'unread' }), base({ id: 'conflict', title: 'Conflict', state: 'dormant', claimConflicts: [conflict] }), base({ id: 'remote', title: 'Remote', state: 'unread', app: 'cloud' })])
  await page.goto('/?place=local')
  await page.getByRole('group', { name: 'Filter by tool' }).getByRole('button', { name: 'Claude Code', exact: true }).click()
  const needs = page.getByRole('region', { name: /^Needs you/ })
  await expect(needs.getByRole('button', { name: 'Mark all as read' })).toBeVisible()
  await needs.locator('summary').click()
  await expect(needs.getByRole('button', { name: 'Mark all as read' })).toHaveCount(0)
  await needs.locator('summary').click()
  await needs.getByRole('button', { name: 'Mark all as read' }).click()
  await expect(page.getByText('1 conversation marked as read.', { exact: true })).toBeFocused()
  expect(state.posts.map(({ body }) => body)).toEqual([{ tool: 'claude', id: 'read', as: 'seen' }])
  await expect(needs.getByRole('heading', { level: 3 })).toHaveText(['Conflict'])
})

test('bulk actions stay disabled during a new query debounce and its delayed response', async ({ page }) => {
  const state = await setup(page, [base()])
  let release!: () => void
  await page.route('**/api/views/conversations**', async (route) => {
    if (new URL(route.request().url()).searchParams.get('q')) await new Promise<void>((resolve) => { release = resolve })
    return route.fulfill({ json: { ...fixture, conversations: state.rows } })
  })
  await page.goto('/')
  const bulk = page.getByRole('button', { name: 'Mark all complete' })
  await expect(bulk).toBeEnabled()
  await page.getByRole('searchbox', { name: 'Search conversations' }).fill('another query')
  expect(await bulk.isDisabled()).toBe(true)
  await expect.poll(() => typeof release).toBe('function')
  await expect(bulk).toBeDisabled()
  await bulk.dispatchEvent('click')
  expect(state.posts).toHaveLength(0)
  release()
  await expect(bulk).toBeEnabled()
})

test('old mutation feedback preserves newly chosen search and older-page button focus', async ({ page }) => {
  const state = await setup(page, [base({ state: 'unread' })])
  const before = '2026-10-31T12:00:00Z'
  await page.route('**/api/views/conversations**', (route) => {
    const older = new URL(route.request().url()).searchParams.has('before')
    return route.fulfill({ json: { ...fixture, conversations: older ? [base({ id: 'older', title: 'Older' })] : state.rows, next: older ? '2026-10-24T12:00:00Z' : before } })
  })
  await page.goto('/')
  await (await menu(page)).getByRole('menuitem', { name: 'Mark as read', exact: true }).click()
  await expect(row(page, 'Parser').getByRole('status')).toBeFocused()
  const search = page.getByRole('searchbox', { name: 'Search conversations' })
  await search.fill('Parser')
  await expect(page.locator('.section-search-status')).not.toHaveText('Searching…')
  await expect(search).toBeFocused()
  await search.clear()
  await expect(page.locator('.section-search-status')).toBeEmpty()
  const older = page.getByRole('button', { name: 'Load the week before' })
  await older.click()
  await expect(row(page, 'Older')).toBeVisible()
  await expect(older).toBeFocused()
})

test('bulk freshness stops on a first-page match without reading a failing later week', async ({ page }) => {
  const state = await setup(page, [base()])
  let failOlder = false
  await page.route('**/api/views/conversations**', (route) => {
    const older = new URL(route.request().url()).searchParams.has('before')
    if (older && failOlder) return route.fulfill({ status: 500, json: { error: 'Older week unavailable.' } })
    return route.fulfill({ json: { ...fixture, conversations: older ? [base({ id: 'older', title: 'Older', state: 'working' })] : state.rows, next: older ? undefined : '2026-10-31T12:00:00Z' } })
  })
  await page.goto('/')
  await page.getByRole('button', { name: 'Load the week before' }).click()
  await expect(row(page, 'Older')).toBeVisible()
  failOlder = true
  await page.getByRole('button', { name: 'Mark all complete' }).click()
  await expect(page.getByText('1 conversation marked complete.', { exact: true })).toBeVisible()
  expect(state.posts.map(({ body }) => body.id)).toEqual(['internal'])
})

test('bulk capture skips a shared local row that no longer matches Cloud', async ({ page }) => {
  const state = await setup(page, [base({ cloud: { id: 'shared', link: 'https://claude.ai/code/shared' } })])
  await page.goto('/?place=cloud')
  await expect(row(page, 'Parser')).toBeVisible()
  state.beforeRead = () => { state.rows[0] = { ...state.rows[0], cloud: undefined } }
  await page.getByRole('button', { name: 'Mark all complete' }).click()
  await expect(page.getByText('0 conversations marked complete. 1 changed and skipped.', { exact: true })).toBeVisible()
  expect(state.posts).toHaveLength(0)
})

test('menu action tooltips match the Companion', async ({ page }) => {
  const state = await setup(page, [base({ state: 'unread', live: true, cloud: { id: 'shared', link: 'https://claude.ai/code/shared' }, stuckShells: [{ pid: 37, procStart: 'start', command: 'sleep 90', processCount: 1, since: '2026-11-07T12:00:00Z' }] })])
  await page.goto('/')
  let actions = await menu(page)
  for (const [label, title] of [
    ['Mark as read', 'Takes this chat out of Needs you without opening it, until something new is written.'],
    ['Mark complete', 'Moves this chat to Archived until it has new activity.'],
    ['Rename…', 'Changes this chat’s name in the list.'],
    ['Copy session ID', 'Copies the ID your tool uses to identify this chat.'],
    ['Open on claude.ai', 'Opens this chat on claude.ai.'],
    ['Stop shell…', 'Asks which shell command to stop when the chat has one still running.'],
  ]) await expect(actions.getByRole('menuitem', { name: label, exact: true })).toHaveAttribute('title', title)
  await expect(page.getByRole('button', { name: 'Mark all as read' })).toHaveAttribute('title', 'Mark each chat shown here as read without opening it. A chat that waits on something else, such as a permission or a path conflict, stays here.')
  await page.keyboard.press('Escape')
  state.rows = [base({ state: 'completed', completedAt: 'now' }), base({ id: 'cloud', title: 'Hosted', app: 'cloud', cloudResumeCommand: 'claude --teleport cloud', open: { kind: 'link', label: 'Open on claude.ai' } })]
  await page.reload()
  await expect(page.getByRole('button', { name: 'Mark all complete' })).toHaveAttribute('title', 'Mark every active conversation complete, until something new is written in it or it starts working')
  await expect((await menu(page, 'Hosted')).getByRole('menuitem', { name: 'Copy resume command', exact: true })).toHaveAttribute('title', 'Copies the command to resume this cloud session in Claude Code.')
  await page.keyboard.press('Escape')
  await page.getByRole('region', { name: /^Archived/ }).locator('summary').click()
  actions = await menu(page)
  await expect(actions.getByRole('menuitem', { name: 'Undo complete', exact: true })).toHaveAttribute('title', 'Clears the completion mark you set for this chat.')
  await expect(actions.getByRole('menuitem', { name: 'Hide from agnostic-ai', exact: true })).toHaveAttribute('title', 'Hides this chat from the list and search until it has new activity.')
  await expect(actions.getByRole('menuitem', { name: 'Delete permanently…', exact: true })).toHaveAttribute('title', 'Permanently removes this chat from this computer.')
})

for (const action of ['individual', 'bulk'] as const) test(`${action} delayed POST keeps new search focus after its response and refresh`, async ({ page }) => {
  const state = await setup(page, [base({ state: 'unread' })])
  let release!: () => void
  const released = new Promise<void>((resolve) => { release = resolve })
  await page.route('**/api/conversations/mark', async (route) => {
    state.posts.push({ action: 'mark', body: route.request().postDataJSON() })
    await released
    state.rows[0] = { ...state.rows[0], state: 'active' }
    return route.fulfill({ status: 204 })
  })
  await page.goto('/')
  if (action === 'individual') await (await menu(page)).getByRole('menuitem', { name: 'Mark as read', exact: true }).click()
  else await page.getByRole('button', { name: 'Mark all as read', exact: true }).click()
  await expect.poll(() => state.posts.length).toBe(1)
  const search = page.getByRole('searchbox', { name: 'Search conversations' })
  await search.fill('Parser')
  await expect.poll(() => state.views.some((view) => new URLSearchParams(view).get('q') === 'Parser')).toBe(true)
  await expect(page.locator('.section-search-status')).not.toHaveText('Searching…')
  await expect(search).toBeFocused()
  release()
  await expect(page.getByRole('region', { name: /^Active/ }).getByRole('heading', { level: 3 })).toHaveText(['Parser'])
  await expect(page.getByText(action === 'individual' ? 'Marked as read.' : '1 conversation marked as read.', { exact: true })).toBeVisible()
  await expect(search).toBeFocused()
})

test('a rename field closes on a click elsewhere without taking the focus back', async ({ page }) => {
  const state = await setup(page, [base()])
  await page.goto('/')
  await (await menu(page)).getByRole('menuitem', { name: 'Rename…', exact: true }).click()
  await expect(row(page, 'Parser').getByRole('textbox', { name: 'New name for Parser' })).toBeFocused()
  const search = page.getByRole('searchbox', { name: 'Search conversations' })
  await search.click()
  await page.keyboard.type('pars')
  await expect(search).toHaveValue('pars')
  await expect(search).toBeFocused()
  await expect(row(page, 'Parser').getByRole('textbox')).toHaveCount(0)
  expect(state.posts).toHaveLength(0)
})

test('a second group click during a run is ignored', async ({ page }) => {
  const state = await setup(page, [base()])
  let release!: () => void
  const released = new Promise<void>((resolve) => { release = resolve })
  await page.route('**/api/conversations/mark', async (route) => {
    state.posts.push({ action: 'mark', body: route.request().postDataJSON() })
    await released
    return route.fulfill({ status: 204 })
  })
  await page.goto('/')
  const bulk = page.getByRole('button', { name: 'Mark all complete' })
  await bulk.click()
  await expect.poll(() => state.posts.length).toBe(1)
  await bulk.dispatchEvent('click')
  release()
  await expect(page.getByText('1 conversation marked complete.', { exact: true })).toBeVisible()
  expect(state.posts).toHaveLength(1)
})

test('an open delete dialog closes when a reload shows the row can no longer be deleted', async ({ page }) => {
  const state = await setup(page, [base({ id: 'active', title: 'Active chat' }), base({ state: 'completed', completedAt: 'now' })])
  let release!: () => void
  const released = new Promise<void>((resolve) => { release = resolve })
  await page.route('**/api/conversations/mark', async (route) => {
    state.posts.push({ action: 'mark', body: route.request().postDataJSON() })
    await released
    state.rows[1] = { ...state.rows[1], live: true }
    return route.fulfill({ status: 204 })
  })
  await page.goto('/')
  await page.getByRole('button', { name: 'Mark all complete' }).click()
  await expect.poll(() => state.posts.length).toBe(1)
  await page.getByRole('region', { name: /^Archived/ }).locator('summary').click()
  await (await menu(page)).getByRole('menuitem', { name: 'Delete permanently…', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: /Delete.*permanently/ })
  await expect(dialog).toBeVisible()
  release()
  await expect(dialog).toHaveCount(0)
  await expect(row(page, 'Parser').getByRole('status')).toHaveText('This conversation is still running.')
  await expect(row(page, 'Parser').getByRole('button', { name: 'More actions for Parser' })).toBeFocused()
  expect(state.posts.map(({ action }) => action)).toEqual(['mark'])
})
