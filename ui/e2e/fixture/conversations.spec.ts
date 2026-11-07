import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { saveScreenshot } from '../helpers'

const overview = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))
const conversations = JSON.parse(readFileSync(new URL('../../src/fixtures/conversations.mock.json', import.meta.url), 'utf8'))

// The fixture's rows fall on Open now, Today, Yesterday, This week, and
// Earlier for this time in Berlin.
const NOW = new Date('2026-11-07T15:30:00Z')
test.use({ timezoneId: 'Europe/Berlin' })

interface Mocks {
  views: string[]
  opens: { body: unknown; contentType: string | undefined }[]
}

type Answer = { status?: number; json: unknown } | Promise<{ status?: number; json: unknown }>
type Reply = (body: unknown) => Answer
/** Answers GET /api/views/conversations; `before` is null for the first page. */
type Week = (before: string | null) => Answer

const opened: Reply = () => ({ json: { opened: true, message: 'Switched to the Terminal tab that runs it.' } })

// Every /api request is answered here, so no test reaches a real server
// or opens a real terminal. Routes added later win over the catch-all.
async function mockServer(page: Page, reply: Reply = opened, week: Week = () => ({ json: conversations })): Promise<Mocks> {
  const mocks: Mocks = { views: [], opens: [] }
  await page.clock.setFixedTime(NOW)
  await page.route('**/api/**', (route) => route.fulfill({ status: 404, json: { error: 'Not mocked in this test.' } }))
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/Users/you/Products/agnostic-ai', global: false, ui: true } }))
  await page.route('**/api/views/conversations**', async (route) => {
    const url = new URL(route.request().url())
    mocks.views.push(url.search)
    const answer = await week(url.searchParams.get('before'))
    return route.fulfill({ status: answer.status ?? 200, json: answer.json })
  })
  await page.route('**/api/conversations/open', async (route) => {
    const request = route.request()
    const body = request.postDataJSON()
    mocks.opens.push({ body, contentType: request.headers()['content-type'] })
    const answer = await reply(body)
    return route.fulfill({ status: answer.status ?? 200, json: answer.json })
  })
  return mocks
}

const row = (page: Page, title: string) => page.locator('li.conversation', { has: page.getByRole('heading', { level: 3, name: title, exact: true }) })
const chips = (page: Page) => page.getByRole('list', { name: 'Sources' }).getByRole('button')
const chip = (page: Page, label: string) => chips(page).filter({ hasText: label })

test.describe('conversations', () => {
  test('opens at the root address, first in the navigation, with open chats above the days', async ({ page }) => {
    const mocks = await mockServer(page)
    await page.goto('/')
    await expect(page.getByRole('heading', { level: 1, name: 'Conversations', exact: true })).toBeVisible()
    await expect(page.locator('.page-verdict')).toHaveText('3 chats are open now; 1 waits for you.')
    await expect(page.locator('.page-facts')).toHaveText('9 conversations. Pick up a chat where you left it, in the tool where it lives.')
    const nav = page.getByRole('navigation', { name: 'Configuration sections' })
    const conversationsLink = nav.getByRole('link').first()
    await expect(conversationsLink).toHaveAccessibleName(/^Conversations/)
    await expect(conversationsLink).toHaveAttribute('aria-current', 'page')
    await expect(conversationsLink).toHaveAttribute('href', '/')
    await expect(nav.getByRole('link', { name: /^Context/ })).toHaveAttribute('href', '?view=context')

    await expect(chips(page)).toHaveText(['Claude Code5 of 37', 'Codex2 of 14', 'Cursor2 of 5'])
    for (const button of await chips(page).all()) await expect(button).toHaveAttribute('aria-pressed', 'false')

    await expect(page.getByRole('heading', { level: 2 })).toHaveText(['Open now 3', 'Today 2', 'Yesterday 2', 'This week 2'])
    await expect(page.locator('li.conversation')).toHaveCount(9)
    await expect(page.locator('.section-search-status')).toBeEmpty()
    await expect(page.getByText('Search covers the weeks loaded so far.')).toBeVisible()
    await expect(page.getByText("That's every conversation.")).toBeVisible()
    await expect(page.getByRole('button', { name: 'Load the week before' })).toHaveCount(0)
    const open = page.getByRole('region', { name: /^Open now/ })
    await expect(open.getByRole('heading', { level: 3 })).toHaveText(['Add a Conversations page to the local UI', 'Plan the sync --global dry run', 'Tighten the nav labels on phones'])
    await expect(page.getByRole('region', { name: /^Today/ }).getByRole('heading', { level: 3 })).toHaveText(['Speed up the sqlite reader', 'Explain the sync --watch debounce'])

    const working = row(page, 'Add a Conversations page to the local UI')
    await expect(working.locator('.conversation-meta')).toHaveText('Claude CodeTerminalWorkingOpen now: the agent is workingagnostic-aimotechfr/conversations-tab6 min ago')
    await expect(working.locator('.conversation-live')).toHaveAttribute('title', 'Open now: the agent is working')
    await expect(working.locator('.conversation-folder')).toHaveAttribute('title', '/Users/you/Products/agnostic-ai')
    await expect(working.locator('.conversation-opening p')).toHaveCount(3)
    await expect(working.locator('.conversation-prompt')).toHaveText('Last prompt Run the e2e suite again and tell me which tests still fail.')
    await expect(working.locator('.conversation-tool')).toHaveText('Claude Code')
    await expect(working.getByRole('button', { name: 'Switch to its terminal: Add a Conversations page to the local UI' })).toBeVisible()
    await expect(working.getByRole('button', { name: 'Copy the command that resumes Add a Conversations page to the local UI' })).toBeVisible()

    const waiting = row(page, 'Plan the sync --global dry run').locator('.conversation-live')
    await expect(waiting).toHaveText('Waiting for youOpen now: the agent is waiting for you')
    await expect(waiting).toHaveClass(/badge-approval/)
    const plain = row(page, 'Tighten the nav labels on phones')
    await expect(plain.locator('.conversation-live')).toHaveText('Open nowOpen now')
    await expect(plain.locator('.conversation-tool')).toHaveText('Cursor')
    await expect(plain.locator('.conversation-withheld')).toHaveText('Opening Hidden: may hold a credential')

    await expect(row(page, 'Explain the sync --watch debounce').locator('.conversation-withheld')).toHaveText('Last prompt Hidden: may hold a credential')
    const desktop = page.locator('li.conversation', { hasText: 'Desktop app' })
    await expect(desktop.getByRole('heading', { level: 3 })).toHaveText('Untitled')
    await expect(desktop.locator('.conversation-withheld')).toHaveText('Title Hidden: may hold a credential')
    await expect(desktop.getByRole('button', { name: /^Copy/ })).toHaveCount(0)
    for (const app of ['Editor', 'Background']) await expect(page.locator('.conversation-meta', { hasText: app }).first()).toBeVisible()
    await expect(page.locator('.conversation-live')).toHaveCount(3)

    await nav.getByRole('link', { name: /^Context/ }).click()
    await expect(page).toHaveURL(/\?view=context$/)
    await expect(page.getByRole('heading', { level: 1, name: 'Context', exact: true })).toBeFocused()
    await conversationsLink.click()
    await expect(page).toHaveURL(/\/$/)
    await expect(page.getByRole('heading', { level: 1, name: 'Conversations', exact: true })).toBeFocused()
    expect(mocks.views).toEqual(['?editor=cursor', '?editor=cursor'])
  })

  test('filters by tool with the source chips and narrows with the search', async ({ page }) => {
    await mockServer(page)
    await page.goto('/')
    const status = page.locator('.section-search-status')
    const codex = chip(page, 'Codex')
    await codex.click()
    await expect(codex).toHaveAttribute('aria-pressed', 'true')
    await expect(page.locator('li.conversation')).toHaveCount(2)
    await expect(page.locator('li.conversation[data-tool="codex"]')).toHaveCount(2)
    await expect(page.getByRole('heading', { level: 2 })).toHaveText(['Today 1', 'This week 1'])
    await expect(status).toHaveText('2 of 9 conversations match.')

    const cursor = chip(page, 'Cursor')
    await cursor.click()
    await expect(cursor).toHaveAttribute('aria-pressed', 'true')
    await expect(codex).toHaveAttribute('aria-pressed', 'false')
    await expect(page.getByRole('heading', { level: 2 })).toHaveText(['Open now 1', 'This week 1'])
    await cursor.click()
    await expect(cursor).toHaveAttribute('aria-pressed', 'false')
    await expect(page.locator('li.conversation')).toHaveCount(9)
    await expect(status).toBeEmpty()

    const claude = chip(page, 'Claude Code')
    await claude.focus()
    await page.keyboard.press('Enter')
    await expect(claude).toHaveAttribute('aria-pressed', 'true')
    await expect(page.locator('li.conversation')).toHaveCount(5)
    await page.keyboard.press('Space')
    await expect(claude).toHaveAttribute('aria-pressed', 'false')
    await expect(claude).toBeFocused()

    const search = page.getByRole('searchbox', { name: 'Search conversations' })
    await search.fill('SQLITE')
    await expect(page.getByRole('heading', { level: 3 })).toHaveText(['Speed up the sqlite reader'])
    await expect(status).toHaveText('1 of 9 conversations match.')
    await expect(page).toHaveURL(/\?q=SQLITE$/)
    await search.fill('sync --global')
    await expect(page.getByRole('heading', { level: 2 })).toHaveText(['Open now 1'])
    await search.fill('website')
    await expect(page.getByRole('heading', { level: 3 })).toHaveText(['Explain the sync --watch debounce'])
    await search.fill('fix/ci-cache')
    await expect(page.getByRole('heading', { level: 3 })).toHaveText(['Summarize failing CI jobs'])
    await search.fill('immutable=1')
    await expect(page.getByRole('heading', { level: 3 })).toHaveText(['Speed up the sqlite reader'])
    await search.fill('Group the changes by section')
    await expect(page.getByRole('heading', { level: 3 })).toHaveText(['Untitled'])

    await page.getByRole('button', { name: 'Clear search', exact: true }).click()
    await expect(search).toBeFocused()
    await expect(page.locator('li.conversation')).toHaveCount(9)

    await search.fill('release')
    await expect(page.getByRole('heading', { level: 3 })).toHaveText(['Untitled', 'Nightly dependency audit', 'Draft the release notes for v0.92'])
    await claude.click()
    await expect(page.getByRole('heading', { level: 3 })).toHaveText(['Untitled', 'Nightly dependency audit'])
    await expect(status).toHaveText('2 of 9 conversations match.')
    await cursor.click()
    await expect(page.locator('li.conversation')).toHaveCount(0)
    await expect(page.getByText('No conversation matches.')).toBeVisible()
  })

  test('shows a source the server did not find as text with its note', async ({ page }) => {
    const missing = structuredClone(conversations)
    missing.tools[2] = { tool: 'cursor', label: 'Cursor', found: false, count: 0, shown: 0, note: "No Cursor chats in /Users/you/.cursor/chats or Cursor's state.vscdb." }
    missing.conversations = missing.conversations.filter((conversation: { tool: string }) => conversation.tool !== 'cursor')
    await mockServer(page, opened, () => ({ json: missing }))
    await page.goto('/')
    await expect(chips(page)).toHaveText(['Claude Code5 of 37', 'Codex2 of 14'])
    const cursor = page.getByRole('list', { name: 'Sources' }).getByRole('listitem').filter({ hasText: 'Cursor' })
    await expect(cursor).toHaveText("CursorNot foundNo Cursor chats in /Users/you/.cursor/chats or Cursor's state.vscdb.")
    await expect(cursor.getByRole('button')).toHaveCount(0)
    await expect(page.locator('li.conversation')).toHaveCount(7)
    await expect(page.locator('.section-search-status')).toBeEmpty()
    await expect(page.locator('.page-facts')).toHaveText(/^7 conversations\./)
  })

  test('asks the server to open a conversation and shows its reply', async ({ page }) => {
    let answer!: () => void
    const answered = new Promise<void>((resolve) => { answer = resolve })
    const mocks = await mockServer(page, async () => {
      await answered
      return { json: { opened: true, message: 'Switched to the Terminal tab that runs it.' } }
    })
    await page.goto('/?view=conversations')
    const live = row(page, 'Add a Conversations page to the local UI')
    const open = live.getByRole('button', { name: /^Switch to its terminal/ })
    await open.click()
    await expect(open).toBeDisabled()
    await expect(live.getByRole('status')).toHaveText('Opening…')
    await open.click({ force: true })
    answer()
    await expect(live.getByRole('status')).toHaveText('Switched to the Terminal tab that runs it.')
    await expect(open).toBeEnabled()
    await expect(open).toBeFocused()
    expect(mocks.opens).toEqual([{ body: { tool: 'claude', id: '6f1c2a4e-8b3d-4c1e-9a7f-2d5e8b1c4a90', editor: 'cursor' }, contentType: 'application/json' }])
  })

  test('offers the command to copy when the server cannot open it', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    const command = "cd '/Users/you/Products/agnostic-ai' && claude --resume e8a91c3f-2d47-4b6e-a0c5-71f2d9b3e864"
    const mocks = await mockServer(page, () => ({ json: { opened: false, message: 'The Claude app is not installed. Resume it in a terminal instead.', command } }))
    await page.addInitScript(() => window.localStorage.setItem('agnostic-ai.editor', 'zed'))
    await page.goto('/?view=conversations')
    const desktop = page.locator('li.conversation', { hasText: 'Desktop app' })
    await desktop.getByRole('button', { name: 'Open in Claude app: Untitled' }).click()
    await expect(desktop.getByRole('status')).toHaveText('The Claude app is not installed. Resume it in a terminal instead.')
    const copy = desktop.getByRole('button', { name: 'Copy the command the server suggests for Untitled' })
    await expect(copy).toHaveText('Copy command')
    await copy.click()
    await expect(desktop.getByRole('button', { name: 'Copied', exact: true })).toBeVisible()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(command)
    expect(mocks.opens.map((open) => open.body)).toEqual([{ tool: 'claude', id: 'e8a91c3f-2d47-4b6e-a0c5-71f2d9b3e864', editor: 'zed' }])
  })

  test('shows a failed request in the status line', async ({ page }) => {
    await mockServer(page, () => ({ status: 500, json: { error: 'scan failed' } }))
    await page.goto('/?view=conversations')
    const sqlite = row(page, 'Speed up the sqlite reader')
    await sqlite.getByRole('button', { name: /^Resume in Terminal/ }).click()
    await expect(sqlite.getByRole('status')).toHaveText('The server answered 500: scan failed')
    await expect(page.getByRole('heading', { level: 1, name: 'Conversations', exact: true })).toBeVisible()
  })

  test('says a static snapshot needs a running server', async ({ page }) => {
    await page.route('**/api/**', (route) => route.fulfill({ status: 404, json: { error: 'Not mocked in this test.' } }))
    await page.route('**/snapshot/overview.json', (route) => route.fulfill({ json: overview }))
    await page.goto('/?view=conversations&source=./snapshot')
    await expect(page.locator('.page-verdict')).toHaveText('Conversations need agnostic-ai running on your computer.')
    await expect(page.locator('.page-facts')).toHaveText('A static snapshot has no chats to list.')
    await expect(page.locator('li.conversation')).toHaveCount(0)
  })

  test('fits a phone screen in both themes, with a pressed chip that stands out', async ({ page }) => {
    await mockServer(page)
    await page.setViewportSize({ width: 390, height: 844 })
    for (const colorScheme of ['light', 'dark'] as const) {
      // Without motion the pressed style applies at once, so it can be read right after the click.
      await page.emulateMedia({ colorScheme, reducedMotion: 'reduce' })
      await page.goto('/')
      await expect(page.locator('li.conversation')).toHaveCount(9)
      const nav = page.getByRole('navigation', { name: 'Configuration sections' })
      for (const link of await nav.getByRole('link').all()) await expect(link).toBeInViewport({ ratio: 1 })
      await expect(nav.getByRole('link', { name: /^Conversations/ })).toHaveAttribute('aria-current', 'page')
      for (const control of [...await chips(page).all(), ...await page.locator('li.conversation .button').all()]) {
        const box = (await control.boundingBox())!
        expect(box.height).toBeGreaterThanOrEqual(44)
        expect(box.x + box.width).toBeLessThanOrEqual(390)
      }
      const codex = chip(page, 'Codex')
      const style = () => codex.evaluate((element) => { const css = getComputedStyle(element); return [css.borderColor, css.backgroundColor, css.color] })
      const idle = await style()
      await codex.click()
      await expect(codex).toHaveAttribute('aria-pressed', 'true')
      const pressed = await style()
      expect(pressed.every((value, index) => value !== idle[index])).toBe(true)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    }
  })
})

// The fixture is the first week; the week before holds two older rows and
// a later copy of one first-week row, and nothing older remains.
const NEXT = '2026-10-31T15:30:00Z'
const firstWeek = { ...structuredClone(conversations), next: NEXT }
const sqlite = conversations.conversations.find((conversation: { id: string }) => conversation.id === '0199f3a2-7c41-7d20-b8e5-3a9c1f0e6d27')
const weekBefore = {
  schemaVersion: 'conversations/v1',
  generatedAt: '2026-11-07T15:30:00Z',
  from: '2026-10-24T15:30:00Z',
  to: NEXT,
  tools: [
    { tool: 'claude', label: 'Claude Code', found: true, count: 37, shown: 1 },
    { tool: 'codex', label: 'Codex', found: true, count: 14, shown: 2 },
    { tool: 'cursor', label: 'Cursor', found: true, count: 5, shown: 0 },
  ],
  conversations: [
    { ...sqlite, title: 'Speed up the sqlite reader, measured' },
    {
      tool: 'codex', app: 'cli', id: '0199b0d4-6e21-7a58-9c03-2f7d1e8b4a65', title: 'Port the docs search to Pagefind',
      folder: '/Users/you/Products/website', branch: 'docs/pagefind', updatedAt: '2026-10-29T11:20:00Z',
      opening: ['Replace the docs search index with Pagefind.'], prompt: 'Keep the old index until the new one ships.',
      open: { kind: 'terminal', label: 'Resume in Terminal', command: "cd '/Users/you/Products/website' && codex resume 0199b0d4-6e21-7a58-9c03-2f7d1e8b4a65" },
    },
    {
      tool: 'claude', app: 'cli', id: '5a0e3f8c-9d21-4b7a-8c64-e2f1a7d09b38', title: '', updatedAt: '2026-10-27T19:34:00Z',
      prompt: 'What does the --global flag change for sync?',
      open: { kind: 'terminal', label: 'Resume in Terminal', command: 'claude --resume 5a0e3f8c-9d21-4b7a-8c64-e2f1a7d09b38' },
    },
  ],
}
const FIRST_REQUEST = '?editor=cursor'
const OLDER_REQUEST = `?editor=cursor&before=${encodeURIComponent(NEXT)}`

const weeks: Week = (before) => before === null ? { json: firstWeek } : { json: weekBefore }
const scrollToEnd = (page: Page) => page.evaluate(() => window.scrollTo({ top: document.scrollingElement!.scrollHeight }))

test.describe('conversations by week', () => {
  // Short enough that the end of the first week starts out of reach.
  test.use({ viewport: { width: 1280, height: 700 } })

  test('loads the week before as the list nears its end and keeps one row per conversation', async ({ page }) => {
    const mocks = await mockServer(page, opened, weeks)
    await page.goto('/')
    await expect(page.locator('li.conversation')).toHaveCount(9)
    await expect(page.getByRole('button', { name: 'Load the week before' })).toBeVisible()
    await expect(page.locator('.page-facts')).toHaveText(/^9 conversations loaded so far\./)
    expect(mocks.views).toEqual([FIRST_REQUEST])

    await scrollToEnd(page)
    await expect(page.locator('li.conversation')).toHaveCount(11)
    expect(mocks.views).toEqual([FIRST_REQUEST, OLDER_REQUEST])
    await expect(page.getByRole('heading', { level: 2 })).toHaveText(['Open now 3', 'Today 2', 'Yesterday 2', 'This week 2', 'Earlier 2'])
    await expect(page.getByRole('region', { name: /^Earlier/ }).getByRole('heading', { level: 3 })).toHaveText(['Port the docs search to Pagefind', 'Untitled'])
    await expect(row(page, 'Speed up the sqlite reader')).toHaveCount(0)
    await expect(page.getByRole('region', { name: /^Today/ }).getByRole('heading', { level: 3 })).toHaveText(['Speed up the sqlite reader, measured', 'Explain the sync --watch debounce'])
    await expect(chips(page)).toHaveText(['Claude Code6 of 37', 'Codex3 of 14', 'Cursor2 of 5'])
    await expect(page.locator('.page-facts')).toHaveText(/^11 conversations\./)
    await expect(page.getByText("That's every conversation.")).toBeVisible()
    await expect(page.getByRole('button', { name: 'Load the week before' })).toHaveCount(0)

    await scrollToEnd(page)
    expect(mocks.views).toHaveLength(2)
    await saveScreenshot(page, 'conversations-second-page.png')
  })

  test('loads the week before from the button by keyboard, one request at a time, under a tool filter', async ({ page }) => {
    let release!: () => void
    const released = new Promise<void>((resolve) => { release = resolve })
    const mocks = await mockServer(page, opened, async (before) => {
      if (before === null) return { json: firstWeek }
      await released
      return { json: weekBefore }
    })
    // Without IntersectionObserver the button is the only way to load more.
    await page.addInitScript(() => { Reflect.deleteProperty(window, 'IntersectionObserver') })
    await page.goto('/')
    await chip(page, 'Cursor').click()
    await expect(page.locator('.section-search-status')).toHaveText('2 of 9 conversations match.')
    await scrollToEnd(page)
    expect(mocks.views).toEqual([FIRST_REQUEST])

    // The label changes while it loads, so find the button by its place.
    const more = page.locator('.conversations-more').getByRole('button')
    await expect(more).toHaveText('Load the week before')
    await more.focus()
    await page.keyboard.press('Enter')
    await expect(more).toHaveText('Loading the week before…')
    await expect(more).toHaveAttribute('aria-busy', 'true')
    await expect(more).toBeDisabled()
    await page.keyboard.press('Enter')
    release()
    await expect(page.locator('.section-search-status')).toHaveText('2 of 11 conversations match.')
    await expect(page.getByText("That's every conversation.")).toBeFocused()
    expect(mocks.views).toEqual([FIRST_REQUEST, OLDER_REQUEST])
  })

  test('shows a week that failed to load inline and retries the same week', async ({ page }) => {
    let attempts = 0
    const mocks = await mockServer(page, opened, (before) => {
      if (before === null) return { json: firstWeek }
      attempts += 1
      return attempts === 1 ? { status: 500, json: { error: 'scan failed' } } : { json: weekBefore }
    })
    await page.goto('/')
    await expect(page.locator('li.conversation')).toHaveCount(9)
    await scrollToEnd(page)
    const failure = page.getByRole('alert').filter({ hasText: 'Could not load the week before.' })
    await expect(failure.getByText('Could not load the week before.')).toBeVisible()
    await failure.getByText('Details', { exact: true }).click()
    await expect(failure.locator('pre')).toHaveText(/answered 500: scan failed/)
    await expect(page.locator('li.conversation')).toHaveCount(9)
    await scrollToEnd(page)
    expect(mocks.views).toEqual([FIRST_REQUEST, OLDER_REQUEST])

    await page.getByRole('button', { name: 'Try again' }).click()
    await expect(page.locator('li.conversation')).toHaveCount(11)
    await expect(failure).toHaveCount(0)
    expect(mocks.views).toEqual([FIRST_REQUEST, OLDER_REQUEST, OLDER_REQUEST])
  })
})
