import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { escapeTarget, hideLabelOf, menuRowsOf } from '../frontend/model.js'

const items = JSON.parse(await readFile(new URL('../frontend/menu.json', import.meta.url), 'utf8'))
const preferences = { shortcut: 'Ctrl+Alt+Space', sendShortcut: 'Ctrl+Alt+V', startAtLogin: true }
const appearance = { animate: false, dimWhenIdle: true, smallCompanion: false, presence: 'shown' }
const mailbox = { approveAgentSends: true }

test('the menu shows the nine items in the list order, with the values in effect', () => {
  const rows = menuRowsOf(items, { preferences, appearance, mailbox })
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
    items.map((item) => item.label),
  )
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
