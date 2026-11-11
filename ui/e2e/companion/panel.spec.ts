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

  await expect(page.getByRole('status').filter({ hasText: 'agnostic-ai could not save the mark' })).toBeVisible()
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

async function openFailing(page: Page, fixture: string) {
  await openPanel(page, fixture)
  await row(page, 'Plan the sync dry run').click()
  await expect(notice(page)).toBeVisible()
}

test('a failed open keeps its reason on screen, with no copy button when the server sent no command', async ({ page }) => {
  await openFailing(page, 'open-failure')
  await expect(notice(page)).toContainText('Codex runs in an app that cannot be brought forward. Open it from the Codex app.')
  await expect(notice(page).getByRole('button', { name: 'Copy resume command' })).toHaveCount(0)
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

  await expect(page.getByRole('status').filter({ hasText: 'agnostic-ai could not save the mark' })).toBeVisible()
  await expect(needsYou(page).filter({ hasText: 'Draft the release notes' })).toBeVisible()
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

test('the circle counts the conversations that need you', async ({ page }) => {
  await page.request.post('/dev/fixture', { data: { name: 'mixed' } })
  await page.goto('/')
  await expect(page.locator('#badge')).toHaveText('2')
  await expect(page.getByRole('button', { name: 'Conversations: 2 need you' })).toBeVisible()
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

test('a refused delete keeps the row and its reason stays on screen', async ({ page }) => {
  await openArchived(page, 'delete-failure')
  await row(page, 'Tidy the old branches').click({ button: 'right' })
  await deleteItem(page).click()
  await deleteDialog(page).getByRole('button', { name: 'Delete permanently' }).click()

  const reason = page.getByRole('region', { name: 'Why it was not deleted' })
  await expect(reason).toContainText('Deleted 1 of 4 locations, then stopped: permission denied. Still on disk:')
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
  await expect(row(page, 'Bump the Go version')).toBeVisible()
  expect(sent).toEqual([])
})
