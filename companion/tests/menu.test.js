import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { approvalsConfirmOf, approvalsStatusOf, escapeTarget, hideLabelOf, menuRowsOf, syncConfirmOf, syncFailureOf, syncResultOf, updateBadgeOf, updateFeed, updateConfirmOf, updateLabelOf, updateMessageOf } from '../frontend/model.js'

const items = JSON.parse(await readFile(new URL('../frontend/menu.json', import.meta.url), 'utf8'))
const preferences = { shortcut: 'Ctrl+Alt+Space', sendShortcut: 'Ctrl+Alt+V', startAtLogin: true, checkForUpdates: true }
const appearance = { animate: false, dimWhenIdle: true, smallCompanion: false, presence: 'shown' }
const mailbox = { approveAgentSends: true }
const approvals = { approvals: { mode: 'ask', companion: false, overrides: { tools: 0, projects: 0, alwaysAsk: 0 }, shadowed: [], tools: [] }, sync: { needed: false, reasons: [] } }

test('without an update entry, the menu shows the eleven items in the list order, with the values in effect', () => {
  const rows = menuRowsOf(items, { preferences, appearance, mailbox, approvals, update: { state: 'unavailable' } })
  assert.deepEqual(
    rows.map(({ id, kind, value }) => [id, kind, value]),
    [
      ['animate', 'switch', false],
      ['dimWhenIdle', 'switch', true],
      ['smallCompanion', 'switch', false],
      ['startAtLogin', 'switch', true],
      ['approvalsMode', 'choice', 'ask'],
      ['approvalsCompanion', 'switch', false],
      ['approveAgentSends', 'switch', true],
      ['shortcut', 'shortcut', 'Ctrl+Alt+Space'],
      ['sendShortcut', 'shortcut', 'Ctrl+Alt+V'],
      ['hide', 'action', undefined],
      ['quit', 'action', undefined],
    ],
  )
  assert.deepEqual(
    rows.map((row) => row.label),
    items.filter((item) => item.id !== 'update' && item.id !== 'checkForUpdates').map((item) => item.label),
  )
})

test('the update entry sits above Hide and Quit and follows the state', () => {
  const ids = (update) => menuRowsOf(items, { preferences, appearance, update }).map((row) => row.id)
  assert.deepEqual(ids({ state: 'idle' }).slice(-4), ['checkForUpdates', 'update', 'hide', 'quit'])
  assert.ok(!ids(undefined).includes('update'))
  assert.ok(!ids(undefined).includes('checkForUpdates'))
  const automatic = (checkForUpdates) => menuRowsOf(items, { preferences: { ...preferences, checkForUpdates }, appearance, update: { state: 'idle' } }).find((item) => item.id === 'checkForUpdates')
  assert.deepEqual([automatic(true).kind, automatic(true).value, automatic(false).value], ['switch', true, false])
  const row = (update) => menuRowsOf(items, { preferences, appearance, update }).find((item) => item.id === 'update')
  assert.deepEqual([row({ state: 'idle' }).label, row({ state: 'idle' }).disabled], ['Check for updates', false])
  assert.deepEqual([row({ state: 'checking' }).label, row({ state: 'checking' }).disabled], ['Checking for updates…', true])
  assert.deepEqual([row({ state: 'available', version: '0.97.0' }).label, row({ state: 'available', version: '0.97.0' }).disabled], ['Update to v0.97.0', false])
  assert.deepEqual([row({ state: 'updating', version: '0.97.0' }).label, row({ state: 'updating', version: '0.97.0' }).disabled], ['Updating to v0.97.0…', true])
  assert.equal(updateLabelOf('Check for updates', { state: 'failed', version: '0.97.0' }), 'Update to v0.97.0')
})

test('the settings button shows a badge only while an update is known', () => {
  assert.equal(updateBadgeOf({ state: 'idle' }), undefined)
  assert.equal(updateBadgeOf({ state: 'unavailable' }), undefined)
  assert.equal(updateBadgeOf({ state: 'checking' }), undefined)
  assert.equal(updateBadgeOf({ state: 'available', version: '0.97.0' }), 'Settings: update to v0.97.0 available')
  assert.equal(updateBadgeOf({ state: 'failed', version: '0.97.0' }), 'Settings: update to v0.97.0 available')
  assert.equal(updateBadgeOf({ state: 'checking', version: '0.97.0' }), 'Settings: update to v0.97.0 available')
  assert.equal(updateBadgeOf({ state: 'updating', version: '0.97.0' }), undefined)
})

test('Hide stays plain while the shortcut works: the item above shows it', () => {
  assert.equal(hideLabelOf('Hide Agnostic AI companion', preferences), 'Hide Agnostic AI companion')
  const rows = menuRowsOf(items, { preferences: { ...preferences, shortcut: 'Cmd+Shift+K' }, appearance, mailbox }, true)
  assert.equal(rows.find((row) => row.id === 'hide').label, 'Hide Agnostic AI companion')
})

test('Hide names the command when the shortcut does not work', () => {
  const refused = { ...preferences, shortcutProblem: 'Ctrl+Alt+Space is not active: another app holds these keys' }
  assert.equal(hideLabelOf('Hide Agnostic AI companion', refused), 'Hide Agnostic AI companion (agnostic-ai companion brings it back)')
})

test('Quit says it asks first and the shortcut stops working', () => {
  assert.match(items.find((item) => item.id === 'quit').hint, /^Asks first.*shortcut stops working/)
})

test('Escape answers the quit question first, as Cancel', () => {
  assert.equal(escapeTarget({ quitConfirmOpen: true, deleteConfirmOpen: true, rowMenuOpen: true, renaming: true, settingsOpen: true }), 'quit-confirm')
  assert.equal(escapeTarget({ quitConfirmOpen: false, settingsOpen: true }), 'settings')
})

test('a switch with no value in either reply is refused', () => {
  assert.throws(() => menuRowsOf([{ id: 'sparkle', label: 'Sparkle', kind: 'switch' }], { preferences, appearance }), /no value for sparkle/)
})

test('an unknown kind is refused', () => {
  assert.throws(() => menuRowsOf([{ id: 'animate', label: 'Animate', kind: 'slider' }], { preferences, appearance }), /unknown kind slider/)
})

test('a page setting the server could not answer is left out, not refused', () => {
  const rows = menuRowsOf(items, { preferences, appearance, mailbox: undefined })
  assert.equal(rows.find((row) => row.id === 'approveAgentSends').unavailable, true)
  assert.deepEqual(rows.filter((row) => row.unavailable).map((row) => row.id), ['approvalsMode', 'approvalsCompanion', 'approveAgentSends'])
})

test('the approvals rows show the home config values, and a mode the menu does not offer hides the row', () => {
  const doc = { ...approvals, approvals: { ...approvals.approvals, mode: 'yolo', companion: true } }
  const rows = menuRowsOf(items, { preferences, appearance, mailbox, approvals: doc })
  assert.equal(rows.find((row) => row.id === 'approvalsMode').value, 'yolo')
  assert.equal(rows.find((row) => row.id === 'approvalsCompanion').value, true)
  const odd = menuRowsOf(items, { preferences, appearance, mailbox, approvals: { ...doc, approvals: { ...doc.approvals, mode: 'sometimes' } } })
  assert.equal(odd.find((row) => row.id === 'approvalsMode').unavailable, true)
})

test('only review and yolo ask before saving', () => {
  assert.equal(approvalsConfirmOf('ask'), undefined)
  assert.equal(approvalsConfirmOf('review').go, 'Turn on')
  assert.match(approvalsConfirmOf('yolo').body, /one at a time/)
})

test('the status line says why the mode is not in effect, what has its own mode, and what a local block hides', () => {
  const doc = {
    approvals: {
      mode: 'yolo',
      overrides: { tools: 2, projects: 1, alwaysAsk: 0 },
      shadowed: ['approvals.tools'],
      tools: [
        { tool: 'codex', mode: 'yolo', effective: 'ask', reason: 'the hook is not installed; add approvals to builtins in the home config and run sync --scope user' },
        { tool: 'cursor', mode: 'yolo', effective: 'ask', reason: 'Approvals not supported for cursor' },
        { tool: 'claude', mode: 'yolo', effective: 'yolo' },
      ],
    },
    sync: { needed: true, reasons: ['codex: the hook is not installed; add approvals to builtins in the home config and run sync --scope user'] },
  }
  assert.deepEqual(approvalsStatusOf(doc), {
    lines: [
      'Not in effect yet: the hook is not installed for Codex.',
      'Cursor still asks: Approvals not supported for cursor.',
      '2 tools and 1 folder have their own mode. Edit them in the dashboard.',
      'Your local home config hides approvals.tools from the shared one.',
    ],
    apply: true,
  })
  assert.deepEqual(approvalsStatusOf(approvals), { lines: [], apply: false })
  assert.deepEqual(approvalsStatusOf(undefined), { lines: [], apply: false })
})

test('the sync question lists every file and says it applies all pending changes', () => {
  const question = syncConfirmOf({ id: 'x', files: [{ path: '/h/.codex/hooks.json', change: 'create' }] })
  assert.deepEqual(question.files, ['create /h/.codex/hooks.json'])
  assert.match(question.body, /all pending changes of your user configuration/)
  assert.deepEqual(syncConfirmOf({ id: 'x', files: [{ path: 'a', change: 'update' }], notes: ['claude: a warning'] }).notes, ['claude: a warning'])
  assert.equal(syncConfirmOf({ id: 'x', files: [] }).go, undefined)
  assert.deepEqual(syncConfirmOf({ id: 'x', files: [{ path: 'a', change: 'update' }], commands: ['hook staged: curl x | sh'] }).commands, ['hook staged: curl x | sh'])
  assert.equal(syncResultOf({ written: ['a'], notes: ['Codex will show Hooks need review once. Open /hooks in Codex and trust it.'] }), 'Applied: 1 file written. Codex will show Hooks need review once. Open /hooks in Codex and trust it.')
  assert.equal(syncFailureOf(Object.assign(new Error('conflict.'), { command: 'agnostic-ai sync --scope user' })), 'conflict. Run agnostic-ai sync --scope user in a terminal.')
})

test('an update read that an event overtook does not overwrite the event', () => {
  const shown = []
  const feed = updateFeed((update) => shown.push(update.state))
  const showRead = feed.since()
  feed.event({ state: 'available', version: '0.97.0' })
  showRead({ state: 'checking' })
  feed.since()({ state: 'idle' })
  assert.deepEqual(shown, ['available', 'idle'])
})

test('the panel says each update step in plain words', () => {
  const current = '0.96.0'
  assert.equal(updateMessageOf({ state: 'idle', current }), undefined)
  assert.equal(updateMessageOf({ state: 'available', version: '0.97.0', current }), undefined)
  assert.deepEqual(updateMessageOf({ state: 'checking', current }), { text: 'Checking for updates…' })
  assert.deepEqual(updateMessageOf({ state: 'idle', result: 'latest', current }), { text: 'No newer version is available for this computer. You have v0.96.0.' })
  assert.deepEqual(updateMessageOf({ state: 'idle', result: 'unreachable', current }), { text: 'Could not check for updates. Check your connection and try again.' })
  assert.deepEqual(updateMessageOf({ state: 'available', version: '0.97.0', result: 'unreachable', current }), { text: 'Could not check for updates. Check your connection and try again.' })
  assert.deepEqual(updateMessageOf({ state: 'updating', version: '0.97.0', step: 'downloading', current }), { text: 'Downloading v0.97.0…' })
  assert.deepEqual(updateMessageOf({ state: 'updating', version: '0.97.0', step: 'verifying', current }), { text: 'Checking the download…' })
  assert.deepEqual(updateMessageOf({ state: 'updating', version: '0.97.0', step: 'restarting', current }), { text: 'Restarting…' })
  assert.deepEqual(updateMessageOf({ state: 'failed', version: '0.97.0', reason: 'the download did not match its checksum', log: '/tmp/companion-update.log', current }), {
    text: 'The update did not finish: the download did not match its checksum.',
    retry: true,
    log: '/tmp/companion-update.log',
  })
})

test('a separate older command line tool gets a note with its own upgrade command', () => {
  const olderCLI = { path: '/opt/homebrew/bin/agnostic-ai', version: '0.95.0', command: "'/opt/homebrew/bin/agnostic-ai' upgrade" }
  assert.deepEqual(updateMessageOf({ state: 'idle', current: '0.96.0', olderCLI }), {
    text: 'The agnostic-ai command at /opt/homebrew/bin/agnostic-ai is v0.95.0, older than this app (v0.96.0). Run this in a terminal to upgrade it.',
    command: "'/opt/homebrew/bin/agnostic-ai' upgrade",
  })
  assert.deepEqual(updateMessageOf({ state: 'checking', current: '0.96.0', olderCLI }), { text: 'Checking for updates…' })
  assert.deepEqual(updateMessageOf({ state: 'idle', result: 'latest', current: '0.96.0', olderCLI }), {
    text: 'No newer version is available for this computer. You have v0.96.0. The agnostic-ai command at /opt/homebrew/bin/agnostic-ai is v0.95.0, older than this app (v0.96.0). Run this in a terminal to upgrade it.',
    command: "'/opt/homebrew/bin/agnostic-ai' upgrade",
  })
})

test('Escape answers the update question before the menus', () => {
  assert.equal(escapeTarget({ updateConfirmOpen: true, rowMenuOpen: true, settingsOpen: true }), 'update-confirm')
})

test('a terminal install that cannot update itself names the command to copy', () => {
  assert.deepEqual(updateMessageOf({ state: 'failed', version: '0.97.0', reason: 'the package manager could not run from the companion', log: '/tmp/u.log', command: 'agnostic-ai upgrade', current: '0.96.0' }), {
    text: 'The update did not finish: the package manager could not run from the companion.',
    retry: true,
    log: '/tmp/u.log',
    command: 'agnostic-ai upgrade',
    releasePage: true,
  })
  assert.deepEqual(updateConfirmOf({ state: 'available', version: '0.97.0' }), {
    title: 'Update to v0.97.0 and restart?',
    body: 'The app and its command line tool are replaced together. Your settings and chats stay. The companion is gone for a few seconds while it restarts.',
    go: 'Update and restart',
  })
  assert.equal(updateConfirmOf({ state: 'failed', version: '0.97.0', command: 'agnostic-ai upgrade', terminal: true }).go, 'Update and restart')
  assert.match(updateConfirmOf({ state: 'available', version: '0.97.0', terminal: true }).body, /^Runs agnostic-ai upgrade/)
  assert.deepEqual(updateConfirmOf({ state: 'available', version: '0.97.0', command: 'agnostic-ai upgrade' }), {
    title: 'Update to v0.97.0',
    body: 'The companion cannot run this update itself. Run this in a terminal: it updates the command line tool and the companion together. The release page lists what changed.',
    go: 'Copy command',
    command: 'agnostic-ai upgrade',
    releasePage: true,
  })
})
