import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { hideLabelOf, menuRowsOf } from '../frontend/model.js'

const items = JSON.parse(await readFile(new URL('../frontend/menu.json', import.meta.url), 'utf8'))
const preferences = { shortcut: 'Ctrl+Alt+Space', startAtLogin: true }
const appearance = { animate: false, dimWhenIdle: true, hidden: false }

test('the menu shows the six items in the list order, with the values in effect', () => {
  const rows = menuRowsOf(items, { preferences, appearance })
  assert.deepEqual(
    rows.map(({ id, kind, value }) => [id, kind, value]),
    [
      ['animate', 'switch', false],
      ['dimWhenIdle', 'switch', true],
      ['startAtLogin', 'switch', true],
      ['shortcut', 'shortcut', 'Ctrl+Alt+Space'],
      ['hide', 'action', undefined],
      ['quit', 'action', undefined],
    ],
  )
  assert.deepEqual(
    rows.map((row) => row.label),
    items.map((item) => (item.id === 'hide' ? `${item.label} (Ctrl+Alt+Space brings it back)` : item.label)),
  )
})

test('Hide names the shortcut that brings the companion back, as each system writes it', () => {
  assert.equal(hideLabelOf('Hide Agnostic AI companion', preferences, true), 'Hide Agnostic AI companion (⌃⌥Space brings it back)')
  assert.equal(hideLabelOf('Hide Agnostic AI companion', preferences, false), 'Hide Agnostic AI companion (Ctrl+Alt+Space brings it back)')
  const rows = menuRowsOf(items, { preferences: { ...preferences, shortcut: 'Cmd+Shift+K' }, appearance }, true)
  assert.equal(rows.find((row) => row.id === 'hide').label, 'Hide Agnostic AI companion (⌘⇧K brings it back)')
})

test('Hide names the command when the shortcut does not work', () => {
  const refused = { ...preferences, shortcutProblem: 'Ctrl+Alt+Space is not active: another app holds these keys' }
  assert.equal(hideLabelOf('Hide Agnostic AI companion', refused, true), 'Hide Agnostic AI companion (agnostic-ai companion brings it back)')
})

test('Quit says the shortcut stops working', () => {
  assert.match(items.find((item) => item.id === 'quit').hint, /shortcut stops working/)
})

test('a switch with no value in either reply is refused', () => {
  assert.throws(() => menuRowsOf([{ id: 'sparkle', label: 'Sparkle', kind: 'switch' }], { preferences, appearance }), /no value for sparkle/)
})

test('an unknown kind is refused', () => {
  assert.throws(() => menuRowsOf([{ id: 'animate', label: 'Animate', kind: 'slider' }], { preferences, appearance }), /unknown kind slider/)
})
