import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { escapeTarget, hideLabelOf, menuRowsOf, updateBadgeOf, updateFeed, updateLabelOf, updateMessageOf } from '../frontend/model.js'

const items = JSON.parse(await readFile(new URL('../frontend/menu.json', import.meta.url), 'utf8'))
const preferences = { shortcut: 'Ctrl+Alt+Space', sendShortcut: 'Ctrl+Alt+V', startAtLogin: true }
const appearance = { animate: false, dimWhenIdle: true, smallCompanion: false, presence: 'shown' }
const mailbox = { approveAgentSends: true }

test('without an update entry, the menu shows the nine items in the list order, with the values in effect', () => {
  const rows = menuRowsOf(items, { preferences, appearance, mailbox, update: { state: 'unavailable' } })
  assert.deepEqual(
    rows.map(({ id, kind, value }) => [id, kind, value]),
    [
      ['animate', 'switch', false],
      ['dimWhenIdle', 'switch', true],
      ['smallCompanion', 'switch', false],
      ['startAtLogin', 'switch', true],
      ['approveAgentSends', 'switch', true],
      ['shortcut', 'shortcut', 'Ctrl+Alt+Space'],
      ['sendShortcut', 'shortcut', 'Ctrl+Alt+V'],
      ['hide', 'action', undefined],
      ['quit', 'action', undefined],
    ],
  )
  assert.deepEqual(
    rows.map((row) => row.label),
    items.filter((item) => item.id !== 'update').map((item) => item.label),
  )
})

test('the update entry sits above Hide and Quit and follows the state', () => {
  const ids = (update) => menuRowsOf(items, { preferences, appearance, update }).map((row) => row.id)
  assert.deepEqual(ids({ state: 'idle' }).slice(-3), ['update', 'hide', 'quit'])
  assert.ok(!ids(undefined).includes('update'))
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
  assert.equal(rows.filter((row) => row.unavailable).length, 1)
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
  assert.deepEqual(updateMessageOf({ state: 'idle', result: 'latest', current }), { text: 'No newer version is available for this Mac. You have v0.96.0.' })
  assert.deepEqual(updateMessageOf({ state: 'idle', result: 'unreachable', current }), { text: 'Could not check for updates. Check your connection and try again.' })
  assert.deepEqual(updateMessageOf({ state: 'available', version: '0.97.0', result: 'unreachable', current }), { text: 'Could not check for updates. Check your connection and try again.' })
  assert.deepEqual(updateMessageOf({ state: 'updating', version: '0.97.0', step: 'downloading', current }), { text: 'Downloading v0.97.0…' })
  assert.deepEqual(updateMessageOf({ state: 'updating', version: '0.97.0', step: 'verifying', current }), { text: 'Checking the download…' })
  assert.deepEqual(updateMessageOf({ state: 'updating', version: '0.97.0', step: 'restarting', current }), { text: 'Restarting…' })
  assert.deepEqual(updateMessageOf({ state: 'failed', version: '0.97.0', reason: 'the download did not match its checksum', log: '/tmp/companion-update.log', current }), {
    text: 'The update did not finish: the download did not match its checksum. Your current version is still running.',
    retry: true,
    log: '/tmp/companion-update.log',
  })
})

test('Escape answers the update question before the menus', () => {
  assert.equal(escapeTarget({ updateConfirmOpen: true, rowMenuOpen: true, settingsOpen: true }), 'update-confirm')
})
