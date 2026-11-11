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
/** Answers GET /api/views/conversations; `before` is null for the first page, and params holds the whole query. */
type Week = (before: string | null, params: URLSearchParams) => Answer

const opened: Reply = () => ({ json: { opened: true, message: 'Switched to the Terminal tab that runs it.' } })

// Every /api request is answered here, so no test reaches a real server
// or opens a real terminal. Routes added later win over the catch-all.
const TOOL_LABELS: Record<string, string> = { claude: 'Claude Code', codex: 'Codex', cursor: 'Cursor' }

interface Row { tool: string; app: string; title?: string; folder?: string; branch?: string; prompt?: string; reply?: string }

// The server searches every conversation: every word of q in the row's
// title, folder, branch, prompt, reply, tool or app. The mock does the same to
// whatever page a test answers with, and counts the matches.
function asTheServerSearches(json: unknown, params: URLSearchParams): unknown {
  const view = json as { conversations?: Row[] }
  const words = (params.get('q') ?? '').toLowerCase().split(/\s+/).filter(Boolean)
  if (!Array.isArray(view?.conversations) || words.length === 0) return json
  // A typed shell command reads "! command" on the page, so that is what matches.
  const shown = (text?: string) => (text ?? '').replace(/^\s*<bash-input>([\s\S]*?)(?:<\/bash-input>\s*)?$/, '! $1')
  const rows = view.conversations.filter((c) => {
    const text = [shown(c.title), c.folder, c.branch, shown(c.prompt), c.reply, TOOL_LABELS[c.tool], c.app].join('\n').toLowerCase()
    return words.every((word) => text.includes(word))
  })
  return { ...view, conversations: rows, matches: rows.length }
}

async function mockServer(page: Page, reply: Reply = opened, week: Week = () => ({ json: conversations })): Promise<Mocks> {
  const mocks: Mocks = { views: [], opens: [] }
  await page.clock.setFixedTime(NOW)
  await page.route('**/api/**', (route) => route.fulfill({ status: 404, json: { error: 'Not mocked in this test.' } }))
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/Users/you/Products/agnostic-ai', global: false, ui: true } }))
  await page.route('**/api/views/conversations**', async (route) => {
    const url = new URL(route.request().url())
    mocks.views.push(url.search)
    const answer = await week(url.searchParams.get('before'), url.searchParams)
    return route.fulfill({ status: answer.status ?? 200, json: asTheServerSearches(answer.json, url.searchParams) })
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
const chips = (page: Page) => page.getByRole('group', { name: 'Filter by tool' }).getByRole('button')
const chip = (page: Page, label: string) => chips(page).filter({ hasText: label })

test.describe('conversations', () => {
  test('opens at the root address, right after Setup health in the navigation, with open chats above the days', async ({ page }) => {
    const mocks = await mockServer(page)
    await page.goto('/')
    await expect(page.getByRole('heading', { level: 1, name: 'Conversations', exact: true })).toBeVisible()
    await expect(page.locator('.page-verdict')).toHaveText('3 chats are open now; 1 waits for you.')
    await expect(page.locator('.page-facts .page-fact')).toHaveText(['9 conversations loaded', 'Pick up where you left off, in the tool where it lives.'])
    const nav = page.getByRole('navigation', { name: 'Configuration sections' })
    await expect(nav.getByRole('link').first()).toHaveAccessibleName(/^Setup health/)
    const conversationsLink = nav.getByRole('link').nth(1)
    await expect(conversationsLink).toHaveAccessibleName(/^Conversations/)
    await expect(conversationsLink).toHaveAttribute('aria-current', 'page')
    await expect(conversationsLink).toHaveAttribute('href', '/')
    await expect(nav.getByRole('link', { name: /^Context/ })).toHaveAttribute('href', '?view=context')

    await expect(chips(page)).toHaveText(['All 9', 'Claude Code 5', 'Codex 2', 'Cursor 2'])
    await expect(chip(page, 'All')).toHaveAttribute('aria-pressed', 'true')
    for (const button of await chips(page).all()) if (!(await button.textContent())?.startsWith('All')) await expect(button).toHaveAttribute('aria-pressed', 'false')
    await expect(chip(page, 'Claude Code')).toHaveAttribute('title', '5 of 37 Claude Code chats loaded.')

    await expect(page.getByRole('heading', { level: 2 })).toHaveText(['Open now 3', 'Today 2', 'Yesterday 2', 'This week 2'])
    await expect(page.locator('li.conversation')).toHaveCount(9)
    await expect(page.locator('.section-search-status')).toBeEmpty()
    await expect(page.getByRole('searchbox', { name: 'Search conversations' })).toHaveAttribute('title', 'Search covers every conversation, loaded or not.')
    await expect(page.getByRole('searchbox', { name: 'Search conversations' })).toHaveAccessibleDescription('Search covers every conversation, loaded or not.')
    await expect(page.getByText("That's every conversation.")).toBeVisible()
    await expect(page.getByRole('button', { name: 'Load the week before' })).toHaveCount(0)
    const open = page.getByRole('region', { name: /^Open now/ })
    await expect(open.getByRole('heading', { level: 3 })).toHaveText(['Add a Conversations page to the local UI', 'Tighten the nav labels on phones', 'Plan the sync --global dry run'])
    await expect(page.getByRole('region', { name: /^Today/ }).getByRole('heading', { level: 3 })).toHaveText(['Speed up the sqlite reader', 'Explain the sync --watch debounce'])

    const working = row(page, 'Add a Conversations page to the local UI')
    await expect(working.locator('.conversation-meta')).toHaveText('WorkingOpen now: the agent is workingClaude Code, Terminalagnostic-aimotechfr/conversations-tab6 min ago')
    await expect(working.locator('.conversation-live')).toHaveAttribute('title', 'Open now: the agent is working')
    await expect(working.locator('.conversation-folder')).toHaveAttribute('title', '/Users/you/Products/agnostic-ai')
    await expect(working.locator('.conversation-prompt')).toHaveText('Last prompt: Run the e2e suite again and tell me which tests still fail.')
    // The agent's latest reply sits right under the title, above the meta line.
    await expect(working.locator('.conversation-body > *')).toHaveClass([/conversation-title/, /conversation-reply/, /conversation-meta/, /conversation-prompt/])
    await expect(working.locator('.conversation-reply')).toHaveText('Latest reply: Two specs still fail: the empty state and the tool filter on phones.')
    await expect(row(page, 'Plan the sync --global dry run').locator('.conversation-reply')).toHaveText('Latest reply: Should the dry run list the files it would remove, or only the ones it writes?')
    await expect(row(page, 'Tighten the nav labels on phones').locator('.conversation-reply')).toHaveCount(0)
    await expect(row(page, 'Nightly dependency audit').locator('.conversation-reply')).toHaveCount(0)
    await expect(row(page, 'Nightly dependency audit').locator('.conversation-withheld')).toHaveText('Latest reply Hidden: may hold a credential')
    await expect(working.getByRole('button', { name: /^Open in Terminal/ })).toHaveClass(/button-primary/)
    await expect(working.locator('.conversation-tool')).toHaveText('Claude Code, Terminal')
    await expect(working.getByRole('button', { name: 'Open in Terminal: Add a Conversations page to the local UI' })).toBeVisible()
    await expect(working.getByRole('button', { name: 'Copy the command that resumes Add a Conversations page to the local UI' })).toHaveCount(0)

    const waiting = row(page, 'Plan the sync --global dry run').locator('.conversation-live')
    await expect(waiting).toHaveText('Waiting for youOpen now: the agent is waiting for you')
    await expect(waiting).toHaveClass(/badge-approval/)
    // The person's own marks show as chips too: a reply not yet read, and a chat they archived.
    const unread = row(page, 'Speed up the sqlite reader').locator('.conversation-state')
    await expect(unread).toHaveText('New replySomething was written since you last opened it')
    await expect(unread).toHaveClass(/badge-approval/)
    const done = row(page, 'Summarize failing CI jobs').locator('.conversation-state')
    await expect(done).toHaveText('ArchivedYou archived it; it comes back with a new message or when it starts working')
    await expect(done).toHaveClass(/badge-neutral/)
    await expect(row(page, 'Explain the sync --watch debounce').locator('.conversation-state')).toHaveCount(0)
    // An open chat whose tool does not say what it does shows no status chip: the group says it is open.
    const plain = row(page, 'Tighten the nav labels on phones')
    await expect(plain.locator('.conversation-live')).toHaveCount(0)
    await expect(plain.locator('.conversation-tool')).toHaveText('Cursor, Editor')
    await expect(plain.locator('.conversation-tool')).toHaveAttribute('title', 'Cursor, Editor')
    await expect(plain.locator('.conversation-withheld')).toHaveCount(0)

    await expect(row(page, 'Explain the sync --watch debounce').locator('.conversation-withheld')).toHaveText('Last prompt Hidden: may hold a credential')
    const desktop = page.locator('li.conversation', { has: page.locator('.conversation-tool[title="Claude Code, Desktop app"]') })
    await expect(desktop.getByRole('heading', { level: 3 })).toHaveText('Untitled')
    await expect(desktop.locator('.conversation-withheld')).toHaveText('Title Hidden: may hold a credential')
    await expect(desktop.getByRole('button', { name: /^Copy/ })).toHaveCount(0)
    for (const app of ['Editor', 'Background']) await expect(page.locator(`.conversation-tool[title$=", ${app}"]`).first()).toBeAttached()
    await expect(page.locator('.conversation-live')).toHaveCount(2)
    await expect(row(page, 'Speed up the sqlite reader').getByRole('button', { name: /^Open in/ })).not.toHaveClass(/button-primary/)

    await nav.getByRole('link', { name: /^Context/ }).click()
    await expect(page).toHaveURL(/\?view=context$/)
    await expect(page.getByRole('heading', { level: 1, name: 'Context', exact: true })).toBeFocused()
    await conversationsLink.click()
    await expect(page).toHaveURL(/\/$/)
    await expect(page.getByRole('heading', { level: 1, name: 'Conversations', exact: true })).toBeFocused()
    expect(mocks.views).toEqual(['?editor=cursor', '?editor=cursor'])
  })

  test('filters by tool with the source chips and narrows with the search', async ({ page }) => {
    const mocks = await mockServer(page)
    await page.goto('/')
    const status = page.locator('.section-search-status')
    const codex = chip(page, 'Codex')
    await codex.click()
    await expect(codex).toHaveAttribute('aria-pressed', 'true')
    await expect(page.locator('li.conversation')).toHaveCount(2)
    await expect(page.locator('li.conversation[data-tool="codex"]')).toHaveCount(2)
    await expect(page.getByRole('heading', { level: 2 })).toHaveText(['Today 1', 'This week 1'])
    await expect(status).toHaveText('2 of 9 conversations match.')
    await expect(chip(page, 'All')).toHaveAttribute('aria-pressed', 'false')

    const cursor = chip(page, 'Cursor')
    await cursor.click()
    await expect(cursor).toHaveAttribute('aria-pressed', 'true')
    await expect(codex).toHaveAttribute('aria-pressed', 'false')
    await expect(page.getByRole('heading', { level: 2 })).toHaveText(['Open now 1', 'This week 1'])
    await cursor.click()
    await expect(cursor).toHaveAttribute('aria-pressed', 'false')
    await expect(page.locator('li.conversation')).toHaveCount(9)
    await expect(status).toBeEmpty()
    await codex.click()
    await chip(page, 'All').click()
    await expect(chip(page, 'All')).toHaveAttribute('aria-pressed', 'true')
    await expect(page.locator('li.conversation')).toHaveCount(9)

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
    await expect(status).toHaveText('1 conversation matches.')
    await expect(page).toHaveURL(/\?q=SQLITE$/)
    // The search goes to the server, which searches every conversation, not just the loaded weeks.
    expect(mocks.views).toContain('?editor=cursor&q=SQLITE')
    await search.fill('sync --global')
    await expect(page.getByRole('heading', { level: 2 })).toHaveText(['Open now 1'])
    await search.fill('website')
    await expect(page.getByRole('heading', { level: 3 })).toHaveText(['Explain the sync --watch debounce'])
    await search.fill('fix/ci-cache')
    await expect(page.getByRole('heading', { level: 3 })).toHaveText(['Summarize failing CI jobs'])
    await search.fill('immutable=1')
    await expect(page.getByRole('heading', { level: 3 })).toHaveText(['Speed up the sqlite reader'])
    await search.fill('Move the Settings changes')
    await expect(page.getByRole('heading', { level: 3 })).toHaveText(['Untitled'])
    await search.fill('measured 7 ms')
    await expect(page.getByRole('heading', { level: 3 })).toHaveText(['Speed up the sqlite reader'])
    await search.fill('Group the changes by section')
    await expect(page.getByRole('heading', { level: 3 })).toHaveCount(0)

    await page.getByRole('button', { name: 'Clear search', exact: true }).click()
    await expect(search).toBeFocused()
    await expect(page.locator('li.conversation')).toHaveCount(9)

    await search.fill('release')
    await expect(page.getByRole('heading', { level: 3 })).toHaveText(['Nightly dependency audit', 'Draft the release notes for v0.92'])
    await claude.click()
    await expect(page.getByRole('heading', { level: 3 })).toHaveText(['Nightly dependency audit'])
    await expect(status).toHaveText('1 of 2 conversations match.')
    await cursor.click()
    await expect(page.locator('li.conversation')).toHaveCount(0)
    await expect(page.getByText('No conversation matches.')).toBeVisible()
  })

  test('leaves a tool the server did not find out of the filter', async ({ page }) => {
    const missing = structuredClone(conversations)
    missing.tools[2] = { tool: 'cursor', label: 'Cursor', found: false, count: 0, shown: 0, note: "No Cursor chats in /Users/you/.cursor/chats or Cursor's state.vscdb." }
    missing.conversations = missing.conversations.filter((conversation: { tool: string }) => conversation.tool !== 'cursor')
    await mockServer(page, opened, () => ({ json: missing }))
    await page.goto('/')
    await expect(chips(page)).toHaveText(['All 7', 'Claude Code 5', 'Codex 2'])
    await expect(page.locator('li.conversation')).toHaveCount(7)
    await expect(page.locator('.section-search-status')).toBeEmpty()
    await expect(page.locator('.page-facts')).toHaveText(/^7 conversations loaded/)
  })

  test('draws a conversation under the one it was started from, only while both sit in the same group', async ({ page }) => {
    await mockServer(page)
    await page.goto('/')
    const parent = row(page, 'Add a Conversations page to the local UI')
    const child = row(page, 'Tighten the nav labels on phones')
    await expect(page.locator('li.conversation.is-nested')).toHaveCount(1)
    await expect(child).toHaveClass(/is-nested/)
    await expect(child.locator('.sr-only').first()).toHaveText('Started from Add a Conversations page to the local UI')
    await expect(parent).not.toHaveClass(/is-nested/)
    // Started from the open parent, but on another day group; and started from a chat not in the list.
    await expect(row(page, 'Explain the sync --watch debounce')).not.toHaveClass(/is-nested/)
    await expect(row(page, 'Nightly dependency audit')).not.toHaveClass(/is-nested/)
    const indent = async (locator: typeof child) => (await locator.locator('.tool-avatar').boundingBox())!.x
    expect(await indent(child)).toBeGreaterThan(await indent(parent))
    const buttons = await child.getByRole('button').count()

    // Tab follows the drawn order: from the parent's link to the child's.
    await parent.getByRole('button', { name: /^Open in Terminal: / }).focus()
    await page.keyboard.press('Tab')
    await expect(child.getByRole('button', { name: 'Open in Cursor: Tighten the nav labels on phones' })).toBeFocused()

    await chip(page, 'Cursor').click()
    await expect(child).toBeVisible()
    await expect(page.locator('li.conversation.is-nested')).toHaveCount(0)
    await expect(child.getByRole('button')).toHaveCount(buttons)
    await chip(page, 'Cursor').click()

    await page.getByRole('searchbox', { name: 'Search conversations' }).fill('nav labels')
    await expect(page.getByRole('heading', { level: 3 })).toHaveText(['Tighten the nav labels on phones'])
    await expect(page.locator('li.conversation.is-nested')).toHaveCount(0)
  })

  test('shows a typed shell command as "! command" and hides a last prompt that repeats the title', async ({ page }) => {
    const shell = structuredClone(conversations)
    shell.conversations[3] = { ...shell.conversations[3], title: '<bash-input>gl</bash-input>', prompt: '! gl' }
    shell.conversations[4] = { ...shell.conversations[4], title: '/mcp', prompt: '/mcp', withheld: undefined }
    await mockServer(page, opened, () => ({ json: shell }))
    await page.goto('/')
    const typed = row(page, '! gl')
    await expect(typed.locator('.conversation-title code')).toHaveText('! gl')
    await expect(typed.locator('.conversation-prompt')).toHaveCount(0)
    await expect(row(page, '/mcp').locator('.conversation-prompt')).toHaveCount(0)
    await page.getByRole('searchbox', { name: 'Search conversations' }).fill('! gl')
    await expect(page.getByRole('heading', { level: 3 })).toHaveText(['! gl'])
    await page.getByRole('searchbox', { name: 'Search conversations' }).fill('bash-input')
    await expect(page.getByRole('heading', { level: 3 })).toHaveCount(0)
  })

  test('turns the row link into the ink pill and shows Copy command on hover and keyboard focus', async ({ page }) => {
    await mockServer(page)
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await page.goto('/')
    const sqlite = row(page, 'Speed up the sqlite reader')
    const open = sqlite.getByRole('button', { name: /^Open in Terminal/ })
    const copy = sqlite.getByRole('button', { name: /^Copy the command that resumes/ })
    const look = async () => [await sqlite.evaluate((element) => getComputedStyle(element).backgroundColor), await open.evaluate((element) => getComputedStyle(element).backgroundColor), await copy.evaluate((element) => getComputedStyle(element).opacity)]
    const [restRow, restOpen, restCopy] = await look()
    expect(restCopy).toBe('0')
    await sqlite.hover()
    const [hoverRow, hoverOpen, hoverCopy] = await look()
    expect(hoverRow).not.toBe(restRow)
    expect(hoverOpen).not.toBe(restOpen)
    expect(hoverCopy).toBe('1')
    await page.mouse.move(0, 0)
    await expect.poll(async () => (await look())[2]).toBe('0')
    await open.focus()
    await expect.poll(look).toEqual([hoverRow, hoverOpen, '1'])
    await page.keyboard.press('Tab')
    await expect(copy).toBeFocused()
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
    const open = live.getByRole('button', { name: /^Open in Terminal/ })
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

  test('hides the resume command of a chat it opened, since it may run now', async ({ page }) => {
    let answer!: () => void
    const answered = new Promise<void>((resolve) => { answer = resolve })
    const mocks = await mockServer(page, async () => {
      await answered
      return { json: { opened: true, message: 'Resumed it in a new Terminal tab.' } }
    })
    await page.goto('/')
    const sqlite = row(page, 'Speed up the sqlite reader')
    const copy = sqlite.getByRole('button', { name: /^Copy the command that resumes/ })
    const open = sqlite.getByRole('button', { name: /^Open in Terminal/ })
    await open.click()
    await page.keyboard.press('Tab')
    await expect(copy).toBeFocused()

    answer()
    await expect(sqlite.getByRole('status')).toHaveText('Resumed it in a new Terminal tab.')
    await expect(copy).toHaveCount(0)
    // The button it hid held the focus, so the focus moves to Open.
    await expect(open).toBeFocused()
    await expect(row(page, 'Summarize failing CI jobs').getByRole('button', { name: /^Copy the command that resumes/ })).toHaveCount(1)

    // A return to the page redraws the kept list; the chat is still not reported open, so it still offers none.
    const nav = page.getByRole('navigation', { name: 'Configuration sections' })
    await nav.getByRole('link', { name: /^Context/ }).click()
    await expect(page.getByRole('heading', { level: 1, name: 'Context', exact: true })).toBeFocused()
    await nav.getByRole('link', { name: /^Conversations/ }).click()
    await expect.poll(() => mocks.views.length).toBe(2)
    await expect(sqlite.getByRole('button', { name: /^Open in Terminal/ })).toBeVisible()
    await expect(copy).toHaveCount(0)
  })

  test('offers the command to copy when the server cannot open it', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    const command = "cd '/Users/you/Products/agnostic-ai' && claude --resume e8a91c3f-2d47-4b6e-a0c5-71f2d9b3e864"
    const mocks = await mockServer(page, () => ({ json: { opened: false, message: 'The Claude app is not installed. Resume it in a terminal instead.', command } }))
    await page.addInitScript(() => window.localStorage.setItem('agnostic-ai.editor', 'zed'))
    await page.goto('/?view=conversations')
    const desktop = page.locator('li.conversation', { has: page.locator('.conversation-tool[title="Claude Code, Desktop app"]') })
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
    await sqlite.getByRole('button', { name: /^Open in Terminal/ }).click()
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
      for (const control of [...await chips(page).all(), ...await page.locator('li.conversation .button:visible').all()]) {
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
      open: { kind: 'terminal', label: 'Open in Terminal', command: "cd '/Users/you/Products/website' && codex resume 0199b0d4-6e21-7a58-9c03-2f7d1e8b4a65" },
    },
    {
      tool: 'claude', app: 'cli', id: '5a0e3f8c-9d21-4b7a-8c64-e2f1a7d09b38', title: '', updatedAt: '2026-10-27T19:34:00Z',
      prompt: 'What does the --global flag change for sync?',
      open: { kind: 'terminal', label: 'Open in Terminal', command: 'claude --resume 5a0e3f8c-9d21-4b7a-8c64-e2f1a7d09b38' },
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
    await expect(page.locator('.page-facts')).toHaveText(/^9 conversations loaded so far/)
    expect(mocks.views).toEqual([FIRST_REQUEST])

    await scrollToEnd(page)
    await expect(page.locator('li.conversation')).toHaveCount(11)
    expect(mocks.views).toEqual([FIRST_REQUEST, OLDER_REQUEST])
    await expect(page.getByRole('heading', { level: 2 })).toHaveText(['Open now 3', 'Today 2', 'Yesterday 2', 'This week 2', 'Earlier 2'])
    await expect(page.getByRole('region', { name: /^Earlier/ }).getByRole('heading', { level: 3 })).toHaveText(['Port the docs search to Pagefind', 'Untitled'])
    await expect(row(page, 'Speed up the sqlite reader')).toHaveCount(0)
    await expect(page.getByRole('region', { name: /^Today/ }).getByRole('heading', { level: 3 })).toHaveText(['Speed up the sqlite reader, measured', 'Explain the sync --watch debounce'])
    await expect(chips(page)).toHaveText(['All 11', 'Claude Code 6', 'Codex 3', 'Cursor 2'])
    await expect(page.locator('.page-facts')).toHaveText(/^11 conversations loaded(?! so far)/)
    await expect(page.getByText("That's every conversation.")).toBeVisible()
    await expect(page.getByRole('button', { name: 'Load the week before' })).toHaveCount(0)

    await scrollToEnd(page)
    expect(mocks.views).toHaveLength(2)
    await saveScreenshot(page, 'conversations-second-page.png')
  })

  test('moves a conversation under its parent once the week that holds the parent loads', async ({ page }) => {
    const parentId = '1f0c9b8a-7d6e-4f5a-8b3c-2d1e0f9a8b7c'
    const childRow = {
      tool: 'codex', app: 'cli', id: '0199d2e3-f4a5-7b6c-8d9e-0f1a2b3c4d5e', parent: { tool: 'claude', id: parentId }, title: 'Write the changelog line',
      updatedAt: '2026-11-07T15:00:00Z', live: true, activity: 'working', open: { kind: 'focus', label: 'Open in Terminal' },
    }
    const parentRow = {
      tool: 'claude', app: 'cli', id: parentId, title: 'Ship the parser change',
      updatedAt: '2026-10-28T09:00:00Z', live: true, open: { kind: 'focus', label: 'Open in Terminal' },
    }
    const first = { ...structuredClone(conversations), next: NEXT, conversations: [childRow] }
    const before = { ...weekBefore, conversations: [parentRow] }
    let release!: () => void
    const released = new Promise<void>((resolve) => { release = resolve })
    await mockServer(page, opened, async (asked) => {
      if (asked === null) return { json: first }
      await released
      return { json: before }
    })
    await page.goto('/')
    const open = page.getByRole('region', { name: /^Open now/ })
    await expect(open.getByRole('heading', { level: 3 })).toHaveText(['Write the changelog line'])
    await expect(page.locator('li.conversation.is-nested')).toHaveCount(0)

    // The short list puts its end in view, so the week before is already on its way.
    release()
    await expect(open.getByRole('heading', { level: 3 })).toHaveText(['Ship the parser change', 'Write the changelog line'])
    await expect(row(page, 'Write the changelog line')).toHaveClass(/is-nested/)
    await expect(page.getByRole('heading', { level: 2 })).toHaveText(['Open now 2'])
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

  test('searches every conversation, including weeks not loaded yet', async ({ page }) => {
    // The server holds an older week; the first page leaves it out until a search asks.
    const everything = { ...firstWeek, conversations: [...firstWeek.conversations, ...weekBefore.conversations], next: undefined }
    const mocks = await mockServer(page, opened, (_before, params) => ({ json: params.get('q') ? everything : firstWeek }))
    await page.goto('/')
    await expect(page.locator('li.conversation')).toHaveCount(9)
    await expect(row(page, 'Port the docs search to Pagefind')).toHaveCount(0)

    await page.getByRole('searchbox', { name: 'Search conversations' }).fill('pagefind')
    await expect(page.getByRole('heading', { level: 3 })).toHaveText(['Port the docs search to Pagefind'])
    await expect(page.locator('.section-search-status')).toHaveText('1 conversation matches.')
    expect(mocks.views).toEqual([FIRST_REQUEST, '?editor=cursor&q=pagefind'])
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

  test('holds the page layout with placeholders on the first load, so the search row stays put', async ({ page }) => {
    let release!: () => void
    const released = new Promise<void>((resolve) => { release = resolve })
    await mockServer(page, opened, async () => {
      await released
      return { json: conversations }
    })
    await page.goto('/')
    const loading = page.getByRole('region', { name: 'Loading conversations' })
    await expect(loading).toHaveAttribute('aria-busy', 'true')
    await expect(page.locator('.page-verdict .skeleton')).toBeVisible()
    await expect(page.locator('.conversation-tools .skeleton-line')).toHaveCount(3)
    const search = page.getByRole('search', { name: 'Search conversations' })
    await expect(search.getByRole('searchbox', { name: 'Search conversations' })).toBeEditable()
    const before = await search.boundingBox()

    release()
    await expect(page.locator('li.conversation')).toHaveCount(9)
    await expect(loading).toHaveCount(0)
    expect(await search.boundingBox()).toEqual(before)
  })

  test('keeps the heading, chips and search row in place while the overview and the list load', async ({ page }) => {
    let releaseList!: () => void
    const list = new Promise<void>((resolve) => { releaseList = resolve })
    await mockServer(page, opened, async () => {
      await list
      return { json: conversations }
    })
    let releaseOverview!: () => void
    const held = new Promise<void>((resolve) => { releaseOverview = resolve })
    await page.route('**/api/views/overview', async (route) => {
      await held
      return route.fulfill({ json: overview })
    })
    await page.goto('/')
    const heading = page.getByRole('heading', { level: 1, name: 'Conversations', exact: true })
    // The tool filter's width follows its labels, so only where it starts and its height must hold.
    const parts = [heading, page.locator('.conversation-tools'), page.getByRole('search', { name: 'Search conversations' })]
    const boxes = async () => (await Promise.all(parts.map((part) => part.boundingBox()))).map((box, index) => box && index === 1 ? { ...box, width: 0 } : box)
    await expect(page.locator('.page-header.has-verdict')).toBeVisible()
    await expect(page.getByRole('region', { name: 'Loading conversations' })).toBeVisible()
    const loading = await boxes()
    expect(loading.every((box) => box !== null)).toBe(true)

    releaseOverview()
    await expect(page.locator('.rail .skeleton')).toHaveCount(0)
    expect(await boxes()).toEqual(loading)

    releaseList()
    await expect(page.locator('li.conversation')).toHaveCount(9)
    expect(await boxes()).toEqual(loading)
  })

  test('keeps focus on the heading and in the search box when the first load lands', async ({ page }) => {
    let release!: () => void
    const hold = () => new Promise<void>((resolve) => { release = resolve })
    let gate = hold()
    await mockServer(page, opened, async () => {
      await gate
      return { json: conversations }
    })
    await page.goto('/?view=context')
    await page.getByRole('navigation', { name: 'Configuration sections' }).getByRole('link', { name: /^Conversations/ }).click()
    const heading = page.getByRole('heading', { level: 1, name: 'Conversations', exact: true })
    await expect(heading).toBeFocused()
    await expect(page.getByRole('region', { name: 'Loading conversations' })).toBeVisible()
    release()
    await expect(page.locator('li.conversation')).toHaveCount(9)
    await expect(heading).toBeFocused()

    // A full reload empties the kept list, so the placeholders show again.
    gate = hold()
    await page.reload()
    const search = page.getByRole('searchbox', { name: 'Search conversations' })
    await expect(page.getByRole('region', { name: 'Loading conversations' })).toBeVisible()
    await search.fill('sync')
    release()
    await expect(page.locator('.section-search-status')).toHaveText(/^\d+ conversations? match(es)?\.$/)
    await expect(search).toBeFocused()
    await page.keyboard.type(' --')
    await expect(search).toHaveValue('sync --')
  })

  test('shows the kept list at once on a return, then the fresh copy when it differs', async ({ page }) => {
    let released = Promise.resolve()
    let answer = conversations
    const mocks = await mockServer(page, opened, async () => {
      await released
      return { json: answer }
    })
    await page.goto('/')
    await expect(page.locator('li.conversation')).toHaveCount(9)
    const nav = page.getByRole('navigation', { name: 'Configuration sections' })
    await nav.getByRole('link', { name: /^Context/ }).click()
    await expect(page.getByRole('heading', { level: 1, name: 'Context', exact: true })).toBeFocused()

    let release!: () => void
    released = new Promise<void>((resolve) => { release = resolve })
    answer = structuredClone(conversations)
    answer.conversations[0].title = 'Renamed while away'
    await nav.getByRole('link', { name: /^Conversations/ }).click()
    await expect(page.locator('li.conversation')).toHaveCount(9)
    await expect(row(page, 'Add a Conversations page to the local UI')).toBeVisible()
    await expect(page.getByRole('region', { name: 'Loading conversations' })).toHaveCount(0)
    await expect.poll(() => mocks.views.length).toBe(2)

    release()
    await expect(row(page, 'Renamed while away')).toBeVisible()
    await expect(page.locator('li.conversation')).toHaveCount(9)
  })
})

test.describe('conversations on a touch screen', () => {
  test.use({ hasTouch: true, isMobile: true, viewport: { width: 390, height: 844 } })

  test('puts Copy command behind the row menu', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await mockServer(page)
    await page.goto('/')
    const sqlite = row(page, 'Speed up the sqlite reader')
    await expect(sqlite.getByRole('button', { name: /^Copy the command/ })).toBeHidden()
    await sqlite.getByRole('button', { name: 'More actions for Speed up the sqlite reader' }).click()
    await sqlite.getByRole('button', { name: /^Copy the command that resumes/ }).click()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toMatch(/resume/)
    const live = row(page, 'Add a Conversations page to the local UI').getByRole('button', { name: /^Open in Terminal/ })
    expect((await live.boundingBox())!.width).toBeGreaterThan(200)
    expect(await page.evaluate(() => document.scrollingElement!.scrollWidth <= window.innerWidth)).toBe(true)
  })
})
