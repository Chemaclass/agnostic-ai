import { expect, test, type Page } from '@playwright/test'

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

test('a failed archive keeps the row and says why', async ({ page }) => {
  await openPanel(page, 'mark-failure')
  const target = row(page, 'Fix the login redirect')
  await target.hover()
  await target.getByRole('button', { name: /^Archive,/ }).click()

  await expect(page.getByRole('status').filter({ hasText: 'agnostic-ai could not save the mark' })).toBeVisible()
  await expect(target).toBeVisible()
})

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

test('folders with the same name show on each row', async ({ page }) => {
  await openPanel(page, 'duplicate-projects')
  await expect(page.locator('.row .meta').filter({ hasText: ' · api · ' })).toHaveCount(3)
})

test('the circle counts the conversations that need you', async ({ page }) => {
  await page.request.post('/dev/fixture', { data: { name: 'mixed' } })
  await page.goto('/')
  await expect(page.locator('#badge')).toHaveText('2')
  await expect(page.getByRole('button', { name: 'Conversations: 2 need you' })).toBeVisible()
})
