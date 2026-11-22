import { expect, test, type Locator, type Page } from '@playwright/test'
import type { ConversationsView } from '../../src/contract'

// The real companion pages, served by companion/dev/preview.mjs with fixture data.
// Every test starts from a fixture; the preview holds one state, so tests run one at a time.

// The time playwright.companion.config.ts gives the preview.
const NOW = new Date('2026-11-07T15:30:00Z')

async function openPanel(page: Page, fixture = 'mixed') {
  const reset = await page.request.post('/dev/fixture', { data: { name: fixture } })
  expect(reset.ok()).toBe(true)
  await page.clock.setFixedTime(NOW)
  await page.goto('/panel.html')
  await expect(page.getByLabel('Search every conversation')).toBeFocused()
  // The search takes focus before the first list arrives; keys pressed earlier have no rows to move through.
  await expect(page.locator('#list > .row, #list > .empty').first()).toBeVisible()
}

const row = (page: Page, title: string) => page.locator('#list').getByRole('option').filter({ hasText: title })
const selected = (page: Page) => page.locator('.row[aria-selected="true"]')
/** The --alert token, as a computed style spells it. */
const ALERT_RED = /rgb\(255, 98, 80\)/

test('the packaged panel opens once while conversations are still loading', async ({ page }) => {
  await page.request.post('/dev/fixture', { data: { name: 'empty' } })
  let finish!: () => void
  const loading = new Promise<void>((resolve) => { finish = resolve })
  await page.route('**/api/inbox', async (route) => {
    await loading
    await route.fulfill({ json: { view: { conversations: [], needsYou: 0 }, configRoot: '/Users/test/global settings' } })
  })
  await page.goto('/panel.html?packaged')
  await expect.poll(() => calledPaths(page)).toContain('/api/panel/show')
  await expect(page.getByText('Loading conversations…')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Open the dashboard' })).toBeVisible()
  finish()
  await expect(page.getByText('No conversations yet')).toBeVisible()
  await page.getByRole('button', { name: 'Settings', exact: true }).click()
  await expect(page.getByText('Configuration folder: /Users/test/global settings')).toBeVisible()
  await page.reload()
  await expect(page.getByText('No conversations yet')).toBeVisible()
  expect((await calledPaths(page)).filter((path) => path === '/api/panel/show')).toHaveLength(1)
})

test('search, select and open names the chosen conversation', async ({ page }) => {
  await openPanel(page)
  await page.getByLabel('Search every conversation').fill('login')
  await expect(page.getByRole('option')).toHaveCount(1)
  await expect(row(page, 'Fix the login redirect')).toContainText('Cursor · shop · 3h')

  await page.keyboard.press('ArrowDown')
  await expect(selected(page)).toContainText('Fix the login redirect')
  const opened = page.waitForRequest((request) => request.url().endsWith('/api/open'))
  await page.keyboard.press('Enter')
  expect((await opened).postDataJSON()).toEqual({ tool: 'cursor', id: 'active-1', title: 'Fix the login redirect' })
})

test('Escape clears a search first and then closes the list', async ({ page }) => {
  await openPanel(page)
  const search = page.getByLabel('Search every conversation')
  const rows = page.getByRole('option')
  const total = await rows.count()
  await search.fill('login')
  await expect(rows).toHaveCount(1)

  await search.press('Escape')
  await expect(search).toHaveValue('')
  await expect(search).toBeFocused()
  await expect(rows).toHaveCount(total)
  expect(await calledPaths(page)).not.toContain('/api/panel/hide')

  await search.press('Escape')
  await expect.poll(() => calledPaths(page)).toContain('/api/panel/hide')
})

test('a row shows the agent\'s latest reply under its title, and a row without one shows none', async ({ page }) => {
  await openPanel(page)
  const waiting = row(page, 'Approve the database migration')
  await expect(waiting.locator('.text > *')).toHaveClass(['title', 'needs-reason is-waiting', 'reply', /meta/])
  await expect(waiting.locator('.reply')).toHaveText('Should I run the migration on the staging copy first?')
  await expect(row(page, 'Fix the login redirect').locator('.reply')).toHaveCount(0)
})

test('rename then Escape keeps the old name and gives the keys back to the list', async ({ page }) => {
  await openPanel(page)
  const renames: string[] = []
  page.on('request', (request) => {
    if (request.url().endsWith('/api/rename')) renames.push(request.url())
  })
  await row(page, 'Plan the sync dry run').click({ button: 'right' })
  await expect(page.getByRole('menuitem', { name: 'Rename…' })).toBeFocused()
  await page.getByRole('menuitem', { name: 'Rename…' }).press('Enter')

  const field = page.getByLabel('New name')
  await expect(field).toBeFocused()
  await expect(field).toHaveValue('Plan the sync dry run')
  await field.fill('Something else')
  await field.press('Escape')

  await expect(field).toHaveCount(0)
  await expect(row(page, 'Plan the sync dry run')).toBeVisible()
  expect(renames).toEqual([])
  await expect(page.getByLabel('Search every conversation')).toBeFocused()
  const before = await selected(page).textContent()
  await page.keyboard.press('ArrowDown')
  await expect(selected(page)).not.toHaveText(before ?? '')
})

const calledPaths = async (page: Page) => ((await (await page.request.get('/dev/calls')).json()) as { path: string }[]).map((call) => call.path)

test('the pencil starts the rename and opens nothing; Escape cancels it', async ({ page }) => {
  await openPanel(page)
  const target = row(page, 'Plan the sync dry run')
  await target.hover()
  const pencil = target.getByRole('button', { name: 'Rename', exact: true })
  await expect(pencil).toBeVisible()
  await expect(pencil).toHaveAttribute('title', 'Rename')
  await pencil.click()

  const field = page.getByLabel('New name')
  await expect(field).toBeFocused()
  await expect(field).toHaveValue('Plan the sync dry run')
  await field.fill('Something else')
  await field.press('Escape')

  await expect(field).toHaveCount(0)
  await expect(row(page, 'Plan the sync dry run')).toBeVisible()
  await expect(page.getByLabel('Search every conversation')).toBeFocused()
  const calls = await calledPaths(page)
  expect(calls).not.toContain('/api/open')
  expect(calls).not.toContain('/api/panel/hide')
  expect(calls).not.toContain('/api/rename')
})

test('Enter on the focused pencil starts the rename and opens nothing', async ({ page }) => {
  await openPanel(page)
  const target = row(page, 'Plan the sync dry run')
  await target.hover()
  await target.getByRole('button', { name: 'Rename', exact: true }).press('Enter')

  await expect(page.getByLabel('New name')).toBeFocused()
  const calls = await calledPaths(page)
  expect(calls).not.toContain('/api/open')
  expect(calls).not.toContain('/api/panel/hide')
})

test('Enter on a focused header or footer button runs only that button, with a row selected', async ({ page }) => {
  for (const button of [
    page.getByRole('button', { name: 'Settings', exact: true }),
    page.getByRole('button', { name: 'Open the dashboard' }),
    page.getByRole('button', { name: 'Local', exact: true }),
  ]) {
    await openPanel(page)
    await page.keyboard.press('ArrowDown')
    await expect(selected(page)).toHaveCount(1)
    await button.press('Enter')
    expect(await calledPaths(page)).not.toContain('/api/open')
  }
  await expect(page.getByRole('button', { name: 'Local', exact: true })).toHaveAttribute('aria-pressed', 'true')

  await openPanel(page)
  await page.getByLabel('Search every conversation').fill('login')
  await page.keyboard.press('ArrowDown')
  await page.getByRole('button', { name: 'Clear the search' }).press('Enter')
  await expect(page.getByLabel('Search every conversation')).toHaveValue('')
  expect(await calledPaths(page)).not.toContain('/api/open')
})

test('the keys leave the list alone while the settings menu is open', async ({ page }) => {
  await openPanel(page)
  await page.keyboard.press('ArrowDown')
  const before = await selected(page).textContent()
  await page.getByRole('button', { name: 'Settings', exact: true }).press('Enter')
  await expect(page.getByRole('region', { name: 'Settings', exact: true })).toBeVisible()

  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('ArrowDown')
  await expect(selected(page)).toHaveText(before ?? '')
  await page.getByLabel('Search every conversation').focus()
  await page.keyboard.press('Enter')
  expect(await calledPaths(page)).not.toContain('/api/open')

  await page.keyboard.press('Escape')
  await expect(page.getByRole('region', { name: 'Settings', exact: true })).toBeHidden()
  await expect(page.getByLabel('Search every conversation')).toBeFocused()
})

test('the search takes at most 200 characters and keeps the rows', async ({ page }) => {
  await openPanel(page)
  const search = page.getByLabel('Search every conversation')
  await search.focus()
  await page.keyboard.insertText('login '.repeat(50))
  expect((await search.inputValue()).length).toBe(200)
  await expect(row(page, 'Fix the login redirect')).toBeVisible()
})

test('F2 renames the selected row and Enter saves the name', async ({ page }) => {
  await openPanel(page)
  await page.getByLabel('Search every conversation').fill('sync dry run')
  await expect(page.getByRole('option')).toHaveCount(1)
  await page.keyboard.press('ArrowDown')
  await expect(selected(page)).toContainText('Plan the sync dry run')
  await page.keyboard.press('F2')

  const field = page.getByLabel('New name')
  await expect(field).toBeFocused()
  await expect(field).toHaveValue('Plan the sync dry run')
  const renamed = page.waitForRequest((request) => request.url().endsWith('/api/rename'))
  await field.fill('Ship the dry run')
  await field.press('Enter')
  expect((await renamed).postDataJSON()).toMatchObject({ id: 'active-2', title: 'Ship the dry run' })
  await expect(field).toHaveCount(0)
  expect(await calledPaths(page)).not.toContain('/api/open')
})

test('a rename during a search asks the server for the search once', async ({ page }) => {
  await openPanel(page)
  await page.getByLabel('Search every conversation').fill('sync dry run')
  await expect(page.getByRole('option')).toHaveCount(1)
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('F2')
  const field = page.getByLabel('New name')
  await expect(field).toBeFocused()
  const searches: string[] = []
  let renamed = false
  page.on('request', (request) => {
    if (request.url().endsWith('/api/rename')) renamed = true
    else if (renamed && request.url().includes('/api/inbox?q=')) searches.push(request.url())
  })

  await field.fill('Ship the dry run')
  await field.press('Enter')
  await expect(field).toHaveCount(0)
  // Nothing on screen marks a second search, so give one time to start.
  await page.waitForTimeout(500)
  expect(searches).toHaveLength(1)
})

test('a click on the name still opens the chat', async ({ page }) => {
  await openPanel(page)
  const opened = page.waitForRequest((request) => request.url().endsWith('/api/open'))
  await row(page, 'Plan the sync dry run').locator('.title').click()
  expect((await opened).postDataJSON()).toEqual({ tool: 'claude', id: 'active-2', title: 'Plan the sync dry run' })
  await expect(page.getByLabel('New name')).toHaveCount(0)
})

test('a refused rename keeps the typed name in focus and says why', async ({ page }) => {
  await openPanel(page, 'rename-failure')
  await row(page, 'Plan the sync dry run').click({ button: 'right' })
  await page.getByRole('menuitem', { name: 'Rename…' }).click()
  const field = page.getByLabel('New name')
  await field.fill('Ship the dry run')
  await field.press('Enter')

  await expect(page.getByText('agnostic-ai could not save the name: the marks file is read-only.')).toBeVisible()
  await expect(field).toBeFocused()
  await expect(field).toHaveValue('Ship the dry run')

  await field.press('Escape')
  await expect(row(page, 'Plan the sync dry run')).toBeVisible()
})

const completeButton = (target: Locator) => target.getByRole('button', { name: /^Mark complete,/ })
const groupTitle = (page: Page, name: string) => page.locator('.group-title').filter({ hasText: name })

test('marking one chat complete shows a green tick, then archives it', async ({ page }) => {
  await openPanel(page)
  const target = row(page, 'Fix the login redirect')
  await target.hover()
  await completeButton(target).click()

  await expect(target.locator('.action.is-done')).toBeVisible()
  // Archived starts folded; unfolded, the chat is in it with the button archived chats have.
  await expect(target).toBeHidden()
  await groupTitle(page, 'Archived').click()
  await expect(target.getByRole('button', { name: /^Hide from agnostic-ai,/ })).toBeAttached()
})

test('the row menu marks a chat complete with a tick and can return it to Active', async ({ page }) => {
  await openPanel(page)
  const marks = markBodies(page)
  const target = row(page, 'Fix the login redirect')
  await target.click({ button: 'right' })
  await page.getByRole('menuitem', { name: 'Mark complete' }).click()

  await expect(target.locator('.action.is-done')).toBeAttached()
  await expect(target).toBeHidden()
  await groupTitle(page, 'Archived').click()
  await target.click({ button: 'right' })
  await page.getByRole('menuitem', { name: 'Undo complete' }).click()
  await expect(completeButton(target)).toBeAttached()
  expect(marks).toEqual([
    { tool: 'cursor', id: 'active-1', as: 'completed' },
    { tool: 'cursor', id: 'active-1', as: 'open' },
  ])
})

test('the row menu of an archived chat undoes complete or hides it', async ({ page }) => {
  await openArchived(page)
  const marks = markBodies(page)
  await row(page, 'Tidy the old branches').click({ button: 'right' })
  await expect(page.getByRole('menuitem', { name: 'Mark complete' })).toBeHidden()
  await page.getByRole('menuitem', { name: 'Undo complete' }).click()
  // Closed and quiet for days, it stays under Archived, now without the mark to undo.
  await row(page, 'Tidy the old branches').click({ button: 'right' })
  await expect(page.getByRole('menuitem', { name: 'Undo complete' })).toBeHidden()
  await page.keyboard.press('Escape')

  await row(page, 'Sketch the pricing page').click({ button: 'right' })
  await expect(page.getByRole('menuitem', { name: 'Undo complete' })).toBeHidden()
  await page.getByRole('menuitem', { name: 'Hide from agnostic-ai' }).click()
  await expect(row(page, 'Sketch the pricing page')).toHaveCount(0)
  expect(marks).toEqual([
    { tool: 'claude', id: 'done-2', as: 'open' },
    { tool: 'claude', id: 'old-2', as: 'hidden' },
  ])
})

test('a chat waiting on a forgotten shell keeps its place and only offers Undo after completion', async ({ page }) => {
  await openPanel(page, 'stuck-shells')
  const target = row(page, 'Preview the reporting changes')
  await target.click({ button: 'right' })
  await page.getByRole('menuitem', { name: 'Mark complete' }).click()
  await expect(completeButton(target)).toHaveCount(0)
  await expect(target).toHaveClass(/needs-you/)

  await target.click({ button: 'right' })
  await expect(page.getByRole('menuitem', { name: 'Mark complete' })).toBeHidden()
  await page.getByRole('menuitem', { name: 'Undo complete' }).click()
  await expect(completeButton(target)).toBeAttached()
})

test('a refused mark complete gives the tick back, keeps the row and says why', async ({ page }) => {
  await openPanel(page, 'mark-failure')
  const target = row(page, 'Fix the login redirect')
  await target.hover()
  await completeButton(target).click()

  await expect(page.getByRole('alert').filter({ hasText: 'agnostic-ai could not save the mark' })).toBeVisible()
  await expect(page.locator('#toast')).toHaveAttribute('data-tone', 'error')
  await expect(page.locator('#toast')).toHaveCSS('box-shadow', ALERT_RED)
  await expect(page.locator('#toast .tone-icon')).toBeVisible()
  await expect(target).toBeVisible()
  await expect(target.locator('.action.is-done')).toHaveCount(0)
})

test('mark all complete archives every active chat and shows no tick', async ({ page }) => {
  await openPanel(page)
  const ticks: number[] = []
  await page.exposeFunction('tickSeen', () => ticks.push(1))
  await page.evaluate(() => {
    const seen = new MutationObserver(() => {
      if (document.querySelector('.action.is-done')) (window as unknown as { tickSeen: () => void }).tickSeen()
    })
    seen.observe(document.getElementById('list')!, { subtree: true, attributes: true, attributeFilter: ['class'] })
  })
  const marks: string[] = []
  page.on('request', (request) => {
    if (new URL(request.url()).pathname === '/api/mark') marks.push(new URL(request.url()).search)
  })
  await expect(groupTitle(page, 'Active')).toBeVisible()
  await page.getByRole('button', { name: 'Mark all complete' }).click()

  await expect(groupTitle(page, 'Active')).toHaveCount(0)
  expect(ticks).toHaveLength(0)
  // Only the last mark waits for the list read after it.
  expect(marks.length).toBeGreaterThan(1)
  expect(marks).toEqual([...marks.slice(0, -1).map(() => '?read=later'), ''])
})

const RESUME = "cd '/Users/you/Code/agnostic-ai' && claude --resume active-2"
const notice = (page: Page) => page.getByRole('region', { name: 'Why it did not open' })

const noticeCard = (page: Page) => page.locator('#notice')

/** An error notice has a red border and a warning icon, and is read out as an alert that starts with what it is about. */
async function expectErrorNotice(page: Page, label: string) {
  await expect(noticeCard(page)).toHaveAttribute('data-tone', 'error')
  await expect(noticeCard(page)).toHaveCSS('box-shadow', ALERT_RED)
  await expect(noticeCard(page).locator('.tone-icon')).toBeVisible()
  await expect(noticeCard(page).getByRole('alert')).toContainText(label)
  await expect(noticeCard(page).getByRole('status')).toHaveCount(0)
}

/** An info notice keeps the neutral card: no red, no icon, and it waits its turn to be read. */
async function expectInfoNotice(page: Page, label: string) {
  await expect(noticeCard(page)).toHaveAttribute('data-tone', 'info')
  await expect(noticeCard(page)).not.toHaveCSS('box-shadow', ALERT_RED)
  await expect(noticeCard(page).locator('.tone-icon')).toBeHidden()
  await expect(noticeCard(page).getByRole('status')).toContainText(label)
  await expect(noticeCard(page).getByRole('alert')).toHaveCount(0)
}

async function openFailing(page: Page, fixture: string) {
  await openPanel(page, fixture)
  await row(page, 'Plan the sync dry run').click()
  await expect(notice(page)).toBeVisible()
}

test('a failed open keeps its reason on screen, with no copy button when the server sent no command', async ({ page }) => {
  await openFailing(page, 'open-failure')
  await expect(notice(page)).toContainText('Codex runs in an app that cannot be brought forward. Open it from the Codex app.')
  await expect(notice(page).getByRole('button', { name: 'Copy resume command' })).toHaveCount(0)
  await expectErrorNotice(page, 'Why it did not open')
  await expect(selected(page)).toContainText('Plan the sync dry run')
  // The toast this replaces was gone after 3.2 seconds.
  await page.waitForTimeout(3_500)
  await expect(notice(page)).toBeVisible()
  // The panel hides when it loses focus; showing it again keeps the notice.
  expect((await page.request.post('/api/panel/show')).ok()).toBe(true)
  await expect(notice(page)).toBeVisible()
  // Only the return from the failed open keeps its row highlighted.
  await expect(selected(page)).toHaveCount(0)

  const opens: string[] = []
  page.on('request', (request) => {
    if (request.url().endsWith('/api/open')) opens.push(request.url())
  })
  await notice(page).getByRole('button', { name: 'Dismiss' }).press('Enter')
  await expect(notice(page)).toBeHidden()
  await expect(page.getByLabel('Search every conversation')).toBeFocused()
  expect(opens).toEqual([])
})

test('an open the server is slow to answer hides the list and shows no failure', async ({ page }) => {
  await openPanel(page, 'slow-open')
  const answer = page.waitForResponse((response) => response.url().endsWith('/api/open'))
  await row(page, 'Plan the sync dry run').click()
  expect((await answer).status()).toBe(202)
  // Nothing on screen marks an answer the panel ignored, so give it time to wrongly come back.
  await page.waitForTimeout(300)

  const calls = await calledPaths(page)
  expect(calls).toContain('/api/panel/hide')
  expect(calls).not.toContain('/api/panel/show')
  await expect(notice(page)).toBeHidden()
})

const LATE_RESUME = "cd '/Users/you/Code/agnostic-ai' && claude --resume done-2"

/** Opens "Tidy the old branches", which the companion answers as still opening. */
async function openSlowly(page: Page) {
  await openArchived(page, 'late-open-failure')
  const answer = page.waitForResponse((response) => response.url().endsWith('/api/open'))
  // The panel asks once for a late failure after it handled the answer, so later reads are its own.
  const handled = page.waitForResponse((response) => response.url().endsWith('/api/open/late'))
  await row(page, 'Tidy the old branches').click()
  expect((await answer).status()).toBe(202)
  await handled
}

/** Ends the open answered as still opening, with the fixture's late failure. */
async function settleLateOpen(page: Page) {
  expect(await (await page.request.post('/dev/settle-late-opens')).json()).toEqual({ settled: 1 })
}

/** Shows the list again, as a click on the circle does, and waits for the panel to ask for a late failure. */
async function showAgain(page: Page) {
  const asked = page.waitForResponse((response) => response.url().endsWith('/api/open/late'))
  expect((await page.request.post('/api/panel/show')).ok()).toBe(true)
  await asked
}

async function expectLateFailure(page: Page) {
  await expect(notice(page)).toContainText('Tidy the old branches: macOS blocked opening Terminal.')
  await expect(notice(page)).toContainText(LATE_RESUME)
  await expect(notice(page).getByRole('button', { name: 'Copy resume command' })).toBeVisible()
  await expectErrorNotice(page, 'Why it did not open')
}

test('a late open failure that lands while the list shows appears at once', async ({ page }) => {
  await openSlowly(page)
  await showAgain(page)
  await expect(notice(page)).toBeHidden()

  await settleLateOpen(page)
  await expectLateFailure(page)
  expect(await calledPaths(page)).toContain('/api/panel/show')
})

test('a late open failure shows after the page reloaded during the wait', async ({ page }) => {
  await openSlowly(page)
  await page.reload()
  await expect(page.locator('#list > .row, #list > .empty').first()).toBeVisible()
  await settleLateOpen(page)

  await showAgain(page)
  await expectLateFailure(page)
})

test('a late open failure shows with a search typed', async ({ page }) => {
  await openSlowly(page)
  await page.getByLabel('Search every conversation').fill('branches')
  await expect(page.getByRole('option')).toHaveCount(1)
  await settleLateOpen(page)

  await showAgain(page)
  await expectLateFailure(page)
})

test('a late open failure survives the list opening through the send shortcut', async ({ page }) => {
  await openSlowly(page)
  await settleLateOpen(page)
  await expectLateFailure(page)

  await pressSendShortcut(page)
  await expect(pickBanner(page)).toBeVisible()
  await expectLateFailure(page)
})

test('a send made while an open was still opening does not drop its late failure', async ({ page }) => {
  await openSlowly(page)
  await page.keyboard.press('Meta+Shift+KeyV')
  await expect(pickBanner(page)).toBeVisible()
  const sent = page.waitForResponse((response) => response.url().endsWith('/api/send'))
  await page.keyboard.press('Enter')
  expect((await sent).ok()).toBe(true)
  await settleLateOpen(page)

  await showAgain(page)
  await expectLateFailure(page)
})

test('a notice shown and dismissed while an open was still opening does not drop its late failure', async ({ page }) => {
  await openArchived(page, 'held-late-open-failure')
  const answer = page.waitForResponse((response) => response.url().endsWith('/api/open'))
  await row(page, 'Tidy the old branches').click()
  await expect.poll(async () => (await calledPaths(page)).includes('/api/open')).toBe(true)
  await row(page, 'Bump the Go version').click({ button: 'right' })
  await deleteItem(page).click()
  await expectErrorNotice(page, 'Why it cannot be deleted')
  expect(await (await page.request.post('/dev/release-opens')).json()).toEqual({ released: 1 })
  expect((await answer).status()).toBe(202)
  await noticeCard(page).getByRole('button', { name: 'Dismiss' }).click()
  await expect(noticeCard(page)).toBeHidden()

  await settleLateOpen(page)
  await expectLateFailure(page)
})

type HeldOpens = Window & { releaseOpens: () => void; inboxesHandled: number }

/** Holds the page's view of each /api/open answer until releaseOpens, and counts the inbox events the page handled. */
function holdOpenAnswers() {
  const held: (() => void)[] = []
  const page = window as unknown as HeldOpens
  page.releaseOpens = () => held.splice(0).forEach((release) => release())
  page.inboxesHandled = 0
  const fetchFirst = window.fetch
  window.fetch = async (input, init) => {
    const response = await fetchFirst(input, init)
    if (String(input).endsWith('/api/open')) await new Promise<void>((resolve) => held.push(resolve))
    return response
  }
  const listen = EventSource.prototype.addEventListener
  EventSource.prototype.addEventListener = function (this: EventSource, type: string, listener: EventListener, options?: boolean | AddEventListenerOptions) {
    if (type !== 'message') return listen.call(this, type, listener, options)
    listen.call(this, type, (event: Event) => {
      listener.call(this, event)
      if (JSON.parse((event as MessageEvent).data).name === 'inbox') page.inboxesHandled += 1
    }, options)
  } as typeof listen
}

test('a late open failure that lands before the panel read its open\'s answer still shows', async ({ page }) => {
  await page.addInitScript(holdOpenAnswers)
  await openArchived(page, 'late-open-failure')
  const answer = page.waitForResponse((response) => response.url().endsWith('/api/open'))
  await row(page, 'Tidy the old branches').click()
  expect((await answer).status()).toBe(202)
  const handled = await page.evaluate(() => (window as unknown as HeldOpens).inboxesHandled)

  await settleLateOpen(page)
  await page.waitForFunction((before) => (window as unknown as HeldOpens).inboxesHandled > before, handled)
  await page.evaluate(() => (window as unknown as HeldOpens).releaseOpens())
  await expectLateFailure(page)
})

test('a dismissed late open failure stays gone', async ({ page }) => {
  await openSlowly(page)
  await settleLateOpen(page)
  await expectLateFailure(page)

  const dismissed = page.waitForResponse((response) => response.url().endsWith('/api/open/late/dismiss'))
  await notice(page).getByRole('button', { name: 'Dismiss' }).click()
  expect((await dismissed).status()).toBe(204)
  await showAgain(page)
  await page.waitForTimeout(300)
  await expect(noticeCard(page)).toBeHidden()
  expect((await page.request.get('/api/open/late')).status()).toBe(204)
})

test('a newer open drops a late open failure', async ({ page }) => {
  await openSlowly(page)
  await settleLateOpen(page)
  await expectLateFailure(page)

  const answer = page.waitForResponse((response) => response.url().endsWith('/api/open'))
  await row(page, 'Sketch the pricing page').click()
  expect((await answer).status()).toBe(202)
  await expect(noticeCard(page)).toBeHidden()
  expect((await page.request.get('/api/open/late')).status()).toBe(204)
  await showAgain(page)
  await page.waitForTimeout(300)
  await expect(noticeCard(page)).toBeHidden()
})

test('a newer notice wins while it shows, and the late open failure follows once it is dismissed', async ({ page }) => {
  await openSlowly(page)
  await row(page, 'Bump the Go version').click({ button: 'right' })
  await deleteItem(page).click()
  await expectErrorNotice(page, 'Why it cannot be deleted')
  await settleLateOpen(page)

  await showAgain(page)
  // Nothing on screen marks a failure the panel held back, so give it time to wrongly show one.
  await page.waitForTimeout(300)
  await expectErrorNotice(page, 'Why it cannot be deleted')
  await expect(noticeCard(page)).not.toContainText('macOS blocked opening Terminal.')

  await noticeCard(page).getByRole('button', { name: 'Dismiss' }).click()
  await expectLateFailure(page)
})

test('the next open replaces the notice', async ({ page }) => {
  await openFailing(page, 'resume-failure')
  await expect(notice(page)).toContainText(RESUME)

  expect((await page.request.post('/dev/fixture', { data: { name: 'open-failure' } })).ok()).toBe(true)
  await row(page, 'Fix the login redirect').click()
  await expect(notice(page)).toContainText('Codex runs in an app that cannot be brought forward.')
  await expect(notice(page).getByRole('button', { name: 'Copy resume command' })).toHaveCount(0)

  expect((await page.request.post('/dev/fixture', { data: { name: 'mixed' } })).ok()).toBe(true)
  await row(page, 'Fix the login redirect').click()
  await expect(notice(page)).toBeHidden()
})

test('a held failed open that answers after a newer open leaves the newer notice alone', async ({ page }) => {
  await openPanel(page, 'held-open-failure')
  const heldAnswer = page.waitForResponse((response) => response.url().endsWith('/api/open') && response.request().postDataJSON().id === 'active-2')
  await row(page, 'Plan the sync dry run').click()
  // The preview holds this open under its fixture before the switch below.
  await expect.poll(async () => (await (await page.request.get('/dev/calls')).json()).some((call: { path: string }) => call.path === '/api/open')).toBe(true)

  expect((await page.request.post('/api/panel/show')).ok()).toBe(true)
  expect((await page.request.post('/dev/fixture', { data: { name: 'open-failure' } })).ok()).toBe(true)
  await row(page, 'Fix the login redirect').click()
  await expect(notice(page)).toContainText('Codex runs in an app that cannot be brought forward.')

  expect(await (await page.request.post('/dev/release-opens')).json()).toEqual({ released: 1 })
  expect((await (await heldAnswer).json()).command).toBe(RESUME)
  // Nothing on screen marks an answer the panel ignored, so give it time to wrongly show one.
  await page.waitForTimeout(300)
  await expect(notice(page)).toContainText('Codex runs in an app that cannot be brought forward.')
  await expect(notice(page)).not.toContainText(RESUME)
  await expect(notice(page).getByRole('button', { name: 'Copy resume command' })).toHaveCount(0)
})

for (const [title, id, archived] of [
  ['Draft the release notes', 'reply-1'],
  ['Refactor the search index', 'work-1'],
  ['Bump the Go version', 'done-3', true],
  ['Write the onboarding guide', 'cloud-1'],
] as const) {
  test(`Copy session ID copies the exact row ID for ${title}`, async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    if (archived) await openArchived(page)
    else await openPanel(page)
    const chat = row(page, title)
    const item = page.getByRole('menuitem', { name: 'Copy session ID' })
    const menu = page.getByRole('menu', { name: 'Conversation' })
    await chat.click({ button: 'right' })
    await page.getByRole('menuitem', { name: 'Rename…' }).press('ArrowDown')
    await expect(item).toBeFocused()
    await page.keyboard.press('Escape')
    await expect(menu).toBeHidden()
    await chat.click({ button: 'right' })
    await page.getByRole('menuitem', { name: 'Rename…' }).press('ArrowDown')
    await page.keyboard.press('Enter')

    await expect(menu).toBeHidden()
    await expect(page.locator('#toast[role=status]')).toHaveText('Session ID copied.')
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(id)
    expect((await calledPaths(page)).filter((path) => path === '/api/mark' || path === '/api/open')).toEqual([])
  })
}

test('Copy session ID shows an error when the clipboard refuses', async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(navigator, 'clipboard', { value: { writeText: () => Promise.reject(new Error('Write permission denied.')) } })
  })
  await openPanel(page)
  await row(page, 'Draft the release notes').click({ button: 'right' })
  await page.getByRole('menuitem', { name: 'Copy session ID' }).click()

  await expect(page.getByRole('menu', { name: 'Conversation' })).toBeHidden()
  await expect(page.locator('#toast[data-tone=error]')).toHaveText('Could not copy the session ID.')
  expect((await calledPaths(page)).filter((path) => path === '/api/mark' || path === '/api/open')).toEqual([])
})

const mailboxTarget = '0199a2b4-5c1e-7d30-9f2a-3b6c8d1e4f50'
const mailboxThread = (page: Page) => page.getByRole('region', { name: 'Thread with Draft the tests' })
const mailboxSummary = (page: Page) => page.getByRole('button', { name: /^With Draft the tests/ })

const mailboxSend = (page: Page) => page.getByRole('button', { name: /^Send( to .+)?$/ })
const mailboxTo = (page: Page) => page.getByRole('combobox', { name: 'To', exact: true })
const mailboxTargetLabel = 'Draft the tests · Codex · agnostic-ai'

async function chooseMailboxTarget(page: Page) {
  await mailboxTo(page).selectOption({ label: mailboxTargetLabel })
}

async function composeMailbox(page: Page) {
  await row(page, 'Review the parser').click({ button: 'right' })
  await page.getByRole('menuitem', { name: 'Message another session…' }).click()
  await expect(page.getByRole('form', { name: 'Message another session' })).toBeVisible()
}

async function askMailbox(page: Page, text = 'Is the parser ready?') {
  await composeMailbox(page)
  await chooseMailboxTarget(page)
  await page.getByLabel('Message', { exact: true }).fill(text)
  await mailboxSend(page).click()
  await expect(mailboxSummary(page)).toBeVisible()
}

async function replyInMailbox(page: Page, text: string) {
  await mailboxThread(page).getByRole('button', { name: 'Reply' }).click()
  await page.getByLabel('Message', { exact: true }).fill(text)
  await mailboxSend(page).click()
}

test('sends a question to a pasted session ID and shows the thread under both sessions', async ({ page }) => {
  await openPanel(page, 'mailbox')
  await expect(page.locator('.mailbox-summary')).toHaveCount(0)
  const send = page.waitForRequest((request) => request.url().endsWith('/api/mailbox/send'))
  await composeMailbox(page)
  await page.getByRole('button', { name: 'Enter an ID instead' }).click()
  await page.getByLabel('Session ID', { exact: true }).fill(mailboxTarget)
  await expect(page.locator('#mailbox-target-status')).toHaveText('Found: Draft the tests · Codex.')
  await page.getByLabel('Message', { exact: true }).fill('Is the parser ready?')
  await page.getByRole('button', { name: 'Send to Draft the tests' }).click()
  expect((await send).postDataJSON()).toMatchObject({ to: { tool: 'codex', id: mailboxTarget }, type: 'question', body: 'Is the parser ready?' })
  await expect(page.locator('#toast')).toHaveText('Message waiting in the thread with Draft the tests.')
  await expect(mailboxSummary(page)).toContainText('Question · Open · 1 of 8 messages, 1 waiting')
  await expect(page.getByRole('button', { name: /^With Review the parser/ })).toContainText('1 of 8 messages, 1 waiting')
  await mailboxSummary(page).press('Enter')
  await expect(mailboxThread(page)).toContainText('Review the parser · Question · Waiting')
  await expect(mailboxThread(page)).toContainText('Is the parser ready?')
  expect(await calledPaths(page)).not.toContain('/api/mark')
})

test('chooses a mailbox target from a row without opening it and keeps the message keyboard usable', async ({ page }) => {
  await openPanel(page, 'mailbox')
  await page.keyboard.press('ArrowDown')
  await expect(selected(page)).toContainText('Review the parser')
  await page.keyboard.press('Shift+F10')
  for (let step = 0; step < 3; step += 1) await page.keyboard.press('ArrowDown')
  await expect(page.getByRole('menuitem', { name: 'Message another session…' })).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(row(page, 'Draft the tests')).toHaveClass(/is-mailbox-receivable/)
  await expect(row(page, 'Review the parser').locator('.mailbox-tag')).toHaveText('From')
  await row(page, 'Draft the tests').click()
  await expect(mailboxTo(page)).toHaveValue(`codex:${mailboxTarget}`)
  await expect(row(page, 'Draft the tests').locator('.mailbox-tag')).toHaveText('To')
  await expect(page.getByRole('button', { name: 'Send to Draft the tests' })).toBeVisible()
  await page.getByRole('combobox', { name: 'Type', exact: true }).selectOption('finding')
  const field = page.getByLabel('Message', { exact: true })
  await field.fill('First finding')
  await field.press('End')
  await field.press('Enter')
  await field.press('ArrowUp')
  await expect(field).toHaveValue('First finding\n')
  await mailboxSend(page).press('Enter')
  await expect(mailboxSummary(page)).toContainText('Finding')
  const calls = await calledPaths(page)
  expect(calls).not.toContain('/api/open')
  expect(calls).not.toContain('/api/mark')
})

test('a mailbox refusal shows its guard, reason and next step and keeps the draft', async ({ page }) => {
  await openPanel(page, 'mailbox')
  await composeMailbox(page)
  await page.getByRole('button', { name: 'Enter an ID instead' }).click()
  await page.getByLabel('Session ID', { exact: true }).fill('4e8b1c2d-9a3f-4b6e-8c7d-1f2a3b4c5d6e')
  await expect(page.locator('#mailbox-target-status')).toHaveText('No session in the list has this ID. It may not be loaded yet.')
  await expect(page.locator('#mailbox-arrival')).toBeHidden()
  await page.getByLabel('Message', { exact: true }).fill('Still here')
  await page.getByRole('button', { name: 'Send', exact: true }).click()
  await expect(page.locator('#toast[data-tone=error]')).toContainText('guard: unknown-session')
  await expect(page.locator('#toast')).toContainText('Choose a local session')
  await expect(page.getByLabel('Message', { exact: true })).toHaveValue('Still here')
  await expect(page.locator('.mailbox-summary')).toHaveCount(0)
})

test('names both ends of a new message and sends the chosen tool and ID', async ({ page }) => {
  await openPanel(page, 'mailbox')
  await composeMailbox(page)
  const form = page.getByRole('form', { name: 'Message another session' })
  await expect(page.locator('#mailbox-from')).toHaveText('Review the parser · Claude Code')
  await expect(mailboxTo(page)).toHaveValue('')
  await expect(mailboxSend(page)).toHaveText('Send')
  await expect(mailboxSend(page)).toBeDisabled()
  await expect(form).toContainText('Ask the other session something.')
  await expect(page.locator('#mailbox-arrival')).toBeHidden()
  await chooseMailboxTarget(page)
  await expect(form).toContainText('Ask Draft the tests something. Its reply comes back to this thread.')
  await expect(page.locator('#mailbox-arrival')).toHaveText('This waits in the thread. With the mailbox built-in on, Draft the tests gets it with its next prompt, after its current work.')
  await page.getByRole('combobox', { name: 'Type', exact: true }).selectOption('finding')
  await expect(form).toContainText('Tell Draft the tests what you found. It can reply or close the thread.')
  await page.getByLabel('Message', { exact: true }).fill('The parser is ready.')
  const send = page.waitForRequest((request) => request.url().endsWith('/api/mailbox/send'))
  await page.getByRole('button', { name: 'Send to Draft the tests' }).click()
  expect((await send).postDataJSON()).toMatchObject({ to: { tool: 'codex', id: mailboxTarget }, type: 'finding' })
  await expect(page.locator('#toast')).toHaveText('Message waiting in the thread with Draft the tests.')
  await expect(row(page, 'Draft the tests')).not.toHaveClass(/is-mailbox-receivable/)
  await expect(page.locator('.mailbox-tag')).toHaveCount(0)
})

test('the chosen mailbox receiver stays chosen when the list refreshes, even after it leaves the list', async ({ page }) => {
  await openPanel(page, 'mailbox')
  await composeMailbox(page)
  await chooseMailboxTarget(page)
  await page.request.post('/dev/fixture', { data: { name: 'mailbox-handoff' } })
  await expect(mailboxTo(page).locator('option')).toHaveCount(3)
  await expect(mailboxTo(page)).toHaveValue(`codex:${mailboxTarget}`)
  await page.request.post('/dev/fixture', { data: { name: 'mailbox-alone' } })
  await expect(row(page, 'Draft the tests')).toHaveCount(0)
  await expect(mailboxTo(page)).toHaveValue(`codex:${mailboxTarget}`)
  await expect(mailboxTo(page)).toBeEnabled()
  await page.getByLabel('Message', { exact: true }).fill('Still there?')
  const send = page.waitForRequest((request) => request.url().endsWith('/api/mailbox/send'))
  await page.getByRole('button', { name: 'Send to Draft the tests' }).click()
  expect((await send).postDataJSON()).toMatchObject({ to: { tool: 'codex', id: mailboxTarget } })
})

test('groups mailbox receivers, says when an archived one gets the message and checks a typed ID', async ({ page }) => {
  await openPanel(page, 'mailbox-receivers')
  await composeMailbox(page)
  const groups = await mailboxTo(page).locator('optgroup').evaluateAll((boxes) =>
    boxes.map((box) => [(box as HTMLOptGroupElement).label, [...box.querySelectorAll('option')].map((option) => option.textContent)]))
  expect(groups).toEqual([
    ['Working', ['Draft the tests · Codex · agnostic-ai']],
    ['Active', ['Draft the docs · Claude Code · agnostic-ai']],
    ['Archived · gets it when resumed', ['Handle the conflict · Codex · agnostic-ai']],
  ])
  await mailboxTo(page).selectOption({ label: 'Handle the conflict · Codex · agnostic-ai' })
  await expect(page.locator('#mailbox-arrival')).toHaveText('This waits in the thread. With the mailbox built-in on, Handle the conflict gets it when it is resumed.')

  await page.getByRole('button', { name: 'Enter an ID instead' }).click()
  const id = page.getByLabel('Session ID', { exact: true })
  const status = page.locator('#mailbox-target-status')
  await id.fill(mailboxTarget)
  await expect(status).toHaveText('Two sessions use this ID. Choose from the list.')
  await expect(mailboxSend(page)).toBeDisabled()
  await id.fill(mailboxSource)
  await expect(status).toHaveText('That is the sending session.')
  await expect(mailboxSend(page)).toBeDisabled()
  await id.fill(mailboxThird)
  await expect(status).toHaveText('Found: Handle the conflict · Codex.')
  await expect(page.getByRole('button', { name: 'Send to Handle the conflict' })).toBeEnabled()
  await page.getByRole('button', { name: 'Choose from the list' }).click()
  await expect(mailboxTo(page)).toHaveValue(`codex:${mailboxThird}`)
})

test('with nobody to message the receiver list is empty and an ID can still be typed', async ({ page }) => {
  await openPanel(page, 'mailbox-alone')
  await composeMailbox(page)
  await expect(mailboxTo(page)).toBeDisabled()
  await expect(mailboxTo(page)).toContainText('No other local Claude Code or Codex sessions')
  await expect(page.getByRole('form', { name: 'Message another session' })).toContainText('Start another session, or enter an ID.')
  await expect(mailboxSend(page)).toHaveText('Send')
  await expect(mailboxSend(page)).toBeDisabled()
  await page.getByRole('button', { name: 'Enter an ID instead' }).click()
  await page.getByLabel('Session ID', { exact: true }).fill(mailboxTarget)
  await expect(page.getByRole('button', { name: 'Send', exact: true })).toBeEnabled()
})

test('a mailbox reply names the other session and offers no receiver choice', async ({ page }) => {
  await openPanel(page, 'mailbox')
  await askMailbox(page)
  await mailboxSummary(page).click()
  await mailboxThread(page).getByRole('button', { name: 'Reply' }).click()
  const form = page.getByRole('form', { name: 'Reply to Draft the tests' })
  await expect(form).toBeVisible()
  await expect(page.locator('#mailbox-to-name')).toHaveText('Draft the tests · Codex · agnostic-ai')
  await expect(mailboxTo(page)).toBeHidden()
  await expect(page.getByRole('button', { name: 'Enter an ID instead' })).toBeHidden()
  await expect(row(page, 'Draft the tests')).not.toHaveClass(/is-mailbox-receivable/)
  await expect(page.locator('#mailbox-arrival')).toContainText('Draft the tests gets it with its next prompt, after its current work.')
  await expect(page.getByRole('button', { name: 'Send to Draft the tests' })).toBeEnabled()
})

test('pause refuses a mailbox reply in one click and resume allows the draft', async ({ page }) => {
  await openPanel(page, 'mailbox')
  await askMailbox(page)
  await mailboxSummary(page).click()
  await mailboxThread(page).getByRole('button', { name: 'Pause' }).click()
  await expect(mailboxThread(page).getByRole('button', { name: 'Resume' })).toBeVisible()
  await replyInMailbox(page, 'Wait for me.')
  await expect(page.locator('#toast[data-tone=error]')).toContainText('guard: paused')
  await page.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(mailboxThread(page)).toContainText('Refused (guard: paused)')
  await mailboxThread(page).getByRole('button', { name: 'Resume' }).click()
  await expect(mailboxThread(page).getByRole('button', { name: 'Pause' })).toBeVisible()
  await replyInMailbox(page, 'Ready now.')
  await expect(mailboxThread(page)).toContainText('Ready now.')
  await expect(mailboxSummary(page)).toContainText('2 of 8 messages')
})

test('the message after the mailbox cap is refused, and Allow 8 more raises the cap', async ({ page }) => {
  await openPanel(page, 'mailbox-cap')
  await mailboxSummary(page).click()
  await expect(mailboxThread(page)).toContainText('8 of 8 messages')
  await replyInMailbox(page, 'One more?')
  await expect(page.locator('#toast[data-tone=error]')).toContainText('guard: cap')
  await expect(page.locator('#toast')).toContainText('raise the limit')
  await page.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(mailboxThread(page)).toContainText('Refused (guard: cap)')
  await mailboxThread(page).getByRole('button', { name: 'Allow 8 more' }).click()
  await expect(mailboxSummary(page)).toContainText('8 of 16 messages')
  await replyInMailbox(page, 'One more.')
  await expect(mailboxSummary(page)).toContainText('9 of 16 messages')
})

test('closing a capped mailbox accepts one done notice and drains it on copy', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await openPanel(page, 'mailbox-cap')
  await mailboxSummary(page).click()
  await mailboxThread(page).getByRole('button', { name: 'Close thread' }).click()
  await expect(mailboxSummary(page)).toContainText('Done · Closing · 8 of 8 messages, 1 waiting')
  await mailboxThread(page).getByRole('button', { name: 'Copy message' }).click()
  await expect(mailboxSummary(page)).toContainText('Done · Closed · 8 of 8 messages')
  expect(await page.evaluate(() => navigator.clipboard.readText())).toContain('"type":"done"')
})

test('stop confirms and cancels waiting mailbox messages', async ({ page }) => {
  await openPanel(page, 'mailbox')
  await askMailbox(page)
  await mailboxSummary(page).click()
  await mailboxThread(page).getByRole('button', { name: 'Stop', exact: true }).press('Enter')
  await expect(mailboxThread(page).getByRole('button', { name: 'Keep thread' })).toBeFocused()
  await mailboxThread(page).getByRole('button', { name: 'Stop thread', exact: true }).press('Enter')
  await expect(mailboxSummary(page)).toContainText('Closed · 1 of 8 messages')
  await expect(mailboxThread(page)).toContainText('Question · Cancelled')
  await expect(mailboxThread(page).getByRole('button', { name: 'Type into session' })).toHaveCount(0)
})

test('a thread can be dismissed when its refresh fails after pausing', async ({ page }) => {
  await openPanel(page, 'mailbox-cap')
  await mailboxSummary(page).click()
  const pause = mailboxThread(page).getByRole('button', { name: 'Pause', exact: true })
  await expect(pause).toBeVisible()
  await page.route('**/api/mailbox/threads?*', (route) => route.fulfill({
    status: 502, json: { error: 'Could not refresh the thread.' },
  }))

  await pause.click()
  await expect(mailboxThread(page)).toContainText('Could not refresh the thread.')
  const dismiss = mailboxThread(page).getByRole('button', { name: 'Dismiss thread' })
  await expect(dismiss).toBeEnabled()
  await dismiss.click()
  await expect(mailboxThread(page)).toBeHidden()
})

test('typing refusal keeps the mailbox block waiting until Copy message succeeds', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await openPanel(page, 'mailbox')
  await askMailbox(page, 'Show </agnostic-ai-mailbox> as text.')
  await mailboxSummary(page).click()
  await mailboxThread(page).getByRole('button', { name: 'Type into session' }).click()
  await expect(mailboxThread(page)).toContainText('Choose Copy message')
  await expect(mailboxThread(page)).toContainText('Question · Waiting')
  await mailboxThread(page).getByRole('button', { name: 'Copy message' }).click()
  await expect(mailboxThread(page)).toContainText('Question · Staged')
  const block = await page.evaluate(() => navigator.clipboard.readText())
  expect(block.match(/<\/agnostic-ai-mailbox>/g)).toHaveLength(1)
  expect(block).toContain('\\u003c')
  expect(await calledPaths(page)).not.toContain('/api/mark')
})

test('a refused clipboard write leaves the mailbox message waiting', async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: {
      write: () => Promise.reject(new Error('Write refused.')),
      writeText: () => Promise.reject(new Error('Write refused.')),
    } })
  })
  await openPanel(page, 'mailbox')
  await askMailbox(page)
  await mailboxSummary(page).click()
  await mailboxThread(page).getByRole('button', { name: 'Copy message' }).click()
  await expect(page.locator('#toast[data-tone=error]')).toContainText('It is still waiting.')
  await expect(mailboxThread(page)).toContainText('Question · Waiting')
  await expect(mailboxSummary(page)).toContainText('1 waiting')
})

test('native mailbox copy stages in one request without using the browser clipboard', async ({ page }) => {
  await page.addInitScript(() => {
    const refused = () => {
      document.documentElement.dataset.browserClipboard = 'called'
      return Promise.reject(new Error('Browser clipboard unavailable.'))
    }
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: {
      write: refused, writeText: refused,
    } })
  })
  await page.route('**/api/inbox', async (route) => {
    const response = await route.fetch()
    await route.fulfill({ response, json: { ...(await response.json()), mailboxNativeCopy: true } })
  })
  const calls: unknown[] = []
  await page.route('**/api/mailbox/stage', async (route) => {
    const body = route.request().postDataJSON()
    calls.push(body)
    const response = await route.fetch({ postData: { ...body, confirm: true } })
    await route.fulfill({ response })
  })
  await openPanel(page, 'mailbox')
  await askMailbox(page)
  await mailboxSummary(page).click()
  await mailboxThread(page).getByRole('button', { name: 'Copy message' }).click()
  await expect(mailboxThread(page)).toContainText('Question · Staged')
  await expect(page.locator('html')).not.toHaveAttribute('data-browser-clipboard', 'called')
  expect(calls).toEqual([{ thread: expect.any(String), n: 1, how: 'copy' }])
})

test('a mailbox typing receipt failure warns that text was already typed', async ({ page }) => {
  await page.route('**/api/mailbox/stage', (route) => route.fulfill({ json: {
    staged: false, typed: true, message: 'The text was typed. Check that session before trying again.',
  } }))
  await openPanel(page, 'mailbox')
  await askMailbox(page)
  await mailboxSummary(page).click()
  await mailboxThread(page).getByRole('button', { name: 'Type into session' }).click()
  await expect(mailboxThread(page)).toContainText('The text was typed. Check that session before trying again.')
  await expect(mailboxThread(page)).not.toContainText('Choose Copy message')
  await expect(mailboxThread(page)).toContainText('Question · Waiting')
})

test('an unreadable mailbox thread leaves the rest of the list available', async ({ page }) => {
  await openPanel(page, 'mailbox-warning')
  await expect(page.getByText('One mailbox thread could not be read.')).toBeVisible()
  await expect(row(page, 'Review the parser')).toBeVisible()
})

const reviewCommit = '1'.repeat(40)
const reviewBase = '2'.repeat(40)
const reviewWorktree = '/Users/you/Code/agnostic-ai'

async function chooseReviewRequest(page: Page) {
  await page.getByRole('combobox', { name: 'Type', exact: true }).selectOption('review_request')
  await expect(page.getByLabel('Base branch or commit')).toBeVisible()
}

test('a mailbox review request has no message field and carries only the previewed refs', async ({ page }) => {
  await openPanel(page, 'mailbox-review')
  await composeMailbox(page)
  await chooseMailboxTarget(page)
  await page.getByLabel('Message', { exact: true }).fill('My earlier opinion must stay in the draft.')
  await chooseReviewRequest(page)
  await expect(page.getByLabel('Message', { exact: true })).toBeHidden()
  await expect(page.getByLabel('Base branch or commit')).toHaveValue(reviewBase)

  const preview = page.getByRole('group', { name: 'Resolved review' })
  await expect(preview).toContainText(reviewWorktree)
  await expect(preview).toContainText(reviewCommit)
  await expect(preview).toContainText(reviewBase)
  await expect(preview).toContainText('2 changed paths')
  await expect(preview).toContainText('2 uncommitted files are outside this review.')
  const sent = page.waitForRequest((request) => request.url().endsWith('/api/mailbox/send'))
  await mailboxSend(page).press('Enter')
  const request = (await sent).postDataJSON()
  expect(request).not.toHaveProperty('body')
  expect(request.refs).toEqual({ worktree: reviewWorktree, commit: reviewCommit, base: reviewBase })
  await expect(mailboxSummary(page)).toContainText('Review request')
  await mailboxSummary(page).click()
  await expect(mailboxThread(page)).toContainText(reviewCommit)
  await expect(mailboxThread(page)).toContainText('2 changed paths')
})

test('a mailbox review compares models across tools and leaves missing models unknown', async ({ page }) => {
  await openPanel(page, 'mailbox-review')
  await composeMailbox(page)
  await chooseMailboxTarget(page)
  await chooseReviewRequest(page)
  await page.getByLabel('Sending model (optional)').fill('Model A')
  const comparison = page.getByRole('status').filter({ hasText: /Sending: Model A/ })
  await expect(comparison).toContainText('Reviewing: model unknown')
  await expect(comparison).not.toContainText('same model')
  await page.getByLabel('Reviewing model (optional)').fill('model a')
  await expect(comparison).toContainText('Both sessions state the same model.')
  const sent = page.waitForRequest((request) => request.url().endsWith('/api/mailbox/send'))
  await mailboxSend(page).click()
  expect((await sent).postDataJSON().models).toEqual({ from: 'Model A', to: 'model a' })
  await mailboxSummary(page).click()
  await expect(mailboxThread(page)).toContainText('Both sessions state the same model.')
})

test('a mailbox review without a default base asks for an explicit base before sending', async ({ page }) => {
  await openPanel(page, 'mailbox-review-no-base')
  await composeMailbox(page)
  await chooseMailboxTarget(page)
  await chooseReviewRequest(page)
  const base = page.getByLabel('Base branch or commit')
  const send = mailboxSend(page)
  await expect(base).toHaveValue('')
  await expect(page.getByText(/guard: refs.*Name a base/)).toBeVisible()
  await expect(send).toBeDisabled()
  await base.fill('main')
  await expect(page.getByRole('group', { name: 'Resolved review' })).toContainText(reviewBase)
  await expect(send).toBeEnabled()
})

test('a mailbox review keeps its fields stable until a delayed send refusal returns', async ({ page }) => {
  let refuse!: () => void
  const refusal = new Promise<void>((resolve) => { refuse = resolve })
  await page.route('**/api/mailbox/send', async (route) => {
    await refusal
    await route.fulfill({ status: 422, json: { guard: 'refs', error: 'The pinned references are unavailable.' } })
  })
  await openPanel(page, 'mailbox-review')
  await composeMailbox(page)
  await chooseMailboxTarget(page)
  await chooseReviewRequest(page)
  const base = page.getByLabel('Base branch or commit')
  const send = mailboxSend(page)
  await expect(base).toHaveValue(reviewBase)
  const requested = page.waitForRequest((request) => request.url().endsWith('/api/mailbox/send'))
  await send.click()
  await requested
  try {
    await expect(base).toBeDisabled()
    await expect(page.getByRole('combobox', { name: 'Type', exact: true })).toBeDisabled()
    await expect(page.getByLabel('Reviewing model (optional)')).toBeDisabled()
  } finally {
    refuse()
  }
  await expect(page.getByText(/guard: refs.*The pinned references are unavailable/)).toBeVisible()
  await expect(base).toBeEnabled()
  await expect(base).toHaveValue(reviewBase)
  await expect(send).toBeEnabled()
})

test('a previous mailbox send cannot enable a new draft before its refs resolve', async ({ page }) => {
  await openPanel(page, 'mailbox-review')
  await composeMailbox(page)
  await chooseMailboxTarget(page)
  await chooseReviewRequest(page)
  await expect(page.getByLabel('Base branch or commit')).toHaveValue(reviewBase)

  let releaseList!: () => void
  const heldList = new Promise<void>((resolve) => { releaseList = resolve })
  await page.route('**/api/inbox', async (route) => {
    await heldList
    await route.continue()
  })
  await mailboxSend(page).click()
  await expect(page.getByRole('form', { name: 'Message another session' })).toBeHidden()
  await composeMailbox(page)
  await chooseMailboxTarget(page)

  let releaseRefs!: () => void
  const heldRefs = new Promise<void>((resolve) => { releaseRefs = resolve })
  await page.route('**/api/mailbox/refs', async (route) => {
    await heldRefs
    await route.continue()
  })
  try {
    await chooseReviewRequest(page)
    await expect(mailboxSend(page)).toBeDisabled()
    releaseList()
    await expect(mailboxSummary(page)).toBeVisible()
    await expect(mailboxSend(page)).toBeDisabled()
  } finally {
    releaseList()
    releaseRefs()
  }
  await expect(mailboxSend(page)).toBeEnabled()
})

test('a late mailbox refs response cannot replace a newer preview', async ({ page }) => {
  let requestArrived!: () => void
  const slowRequest = new Promise<void>((resolve) => { requestArrived = resolve })
  let release!: () => void
  const slowResponse = new Promise<void>((resolve) => { release = resolve })
  await page.route('**/api/mailbox/refs', async (route) => {
    const { base } = route.request().postDataJSON()
    if (base === 'slow') {
      requestArrived()
      await slowResponse
      await route.fulfill({ json: { worktree: '/old-worktree', commit: '4'.repeat(40), base: '5'.repeat(40), paths: [], uncommitted: 0 } })
      return
    }
    if (base === 'fresh') {
      await route.fulfill({ json: { worktree: '/new-worktree', commit: '6'.repeat(40), base: '7'.repeat(40), paths: [], uncommitted: 0 } })
      return
    }
    await route.continue()
  })
  await openPanel(page, 'mailbox-review')
  await composeMailbox(page)
  await chooseReviewRequest(page)
  const base = page.getByLabel('Base branch or commit')
  await expect(base).toHaveValue(reviewBase)
  const canceled = page.waitForEvent('requestfailed', (request) =>
    request.url().endsWith('/api/mailbox/refs') && request.postDataJSON().base === 'slow',
  )
  await base.fill('slow')
  await slowRequest
  await expect(mailboxSend(page)).toBeDisabled()
  await base.fill('fresh')
  await canceled
  const preview = page.getByRole('group', { name: 'Resolved review' })
  await expect(preview).toContainText('/new-worktree')
  release()
  await expect(preview).not.toContainText('/old-worktree')
})

const handoffWorktree = '/Users/you/Code/agnostic-ai'
const handoffPath = 'internal/mailbox/handoff.go'
const mailboxSource = '84fc6d21-820c-4c88-9729-ff8ee40de1f3'
const mailboxThird = 'e50eac57-0dcd-4089-bd24-6c95d4171790'

const fileClaims = (page: Page) => page.getByRole('region', { name: /^Files? claim/ })
const claimsButton = (page: Page) => page.getByRole('button', { name: /^File claims/ })
const CLAIM_HELP = 'A file claim notes which session works on which paths. It does not block edits.'
const receiverThread = (page: Page) => page.getByRole('region', { name: 'Thread with Review the parser' })

async function offerHandoff(page: Page) {
  await composeMailbox(page)
  await chooseMailboxTarget(page)
  await page.getByRole('combobox', { name: 'Type', exact: true }).selectOption('handoff')
  const composer = page.getByRole('form', { name: 'Message another session' })
  await composer.getByLabel('Worktree', { exact: true }).fill(handoffWorktree)
  await composer.getByLabel('Paths, one per line').fill(handoffPath)
  await page.getByLabel('Message', { exact: true }).fill('Please finish the handoff implementation.')
  await page.getByRole('button', { name: 'Claim these paths' }).click()
  await expect(page.locator('#toast')).toContainText('Paths claimed.')
  await mailboxSend(page).click()
  await expect(mailboxSummary(page)).toContainText('Handoff')
}

test('a mailbox handoff uses the Git root when the sending session is in a subdirectory', async ({ page }) => {
  await page.route('**/api/inbox', async (route) => {
    const response = await route.fetch()
    const envelope = await response.json()
    const source = envelope.view.conversations.find(({ id }: { id: string }) => id === mailboxSource)
    source.folder = `${handoffWorktree}/internal/mailbox`
    await route.fulfill({ response, json: envelope })
  })
  await page.route(`**/api/mailbox/claims?tool=claude&id=${mailboxSource}`, (route) =>
    route.fulfill({ json: { worktree: handoffWorktree, claims: [], conflicts: [] } }))
  await openPanel(page, 'mailbox-handoff')
  await composeMailbox(page)
  await page.getByRole('combobox', { name: 'Type', exact: true }).selectOption('handoff')
  const composer = page.getByRole('form', { name: 'Message another session' })
  await expect(composer.getByLabel('Worktree', { exact: true })).toHaveValue(handoffWorktree)
  await composer.getByLabel('Paths, one per line').fill(handoffPath)
  const sent = page.waitForRequest((request) => request.url().endsWith('/api/mailbox/claim'))
  await composer.getByRole('button', { name: 'Claim these paths' }).click()
  expect((await sent).postDataJSON()).toMatchObject({ worktree: handoffWorktree, paths: [handoffPath] })
})

test('an explicit mailbox handoff worktree wins over a pending lookup', async ({ page }) => {
  let release!: () => void
  const held = new Promise<void>((resolve) => { release = resolve })
  const endpoint = `**/api/mailbox/claims?tool=claude&id=${mailboxSource}`
  await page.route(endpoint, async (route) => {
    await held
    await route.fulfill({ json: { worktree: '/late-worktree', claims: [], conflicts: [] } })
  })
  await openPanel(page, 'mailbox-handoff')
  await composeMailbox(page)
  const started = page.waitForRequest((request) => request.url().includes('/api/mailbox/claims?'))
  const type = page.getByRole('combobox', { name: 'Type', exact: true })
  await type.selectOption('handoff')
  await started
  const composer = page.getByRole('form', { name: 'Message another session' })
  await expect(composer.getByRole('button', { name: 'Claim these paths' })).toBeDisabled()
  await expect(mailboxSend(page)).toBeDisabled()
  const worktree = composer.getByLabel('Worktree', { exact: true })
  const canceled = page.waitForEvent('requestfailed', (request) => request.url().includes('/api/mailbox/claims?'))
  await worktree.fill('/chosen-worktree')
  await canceled
  release()
  await type.selectOption('question')
  await type.selectOption('handoff')
  await expect(worktree).toHaveValue('/chosen-worktree')
  await expect(composer.getByRole('button', { name: 'Claim these paths' })).toBeEnabled()
})

test('a mailbox handoff moves no claim until the receiver accepts', async ({ page }) => {
  await openPanel(page, 'mailbox-handoff')
  await offerHandoff(page)
  const claims = await page.request.get('/api/mailbox/claims')
  expect((await claims.json()).claims).toMatchObject([{ owner: { id: mailboxSource }, paths: [handoffPath] }])
  await mailboxSummary(page).click()
  await expect(mailboxThread(page)).toContainText('Handoff · Waiting · Offered')
  await expect(mailboxThread(page).getByRole('button', { name: 'Accept', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: /^With Review the parser/ }).click()
  await receiverThread(page).getByRole('button', { name: 'Accept', exact: true }).click()
  await expect(receiverThread(page)).toContainText('Accepted')
  const moved = await page.request.get('/api/mailbox/claims')
  expect((await moved.json()).claims).toMatchObject([{ owner: { id: mailboxTarget }, paths: [handoffPath] }])
})

test('declining a mailbox handoff leaves ownership with the sender', async ({ page }) => {
  await openPanel(page, 'mailbox-handoff')
  await offerHandoff(page)
  await page.getByRole('button', { name: /^With Review the parser/ }).click()
  await receiverThread(page).getByRole('button', { name: 'Decline', exact: true }).click()
  await expect(receiverThread(page)).toContainText('Declined')
  const retained = await page.request.get('/api/mailbox/claims')
  expect((await retained.json()).claims).toMatchObject([{ owner: { id: mailboxSource }, paths: [handoffPath] }])
})

test('a mailbox handoff conflict names the holder on both sessions', async ({ page }) => {
  await openPanel(page, 'mailbox-handoff')
  await offerHandoff(page)
  expect((await page.request.post('/api/mailbox/release', { data: {
    tool: 'claude', id: mailboxSource, worktree: handoffWorktree, paths: [handoffPath],
  } })).ok()).toBe(true)
  expect((await page.request.post('/api/mailbox/claim', { data: {
    tool: 'codex', id: mailboxThird, worktree: handoffWorktree, paths: [handoffPath],
  } })).ok()).toBe(true)
  const recipient = page.getByRole('button', { name: /^With Review the parser/ })
  await recipient.click()
  await receiverThread(page).getByRole('button', { name: 'Accept', exact: true }).click()
  await expect(page.locator('#toast[data-tone=error]')).toContainText('guard: ownership')
  for (const summary of [mailboxSummary(page), recipient]) {
    await expect(summary).toContainText('Path conflict')
    await expect(summary).toContainText(handoffPath)
    await expect(summary).toContainText('Handle the conflict')
  }
  const retained = await page.request.get('/api/mailbox/claims')
  expect((await retained.json()).claims).toMatchObject([{ owner: { id: mailboxThird }, paths: [handoffPath] }])
})

test('closing a mailbox thread keeps its file claim until Release', async ({ page }) => {
  await openPanel(page, 'mailbox-handoff')
  await offerHandoff(page)
  await mailboxSummary(page).click()
  await mailboxThread(page).getByRole('button', { name: 'Close thread' }).click()
  await expect(mailboxSummary(page)).toContainText('Closing')
  await claimsButton(page).click()
  await expect(fileClaims(page)).toContainText(handoffPath)
  await fileClaims(page).getByRole('button', { name: 'Release', exact: true }).click()
  await expect(fileClaims(page)).not.toContainText(handoffPath)
  await expect(fileClaims(page)).toContainText('No claims left.')
  await expect(claimsButton(page)).toBeHidden()
  const released = await page.request.get('/api/mailbox/claims')
  expect((await released.json()).claims).toEqual([])
})

test('the global mailbox claims card releases a stale owner with no conversation row', async ({ page }) => {
  await openPanel(page, 'mailbox-stale-claim')
  await expect(row(page, 'Handle the conflict')).toHaveCount(0)
  await expect(claimsButton(page)).toHaveAccessibleName('File claims: 1 path, 1 stale')
  await expect(claimsButton(page)).toHaveClass(/is-warning/)
  await claimsButton(page).click()
  const gone = fileClaims(page).getByRole('region', { name: 'Session gone' })
  await expect(gone).toContainText(handoffPath)
  await expect(gone.getByRole('button', { name: 'Release all' })).toHaveCount(0)
  await gone.getByRole('button', { name: 'Release', exact: true }).press('Enter')
  await expect(fileClaims(page)).toContainText('No claims left.')
  await expect(fileClaims(page).getByRole('button', { name: 'Dismiss file claims' })).toBeFocused()
  await expect(claimsButton(page)).toBeHidden()
})

test('Release all frees every claim of a gone session in one click', async ({ page }) => {
  await openPanel(page, 'mailbox-stale-claims')
  await expect(claimsButton(page)).toHaveAccessibleName('File claims: 2 paths, 2 stale')
  await claimsButton(page).click()
  const gone = fileClaims(page).getByRole('region', { name: 'Session gone' })
  await expect(gone.getByRole('article')).toHaveCount(2)
  await gone.getByRole('button', { name: 'Release all' }).click()
  await expect(fileClaims(page)).toContainText('No claims left.')
  expect((await (await page.request.get('/api/mailbox/claims')).json()).claims).toEqual([])
})

test('Release all frees more claims than one request takes, in batches', async ({ page }) => {
  await openPanel(page, 'mailbox-many-stale-claims')
  await expect(claimsButton(page)).toHaveAccessibleName('File claims: 250 paths, 250 stale')
  await claimsButton(page).click()
  const releases: Array<{ paths: string[], gone: boolean }> = []
  page.on('request', (request) => { if (request.url().endsWith('/api/mailbox/release')) releases.push(request.postDataJSON()) })
  await fileClaims(page).getByRole('button', { name: 'Release all' }).click()
  await expect(fileClaims(page)).toContainText('No claims left.')
  expect(releases.map((ask) => [ask.paths.length, ask.gone])).toEqual([[200, true], [50, true]])
  await expect(claimsButton(page)).toBeHidden()
})

test('a Release all that fails after an earlier batch says how far it got', async ({ page }) => {
  await openPanel(page, 'mailbox-many-stale-claims')
  let sent = 0
  await page.route('**/api/mailbox/release', async (route) => {
    sent++
    if (sent === 2) {
      await route.fulfill({ status: 422, json: { error: 'Not sent (guard: ownership). Those claims changed since the list was read.\nReload the file claims and try again.', guard: 'ownership' } })
      return
    }
    await route.continue()
  })
  await claimsButton(page).click()
  await fileClaims(page).getByRole('button', { name: 'Release all' }).click()
  await expect(page.locator('#toast')).toContainText('Released 200 of 250 paths.')
  await expect(page.locator('#toast')).toContainText('Those claims changed since the list was read.')
  await expect(claimsButton(page)).toHaveAccessibleName('File claims: 50 paths, 50 stale')
})

test('Dismiss clears a refused claim from the footer and keeps the holder\'s claim', async ({ page }) => {
  await openPanel(page, 'mailbox-claim-conflict')
  await expect(claimsButton(page)).toHaveAccessibleName('File claims: 1 path, 1 conflict')
  await expect(claimsButton(page)).toHaveClass(/is-warning/)
  await claimsButton(page).click()
  await fileClaims(page).getByRole('button', { name: 'Dismiss', exact: true }).click()
  await expect(page.locator('#toast')).toContainText('Conflict dismissed.')
  await expect(claimsButton(page)).toHaveAccessibleName('File claims: 1 path')
  await expect(claimsButton(page)).not.toHaveClass(/is-warning/)
  await page.reload()
  await expect(claimsButton(page)).toHaveAccessibleName('File claims: 1 path')
  await expect(claimsButton(page)).not.toHaveClass(/is-warning/)
  const left = await (await page.request.get('/api/mailbox/claims')).json()
  expect(left.conflicts).toEqual([])
  expect(left.claims).toMatchObject([{ owner: { id: '0199a2b4-5c1e-7d30-9f2a-3b6c8d1e4f50' }, paths: ['internal/mailbox/handoff.go'] }])
})

test('with no claims the footer shows no File claims button', async ({ page }) => {
  await openPanel(page, 'mailbox-handoff')
  await expect(page.locator('#mailbox-open-claims')).toBeHidden()
  await expect(page.locator('#mailbox-open-claims')).toHaveCSS('display', 'none')
})

test('Claim files explains claims, links the docs, and claims paths from a row', async ({ page }) => {
  await openPanel(page, 'mailbox-handoff')
  await row(page, 'Review the parser').click({ button: 'right' })
  await page.getByRole('menuitem', { name: 'Claim files…' }).click()
  const card = page.getByRole('region', { name: 'Files claimed by Review the parser' })
  await expect(card).toContainText(CLAIM_HELP)
  await expect(card).toContainText('This session has no claims yet.')
  await expect(page.getByText('No file claims.')).toHaveCount(0)
  const help = card.getByRole('button', { name: 'Open the docs: Mailbox: Hand off files' })
  await expect(help).toHaveAttribute('title', 'Open the docs: Mailbox: Hand off files')
  const opened = page.waitForRequest((request) => request.url().endsWith('/api/docs'))
  await help.click()
  expect((await opened).postDataJSON()).toEqual({ path: '/docs/mailbox/#hand-off-files' })

  await card.getByLabel('Worktree', { exact: true }).fill(handoffWorktree)
  await card.getByLabel('Paths, one per line').fill(`${handoffPath}\ninternal/mailbox/claims.go`)
  await card.getByRole('button', { name: 'Claim paths', exact: true }).click()
  await expect(card).toContainText(handoffPath)
  await expect(claimsButton(page)).toHaveText('File claims 2')
  await expect(claimsButton(page)).toHaveAccessibleName('File claims: 2 paths')
  const chip = row(page, 'Review the parser').locator('.meta .claim-chip')
  await expect(chip).toHaveAccessibleName('2 paths claimed by Review the parser')
  await expect(page.locator('.mailbox-threads').getByRole('button', { name: /claim/ })).toHaveCount(0)

  await card.getByRole('button', { name: 'Dismiss file claims' }).click()
  await chip.click()
  await expect(card).toBeVisible()
  await expect(card.getByRole('article')).toHaveCount(2)
  await expect(card).toContainText(handoffPath)
  await expect(card).toContainText('internal/mailbox/claims.go')
  await expect(card.getByRole('article').first()).toContainText(/Claimed by you, \d\d:\d\d/)
  await card.getByRole('button', { name: 'Release', exact: true }).first().click()
  await expect(card.getByRole('article')).toHaveCount(1)
  await expect(chip).toHaveText('1 path claimed')
  await expect(claimsButton(page)).toHaveText('File claims 1')
  await card.getByRole('button', { name: 'Release', exact: true }).click()
  await expect(card).toContainText('This session has no claims yet.')
  await expect(chip).toHaveCount(0)
  await expect(claimsButton(page)).toBeHidden()
})

test('a claim an agent made says so', async ({ page }) => {
  await openPanel(page, 'mailbox-claims')
  await expect(claimsButton(page)).toHaveText('File claims 3')
  await row(page, 'Draft the tests').getByRole('button', { name: '1 path claimed by Draft the tests' }).click()
  const card = page.getByRole('region', { name: 'Files claimed by Draft the tests' })
  await expect(card.getByRole('article')).toContainText('Claimed by the agent')
  await expect(card.getByRole('article')).toContainText('companion/frontend/panel.js')
})

const conflictPath = 'internal/mailbox/handoff.go'
const titled = (page: Page, title: string) => page.locator('#list .row').filter({ has: page.locator('.title').getByText(title, { exact: true }) })

test('a path conflict shows on both rows, under Needs you, until the requester dismisses it', async ({ page }) => {
  await openPanel(page, 'mailbox-claim-conflict')
  const holder = titled(page, 'Draft the tests')
  const requester = titled(page, 'Review the parser')
  await expect(holder.getByRole('button', { name: `Review the parser wants ${conflictPath}, which this session claims` })).toBeVisible()
  await expect(requester.getByRole('button', { name: `${conflictPath} is claimed by Draft the tests` })).toBeVisible()
  await expect(page.getByRole('button', { name: /^Needs you/ })).toContainText('2')
  await expect(claimsButton(page)).toHaveAccessibleName('File claims: 1 path, 1 conflict')
  await expect(claimsButton(page)).toHaveClass(/is-warning/)

  await requester.getByRole('button', { name: `${conflictPath} is claimed by Draft the tests` }).click()
  const card = page.getByRole('region', { name: 'Files claimed by Review the parser' })
  await expect(card).toContainText(conflictPath)
  await card.getByRole('button', { name: 'Dismiss', exact: true }).click()
  await expect(page.locator('#toast')).toContainText('Conflict dismissed.')
  await expect(holder.locator('.needs-reason.is-conflict')).toHaveCount(0)
  await expect(requester.locator('.needs-reason.is-conflict')).toHaveCount(0)
  await expect(page.getByRole('button', { name: /^Needs you/ })).toContainText('1')
  await expect(claimsButton(page)).not.toHaveClass(/is-warning/)
})

test('the holder releasing the path clears the conflict on both rows', async ({ page }) => {
  await openPanel(page, 'mailbox-claim-conflict')
  const holder = titled(page, 'Draft the tests')
  await holder.locator('.needs-reason.is-conflict').click()
  const card = page.getByRole('region', { name: 'Files claimed by Draft the tests' })
  await expect(card.getByRole('button', { name: 'Dismiss', exact: true })).toHaveCount(0)
  await card.getByRole('button', { name: 'Release', exact: true }).click()
  await expect(page.locator('.needs-reason.is-conflict')).toHaveCount(0)
  await expect(claimsButton(page)).toBeHidden()
  await expect(page.getByRole('button', { name: /^Working/ })).toContainText('1')
})

test('the handoff form explains file claims under its paths', async ({ page }) => {
  await openPanel(page, 'mailbox-handoff')
  await composeMailbox(page)
  await chooseMailboxTarget(page)
  await page.getByRole('combobox', { name: 'Type', exact: true }).selectOption('handoff')
  const composer = page.getByRole('form', { name: 'Message another session' })
  await expect(composer.getByLabel('Paths, one per line')).toHaveAccessibleDescription(new RegExp(CLAIM_HELP.replace(/\./g, '\\.')))
  await expect(composer.getByRole('button', { name: 'Open the docs: Mailbox: Hand off files' })).toBeVisible()
})

test('copy resume command writes the exact command and says so', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await openFailing(page, 'resume-failure')
  await expect(notice(page)).toContainText('macOS blocked opening Terminal.')
  await expect(notice(page)).toContainText(RESUME)

  await notice(page).getByRole('button', { name: 'Copy resume command' }).press('Enter')
  await expect(notice(page)).toContainText('Copied. Paste it in a terminal.')
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(RESUME)
})

test('a refused clipboard says so and keeps the notice', async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(navigator, 'clipboard', { value: { writeText: () => Promise.reject(new Error('Write permission denied.')) } })
  })
  await openFailing(page, 'resume-failure')

  await notice(page).getByRole('button', { name: 'Copy resume command' }).click()
  await expect(notice(page)).toContainText('Could not copy it. Select the command above and copy it by hand.')
  await expect(notice(page).getByRole('button', { name: 'Copy resume command' })).toBeVisible()
})

for (const settle of ['resolve', 'reject'] as const) {
  test(`a copy that settles (${settle}) after the next open leaves the new notice alone`, async ({ page }) => {
    await page.addInitScript(() => {
      const pending: { resolve: () => void; reject: (error: Error) => void }[] = []
      Object.assign(window, { pendingCopies: pending })
      Object.defineProperty(navigator, 'clipboard', {
        value: { writeText: () => new Promise<void>((resolve, reject) => pending.push({ resolve, reject })) },
      })
    })
    await openFailing(page, 'resume-failure')
    await notice(page).getByRole('button', { name: 'Copy resume command' }).click()
    await expect.poll(() => page.evaluate(() => (window as unknown as { pendingCopies: unknown[] }).pendingCopies.length)).toBe(1)

    expect((await page.request.post('/dev/fixture', { data: { name: 'open-failure' } })).ok()).toBe(true)
    await row(page, 'Fix the login redirect').click()
    await expect(notice(page)).toContainText('Codex runs in an app that cannot be brought forward.')

    await page.evaluate((how) => {
      const copy = (window as unknown as { pendingCopies: { resolve: () => void; reject: (error: Error) => void }[] }).pendingCopies[0]
      if (how === 'resolve') copy.resolve()
      else copy.reject(new Error('Write permission denied.'))
    }, settle)
    await page.waitForTimeout(200)
    await expect(notice(page)).not.toContainText('Copied')
    await expect(notice(page)).not.toContainText('Could not copy it')
  })
}

const needsYou = (page: Page) => page.locator('.row.needs-you')

for (const [kind, title, id, tool] of [
  ['a wait', 'Approve the database migration', 'waits-1', 'claude'],
  ['an unread reply', 'Draft the release notes', 'reply-1', 'codex'],
] as const) {
  test(`mark as read from the row menu takes ${kind} out of Needs you without opening it`, async ({ page }) => {
    await openPanel(page)
    const opens: string[] = []
    page.on('request', (request) => {
      if (request.url().endsWith('/api/open')) opens.push(request.url())
    })
    await expect(needsYou(page)).toHaveCount(2)
    const marked = page.waitForRequest((request) => request.url().endsWith('/api/mark'))
    await row(page, title).click({ button: 'right' })
    await page.getByRole('menuitem', { name: 'Mark as read' }).click()
    expect((await marked).postDataJSON()).toEqual({ tool, id, as: 'seen' })

    await expect(needsYou(page)).toHaveCount(1)
    await expect(needsYou(page).filter({ hasText: title })).toHaveCount(0)
    await expect(row(page, title)).toBeVisible()
    await expect(page.getByRole('menu', { name: 'Conversation' })).toBeHidden()
    expect(opens).toEqual([])
  })
}

test('the row menu offers Mark as read only on a row that needs you', async ({ page }) => {
  await openPanel(page)
  await row(page, 'Plan the sync dry run').click({ button: 'right' })
  await expect(page.getByRole('menuitem', { name: 'Rename…' })).toBeFocused()
  await expect(page.getByRole('menuitem', { name: 'Mark as read' })).toBeHidden()
})

for (const [change, as] of [
  ['leaves Needs you', 'seen'],
  ['leaves the list', 'hidden'],
] as const) {
  test(`the row menu closes when its row ${change} while it is open`, async ({ page }) => {
    await openPanel(page)
    const marks: unknown[] = []
    page.on('request', (request) => {
      if (new URL(request.url()).pathname === '/api/mark') marks.push(request.postDataJSON())
    })
    await row(page, 'Approve the database migration').click({ button: 'right' })
    await expect(page.getByRole('menuitem', { name: 'Mark as read' })).toBeFocused()

    const changed = await page.request.post('/api/mark', { data: { tool: 'claude', id: 'waits-1', as } })
    expect(changed.ok()).toBe(true)

    await expect(page.getByRole('menu', { name: 'Conversation' })).toBeHidden()
    expect(marks).toEqual([])
  })
}

test('a failed mark as read keeps the row in Needs you and says why', async ({ page }) => {
  await openPanel(page, 'mark-failure')
  await row(page, 'Draft the release notes').click({ button: 'right' })
  await page.getByRole('menuitem', { name: 'Mark as read' }).click()

  await expect(page.getByRole('alert').filter({ hasText: 'agnostic-ai could not save the mark' })).toBeVisible()
  await expect(needsYou(page).filter({ hasText: 'Draft the release notes' })).toBeVisible()
})

const markAllRead = (page: Page) => page.getByRole('button', { name: 'Mark all as read' })

/** The bodies of every /api/mark the page sends, in order. */
function markBodies(page: Page) {
  const marks: unknown[] = []
  page.on('request', (request) => {
    if (new URL(request.url()).pathname === '/api/mark') marks.push(request.postDataJSON())
  })
  return marks
}

const seen = (tool: string, id: string) => ({ tool, id, as: 'seen' })

test('mark all as read marks every Needs you row from the keyboard, and the circle follows', async ({ page, context }) => {
  await openPanel(page, 'needs-you')
  const bubble = await context.newPage()
  await bubble.goto('/index.html')
  await expect(bubble.locator('#bubble')).toHaveAttribute('aria-label', 'Conversations: 1 waiting for you, 3 unread')
  const marks = markBodies(page)
  const opens: string[] = []
  page.on('request', (request) => {
    if (request.url().endsWith('/api/open')) opens.push(request.url())
  })

  // A highlighted row must not open when Enter presses the header button.
  await page.keyboard.press('ArrowDown')
  await expect(selected(page)).toHaveCount(1)
  await expect(markAllRead(page)).toHaveCount(1)
  await markAllRead(page).focus()
  await page.keyboard.press('Enter')

  await expect(groupTitle(page, 'Needs you')).toHaveCount(0)
  expect(opens).toEqual([])
  expect(marks).toEqual([seen('claude', 'wait-2'), seen('codex', 'reply-2'), seen('cursor', 'reply-3'), seen('claude', 'cloud-2')])
  await expect(bubble.locator('#bubble')).toHaveAttribute('aria-label', 'Conversations: none needs you')

  // A new message brings a chat back.
  expect((await page.request.post('/dev/write', { data: { tool: 'codex', id: 'reply-2' } })).ok()).toBe(true)
  await expect(needsYou(page)).toHaveCount(1)
  await expect(needsYou(page)).toContainText('Split the parser module')
  await expect(bubble.locator('#bubble')).toHaveAttribute('aria-label', 'Conversations: 1 unread')
})

test('mark all as read on Local leaves the cloud chat in Needs you', async ({ page }) => {
  await openPanel(page, 'needs-you')
  const marks = markBodies(page)
  await page.getByRole('button', { name: 'Local', exact: true }).click()
  await expect(needsYou(page)).toHaveCount(3)
  await markAllRead(page).click()

  await expect(groupTitle(page, 'Needs you')).toHaveCount(0)
  expect(marks).toEqual([seen('claude', 'wait-2'), seen('codex', 'reply-2'), seen('cursor', 'reply-3')])
  await page.getByRole('button', { name: 'All', exact: true }).click()
  await expect(needsYou(page)).toHaveCount(1)
  await expect(needsYou(page)).toContainText('Write the support guide')
})

test('mark all as read during a search marks only the matching rows', async ({ page }) => {
  await openPanel(page, 'needs-you')
  const marks = markBodies(page)
  await page.getByLabel('Search every conversation').fill('parser')
  await expect(page.getByRole('option')).toHaveCount(2)
  await markAllRead(page).click()

  await expect(groupTitle(page, 'Needs you')).toHaveCount(0)
  expect(marks).toEqual([seen('codex', 'reply-2'), seen('cursor', 'reply-3')])
  await page.getByLabel('Search every conversation').fill('')
  await expect(needsYou(page)).toHaveCount(2)
})

test('mark all as read skips a row that got a new message after it was drawn', async ({ page }) => {
  await openPanel(page, 'needs-you')
  const marks = markBodies(page)
  let first = true
  await page.route('**/api/mark*', async (route) => {
    if (first) {
      first = false
      // Split the parser module gets a new message while the first mark is on its way.
      expect((await page.request.post('/dev/write', { data: { tool: 'codex', id: 'reply-2' } })).ok()).toBe(true)
      await expect(row(page, 'Split the parser module')).toContainText('now')
    }
    await route.continue()
  })
  await markAllRead(page).click()

  await expect(needsYou(page)).toHaveCount(1)
  await expect(needsYou(page)).toContainText('Split the parser module')
  expect(marks).toEqual([seen('claude', 'wait-2'), seen('cursor', 'reply-3'), seen('claude', 'cloud-2')])
})

test('mark all as read leaves a row that a new search took out of the list', async ({ page }) => {
  await openPanel(page, 'needs-you')
  const marks = markBodies(page)
  let release: () => void = () => {}
  const held = new Promise<void>((done) => { release = done })
  let first = true
  await page.route('**/api/mark*', async (route) => {
    if (first) {
      first = false
      await held
    }
    await route.continue()
  })
  const search = page.getByLabel('Search every conversation')
  await search.fill('parser')
  await expect(page.getByRole('option')).toHaveCount(2)
  await markAllRead(page).click()
  await expect.poll(() => marks.length).toBe(1)

  await search.fill('schema')
  await expect(page.getByRole('option')).toHaveCount(1)
  release()

  await search.fill('')
  await expect(needsYou(page)).toHaveCount(3)
  expect(marks).toEqual([seen('codex', 'reply-2')])
  await expect(needsYou(page).filter({ hasText: 'Review the parser tests' })).toHaveCount(1)
})

test('a second press while mark all as read runs starts no second run', async ({ page }) => {
  await openPanel(page, 'needs-you')
  const marks = markBodies(page)
  let release: () => void = () => {}
  const held = new Promise<void>((done) => { release = done })
  let first = true
  await page.route('**/api/mark*', async (route) => {
    if (first) {
      first = false
      await held
    }
    await route.continue()
  })
  await markAllRead(page).click()
  await expect.poll(() => marks.length).toBe(1)
  await markAllRead(page).click()
  release()

  await expect(groupTitle(page, 'Needs you')).toHaveCount(0)
  expect(marks).toEqual([seen('claude', 'wait-2'), seen('codex', 'reply-2'), seen('cursor', 'reply-3'), seen('claude', 'cloud-2')])
})

test('a failed mark all as read stops at the first refused mark, keeps the rows and says why', async ({ page }) => {
  await openPanel(page, 'needs-you-mark-failure')
  const marks = markBodies(page)
  await markAllRead(page).click()

  await expect(page.getByRole('alert').filter({ hasText: 'agnostic-ai could not save the mark' })).toBeVisible()
  await expect(needsYou(page)).toHaveCount(4)
  expect(marks).toEqual([seen('claude', 'wait-2')])
})

test('only Needs you offers mark all as read, and its rows do not repeat the header', async ({ page }) => {
  await openPanel(page)
  await expect(markAllRead(page)).toHaveCount(1)
  await expect(groupTitle(page, 'Needs you').locator('..')).toContainText('Mark all as read')
  await expect(groupTitle(page, 'Active').locator('..')).toContainText('Mark all complete')
  await expect(groupTitle(page, 'Active').locator('..')).not.toContainText('Mark all as read')

  await expect(row(page, 'Draft the release notes').locator('.needs-reason')).toHaveText('New reply')
  await expect(row(page, 'Approve the database migration').locator('.meta')).toHaveText('Claude Code · shop · 2m')
  await expect(row(page, 'Draft the release notes').locator('.meta')).toHaveText('Codex · agnostic-ai · 12m')
  await expect(row(page, 'Refactor the search index').locator('.meta')).toHaveText('Claude Code · agnostic-ai · now')
})

test('a new list while navigating keeps the highlighted row', async ({ page }) => {
  await openPanel(page)
  for (let step = 0; step < 3; step += 1) await page.keyboard.press('ArrowDown')
  await expect(selected(page)).toContainText('Refactor the search index')

  const arrived = await page.request.post('/dev/arrive')
  expect(arrived.ok()).toBe(true)
  await expect(row(page, 'Answer the review comments')).toBeVisible()
  await expect(selected(page)).toContainText('Refactor the search index')

  const opened = page.waitForRequest((request) => request.url().endsWith('/api/open'))
  await page.keyboard.press('Enter')
  expect((await opened).postDataJSON()).toEqual({ tool: 'claude', id: 'work-1', title: 'Refactor the search index' })
})

test('a slower older search never replaces the newer one', async ({ page }) => {
  await openPanel(page, 'slow-search')
  const search = page.getByLabel('Search every conversation')
  const slowAsked = page.waitForRequest((request) => request.url().includes('q=release'))
  await search.fill('release')
  await slowAsked
  await search.fill('login')

  await expect(row(page, 'Fix the login redirect')).toBeVisible()
  // The slow fixture answers "release" 1.5 seconds after it was asked.
  await page.waitForTimeout(2_000)
  await expect(page.getByRole('option')).toHaveCount(1)
  await expect(row(page, 'Draft the release notes')).toHaveCount(0)
})

test('a search shows the newest matches while older weeks are still read', async ({ page }) => {
  await openPanel(page, 'paged-search')
  await page.getByLabel('Search every conversation').fill('the')

  // Eight matches come two per page, each page 0.6 seconds after the one before.
  await expect(row(page, 'Approve the database migration')).toBeVisible()
  await expect(row(page, 'Write the onboarding guide')).toHaveCount(0)
  await expect(page.locator('#summary')).toHaveText('Searching…')

  await expect(row(page, 'Write the onboarding guide')).toBeVisible()
  await expect(page.locator('#summary')).toHaveText('8 conversations match')
})

test('a search read again after a new list keeps its whole answer on screen', async ({ page }) => {
  await openPanel(page, 'paged-search')
  await page.getByLabel('Search every conversation').fill('the')
  await expect(page.locator('#summary')).toHaveText('8 conversations match')

  const readAgain = page.waitForResponse((response) => response.url().includes('q=the') && !response.url().includes('before='))
  await page.request.post('/dev/arrive')
  await readAgain
  // The first page is in; a partial draw would have dropped the older weeks by now.
  await page.waitForTimeout(300)
  expect(await row(page, 'Write the onboarding guide').count()).toBe(1)
  expect(await page.locator('#summary').textContent()).toBe('8 conversations match')
})

test('a search started again while older weeks load keeps the rows already shown', async ({ page }) => {
  await openPanel(page, 'paged-search')
  // A mark that names no list version reads the list again from the first page.
  await page.route('**/api/mark', (route) => route.fulfill({ status: 204 }))
  await page.getByLabel('Search every conversation').fill('the')
  await expect(row(page, 'Refactor the search index')).toBeVisible()
  await expect(page.locator('#summary')).toHaveText('Searching…')

  await row(page, 'Draft the release notes').click({ button: 'right' })
  const startedAgain = page.waitForResponse((response) => response.url().includes('q=the') && !response.url().includes('before='))
  await page.getByRole('menuitem', { name: 'Mark as read' }).click()
  await startedAgain
  // Its first page is in; a draw of that page alone would have dropped the second week, which holds this row, by now.
  await page.waitForTimeout(300)
  expect(await row(page, 'Draft the release notes').count()).toBe(1)

  await expect(row(page, 'Write the onboarding guide')).toBeVisible()
  await expect(page.locator('#summary')).toHaveText('8 conversations match')
})

test('a page of an older search never draws over a newer one', async ({ page }) => {
  await openPanel(page, 'paged-search')
  const search = page.getByLabel('Search every conversation')
  const olderPage = page.waitForRequest((request) => request.url().includes('q=the') && request.url().includes('before='))
  await search.fill('the')
  await olderPage
  await search.fill('login')

  await expect(row(page, 'Fix the login redirect')).toBeVisible()
  await expect(page.locator('#summary')).toHaveText('1 conversation matches')
  await page.waitForTimeout(1_500)
  await expect(page.getByRole('option')).toHaveCount(1)
})

test('an empty inbox says so', async ({ page }) => {
  await openPanel(page, 'empty')
  await expect(page.getByText('No conversations yet')).toBeVisible()
})

test('a missing cloud gets one quiet line and local rows still show', async ({ page }) => {
  await openPanel(page, 'cloud-failure')
  await expect(page.getByText('Claude Code cloud sessions could not be read: the sign-in expired.')).toBeVisible()
  await expect(row(page, 'Fix the login redirect')).toBeVisible()
})

test('folders with the same name show their parent, and the full path on hover', async ({ page }) => {
  await openPanel(page, 'duplicate-projects')
  await expect(row(page, 'Add rate limits')).toContainText(' · shop/api · ')
  await expect(row(page, 'Split the user routes')).toContainText(' · billing/api · ')
  await expect(row(page, 'Fix the health check')).toContainText(' · Work/api · ')
  await expect(row(page, 'Add rate limits')).toHaveAttribute('data-tooltip', 'Resume in Terminal\n/Users/you/Code/shop/api')
  await expect(row(page, 'Add rate limits')).toHaveAccessibleDescription('Resume in Terminal /Users/you/Code/shop/api')
})

test('a folder name no other row shares stays short', async ({ page }) => {
  await openPanel(page)
  await expect(row(page, 'Fix the login redirect')).toContainText('Cursor · shop · 3h')
})

test('the circle lock counts waiting chats and the accessible name includes unread replies', async ({ page }) => {
  await page.request.post('/dev/fixture', { data: { name: 'mixed' } })
  await page.goto('/')
  await expect(page.locator('#badge')).toHaveText('1')
  await expect(page.getByRole('button', { name: 'Conversations: 1 waiting for you, 1 unread' })).toBeVisible()
})

test('a row that waits for the person shows the circle\'s lock; an unread reply keeps a plain dot', async ({ page }) => {
  await openPanel(page)
  const waiting = row(page, 'Approve the database migration').locator('.state')
  await expect(waiting).toHaveClass('state is-blocked has-lock')
  await expect(waiting.locator('svg')).toBeVisible()
  await expect(waiting).toHaveCSS('background-color', 'rgb(240, 163, 94)')
  await expect(row(page, 'Draft the release notes').locator('.state svg')).toHaveCount(0)
})

test('a chat waiting for the person gives the circle a solid badge with a lock; unread replies alone do not', async ({ page }) => {
  await page.request.post('/dev/fixture', { data: { name: 'mixed' } })
  await page.goto('/')
  const badge = page.locator('#badge')
  await expect(badge).toHaveClass(/\bis-blocked\b/)
  await expect(badge.locator('.lock')).toBeVisible()
  await expect(badge).toHaveCSS('background-image', 'none')
  await expect(badge).toHaveCSS('background-color', 'rgb(240, 163, 94)')

  await page.request.post('/dev/fixture', { data: { name: 'duplicate-projects' } })
  await page.goto('/')
  await expect(page.getByRole('button', { name: 'Conversations: 1 unread' })).toBeVisible()
  await expect(badge).not.toHaveClass(/\bis-blocked\b/)
  await expect(badge.locator('.lock')).toBeHidden()
  await expect(badge).toHaveText('1')
})

test('in yolo an approved chat works on outside Needs you and the circle counts only what the gate left to the person', async ({ page }) => {
  await page.request.post('/dev/fixture', { data: { name: 'auto-approvals' } })
  await page.goto('/')
  await expect(page.locator('#badge')).toHaveText('2')
  await expect(page.getByRole('button', { name: 'Conversations: 2 waiting for you' })).toBeVisible()

  await openPanel(page, 'auto-approvals')
  await expect(page.locator('.row.needs-you .title')).toHaveText(['Ship the parser fix', 'Rename the config keys'])
  const working = row(page, 'Fix the flaky login test')
  await expect(working).not.toHaveClass(/\bneeds-you\b/)
  await expect(working.locator('.approval')).toHaveCount(0)
  await expect(working.locator('.approved-toggle')).toHaveText('▸ 3 approved for you')
})

test('a permission the gate left to the person names why, and a tool approvals do not reach says so', async ({ page }) => {
  await openPanel(page, 'auto-approvals')
  const push = row(page, 'Ship the parser fix')
  await expect(push.locator('.text > *')).toHaveClass(['title', 'reply is-approval', 'approval-reason', 'meta', 'approved'])
  await expect(push.locator('.approval-reason')).toHaveText('Always asks: publishing')
  await expect(push).toHaveAttribute('data-tooltip', /Always asks: publishing/)
  await expect(row(page, 'Rename the config keys').locator('.approval-reason')).toHaveText('Approvals not supported for Cursor')
})

test('"N approved for you" folds out the approved requests, newest first, and opens nothing', async ({ page }) => {
  await openPanel(page, 'auto-approvals')
  await row(page, 'Fix the flaky login test').getByRole('button', { name: '▸ 3 approved for you' }).click()
  const list = page.getByRole('list', { name: 'Approved for you' })
  await expect(list.locator('.approved-call')).toHaveText(['Bash go test ./internal/auth/...', 'Bash make lint', 'Bash'])
  await expect(row(page, 'Fix the flaky login test').getByRole('button', { name: '▾ 3 approved for you' })).toHaveAttribute('aria-expanded', 'true')
  expect(await calledPaths(page)).not.toContain('/api/open')
})

test('the approved list folds out inside its row, and the keys stay on the toggle', async ({ page }) => {
  await openPanel(page, 'auto-approvals')
  const working = row(page, 'Fix the flaky login test')
  await working.getByRole('button', { name: '▸ 3 approved for you' }).focus()
  await page.keyboard.press('Space')
  const open = working.getByRole('button', { name: '▾ 3 approved for you' })
  await expect(open).toBeFocused()
  await expect(working.getByRole('list', { name: 'Approved for you' })).toBeVisible()
  await page.keyboard.press('Enter')
  await expect(working.getByRole('button', { name: '▸ 3 approved for you' })).toBeFocused()
  await expect(page.getByRole('list', { name: 'Approved for you' })).toHaveCount(0)
})

test('folding a group by key keeps the keys on its title', async ({ page }) => {
  await openPanel(page, 'auto-approvals')
  await groupTitle(page, 'Needs you').focus()
  await page.keyboard.press('Enter')
  await expect(groupTitle(page, 'Needs you')).toBeFocused()
  await expect(groupTitle(page, 'Needs you')).toHaveAttribute('aria-expanded', 'false')
})

test('the header names the approvals mode in a chip and Ask me switches agnostic-ai to ask', async ({ page }) => {
  await openPanel(page, 'mixed')
  const listTop = (await page.locator('#list').boundingBox())!.y
  await expect(page.locator('#approvals-bar')).toBeHidden()

  await openPanel(page, 'auto-approvals')
  const sentence = 'Approvals: routine prompts are approved for you'
  const chip = page.getByRole('group', { name: sentence })
  await expect(chip).toContainText('Auto-approve on')
  await expect(chip).toHaveAttribute('title', sentence)
  // The chip sits in the header row: the list starts where it starts with no chip.
  expect((await chip.boundingBox())!.y).toBeLessThan((await page.locator('.search').boundingBox())!.y + 20)
  expect((await page.locator('#list').boundingBox())!.y).toBe(listTop)
  const asked = page.waitForRequest((request) => request.url().endsWith('/api/approvals/ask'))
  await chip.getByRole('button', { name: 'Ask me' }).click()
  expect((await asked).postDataJSON()).toEqual({ mode: 'ask' })
  await expect(chip).toBeHidden()
  await expect(page.locator('#toast-text')).toHaveText('Approvals set to ask. Your tool may still use its own reviewer.')
})

test('under Needs you, permission waits come before a question', async ({ page }) => {
  await openPanel(page, 'approvals')
  await expect(page.locator('.row.needs-you .title')).toHaveText(['Refactor PRs', 'Webhooks v2 dashboard', 'Rider lifecycle gaps in Notion', 'Release notes draft', 'SFTP write: discussion'])
})

const deleteItem = (page: Page) => page.getByRole('menuitem', { name: 'Delete permanently…' })
const deleteDialog = (page: Page) => page.getByRole('alertdialog')

test('warns about forgotten shells and only stops the shell confirmed in the row menu', async ({ page }) => {
  await openPanel(page, 'stuck-shells')
  await expect(row(page, 'Preview the reporting changes').locator('.shell-text')).toHaveText('Faramir 15h bash script…review.bash')
  await expect(row(page, 'Preview the reporting changes')).toHaveClass(/needs-you/)
  const working = row(page, 'Refactor the search index')
  await expect(working.locator('.shell-text')).toHaveText(['1h npm run dev', '45m npm run watch'])

  const dialog = page.getByRole('alertdialog', { name: 'Stop shell?' })
  const stop = dialog.getByRole('button', { name: 'Stop shell' })
  const chooseShell = async () => {
    await working.click({ button: 'right' })
    await page.getByRole('menuitem', { name: 'Stop shell…' }).click()
    await expect(stop).toBeDisabled()
    await dialog.getByRole('combobox', { name: 'Choose a shell' }).selectOption({ label: 'Shell running for 1h: npm run dev (PID 840)' })
    await expect(dialog.locator('code')).toHaveText('npm run dev')
    await expect(dialog).toContainText('Running for 1h. This stops 3 processes.')
  }

  await chooseShell()
  await dialog.getByRole('button', { name: 'Cancel' }).click()
  await expect(dialog).toBeHidden()
  expect(await calledPaths(page)).not.toContain('/api/stop-shell')
  await expect(working.locator('.shell-warning')).toHaveCount(2)

  await chooseShell()
  const request = page.waitForRequest((request) => request.url().endsWith('/api/stop-shell'))
  await stop.click()
  expect((await request).postDataJSON()).toEqual({ tool: 'claude', id: 'shells-working', pid: 840, procStart: '2026-11-07T14:00:00.000Z' })
  await expect(dialog).toBeHidden()
  await expect(working.locator('.shell-text')).toHaveText(['45m npm run watch', 'Shell stop sent. Processes still running (PIDs 841).'])
  await expectInfoNotice(page, 'Processes still running')
  await expect(noticeCard(page)).toContainText('Processes still running (PIDs 841).')
})

const shellDialog = (page: Page) => page.getByRole('alertdialog', { name: 'Stop shell?' })
const shellLine = (scope: Locator, command: string) => scope.locator('.shell-warning').filter({ hasText: command })
const keptShells = async (page: Page) => ((await (await page.request.get('/api/kept-shells')).json()) as { shells: { pid: number, keeps: number }[] }).shells

test('Stop on a shell line confirms that shell, and the line follows the stop', async ({ page }) => {
  await openPanel(page, 'stuck-shells')
  const working = row(page, 'Refactor the search index')
  const dialog = shellDialog(page)
  const line = shellLine(working, 'npm run dev')
  const stopOnLine = working.getByRole('button', { name: 'Stop shell: npm run dev' })
  await expect(working.getByRole('button', { name: 'Keep running: npm run watch' })).toBeVisible()
  await expect(line).toHaveCSS('color', 'rgb(240, 163, 94)')
  await expect(stopOnLine).toHaveCSS('color', 'rgb(240, 163, 94)')

  await stopOnLine.click()
  await expect(dialog).toBeVisible()
  await expect(dialog.getByRole('combobox', { name: 'Choose a shell' })).toBeHidden()
  await expect(dialog.locator('code')).toHaveText('npm run dev')
  await expect(dialog).toContainText('Running for 1h. This stops 3 processes.')
  await expect(dialog).toContainText('The agent is still working and may need this shell.')
  await expect(dialog.getByRole('button', { name: 'Cancel' })).toBeFocused()
  await dialog.getByRole('button', { name: 'Cancel' }).click()
  await expect(dialog).toBeHidden()
  await stopOnLine.click()
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
  await stopOnLine.click()
  await page.mouse.click(5, 5)
  await expect(dialog).toBeHidden()
  const paths = await calledPaths(page)
  expect(paths).not.toContain('/api/stop-shell')
  expect(paths).not.toContain('/api/open')

  let answer!: () => void
  const answered = new Promise<void>((resolve) => { answer = resolve })
  await page.route('**/api/stop-shell', async (route) => {
    await answered
    await route.continue()
  })
  await stopOnLine.click()
  const request = page.waitForRequest((request) => request.url().endsWith('/api/stop-shell'))
  await dialog.getByRole('button', { name: 'Stop shell' }).click()
  expect((await request).postDataJSON()).toEqual({ tool: 'claude', id: 'shells-working', pid: 840, procStart: '2026-11-07T14:00:00.000Z' })
  await expect(working.locator('.shell-warning.is-stopping')).toHaveText('Stopping the shell…')
  await expect(working.locator('.shell-warning.is-stopping').getByRole('button')).toHaveCount(0)
  answer()
  const stopped = working.locator('.shell-warning.is-stopped')
  await expect(stopped).toHaveText('Shell stop sent. Processes still running (PIDs 841).')
  await expect(stopped).toHaveCSS('color', ALERT_RED)
  await expect(noticeCard(page)).toContainText('Processes still running (PIDs 841).')
})

test('a row waiting only on a shell shows a terminal, and its confirm has no working line', async ({ page }) => {
  await openPanel(page, 'stuck-shells')
  const waiting = row(page, 'Preview the reporting changes')
  await expect(waiting.locator('.state.has-lock svg path')).toHaveAttribute('d', /^M3\.6 2\.5h8\.8/)
  await waiting.getByRole('button', { name: 'Stop shell: bash scripts/run-preview.bash' }).click()
  const dialog = shellDialog(page)
  await expect(dialog.locator('code')).toHaveText('bash scripts/run-preview.bash')
  await expect(dialog).not.toContainText('The agent is still working')
  await dialog.getByRole('button', { name: 'Stop shell' }).click()
  await expect(dialog).toBeHidden()
  await expect(page.locator('.shell-warning.is-stopped')).toHaveText('Shell stopped.')
  await expect(page.locator('.shell-warning.is-stopped')).not.toHaveCSS('color', ALERT_RED)
  expect(await calledPaths(page)).not.toContain('/api/open')
})

test('Tab reaches the shell buttons, and Enter and Space press them without opening the chat', async ({ page }) => {
  await openPanel(page, 'stuck-shells')
  const waiting = row(page, 'Preview the reporting changes')
  const keep = waiting.getByRole('button', { name: 'Keep running: bash scripts/run-preview.bash' })
  const stop = waiting.getByRole('button', { name: 'Stop shell: bash scripts/run-preview.bash' })
  await keep.focus()
  await page.keyboard.press('Tab')
  await expect(stop).toBeFocused()
  await page.keyboard.press('Space')
  await expect(shellDialog(page)).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(shellDialog(page)).toBeHidden()
  await keep.focus()
  await page.keyboard.press('Enter')
  await expect(groupTitle(page, 'Active')).toBeVisible()
  await expect(page.getByLabel('Search every conversation')).toBeFocused()
  await expect(row(page, 'Preview the reporting changes')).not.toHaveClass(/needs-you/)
  expect(await calledPaths(page)).not.toContain('/api/open')
})

test('Keep running quiets a shell for 30 minutes, then doubles the wait up to 4 hours', async ({ page }) => {
  const reset = await page.request.post('/dev/fixture', { data: { name: 'stuck-shells' } })
  expect(reset.ok()).toBe(true)
  await page.clock.install({ time: NOW })
  await page.goto('/panel.html')
  const waiting = row(page, 'Preview the reporting changes')
  await expect(waiting).toHaveClass(/needs-you/)

  const request = page.waitForRequest((request) => request.url().endsWith('/api/kept-shells/keep'))
  await waiting.getByRole('button', { name: 'Keep running: bash scripts/run-preview.bash' }).click()
  expect((await request).postDataJSON()).toEqual({ tool: 'claude', id: 'shell-blocked', pid: 531, procStart: '2026-11-07T00:30:00.000Z', wakeAt: expect.stringMatching(/^2026-11-07T16:00:00/), keeps: 1 })
  const line = shellLine(waiting, 'review.bash')
  await expect(line.locator('.shell-text')).toHaveText('Kept running · asks again in 30 min: bash script…review.bash')
  await expect(line).toHaveCSS('color', 'rgba(242, 237, 229, 0.62)')
  await expect(line.getByRole('button', { name: /^Keep running/ })).toHaveCount(0)
  await expect(line.getByRole('button', { name: 'Stop shell: bash scripts/run-preview.bash' })).toBeVisible()
  await expect(waiting).not.toHaveClass(/needs-you/)
  await expect(waiting.locator('.state.has-lock')).toHaveCount(0)
  expect(await calledPaths(page)).not.toContain('/api/open')

  for (const [waited, next] of [['30:00', '1 h'], ['01:00:00', '2 h'], ['02:00:00', '4 h'], ['04:00:00', '4 h']]) {
    await page.clock.fastForward(waited)
    await expect(waiting).toHaveClass(/needs-you/)
    await waiting.getByRole('button', { name: 'Keep running: bash scripts/run-preview.bash' }).click()
    await expect(line.locator('.shell-text')).toHaveText(`Kept running · asks again in ${next}: bash script…review.bash`)
  }
  expect((await keptShells(page))[0].keeps).toBe(5)
})

test('Ask about this shell again brings the row back at once', async ({ page }) => {
  await openPanel(page, 'stuck-shells')
  const waiting = row(page, 'Preview the reporting changes')
  await waiting.getByRole('button', { name: 'Keep running: bash scripts/run-preview.bash' }).click()
  await expect(waiting).not.toHaveClass(/needs-you/)
  await waiting.click({ button: 'right' })
  await page.getByRole('menuitem', { name: 'Ask about this shell again' }).click()
  await expect(waiting).toHaveClass(/needs-you/)
  expect(await keptShells(page)).toEqual([])

  await row(page, 'Refactor the search index').click({ button: 'right' })
  await expect(page.getByRole('menuitem', { name: 'Ask about this shell again' })).toBeHidden()
})

test('stopping a kept shell drops its record', async ({ page }) => {
  await openPanel(page, 'stuck-shells')
  const working = row(page, 'Refactor the search index')
  await working.getByRole('button', { name: 'Keep running: npm run watch' }).click()
  await expect(shellLine(working, 'npm run watch')).toHaveClass(/is-kept/)
  await working.getByRole('button', { name: 'Stop shell: npm run watch' }).click()
  await shellDialog(page).getByRole('button', { name: 'Stop shell' }).click()
  await expect(working.locator('.shell-warning.is-stopped')).toHaveText('Shell stopped.')
  await expect.poll(() => keptShells(page)).toEqual([])

  const waiting = row(page, 'Preview the reporting changes')
  await waiting.getByRole('button', { name: 'Keep running: bash scripts/run-preview.bash' }).click()
  await expect(shellLine(waiting, 'review.bash')).toHaveClass(/is-kept/)
  await waiting.getByRole('button', { name: 'Stop shell: bash scripts/run-preview.bash' }).click()
  await shellDialog(page).getByRole('button', { name: 'Stop shell' }).click()
  await expect.poll(async () => (await keptShells(page)).map((shell) => shell.pid)).toEqual([])
})

async function openArchived(page: Page, fixture = 'archived') {
  await openPanel(page, fixture)
  await groupTitle(page, 'Archived').click()
  await expect(row(page, 'Tidy the old branches')).toBeVisible()
}

function deletesSent(page: Page) {
  const sent: unknown[] = []
  page.on('request', (request) => {
    if (request.url().endsWith('/api/delete')) sent.push(request.postDataJSON())
  })
  return sent
}

test('the row menu offers Delete permanently only on a chat under Archived', async ({ page }) => {
  await openArchived(page)
  for (const title of ['Tidy the old branches', 'Sketch the pricing page']) {
    await row(page, title).click({ button: 'right' })
    await expect(deleteItem(page)).toBeVisible()
    await page.keyboard.press('Escape')
  }

  await row(page, 'Write the migration guide').click({ button: 'right' })
  await expect(page.getByRole('menuitem', { name: 'Rename…' })).toBeFocused()
  await expect(deleteItem(page)).toBeHidden()
  await page.keyboard.press('Escape')
})

test('the delete confirmation names the chat and what goes, and Cancel or Escape sends nothing', async ({ page }) => {
  await openArchived(page)
  const sent = deletesSent(page)
  await row(page, 'Tidy the old branches').click({ button: 'right' })
  await deleteItem(page).click()

  await expect(deleteDialog(page)).toContainText('Delete "Tidy the old branches" permanently?')
  await expect(deleteDialog(page)).toContainText('Claude Code · agnostic-ai · 3d')
  await expect(deleteDialog(page)).toContainText('its transcript, its subagent transcripts, saved tool output, file backups and pasted images')
  await expect(deleteDialog(page)).toContainText('It cannot be undone.')
  await expect(deleteDialog(page).getByRole('button', { name: 'Cancel' })).toBeFocused()
  await deleteDialog(page).getByRole('button', { name: 'Cancel' }).click()
  await expect(deleteDialog(page)).toBeHidden()

  await row(page, 'Tidy the old branches').click({ button: 'right' })
  await deleteItem(page).click()
  await expect(deleteDialog(page)).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(deleteDialog(page)).toBeHidden()
  await expect(row(page, 'Tidy the old branches')).toBeVisible()
  expect(sent).toEqual([])
})

test('confirming a delete removes the row and says what it freed', async ({ page }) => {
  await openArchived(page)
  const sent = deletesSent(page)
  await row(page, 'Tidy the old branches').click({ button: 'right' })
  await deleteItem(page).click()
  await deleteDialog(page).getByRole('button', { name: 'Delete permanently' }).click()

  await expect(page.getByRole('status').filter({ hasText: 'Deleted. Freed 12 MB.' })).toBeVisible()
  await expect(row(page, 'Tidy the old branches')).toHaveCount(0)
  await expect(deleteDialog(page)).toBeHidden()
  expect(sent).toEqual([{ tool: 'claude', id: 'done-2' }])
})

test('a chat that went quiet on its own can be deleted through the same confirmation', async ({ page }) => {
  await openArchived(page)
  const sent = deletesSent(page)
  await row(page, 'Sketch the pricing page').click({ button: 'right' })
  await deleteItem(page).click()

  await expect(deleteDialog(page)).toContainText('Delete "Sketch the pricing page" permanently?')
  await expect(deleteDialog(page)).toContainText('It cannot be undone.')
  await deleteDialog(page).getByRole('button', { name: 'Delete permanently' }).click()

  await expect(page.getByRole('status').filter({ hasText: 'Deleted. Freed 12 MB.' })).toBeVisible()
  await expect(row(page, 'Sketch the pricing page')).toHaveCount(0)
  expect(sent).toEqual([{ tool: 'claude', id: 'old-2' }])
})

test('a delete that left something to tidy says so in a neutral notice, and a later failure turns it red', async ({ page }) => {
  await openArchived(page, 'delete-note')
  await row(page, 'Sketch the pricing page').click({ button: 'right' })
  await deleteItem(page).click()
  await deleteDialog(page).getByRole('button', { name: 'Delete permanently' }).click()

  await expect(noticeCard(page)).toContainText('Deleted. Freed 12 MB. Its marks could not be removed: the marks file is read-only.')
  await expectInfoNotice(page, 'What was not tidied')
  await expect(row(page, 'Sketch the pricing page')).toHaveCount(0)

  await row(page, 'Bump the Go version').click({ button: 'right' })
  await deleteItem(page).click()
  await expectErrorNotice(page, 'Why it cannot be deleted')
  await expect(noticeCard(page)).not.toContainText('What was not tidied')
})

test('a refused delete keeps the row and its reason stays on screen', async ({ page }) => {
  await openArchived(page, 'delete-failure')
  await row(page, 'Tidy the old branches').click({ button: 'right' })
  await deleteItem(page).click()
  await deleteDialog(page).getByRole('button', { name: 'Delete permanently' }).click()

  const reason = page.getByRole('region', { name: 'Why it was not deleted' })
  await expect(reason).toContainText('Deleted 1 of 4 locations, then stopped: permission denied. Still on disk:')
  await expectErrorNotice(page, 'Why it was not deleted')
  // The toast was gone after 3.2 seconds; the reason is not.
  await page.waitForTimeout(3_500)
  await expect(reason).toBeVisible()
  await expect(row(page, 'Tidy the old branches')).toBeVisible()
})

test('a chat the server cannot delete says why and sends nothing', async ({ page }) => {
  await openArchived(page)
  const sent = deletesSent(page)
  await row(page, 'Bump the Go version').click({ button: 'right' })
  await deleteItem(page).click()

  await expect(deleteDialog(page)).toBeHidden()
  await expect(page.getByRole('region', { name: 'Why it cannot be deleted' })).toContainText('Deleting Codex threads from here is not supported: Codex deletes a thread even while it is open.')
  await expectErrorNotice(page, 'Why it cannot be deleted')
  await expect(row(page, 'Bump the Go version')).toBeVisible()
  expect(sent).toEqual([])
})

const titles = (page: Page) => page.getByRole('option').locator('.title')
const nestedRows = (page: Page) => page.locator('.row.is-nested')

test('a chat started from another sits under it while both are in the same group, and is a normal row elsewhere', async ({ page }) => {
  await openPanel(page, 'nested')
  await expect(titles(page)).toHaveText([
    'Check the docs links', 'Approve the API change', 'Update the API docs',
    'Plan the docs site', 'Write the docs index', 'Draft the API guide', 'Try the API guide examples', 'Tidy the docs styles',
  ])
  await expect(nestedRows(page).locator('.title')).toHaveText(['Update the API docs', 'Write the docs index', 'Try the API guide examples'])
  await expect(groupTitle(page, 'Needs you').locator('.count')).toHaveText('3')
  await expect(row(page, 'Update the API docs')).toHaveAttribute('aria-description', /^Started from Approve the API change\n/)
  await expect(row(page, 'Check the docs links')).not.toHaveAttribute('aria-description', /Started from/)
  await expect(row(page, 'Write the docs index').getByRole('button')).toHaveCount(await row(page, 'Plan the docs site').getByRole('button').count())
  const indent = async (title: string) => (await row(page, title).locator('.state').boundingBox())!.x
  expect(await indent('Write the docs index')).toBeGreaterThan(await indent('Plan the docs site'))

  // The arrow keys follow the drawn order.
  for (const title of ['Check the docs links', 'Approve the API change', 'Update the API docs', 'Plan the docs site']) {
    await page.keyboard.press('ArrowDown')
    await expect(selected(page)).toContainText(title)
  }

  await page.getByRole('button', { name: 'Local', exact: true }).click()
  await expect(row(page, 'Try the API guide examples')).not.toHaveClass(/is-nested/)
  await page.getByRole('button', { name: 'All', exact: true }).click()

  await page.getByLabel('Search every conversation').fill('docs')
  await expect(titles(page)).toHaveText(['Check the docs links', 'Update the API docs', 'Plan the docs site', 'Write the docs index', 'Tidy the docs styles'])
  await expect(nestedRows(page).locator('.title')).toHaveText(['Write the docs index'])
})

test('archiving, renaming or hiding a parent never changes its child', async ({ page }) => {
  await openPanel(page, 'nested')
  await row(page, 'Approve the API change').click({ button: 'right' })
  await page.getByRole('menuitem', { name: 'Rename…' }).click()
  await page.getByLabel('New name').fill('Approve the API field change')
  await page.getByLabel('New name').press('Enter')
  await expect(row(page, 'Update the API docs')).toHaveAttribute('aria-description', /^Started from Approve the API field change\n/)
  await expect(row(page, 'Update the API docs')).toHaveClass(/is-nested/)

  const parent = row(page, 'Plan the docs site')
  await parent.hover()
  await completeButton(parent).click()
  await expect(parent).toBeHidden()
  await expect(row(page, 'Write the docs index')).toBeVisible()
  await expect(row(page, 'Write the docs index')).not.toHaveClass(/is-nested/)
  await expect(row(page, 'Check the docs links')).not.toHaveClass(/is-nested/)

  await groupTitle(page, 'Archived').click()
  await parent.hover()
  await parent.getByRole('button', { name: /^Hide from agnostic-ai,/ }).click()
  await expect(parent).toHaveCount(0)
  await expect(row(page, 'Write the docs index')).toBeVisible()
  await expect(row(page, 'Write the docs index')).not.toHaveClass(/is-nested/)
})

test('mark all as read marks every Needs you row, the nested one included', async ({ page }) => {
  await openPanel(page, 'nested')
  const marks = markBodies(page)
  await markAllRead(page).click()
  await expect(groupTitle(page, 'Needs you')).toHaveCount(0)
  expect(marks).toEqual([seen('claude', 'nest-blocked'), seen('claude', 'nest-wait'), seen('codex', 'nest-reply')])
})

test('a local chat also open on claude.ai is one row that opens here, and its second action opens claude.ai', async ({ page }) => {
  await openPanel(page, 'shared')
  const shared = row(page, 'Review the payment retries')
  await expect(page.getByRole('option')).toHaveCount(3)
  await expect(shared.locator('.meta')).toContainText('Claude Code · Shared · agnostic-ai')
  await expect(shared.locator('.meta')).toHaveAttribute('data-tooltip', 'Also open on claude.ai')
  await expect(shared).toHaveAccessibleDescription(/Also open on claude\.ai/)

  for (const place of ['Local', 'Cloud']) {
    await page.getByRole('button', { name: place, exact: true }).click()
    await expect(shared).toHaveCount(1)
  }
  await page.getByRole('button', { name: 'All', exact: true }).click()

  const web = page.waitForRequest((request) => request.url().endsWith('/api/open'))
  await shared.hover()
  await shared.getByRole('button', { name: 'Open on claude.ai' }).click()
  expect((await web).postDataJSON()).toEqual({ tool: 'claude', id: 'cse_rc1', title: 'Review the payment retries' })

  await page.goto('/panel.html')
  const local = page.waitForRequest((request) => request.url().endsWith('/api/open'))
  await row(page, 'Review the payment retries').click({ position: { x: 40, y: 10 } })
  expect((await local).postDataJSON()).toEqual({ tool: 'claude', id: 'rc-1', title: 'Review the payment retries' })
})

test('the row menu of a shared chat opens its claude.ai page', async ({ page }) => {
  await openPanel(page, 'shared')
  await row(page, 'Review the payment retries').click({ button: 'right' })
  const web = page.waitForRequest((request) => request.url().endsWith('/api/open'))
  await page.getByRole('menuitem', { name: 'Open on claude.ai' }).click()
  expect((await web).postDataJSON()).toEqual({ tool: 'claude', id: 'cse_rc1', title: 'Review the payment retries' })
})

test('a chat folds its working subagents out under it, newest first, five at most, and each opens where it lives', async ({ page }) => {
  await openPanel(page, 'helpers')
  const parent = row(page, 'Rename chats')
  const toggle = parent.getByRole('button', { name: '▸ 7 subagents' })
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  // The toggle takes the place of the count; the parent's mini orbits stay.
  await expect(parent.locator('.worker-count')).toHaveCount(0)
  await expect(parent.locator('.work-worker')).toHaveCount(4)
  await expect(page.locator('.helper-row')).toHaveCount(0)

  await toggle.click()
  await expect(parent.getByRole('button', { name: '▾ 7 subagents' })).toHaveAttribute('aria-expanded', 'true')
  const helpers = page.locator('.helper-row')
  await expect(helpers).toHaveCount(5)
  await helpers.first().click({ button: 'right' })
  await expect(page.getByRole('menuitem', { name: 'Copy session ID' })).toBeHidden()
  await expect(helpers.locator('.title')).toHaveText(['Find rename call sites', 'Size budget check', 'Review #3511: local app chats', 'Review #3512: rename in each app', 'Helper rows design'])
  await expect(helpers.locator('.meta')).toHaveText(['Claude Code · Explore · 1m', 'Claude Code · 1m', 'Codex · 2m', 'Codex · 4m', 'Claude Code · Plan · 6m'])
  // A subagent's latest message sits between its title and its details; one that wrote none has no line.
  await expect(helpers.first().locator('.text > *')).toHaveClass(['title', 'reply', 'meta'])
  await expect(helpers.first().locator('.reply')).toHaveText('Twelve call sites so far, most in internal/cli.')
  await expect(helpers.locator('.reply')).toHaveCount(1)
  await expect(helpers.locator('.state.is-working > .work-system')).toHaveCount(5)
  // Subagents never become rows of their own.
  await expect(page.getByRole('option')).toHaveCount(2)

  await page.getByRole('button', { name: '+2 more' }).click()
  await expect(helpers).toHaveCount(7)
  await expect(helpers.last().locator('.title')).toHaveText('Untitled subagent')

  const codex = page.waitForRequest((request) => request.url().endsWith('/api/open'))
  await helpers.filter({ hasText: 'Review #3511' }).click()
  expect((await codex).postDataJSON()).toEqual({ tool: 'codex', id: 'run-3511', title: 'Review #3511: local app chats' })
  await page.goto('/panel.html')
  await parent.getByRole('button', { name: '▸ 7 subagents' }).click()
  await expect(helpers.filter({ hasText: 'Review #3511' })).toHaveAttribute('title', 'Open this Codex thread')
  const explore = helpers.filter({ hasText: 'Find rename call sites' })
  await expect(explore).toHaveAttribute('title', 'Open the chat it runs in, and show how to reach this subagent')
  const claude = page.waitForRequest((request) => request.url().endsWith('/api/open'))
  await explore.click()
  expect((await claude).postDataJSON()).toEqual({ tool: 'claude', id: 'helpers-1', title: 'Rename chats', helper: 'agent-explore' })
  await expectInfoNotice(page, 'How to reach the subagent')
  await expect(noticeCard(page)).toContainText("Press the down arrow to pick 'Find rename call sites' below the prompt, then Enter.")
})

test('subagents show as a tree under their real parent, each level folds, and a deep click shows its note at once', async ({ page }) => {
  await openPanel(page, 'subagent-tree')
  const parent = row(page, 'Ship the subagent tree')
  await parent.getByRole('button', { name: '▸ 6 subagents, 4 levels' }).click()
  const helpers = page.locator('.helper-row')
  await expect(helpers.locator('.title')).toHaveText(['echo', 'Review the diff', 'Find call sites', 'Read the open path', 'Check one file', 'Started by a finished subagent'])
  // Each level sits one step further in than the one above it.
  const left = (title: string) => helpers.filter({ hasText: title }).evaluate((element) => element.getBoundingClientRect().left)
  const lefts = await Promise.all(['Review the diff', 'Find call sites', 'Read the open path', 'Check one file'].map(left))
  for (let level = 1; level < lefts.length; level += 1) expect(lefts[level]).toBeGreaterThan(lefts[level - 1])
  expect(await left('Started by a finished subagent')).toBe(lefts[0])
  await expect(helpers.filter({ hasText: 'echo' })).toHaveAttribute('title', 'Switch to its pane')

  await page.getByRole('button', { name: '▾ 1 subagent' }).last().click()
  await expect(helpers.filter({ hasText: 'Check one file' })).toHaveCount(0)
  await page.getByRole('button', { name: '▸ 1 subagent' }).click()
  await expect(helpers.filter({ hasText: 'Check one file' })).toHaveCount(1)

  const opened = page.waitForRequest((request) => request.url().endsWith('/api/open'))
  const peeked = page.waitForRequest((request) => request.url().endsWith('/api/panel/peek'))
  await helpers.filter({ hasText: 'Check one file' }).click()
  expect((await opened).postDataJSON()).toEqual({ tool: 'claude', id: 'tree-1', title: 'Ship the subagent tree', helper: 'acheck-1' })
  await peeked
  await expectInfoNotice(page, 'How to reach the subagent')
  await expect(noticeCard(page)).toContainText("It runs inside 'Read the open path'. Open that row first.")
})

test('a subagent with its own pane opens it and leaves no note', async ({ page }) => {
  await openPanel(page, 'subagent-tree')
  await row(page, 'Ship the subagent tree').getByRole('button', { name: '▸ 6 subagents, 4 levels' }).click()
  const opened = page.waitForResponse((response) => response.url().endsWith('/api/open'))
  await page.locator('.helper-row').filter({ hasText: 'echo' }).click()
  expect(await (await opened).json()).toMatchObject({ opened: true, reached: 'subagent' })
  await expect(noticeCard(page)).toBeHidden()
})

test('a pending approval takes the reply line\'s place, and the reply moves to the tooltip', async ({ page }) => {
  await openPanel(page, 'approvals')
  const bash = row(page, 'Refactor PRs')
  await expect(bash.locator('.text > *')).toHaveClass(['title', 'reply is-approval', 'meta'])
  await expect(bash.locator('.approval')).toHaveText('Approve git push origin…')
  await expect(bash.locator('.approval-code')).toHaveText('git push origin…')
  await expect(bash.locator('.approval svg')).toHaveCount(1)
  await expect(bash).not.toContainText('The seed fix')
  await expect(bash).toHaveAttribute('data-tooltip', 'Open in Terminal\n/Users/you/Code/shop\nThe seed fix and the flag rename are still running.')
  await expect(bash.locator('.meta')).toHaveText('Claude Code · shop · 2m')
  await expect(bash.locator('.approval')).toHaveCSS('color', await page.locator('.group-title').first().evaluate((element) => getComputedStyle(element).color))

  await expect(row(page, 'Webhooks v2 dashboard').locator('.approval')).toHaveText('Approve edit dashboard.tsx')
  // An MCP tool reads as words, and a connector registered by id never shows the id.
  const mcp = row(page, 'Rider lifecycle gaps in Notion')
  await expect(mcp.locator('.approval')).toHaveText('Approve Notion fetch')
  await expect(mcp.locator('.approval-code')).toHaveCount(0)
  await expect(mcp).not.toContainText('bfe9f18c')
  // A long detail stays inside the row, cut with an ellipsis, and reads whole on hover.
  await expect(mcp.locator('.approval')).toHaveAttribute('data-tooltip', 'Approve Notion fetch')
  await mcp.locator('.approval-label').evaluate((element) => { element.textContent = 'Approve Google Calendar list events in every calendar' })
  const fits = await mcp.locator('.approval').evaluate((pill) => pill.getBoundingClientRect().right <= pill.closest('.text')!.getBoundingClientRect().right + 0.5)
  expect(fits).toBe(true)
})

test('a permission whose call is unknown shows a lock, and a question keeps its reply line', async ({ page }) => {
  await openPanel(page, 'approvals')
  const cloud = row(page, 'Release notes draft')
  await expect(cloud.locator('.approval.is-lock')).toHaveText('Needs permission')
  await expect(cloud.locator('.meta')).toHaveText('Codex · Cloud · 12m')

  const question = row(page, 'SFTP write: discussion')
  await expect(question.locator('.approval')).toHaveCount(0)
  await expect(question.locator('.reply')).toHaveText('Should the upload overwrite existing files?')
  await expect(question.locator('.meta')).toHaveText('Claude Code · shop · 6m')
  await expect(page.locator('.meta')).not.toContainText(['Waiting for you'])
})

test('working subagents get their own line under the meta line, and it still folds them out', async ({ page }) => {
  await openPanel(page, 'approvals')
  const question = row(page, 'SFTP write: discussion')
  await expect(question.locator('.text > *')).toHaveClass(['title', 'needs-reason is-question', 'reply', 'meta', 'subagents'])
  await expect(question.locator('.meta')).not.toContainText('subagent')
  await question.getByRole('button', { name: '▸ 1 subagent' }).click()
  await expect(page.locator('.helper-row .title')).toHaveText(['Check the SFTP folder'])
  await expect(row(page, 'SFTP write: discussion').getByRole('button', { name: '▾ 1 subagent' })).toHaveAttribute('aria-expanded', 'true')
  await expect(row(page, 'Rider lifecycle gaps in Notion').locator('.text > *')).toHaveClass(['title', 'reply is-approval', 'meta', 'subagents'])
  // A chat with no working subagent has no such line.
  await expect(row(page, 'Webhooks v2 dashboard').locator('.subagents')).toHaveCount(0)
})

test('a chat whose subagents all finished shows no subagent toggle', async ({ page }) => {
  await openPanel(page, 'helpers')
  const done = row(page, 'Plan the migration')
  await expect(done).toBeVisible()
  await expect(done.locator('.helpers-toggle')).toHaveCount(0)
  await expect(done.locator('.worker-count')).toHaveCount(0)
})

test('Enter on a subagent control opens nothing but what it names, even with its chat selected', async ({ page }) => {
  await openPanel(page, 'helpers')
  const opens: unknown[] = []
  page.on('request', (request) => {
    if (request.url().endsWith('/api/open')) opens.push(request.postDataJSON())
  })
  const parent = row(page, 'Rename chats')
  await parent.hover()
  await expect(selected(page)).toContainText('Rename chats')

  await parent.getByRole('button', { name: '▸ 7 subagents' }).focus()
  await page.keyboard.press('Enter')
  await expect(parent.getByRole('button', { name: '▾ 7 subagents' })).toHaveAttribute('aria-expanded', 'true')
  await page.getByRole('button', { name: '+2 more' }).focus()
  await page.keyboard.press('Enter')
  await expect(page.locator('.helper-row')).toHaveCount(7)
  await expect(selected(page)).toContainText('Rename chats')

  const codex = page.waitForRequest((request) => request.url().endsWith('/api/open'))
  await page.locator('.helper-row').filter({ hasText: 'Review #3511' }).focus()
  await page.keyboard.press('Enter')
  await codex
  // Both opens of one key press would leave together; the reload comes after them.
  await page.goto('/panel.html')
  expect(opens).toEqual([{ tool: 'codex', id: 'run-3511', title: 'Review #3511: local app chats' }])
})

const pickBanner = (page: Page) => page.getByRole('region', { name: 'Send the copied text' })

test('a copied web page does not promise Markdown in the banner, since the server decides', async ({ page }) => {
  await openPanel(page, 'markdown-clipboard')
  await page.keyboard.press('Meta+Shift+KeyV')

  await expect(pickBanner(page)).toContainText('Click a chat or press Enter to paste these 12 words there')
  await expect(pickBanner(page)).not.toContainText('Markdown')
  await expect(pickBanner(page)).toContainText('Release notes')
})

test('Cmd+Shift+V picks a conversation for the copied text, and Enter sends it there', async ({ page }) => {
  await openPanel(page)
  await page.keyboard.press('Meta+Shift+KeyV')

  await expect(pickBanner(page)).toContainText('Click a chat or press Enter to paste these 14 words there')
  await expect(page.locator('#pick-preview')).toHaveText(/^Fix the flaky login test\./)
  // Only this machine's conversations, and no Archived: those cannot take the text.
  await expect(page.getByRole('button', { name: 'Local' })).toHaveAttribute('aria-pressed', 'true')
  await expect(groupTitle(page, 'Archived')).toHaveCount(0)
  await expect(row(page, 'Write the onboarding guide')).toHaveCount(0)
  await expect(selected(page)).toContainText('Approve the database migration')

  await page.keyboard.press('ArrowDown')
  await expect(selected(page)).toContainText('Draft the release notes')
  const copied = await (await page.request.get('/api/clipboard')).json()
  const sent = page.waitForRequest((request) => request.url().endsWith('/api/send'))
  await page.keyboard.press('Enter')
  expect((await sent).postDataJSON()).toEqual({ tool: 'codex', id: 'reply-1', hash: copied.hash })

  await expect(pickBanner(page)).toBeHidden()
  const calls = await calledPaths(page)
  expect(calls).toContain('/api/panel/hide')
  expect(calls).not.toContain('/api/open')
  expect(calls).not.toContain('/api/panel/show')
})

test('pick mode starts on the one row the text can be typed into, and still waits for Enter', async ({ page }) => {
  await openPanel(page, 'one-terminal')
  await page.keyboard.press('Meta+Shift+KeyV')

  await expect(selected(page)).toContainText('Plan the sync dry run')
  expect(await calledPaths(page)).not.toContain('/api/send')
  const sent = page.waitForRequest((request) => request.url().endsWith('/api/send'))
  await page.keyboard.press('Enter')
  expect((await sent).postDataJSON()).toMatchObject({ tool: 'claude', id: 'active-2' })
})

test('a double click sends once, and Escape while the chip flies takes the send back', async ({ page }) => {
  await openPanel(page)
  await page.keyboard.press('Meta+Shift+KeyV')
  await row(page, 'Draft the release notes').dblclick()
  await expect(pickBanner(page)).toBeHidden()
  await expect.poll(async () => (await calledPaths(page)).filter((path) => path === '/api/send').length).toBe(1)
  await page.waitForTimeout(600)
  const calls = await calledPaths(page)
  expect(calls.filter((path) => path === '/api/send')).toHaveLength(1)
  expect(calls).not.toContain('/api/open')

  // With animation off nothing flies, so the send leaves at once and Escape comes too late.
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await openPanel(page)
  await expect(page.locator('html')).toHaveAttribute('data-motion', 'on')
  await page.keyboard.press('Meta+Shift+KeyV')
  await expect(pickBanner(page)).toBeVisible()
  // Escape lands the moment the chip leaves the banner.
  await page.evaluate(() => {
    const card = document.getElementById('card')!
    const watch = new MutationObserver(() => {
      if (!card.querySelector('.pick-chip')) return
      watch.disconnect()
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    })
    watch.observe(card, { childList: true })
  })
  await page.keyboard.press('Enter')
  await expect(pickBanner(page)).toBeHidden()
  await page.waitForTimeout(600)
  expect(await calledPaths(page)).not.toContain('/api/send')
})

test('a send that only switched to the conversation comes back to say to paste', async ({ page }) => {
  await openPanel(page)
  await pressSendShortcut(page)
  await expect(pickBanner(page)).toBeVisible()

  await row(page, 'Fix the login redirect').click()

  await expect(page.locator('#notice')).toContainText('Switched to it. Paste with Cmd+V.')
  await expectInfoNotice(page, 'What to do with the copied text')
  await expect(pickBanner(page)).toBeHidden()
  expect(await calledPaths(page)).toContain('/api/panel/show')
})

for (const [fixture, message] of [
  ['send-failure', 'agnostic-ai serve did not answer in time.'],
  ['switch-failure', 'Cursor runs in an app that cannot be brought forward. The text is on the clipboard: paste it with Cmd+V.'],
]) {
  test(`a send that neither typed nor switched (${fixture}) comes back as an error`, async ({ page }) => {
    await openPanel(page, fixture)
    await pressSendShortcut(page)
    await expect(pickBanner(page)).toBeVisible()

    await row(page, 'Fix the login redirect').click()

    await expect(noticeCard(page)).toContainText(message)
    await expectErrorNotice(page, 'Why nothing was typed')
  })
}

test('nothing copied keeps pick mode closed and says why', async ({ page }) => {
  await openPanel(page, 'empty-clipboard')
  await page.keyboard.press('Meta+Shift+KeyV')

  await expect(page.locator('#toast')).toHaveText('Nothing to send: copy some text first.')
  await expect(page.locator('#toast')).toHaveAttribute('role', 'alert')
  await expect(page.locator('#toast .tone-icon')).toBeVisible()
  await expect(pickBanner(page)).toBeHidden()
})

test('Escape leaves pick mode first, and a second Escape closes the list', async ({ page }) => {
  await openPanel(page)
  await page.keyboard.press('Meta+Shift+KeyV')
  await expect(pickBanner(page)).toBeVisible()

  await page.keyboard.press('Escape')
  await expect(pickBanner(page)).toBeHidden()
  await expect(groupTitle(page, 'Archived')).toHaveCount(1)
  expect(await calledPaths(page)).not.toContain('/api/panel/hide')

  await page.keyboard.press('Escape')
  await expect.poll(() => calledPaths(page)).toContain('/api/panel/hide')
})

test('a clipboard that changed since the banner showed it sends nothing and refreshes the banner', async ({ page }) => {
  await openPanel(page)
  await page.keyboard.press('Meta+Shift+KeyV')
  await expect(pickBanner(page)).toContainText('Click a chat or press Enter to paste these 14 words there')

  expect((await page.request.post('/dev/clipboard', { data: { text: 'Two words' } })).ok()).toBe(true)
  await page.keyboard.press('Enter')

  await expect(page.locator('#notice')).toContainText('The copied text changed. Check the new text, then send again.')
  await expectErrorNotice(page, 'Why nothing was sent')
  await expect(pickBanner(page)).toContainText('Click a chat or press Enter to paste these 2 words there')
  expect(await calledPaths(page)).not.toContain('/api/send')
})

test('a search in pick mode never offers an archived chat, and Enter then sends nothing', async ({ page }) => {
  await openPanel(page)
  await page.getByLabel('Search every conversation').fill('go version')
  await expect(row(page, 'Bump the Go version')).toHaveCount(1)
  await pressSendShortcut(page)
  await expect(pickBanner(page)).toBeVisible()
  await page.getByLabel('Search every conversation').fill('go version')

  // The search's answer is in once the list says nothing matches.
  await expect(page.locator('#list > .empty')).toContainText('Nothing matches')
  await expect(page.locator('#summary')).toHaveText('No conversation matches')
  await expect(groupTitle(page, 'Archived')).toHaveCount(0)
  await expect(row(page, 'Bump the Go version')).toHaveCount(0)
  await page.keyboard.press('Enter')
  await expect(pickBanner(page)).toBeVisible()

  await page.getByLabel('Search every conversation').fill('login')
  await expect(page.getByRole('option')).toHaveCount(1)
  await expect(row(page, 'Fix the login redirect')).toBeVisible()
  await expect(page.locator('#summary')).toHaveText('1 conversation matches')
  const calls = await calledPaths(page)
  expect(calls).not.toContain('/api/send')
  expect(calls).not.toContain('/api/open')
})

/** Stands in for the send shortcut, pressed in another app. */
async function pressSendShortcut(page: Page) {
  expect((await page.request.post('/dev/send-shortcut')).ok()).toBe(true)
}

test('the send shortcut sends straight to the one row the text can be typed into, then hides the list', async ({ page }) => {
  await openPanel(page, 'one-terminal')
  const copied = await (await page.request.get('/api/clipboard')).json()
  const sent = page.waitForRequest((request) => request.url().endsWith('/api/send'))

  await pressSendShortcut(page)

  expect((await sent).postDataJSON()).toEqual({ tool: 'claude', id: 'active-2', hash: copied.hash })
  await expect(pickBanner(page)).toBeHidden()
  const calls = await calledPaths(page)
  expect(calls).toContain('/api/panel/hide')
  expect(calls).not.toContain('/api/open')
  expect(calls).not.toContain('/api/panel/show')
})

test('the send shortcut sends a copied web page straight to the one row as Markdown', async ({ page }) => {
  await openPanel(page, 'one-terminal-markdown')
  await page.evaluate(() => {
    const seen = { named: false }
    ;(window as unknown as { seen: typeof seen }).seen = seen
    new MutationObserver(() => {
      if (document.getElementById('pick-title')?.textContent === 'Sending to Plan the sync dry run') seen.named = true
    }).observe(document.getElementById('card')!, { childList: true, subtree: true, characterData: true })
  })
  const copied = await (await page.request.get('/api/clipboard')).json()
  expect(copied.format).toBeUndefined()
  const sent = page.waitForResponse((response) => response.url().endsWith('/api/send'))

  await pressSendShortcut(page)

  const answer = await sent
  expect(answer.request().postDataJSON()).toEqual({ tool: 'claude', id: 'active-2', hash: copied.hash })
  expect((await answer.json()).message).toBe('Typed the Markdown into the Ghostty tab that runs it. Press Enter there to send it.')
  await expect(pickBanner(page)).toBeHidden()
  expect(await page.evaluate(() => (window as unknown as { seen: { named: boolean } }).seen.named)).toBe(true)
})

test('the send shortcut with no row to type into opens pick mode and sends nothing', async ({ page }) => {
  await openPanel(page)
  await page.getByLabel('Search every conversation').fill('login')
  await expect(page.getByRole('option')).toHaveCount(1)

  await pressSendShortcut(page)

  await expect(pickBanner(page)).toContainText('Click a chat or press Enter to paste these 14 words there')
  await expect(page.getByLabel('Search every conversation')).toHaveValue('')
  await expect(selected(page)).toContainText('Approve the database migration')
  await page.waitForTimeout(600)
  expect(await calledPaths(page)).not.toContain('/api/send')
})

test('the send shortcut with several rows to type into opens pick mode on the first and waits for Enter', async ({ page }) => {
  await openPanel(page, 'two-terminals')

  await pressSendShortcut(page)

  await expect(pickBanner(page)).toContainText('Click a chat or press Enter to paste these 14 words there')
  await expect(selected(page)).toContainText('Refactor the search index')
  await page.waitForTimeout(600)
  expect(await calledPaths(page)).not.toContain('/api/send')
  const sent = page.waitForRequest((request) => request.url().endsWith('/api/send'))
  await row(page, 'Plan the sync dry run').click()
  expect((await sent).postDataJSON()).toMatchObject({ tool: 'claude', id: 'active-2' })
})

test('the send shortcut with a clipboard that cannot be read says so and opens no pick mode', async ({ page }) => {
  await openPanel(page)
  await page.route('**/api/clipboard', (route) => route.abort())

  await pressSendShortcut(page)

  await expect(page.locator('#toast')).toHaveText('Could not read the copied text. The companion could not reach its own shell.')
  await expect(page.locator('#toast')).toHaveAttribute('role', 'alert')
  await expect(pickBanner(page)).toBeHidden()
  expect(await calledPaths(page)).not.toContain('/api/send')
})

test('the header has no clipboard button', async ({ page }) => {
  await openPanel(page)
  await expect(page.locator('.head button:not(.approvals-ask)')).toHaveCount(2)
  await expect(page.locator('.head').getByRole('button', { name: /copied text/i })).toHaveCount(0)
})

test('in pick mode Enter does nothing while a search is on its way, and its answer selects a row it offers', async ({ page }) => {
  await openPanel(page)
  await page.route('**/api/inbox?q=*', async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 800))
    await route.continue()
  })
  await pressSendShortcut(page)
  await expect(selected(page)).toContainText('Approve the database migration')

  await page.getByLabel('Search every conversation').fill('login')
  await expect(page.locator('#summary')).toHaveText('Searching…')
  await page.keyboard.press('Enter')
  await expect(row(page, 'Fix the login redirect')).toBeVisible()
  expect(await calledPaths(page)).not.toContain('/api/send')

  await expect(selected(page)).toContainText('Fix the login redirect')
  const sent = page.waitForRequest((request) => request.url().endsWith('/api/send'))
  await page.keyboard.press('Enter')
  expect((await sent).postDataJSON()).toMatchObject({ tool: 'cursor', id: 'active-1' })
})

test('the send shortcut does not send straight to a row in a folded group, and opens the picker instead', async ({ page }) => {
  await openPanel(page, 'one-terminal')
  await groupTitle(page, 'Active').click()
  await expect(row(page, 'Plan the sync dry run')).toHaveCount(0)

  await pressSendShortcut(page)

  await expect(pickBanner(page)).toContainText('Click a chat or press Enter to paste these 14 words there')
  await expect(row(page, 'Plan the sync dry run')).toHaveCount(0)
  await page.waitForTimeout(600)
  expect(await calledPaths(page)).not.toContain('/api/send')
})

test('a clipboard that cannot be read when a picked row sends says so plainly and sends nothing', async ({ page }) => {
  await openPanel(page)
  await pressSendShortcut(page)
  await expect(pickBanner(page)).toBeVisible()
  await page.route('**/api/clipboard', (route) => route.abort())

  await page.keyboard.press('Enter')

  await expect(page.locator('#toast')).toHaveText('Could not read the copied text. The companion could not reach its own shell.')
  expect(await calledPaths(page)).not.toContain('/api/send')
})

test('the send shortcut with nothing copied says so and opens no pick mode', async ({ page }) => {
  await openPanel(page, 'empty-clipboard')

  await pressSendShortcut(page)

  await expect(page.locator('#toast')).toHaveText('Nothing to send: copy some text first.')
  await expect(pickBanner(page)).toBeHidden()
  expect(await calledPaths(page)).not.toContain('/api/send')
})

test('Archived has no read state: a chat that closed unread shows no ring, no count and no Mark all as read, and can be deleted', async ({ page }) => {
  await openPanel(page, 'closed-unread')
  const resumed = row(page, 'Write the migration guide')
  await expect(resumed.locator('.state.has-unread')).toHaveCount(1)
  await expect(groupTitle(page, 'Active')).toContainText('1 unread')
  const archived = groupTitle(page, 'Archived')
  await expect(archived).toHaveAttribute('aria-expanded', 'false')
  await expect(archived).not.toContainText('unread')
  await archived.click()
  await expect(archived).not.toContainText('unread')
  await expect(page.locator('.group-head').filter({ hasText: 'Archived' }).getByRole('button', { name: 'Mark all as read' })).toHaveCount(0)
  const closed = row(page, 'Port the billing job')
  await expect(closed).not.toHaveAttribute('data-tooltip', /not read/)
  await expect(closed.locator('.state.has-unread')).toHaveCount(0)
  await closed.click({ button: 'right' })
  await expect(page.getByRole('menuitem', { name: 'Delete permanently…' })).toBeVisible()
  await expect(page.getByRole('menuitem', { name: 'Mark as read' })).toBeHidden()
})

test('a folded Needs you offers no Mark all as read, so no row nobody saw is marked', async ({ page }) => {
  await openPanel(page, 'needs-you')
  await expect(markAllRead(page)).toHaveCount(1)
  await groupTitle(page, 'Needs you').click()
  await expect(needsYou(page)).toHaveCount(0)
  await expect(markAllRead(page)).toHaveCount(0)

  await groupTitle(page, 'Needs you').click()
  await expect(markAllRead(page)).toHaveCount(1)
})

test('Enter opens nothing once the selected row was marked complete and folded away', async ({ page }) => {
  await openPanel(page)
  const target = row(page, 'Fix the login redirect')
  await target.hover()
  await expect(selected(page)).toContainText('Fix the login redirect')
  await completeButton(target).click()
  // The pointer leaves the list, so no other row is selected by hovering.
  await page.mouse.move(1, 1)
  await expect(target).toBeHidden()

  await page.keyboard.press('Enter')
  await page.keyboard.press('F2')
  await expect(page.getByLabel('New name')).toHaveCount(0)
  await page.waitForTimeout(300)
  const calls = await calledPaths(page)
  expect(calls).not.toContain('/api/panel/hide')
  expect(calls).not.toContain('/api/open')
})

test('mark all complete skips a row that got a reply after it was drawn', async ({ page }) => {
  await openPanel(page)
  const marks = markBodies(page)
  let first = true
  await page.route('**/api/mark*', async (route) => {
    if (first) {
      first = false
      // Plan the sync dry run gets a reply while the first mark is on its way.
      expect((await page.request.post('/dev/write', { data: { tool: 'claude', id: 'active-2' } })).ok()).toBe(true)
      await expect(needsYou(page).filter({ hasText: 'Plan the sync dry run' })).toBeVisible()
    }
    await route.continue()
  })
  await page.getByRole('button', { name: 'Mark all complete' }).click()

  await expect(groupTitle(page, 'Active')).toHaveCount(0)
  await expect(needsYou(page).filter({ hasText: 'Plan the sync dry run' })).toBeVisible()
  expect(marks).toEqual([
    { tool: 'cursor', id: 'active-1', as: 'completed' },
    { tool: 'claude', id: 'cloud-1', as: 'completed' },
  ])
})

test('a row marked complete keeps its tick until the list read after the mark arrives, then moves to Archived', async ({ page }) => {
  await openPanel(page, 'slow-reread')
  const target = row(page, 'Fix the login redirect')
  await target.hover()
  await completeButton(target).click()
  await expect(target.locator('.action.is-done')).toBeVisible()

  // The fixture answers the mark 2.5 seconds later; the tick alone lasts about half a second.
  await page.waitForTimeout(1_000)
  await expect(target.locator('.action.is-done')).toBeVisible()
  await expect(target).toBeHidden({ timeout: 3_000 })
  await groupTitle(page, 'Archived').click()
  await expect(target).toBeVisible()
})

test('a row marked complete stays where the list read after the mark shows it', async ({ page }) => {
  await openPanel(page, 'marks-ignored')
  const target = row(page, 'Fix the login redirect')
  // Drawn as the selected row, the list read after the mark matches what is on screen.
  await target.hover()
  await groupTitle(page, 'Archived').click()
  await groupTitle(page, 'Archived').click()
  await target.hover()
  await completeButton(target).click()
  await expect(target.locator('.action.is-done')).toBeVisible()

  await expect(target.locator('.action.is-done')).toHaveCount(0)
  // Archived starts folded, so a row moved there would be hidden.
  await expect(target).toBeVisible()
})

test('in pick mode a subagent row sends the copied text to its chat and opens nothing', async ({ page }) => {
  await openPanel(page, 'helpers')
  await page.keyboard.press('Meta+Shift+KeyV')
  await expect(pickBanner(page)).toBeVisible()
  await row(page, 'Rename chats').getByRole('button', { name: '▸ 7 subagents' }).click()

  const copied = await (await page.request.get('/api/clipboard')).json()
  const sent = page.waitForRequest((request) => request.url().endsWith('/api/send'))
  await page.locator('.helper-row').filter({ hasText: 'Review #3511' }).click()
  expect((await sent).postDataJSON()).toEqual({ tool: 'claude', id: 'helpers-1', hash: copied.hash })
  expect(await calledPaths(page)).not.toContain('/api/open')
})

test('Enter twice in a rename sends one rename', async ({ page }) => {
  await openPanel(page)
  const renames: unknown[] = []
  let release: () => void = () => {}
  const held = new Promise<void>((done) => { release = done })
  await page.route('**/api/rename', async (route) => {
    renames.push(route.request().postDataJSON())
    await held
    await route.continue()
  })
  await row(page, 'Plan the sync dry run').click({ button: 'right' })
  await page.getByRole('menuitem', { name: 'Rename…' }).click()
  const field = page.getByLabel('New name')
  await field.fill('Ship the dry run')
  await field.press('Enter')
  await expect.poll(() => renames.length).toBe(1)
  await field.press('Enter')
  release()

  await expect(row(page, 'Ship the dry run')).toBeVisible()
  expect(renames).toEqual([{ tool: 'claude', id: 'active-2', title: 'Ship the dry run' }])
})

test('a slow search finishes while new lists keep arriving', async ({ page }) => {
  await openPanel(page, 'slow-search')
  let arriving = true
  const arrivals = (async () => {
    while (arriving) {
      await page.request.post('/dev/arrive')
      await new Promise((done) => setTimeout(done, 400))
    }
  })()
  await page.getByLabel('Search every conversation').fill('release')
  // The fixture answers "release" after 1.5 seconds, while a new list arrives every 0.4 seconds.
  await expect(row(page, 'Draft the release notes')).toBeVisible({ timeout: 4_000 })
  arriving = false
  await arrivals
})

test('the search field names the selected row for a screen reader', async ({ page }) => {
  await openPanel(page)
  const search = page.getByLabel('Search every conversation')
  await expect(search).toHaveAttribute('aria-controls', 'list')
  await expect(search).not.toHaveAttribute('aria-activedescendant')
  await page.keyboard.press('ArrowDown')
  const id = await selected(page).getAttribute('id')
  expect(id).toBeTruthy()
  await expect(search).toHaveAttribute('aria-activedescendant', id!)
  await page.keyboard.press('ArrowDown')
  await expect(search).toHaveAttribute('aria-activedescendant', (await selected(page).getAttribute('id'))!)
})

test('the arrow keys move through the row menu in the order it shows', async ({ page }) => {
  await openArchived(page, 'archived-shared')
  await row(page, 'Ship the retry queue').click({ button: 'right' })
  await expect(page.getByRole('menuitem', { name: 'Rename…' })).toBeFocused()
  for (const name of ['Copy session ID', 'Message another session…', 'Claim files…', 'Undo complete', 'Hide from agnostic-ai', 'Open on claude.ai']) {
    await page.keyboard.press('ArrowDown')
    await expect(page.getByRole('menuitem', { name })).toBeFocused()
  }
  await page.keyboard.press('ArrowDown')
  await expect(deleteItem(page)).toBeFocused()
})

test('the arrow keys move through the row menu and wrap around', async ({ page }) => {
  await openPanel(page)
  await row(page, 'Approve the database migration').click({ button: 'right' })
  const read = page.getByRole('menuitem', { name: 'Mark as read' })
  const rename = page.getByRole('menuitem', { name: 'Rename…' })
  const complete = page.getByRole('menuitem', { name: 'Mark complete' })
  const stop = page.getByRole('menuitem', { name: 'Stop chat…' })
  await expect(read).toBeFocused()
  const copy = page.getByRole('menuitem', { name: 'Copy session ID' })
  const mailbox = page.getByRole('menuitem', { name: 'Message another session…' })
  const claim = page.getByRole('menuitem', { name: 'Claim files…' })
  for (const item of [rename, copy, mailbox, claim, complete, stop, read]) {
    await page.keyboard.press('ArrowDown')
    await expect(item).toBeFocused()
  }
  await page.keyboard.press('ArrowUp')
  await expect(stop).toBeFocused()
  await expect(selected(page)).toContainText('Approve the database migration')
})

const stopChatDialog = (page: Page) => page.getByRole('alertdialog', { name: /^Stop "/ })

async function askToStopChat(page: Page, title: string) {
  await row(page, title).click({ button: 'right' })
  await page.getByRole('menuitem', { name: 'Stop chat…' }).click()
  return stopChatDialog(page)
}

function stopsSent(page: Page) {
  const sent: unknown[] = []
  page.on('request', (request) => {
    if (new URL(request.url()).pathname === '/api/stop-session') sent.push(request.postDataJSON())
  })
  return sent
}

test('Stop chat shows only on a chat that runs on this computer, and asks first', async ({ page }) => {
  await openPanel(page)
  const sent = stopsSent(page)
  await row(page, 'Fix the login redirect').click({ button: 'right' })
  await expect(page.getByRole('menuitem', { name: 'Stop chat…' })).toBeHidden()
  await page.keyboard.press('Escape')

  const dialog = await askToStopChat(page, 'Approve the database migration')
  await expect(dialog).toContainText('Stop "Approve the database migration"?')
  await expect(dialog).toContainText('Shell commands it started may keep running')
  await expect(dialog).toContainText('The transcript stays')
  await expect(dialog.getByRole('button', { name: 'Cancel' })).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
  expect(sent).toEqual([])
})

test('confirming Stop chat ends the selected process and archives the chat for resuming', async ({ page }) => {
  await openPanel(page)
  const sent = stopsSent(page)
  const dialog = await askToStopChat(page, 'Approve the database migration')
  await dialog.getByRole('button', { name: 'Stop chat' }).click()

  await expect(page.getByRole('status').filter({ hasText: 'Stopped.' })).toBeVisible()
  await expect(dialog).toBeHidden()
  expect(sent).toEqual([{ tool: 'claude', id: 'waits-1', pid: 4007, procStart: 'Fri Nov 20 09:00:00 2026' }])
  const stopped = row(page, 'Approve the database migration')
  await expect(stopped).toBeHidden()
  await groupTitle(page, 'Archived').click()
  await expect(stopped).toHaveAttribute('data-tooltip', /^Resume in Terminal/)
  await stopped.click({ button: 'right' })
  await expect(page.getByRole('menuitem', { name: 'Stop chat…' })).toBeHidden()
})

test('Stop chat asks again when the chat moves to another process', async ({ page }) => {
  await openPanel(page)
  const sent = stopsSent(page)
  const title = 'Approve the database migration'
  const dialog = await askToStopChat(page, title)
  await expect(dialog).toBeVisible()
  await page.route('**/api/inbox', async (route) => {
    const response = await route.fetch()
    const json = await response.json() as { view: ConversationsView }
    const chat = json.view.conversations.find((conversation) => conversation.id === 'waits-1')
    if (chat?.stoppable) chat.stoppable.procStart = 'replacement process'
    await route.fulfill({ response, json })
  })
  expect((await page.request.post('/dev/fixture', { data: { name: 'mixed' } })).ok()).toBe(true)
  await expect(dialog).toBeHidden()
  expect(sent).toEqual([])
  await expect(await askToStopChat(page, title)).toBeVisible()
})

test('closing and reopening the panel clears the Stop chat question', async ({ page }) => {
  await openPanel(page)
  const dialog = await askToStopChat(page, 'Approve the database migration')
  await expect(dialog).toBeVisible()
  expect((await page.request.post('/api/panel/hide')).ok()).toBe(true)
  expect((await page.request.post('/api/panel/show')).ok()).toBe(true)
  await expect(dialog).toBeHidden()
  await expect(page.getByLabel('Search every conversation')).toBeFocused()
})

test('a late Stop chat reply leaves a second question open', async ({ page }) => {
  await openPanel(page)
  let release: () => void = () => {}
  const held = new Promise<void>((done) => { release = done })
  await page.route('**/api/stop-session', async (route) => {
    await held
    await route.continue()
  })
  try {
    const first = await askToStopChat(page, 'Approve the database migration')
    await first.getByRole('button', { name: 'Stop chat' }).click()
    await page.keyboard.press('Escape')
    const second = await askToStopChat(page, 'Refactor the search index')
    release()
    await expect(page.getByRole('status').filter({ hasText: 'Stopped.' })).toBeVisible()
    await expect(second).toHaveAccessibleName('Stop "Refactor the search index"?')
    await expect(second.getByRole('button', { name: 'Cancel' })).toBeFocused()
  } finally {
    release()
  }
})

test('a refused Stop chat says why', async ({ page }) => {
  await openPanel(page)
  await page.route('**/api/stop-session', (route) => route.fulfill({ status: 409, contentType: 'application/json', body: JSON.stringify({ error: 'this chat no longer runs in a terminal here' }) }))
  const dialog = await askToStopChat(page, 'Approve the database migration')
  await dialog.getByRole('button', { name: 'Stop chat' }).click()

  await expect(page.getByText('this chat no longer runs in a terminal here')).toBeVisible()
})

test('the delete question keeps Tab on its buttons and closes on a click outside it', async ({ page }) => {
  await openArchived(page)
  const sent = deletesSent(page)
  await row(page, 'Tidy the old branches').click({ button: 'right' })
  await deleteItem(page).click()
  const cancel = deleteDialog(page).getByRole('button', { name: 'Cancel' })
  const go = deleteDialog(page).getByRole('button', { name: 'Delete permanently' })
  await expect(cancel).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(go).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(cancel).toBeFocused()
  await page.keyboard.press('Shift+Tab')
  await expect(go).toBeFocused()

  await page.getByLabel('Search every conversation').click()
  await expect(deleteDialog(page)).toBeHidden()
  expect(sent).toEqual([])
})

test('a click on the circle when the shell cannot be reached shows the trouble state', async ({ page }) => {
  await openPanel(page)
  await page.goto('/index.html')
  await expect(page.locator('#bubble')).not.toHaveClass(/is-troubled/)
  await page.route('**/api/panel/toggle', (route) => route.abort())
  const errors: Error[] = []
  page.on('pageerror', (error) => errors.push(error))
  await page.locator('#bubble').click()

  await expect(page.locator('#bubble')).toHaveClass(/is-troubled/)
  await expect(page.locator('#bubble')).toHaveAttribute('aria-label', 'Conversations: The companion could not reach its own shell.')
  expect(errors).toEqual([])
})

test('a closed list keeps its search but asks for it again only when it opens', async ({ page }) => {
  await openPanel(page)
  const search = page.getByLabel('Search every conversation')
  await search.fill('login')
  await expect(page.getByRole('option')).toHaveCount(1)

  expect((await page.request.post('/api/panel/hide')).ok()).toBe(true)
  let searches = 0
  page.on('request', (request) => {
    if (request.url().includes('q=login')) searches += 1
  })
  for (let change = 0; change < 3; change += 1) {
    expect((await page.request.post('/dev/arrive')).ok()).toBe(true)
  }
  await page.waitForTimeout(500)
  expect(searches).toBe(0)

  const again = page.waitForRequest((request) => request.url().includes('q=login'))
  expect((await page.request.post('/api/panel/toggle')).ok()).toBe(true)
  await again
  await expect(search).toHaveValue('login')
  await expect(page.getByRole('option')).toHaveCount(1)
  expect(searches).toBe(1)
})

test('a search a new list made stale does not run again once the list closes', async ({ page }) => {
  await openPanel(page, 'slow-search')
  const search = page.getByLabel('Search every conversation')
  const asked = page.waitForRequest((request) => request.url().includes('q=release'))
  await search.fill('release')
  await asked
  let searches = 0
  page.on('request', (request) => {
    if (request.url().includes('q=release')) searches += 1
  })
  expect((await page.request.post('/dev/arrive')).ok()).toBe(true)
  expect((await page.request.post('/api/panel/hide')).ok()).toBe(true)
  // The fixture answers "release" 1.5 seconds after it was asked.
  await page.waitForTimeout(2_500)
  expect(searches).toBe(0)
})

test('a rename during a slow search is not undone by the search that was already on its way', async ({ page }) => {
  await openPanel(page, 'slow-search')
  await page.getByLabel('Search every conversation').fill('release')
  await expect(row(page, 'Draft the release notes')).toBeVisible({ timeout: 4_000 })
  await row(page, 'Draft the release notes').click({ button: 'right' })
  await page.getByRole('menuitem', { name: 'Rename…' }).click()
  await page.getByLabel('New name').fill('Draft the release plan')

  // A new list starts the search again; its answer holds the old name.
  const again = page.waitForRequest((request) => request.url().includes('q=release'))
  expect((await page.request.post('/dev/arrive')).ok()).toBe(true)
  await again
  await page.getByLabel('New name').press('Enter')
  await expect(row(page, 'Draft the release plan')).toBeVisible()
  // The fixture answers "release" 1.5 seconds after it was asked; the old name must not come back meanwhile.
  const titles = new Set<string>()
  for (const end = Date.now() + 3_500; Date.now() < end; await page.waitForTimeout(100)) {
    for (const title of await page.locator('#list .row .title').allTextContents()) titles.add(title)
  }
  expect([...titles]).toEqual(['Draft the release plan'])
  await expect(row(page, 'Draft the release plan')).toBeVisible()
})

test('a click on the circle that works after one that failed clears the trouble state', async ({ page }) => {
  await openPanel(page)
  await page.goto('/index.html')
  await page.route('**/api/panel/toggle', (route) => route.abort())
  await page.locator('#bubble').click()
  await expect(page.locator('#bubble')).toHaveClass(/is-troubled/)

  await page.unroute('**/api/panel/toggle')
  await page.locator('#bubble').click()
  await expect(page.locator('#bubble')).not.toHaveClass(/is-troubled/)
})

test('in pick mode a subagent row says it pastes into its chat', async ({ page }) => {
  await openPanel(page, 'helpers')
  const helper = page.locator('.helper-row').filter({ hasText: 'Review #3511' })
  await row(page, 'Rename chats').getByRole('button', { name: '▸ 7 subagents' }).click()
  await expect(helper).toHaveAttribute('title', 'Open this Codex thread')
  await page.keyboard.press('Meta+Shift+KeyV')
  await expect(pickBanner(page)).toBeVisible()
  await expect(helper).toHaveAttribute('title', 'Paste the copied text into the chat it runs in')
})

const mailboxMessage = (page: Page, text: string) => mailboxThread(page).locator('article.mailbox-message', { hasText: text })

test('a held message goes out only after approval', async ({ page }) => {
  await openPanel(page, 'mailbox-delivery')
  await expect(mailboxSummary(page)).toContainText('2 held for you')
  await mailboxSummary(page).click()
  const held = mailboxMessage(page, 'May I rebase the branch?')
  await expect(held).toContainText('Held for you')
  await expect(held.getByRole('button', { name: 'Copy message' })).toHaveCount(0)
  await expect(mailboxMessage(page, 'Did the parser tests pass?')).toContainText('Delivered')
  const approve = page.waitForRequest((request) => request.url().endsWith('/api/mailbox/approve'))
  await held.getByRole('button', { name: 'Approve' }).click()
  expect((await approve).postDataJSON()).toEqual({ thread: 'th_20261107_00000005', n: 4, action: 'approve' })
  await expect(held).toContainText('Waiting')
  await expect(held.getByRole('button', { name: 'Copy message' })).toBeVisible()
  await expect(mailboxSummary(page)).toContainText('1 held for you')
})

test('a message an agent sent shows as sent by the agent, with its stated model', async ({ page }) => {
  await openPanel(page, 'mailbox-agent')
  await mailboxSummary(page).click()
  const finding = mailboxMessage(page, 'The claims file shares the store lock.')
  await expect(finding).toContainText('Sent by the agent')
  await expect(finding).toContainText('Says it runs gpt-5.5')
})

test('an agent message is held until approved', async ({ page }) => {
  await openPanel(page, 'mailbox-agent')
  await expect(mailboxSummary(page)).toContainText('1 held for you')
  await mailboxSummary(page).click()
  const held = mailboxMessage(page, 'May I rename the claims helper?')
  await expect(held).toContainText('Sent by the agent')
  await expect(held).toContainText('Held for you')
  await expect(held.getByRole('button', { name: 'Copy message' })).toHaveCount(0)
  await held.getByRole('button', { name: 'Approve' }).click()
  await expect(held).toContainText('Waiting')
})

test('a handoff the agent accepted says so', async ({ page }) => {
  await openPanel(page, 'mailbox-agent')
  await mailboxSummary(page).click()
  const handoff = mailboxMessage(page, 'Take the claims tests.')
  await expect(handoff).toContainText('Accepted by the agent')
  await expect(handoff.getByRole('button', { name: 'Accept' })).toHaveCount(0)
})

test('reject shows the refusal', async ({ page }) => {
  await openPanel(page, 'mailbox-delivery')
  await mailboxSummary(page).click()
  const held = mailboxMessage(page, 'Should I drop the old fixtures?')
  await held.getByRole('button', { name: 'Reject' }).click()
  await expect(held).toContainText('Rejected')
  await expect(held.getByRole('button', { name: 'Approve' })).toHaveCount(0)
  await expect(mailboxThread(page).locator('.mailbox-refusal', { hasText: 'Refused (guard: rejected)' })).toHaveCount(2)
})

test('queue for delivery puts a staged message back', async ({ page }) => {
  await openPanel(page, 'mailbox-delivery')
  await mailboxSummary(page).click()
  const staged = mailboxMessage(page, 'Run them again with the race flag.')
  await expect(staged).toContainText('Staged')
  await staged.getByRole('button', { name: 'Queue for delivery' }).click()
  await expect(staged).toContainText('Waiting')
  await expect(staged.getByRole('button', { name: 'Queue for delivery' })).toHaveCount(0)
})

const tooltip = (page: Page) => page.locator('#tooltip')
const boxesOverlap = (a: { x: number, y: number, width: number, height: number }, b: { x: number, y: number, width: number, height: number }) =>
  a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height

test('two shells whose commands share a long start read apart, and the hover shows each whole command', async ({ page }) => {
  await openPanel(page, 'needs-reasons')
  const twins = row(page, 'Preview the reports page')
  const lines = twins.locator('.shell-text')
  await expect(lines).toHaveCount(2)
  const [first, second] = await lines.allTextContents()
  expect(first).not.toBe(second)
  expect(first).toMatch(/^44m npm run preview.*….*\/reports\/$/)
  expect(second).toMatch(/^32m npm run preview.*….*127\.0\.0\.1$/)
  await expect(row(page, 'Deploy the staging stack').locator('.shell-command')).toHaveText(['Command hidden · pid 14477', 'Command hidden · pid 38984'])

  await lines.nth(1).hover()
  await expect(tooltip(page)).toBeVisible()
  await expect(tooltip(page)).toContainText('--base /reports/ --host 127.0.0.1')
  await expect(tooltip(page)).toContainText(/Running for 32 min, since \d\d:\d\d/)
  await expect(tooltip(page)).toContainText('PID 38983 · 2 processes')
  await expect(tooltip(page)).toHaveCSS('pointer-events', 'none')
})

test('every row under Needs you says why, even one whose reply is withheld', async ({ page }) => {
  await openPanel(page, 'needs-reasons')
  const reasons = page.locator('.row.needs-you').locator('.needs-reason, .approval')
  await expect(page.locator('.row.needs-you')).toHaveCount(7)
  await expect(reasons).toHaveText([
    'Waiting on 2 shells still running',
    'Waiting on 2 shells still running',
    'Asks you a question',
    'No activity for 7m',
    'Waiting for you',
    'New reply, hidden because it may hold a password or key',
    'New reply',
  ])
  await openPanel(page, 'approvals')
  for (const needing of await page.locator('.row.needs-you').all()) {
    await expect(needing.locator('.needs-reason, .approval')).toHaveCount(1)
  }
})

test('the row tooltip names the hovered row, never covers it, and follows a redraw under a still pointer', async ({ page }) => {
  await openPanel(page, 'needs-reasons')
  const hovered = row(page, 'Pick a chart library')
  await hovered.locator('.title').hover()
  await expect(tooltip(page)).toBeVisible()
  await expect(tooltip(page)).toHaveText('Open in Terminal\n/Users/you/Code/agnostic-ai')
  expect(await tooltip(page).textContent()).toBe(await hovered.getAttribute('data-tooltip'))
  expect(boxesOverlap((await tooltip(page).boundingBox())!, (await hovered.boundingBox())!)).toBe(false)

  const under = row(page, 'Rotate the API keys')
  await under.locator('.title').scrollIntoViewIfNeeded()
  const title = (await under.locator('.title').boundingBox())!
  const pointer = { x: title.x + 10, y: title.y + title.height / 2 }
  await page.mouse.move(pointer.x, pointer.y)
  await expect(tooltip(page)).toHaveText((await under.getAttribute('data-tooltip'))!)
  expect((await page.request.post('/dev/arrive')).ok()).toBe(true)
  await expect(row(page, 'Answer the review comments')).toBeVisible()
  // Another row now sits under the still pointer: the tooltip shows that row's text, or it closed.
  const now = await page.evaluate(({ x, y }) => {
    const tip = document.getElementById('tooltip')!
    const pointed = document.elementFromPoint(x, y)!.closest('.row')!
    const box = (element: Element) => {
      const { top, bottom } = element.getBoundingClientRect()
      return { top, bottom }
    }
    return { hidden: tip.hidden, text: tip.textContent, pointedTitle: pointed.querySelector('.title')!.textContent, pointedText: pointed.getAttribute('data-tooltip'), tip: box(tip), row: box(pointed) }
  }, pointer)
  expect(now.pointedTitle).toBe('Answer the review comments')
  expect(now.hidden).toBe(false)
  expect(now.text).toBe(now.pointedText)
  const gap = now.tip.bottom <= now.row.top ? now.row.top - now.tip.bottom : now.tip.top - now.row.bottom
  expect(gap).toBeGreaterThanOrEqual(0)
  expect(gap).toBeLessThan(10)
  await page.keyboard.press('Shift')
  await expect(tooltip(page)).toBeHidden()
  await page.mouse.move(pointer.x, pointer.y + 1)
  await expect(tooltip(page)).toBeVisible()
  await page.mouse.wheel(0, 40)
  await expect(tooltip(page)).toBeHidden()
})

test('a click on the circle opens the list on the first waiting row, past a search, a place and a folded group', async ({ page }) => {
  await openPanel(page, 'needs-reasons')
  await page.getByRole('button', { name: 'Cloud', exact: true }).click()
  await page.getByLabel('Search every conversation').fill('nothing matches this')
  await expect(page.locator('.row')).toHaveCount(0)
  expect((await page.request.post('/api/panel/hide')).ok()).toBe(true)

  expect((await page.request.post('/api/panel/toggle', { data: { jump: true } })).ok()).toBe(true)
  await expect(page.getByLabel('Search every conversation')).toHaveValue('')
  await expect(page.getByRole('button', { name: 'All', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(selected(page)).toContainText('Preview the reports page')
  await expect(selected(page)).toBeInViewport()

  await groupTitle(page, 'Needs you').click()
  await expect(page.locator('.row.needs-you')).toHaveCount(0)
  expect((await page.request.post('/api/panel/hide')).ok()).toBe(true)
  expect((await page.request.post('/api/panel/toggle', { data: { jump: true } })).ok()).toBe(true)
  await expect(selected(page)).toContainText('Preview the reports page')
})
