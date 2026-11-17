import { expect, test, type Locator, type Page } from '@playwright/test'

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

const row = (page: Page, title: string) => page.getByRole('option').filter({ hasText: title })
const selected = (page: Page) => page.locator('.row[aria-selected="true"]')
/** The --alert token, as a computed style spells it. */
const ALERT_RED = /rgb\(255, 98, 80\)/

test('search, select and open names the chosen conversation', async ({ page }) => {
  await openPanel(page)
  await page.getByLabel('Search every conversation').fill('login')
  await expect(page.getByRole('option')).toHaveCount(1)
  await expect(row(page, 'Fix the login redirect')).toContainText('Cursor · shop · 3h')

  await page.keyboard.press('ArrowDown')
  await expect(selected(page)).toContainText('Fix the login redirect')
  const opened = page.waitForRequest((request) => request.url().endsWith('/api/open'))
  await page.keyboard.press('Enter')
  expect((await opened).postDataJSON()).toEqual({ tool: 'cursor', id: 'active-1' })
})

test('a row shows the agent\'s latest reply under its title, and a row without one shows none', async ({ page }) => {
  await openPanel(page)
  const waiting = row(page, 'Approve the database migration')
  await expect(waiting.locator('.text > *')).toHaveClass(['title', 'reply', /meta/])
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

test('a click on the name still opens the chat', async ({ page }) => {
  await openPanel(page)
  const opened = page.waitForRequest((request) => request.url().endsWith('/api/open'))
  await row(page, 'Plan the sync dry run').locator('.title').click()
  expect((await opened).postDataJSON()).toEqual({ tool: 'claude', id: 'active-2' })
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
  await expect(groupTitle(page, 'Active')).toBeVisible()
  await page.getByRole('button', { name: 'Mark all complete' }).click()

  await expect(groupTitle(page, 'Active')).toHaveCount(0)
  expect(ticks).toHaveLength(0)
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
      if (request.url().endsWith('/api/mark')) marks.push(request.postDataJSON())
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
    if (request.url().endsWith('/api/mark')) marks.push(request.postDataJSON())
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
  await page.route('**/api/mark', async (route) => {
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
  await page.route('**/api/mark', async (route) => {
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
  await page.route('**/api/mark', async (route) => {
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

  await expect(page.locator('#list')).not.toContainText('New reply')
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
  expect((await opened).postDataJSON()).toEqual({ tool: 'claude', id: 'work-1' })
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
  await expect(row(page, 'Add rate limits')).toHaveAttribute('title', 'Open in Terminal\n/Users/you/Code/shop/api')
  await expect(row(page, 'Add rate limits')).toHaveAccessibleDescription('Open in Terminal /Users/you/Code/shop/api')
})

test('a folder name no other row shares stays short', async ({ page }) => {
  await openPanel(page)
  await expect(row(page, 'Fix the login redirect')).toContainText('Cursor · shop · 3h')
})

test('the circle counts the conversations that need you, and names the waiting and unread ones apart', async ({ page }) => {
  await page.request.post('/dev/fixture', { data: { name: 'mixed' } })
  await page.goto('/')
  await expect(page.locator('#badge')).toHaveText('2')
  await expect(page.getByRole('button', { name: 'Conversations: 1 waiting for you, 1 unread' })).toBeVisible()
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

test('under Needs you, permission waits come before a question', async ({ page }) => {
  await openPanel(page, 'approvals')
  await expect(page.locator('.row.needs-you .title')).toHaveText(['Refactor PRs', 'Webhooks v2 dashboard', 'Rider lifecycle gaps in Notion', 'Release notes draft', 'SFTP write: discussion'])
})

const deleteItem = (page: Page) => page.getByRole('menuitem', { name: 'Delete permanently…' })
const deleteDialog = (page: Page) => page.getByRole('alertdialog')

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
  await expect(shared.locator('.meta')).toHaveAttribute('title', 'Also open on claude.ai')
  await expect(shared).toHaveAccessibleDescription(/Also open on claude\.ai/)

  for (const place of ['Local', 'Cloud']) {
    await page.getByRole('button', { name: place, exact: true }).click()
    await expect(shared).toHaveCount(1)
  }
  await page.getByRole('button', { name: 'All', exact: true }).click()

  const web = page.waitForRequest((request) => request.url().endsWith('/api/open'))
  await shared.hover()
  await shared.getByRole('button', { name: 'Open on claude.ai' }).click()
  expect((await web).postDataJSON()).toEqual({ tool: 'claude', id: 'cse_rc1' })

  await page.goto('/panel.html')
  const local = page.waitForRequest((request) => request.url().endsWith('/api/open'))
  await row(page, 'Review the payment retries').click({ position: { x: 40, y: 10 } })
  expect((await local).postDataJSON()).toEqual({ tool: 'claude', id: 'rc-1' })
})

test('the row menu of a shared chat opens its claude.ai page', async ({ page }) => {
  await openPanel(page, 'shared')
  await row(page, 'Review the payment retries').click({ button: 'right' })
  const web = page.waitForRequest((request) => request.url().endsWith('/api/open'))
  await page.getByRole('menuitem', { name: 'Open on claude.ai' }).click()
  expect((await web).postDataJSON()).toEqual({ tool: 'claude', id: 'cse_rc1' })
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
  await expect(helpers.locator('.title')).toHaveText(['Find rename call sites', 'Size budget check', 'Review #3511: local app chats', 'Review #3512: rename in each app', 'Helper rows design'])
  await expect(helpers.locator('.meta')).toHaveText(['Claude Code · Explore · 1m', 'Claude Code · 1m', 'Codex · 2m', 'Codex · 4m', 'Claude Code · Plan · 6m'])
  await expect(helpers.locator('.state.is-working > .work-system')).toHaveCount(5)
  // Subagents never become rows of their own.
  await expect(page.getByRole('option')).toHaveCount(2)

  await page.getByRole('button', { name: '+2 more' }).click()
  await expect(helpers).toHaveCount(7)
  await expect(helpers.last().locator('.title')).toHaveText('Untitled subagent')

  const codex = page.waitForRequest((request) => request.url().endsWith('/api/open'))
  await helpers.filter({ hasText: 'Review #3511' }).click()
  expect((await codex).postDataJSON()).toEqual({ tool: 'codex', id: 'run-3511' })
  await page.goto('/panel.html')
  await parent.getByRole('button', { name: '▸ 7 subagents' }).click()
  const claude = page.waitForRequest((request) => request.url().endsWith('/api/open'))
  await helpers.filter({ hasText: 'Find rename call sites' }).click()
  expect((await claude).postDataJSON()).toEqual({ tool: 'claude', id: 'helpers-1' })
})

test('a pending approval takes the reply line\'s place, and the reply moves to the tooltip', async ({ page }) => {
  await openPanel(page, 'approvals')
  const bash = row(page, 'Refactor PRs')
  await expect(bash.locator('.text > *')).toHaveClass(['title', 'reply is-approval', 'meta'])
  await expect(bash.locator('.approval')).toHaveText('Approve git push origin…')
  await expect(bash.locator('.approval-code')).toHaveText('git push origin…')
  await expect(bash.locator('.approval svg')).toHaveCount(1)
  await expect(bash).not.toContainText('The seed fix')
  await expect(bash).toHaveAttribute('title', 'Open in Terminal\n/Users/you/Code/shop\nThe seed fix and the flag rename are still running.')
  await expect(bash.locator('.meta')).toHaveText('Claude Code · shop · 2m')
  await expect(bash.locator('.approval')).toHaveCSS('color', await page.locator('.group-title').first().evaluate((element) => getComputedStyle(element).color))

  await expect(row(page, 'Webhooks v2 dashboard').locator('.approval')).toHaveText('Approve edit dashboard.tsx')
  // An MCP tool reads as words, and a connector registered by id never shows the id.
  const mcp = row(page, 'Rider lifecycle gaps in Notion')
  await expect(mcp.locator('.approval')).toHaveText('Approve Notion fetch')
  await expect(mcp.locator('.approval-code')).toHaveCount(0)
  await expect(mcp).not.toContainText('bfe9f18c')
  // A long detail stays inside the row, cut with an ellipsis, and reads whole on hover.
  await expect(mcp.locator('.approval')).toHaveAttribute('title', 'Approve Notion fetch')
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
  await expect(question.locator('.text > *')).toHaveClass(['title', 'reply', 'meta', 'subagents'])
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
  expect(opens).toEqual([{ tool: 'codex', id: 'run-3511' }])
})

const pickBanner = (page: Page) => page.getByRole('region', { name: 'Send the copied text' })

test('a copied web page does not promise Markdown in the banner, since the server decides', async ({ page }) => {
  await openPanel(page, 'markdown-clipboard')
  await page.keyboard.press('Meta+Shift+KeyV')

  await expect(pickBanner(page)).toContainText('Send 12 words to…')
  await expect(pickBanner(page)).not.toContainText('Markdown')
  await expect(pickBanner(page)).toContainText('Release notes')
})

test('Cmd+Shift+V picks a conversation for the copied text, and Enter sends it there', async ({ page }) => {
  await openPanel(page)
  await page.keyboard.press('Meta+Shift+KeyV')

  await expect(pickBanner(page)).toContainText('Send 14 words to…')
  await expect(pickBanner(page)).toContainText('Fix the flaky login test.')
  await expect(page.getByRole('button', { name: 'Send the copied text' })).toHaveAttribute('aria-pressed', 'true')
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
  await page.getByRole('button', { name: 'Send the copied text' }).click()
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
    await page.getByRole('button', { name: 'Send the copied text' }).click()
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
  await expect(pickBanner(page)).toContainText('Send 14 words to…')

  expect((await page.request.post('/dev/clipboard', { data: { text: 'Two words' } })).ok()).toBe(true)
  await page.keyboard.press('Enter')

  await expect(page.locator('#notice')).toContainText('The copied text changed. Check the new text, then send again.')
  await expectErrorNotice(page, 'Why nothing was sent')
  await expect(pickBanner(page)).toContainText('Send 2 words to…')
  expect(await calledPaths(page)).not.toContain('/api/send')
})

test('a search in pick mode shows a matching archived chat dimmed, and it cannot be picked', async ({ page }) => {
  await openPanel(page)
  await page.keyboard.press('Meta+Shift+KeyV')
  await page.getByLabel('Search every conversation').fill('go version')

  const archived = row(page, 'Bump the Go version')
  await expect(archived).toHaveAttribute('aria-disabled', 'true')
  await expect(archived).toHaveCSS('opacity', '0.4')
  await archived.click({ force: true })
  await page.keyboard.press('Enter')

  await expect(pickBanner(page)).toBeVisible()
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

  await expect(pickBanner(page)).toContainText('Send 14 words to…')
  await expect(page.getByLabel('Search every conversation')).toHaveValue('')
  await expect(selected(page)).toContainText('Approve the database migration')
  await page.waitForTimeout(600)
  expect(await calledPaths(page)).not.toContain('/api/send')
})

test('the send shortcut with several rows to type into opens pick mode on the first and waits for Enter', async ({ page }) => {
  await openPanel(page, 'two-terminals')

  await pressSendShortcut(page)

  await expect(pickBanner(page)).toContainText('Send 14 words to…')
  await expect(selected(page)).toContainText('Refactor the search index')
  await page.waitForTimeout(600)
  expect(await calledPaths(page)).not.toContain('/api/send')
  const sent = page.waitForRequest((request) => request.url().endsWith('/api/send'))
  await row(page, 'Plan the sync dry run').click()
  expect((await sent).postDataJSON()).toMatchObject({ tool: 'claude', id: 'active-2' })
})

test('the send shortcut with nothing copied says so and opens no pick mode', async ({ page }) => {
  await openPanel(page, 'empty-clipboard')

  await pressSendShortcut(page)

  await expect(page.locator('#toast')).toHaveText('Nothing to send: copy some text first.')
  await expect(pickBanner(page)).toBeHidden()
  expect(await calledPaths(page)).not.toContain('/api/send')
})

test('a folded Archived names its chat that closed unread, and Mark all as read, shown once unfolded, clears the marker', async ({ page }) => {
  await openPanel(page, 'closed-unread')
  const archived = groupTitle(page, 'Archived')
  await expect(archived).toHaveAttribute('aria-expanded', 'false')
  await expect(archived).toContainText('1 unread')
  await expect(page.locator('.group-head').filter({ hasText: 'Archived' }).getByRole('button', { name: 'Mark all as read' })).toHaveCount(0)
  await archived.click()
  const closed = row(page, 'Port the billing job')
  await expect(closed).toHaveAttribute('title', /Closed with a reply you have not read/)
  await expect(closed.locator('.state.has-unread')).toHaveCount(1)
  await closed.click({ button: 'right' })
  await expect(page.getByRole('menuitem', { name: 'Delete permanently…' })).toBeHidden()
  await page.keyboard.press('Escape')
  await page.locator('.group-head').filter({ hasText: 'Archived' }).getByRole('button', { name: 'Mark all as read' }).click()
  await expect(groupTitle(page, 'Archived')).not.toContainText('unread')
})
