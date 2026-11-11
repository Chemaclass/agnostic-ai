import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { menuRowsOf } from '../frontend/model.js'

const items = JSON.parse(await readFile(new URL('../frontend/menu.json', import.meta.url), 'utf8'))
const preferences = { shortcut: 'Ctrl+Alt+Space', startAtLogin: true }
const appearance = { animate: false, dimWhenIdle: true, paused: false }

test('the menu shows the six items in the list order, with the values in effect', () => {
  const rows = menuRowsOf(items, { preferences, appearance })
  assert.deepEqual(
    rows.map(({ id, kind, value }) => [id, kind, value]),
    [
      ['animate', 'switch', false],
      ['dimWhenIdle', 'switch', true],
      ['startAtLogin', 'switch', true],
      ['shortcut', 'shortcut', 'Ctrl+Alt+Space'],
      ['pause', 'action', undefined],
      ['quit', 'action', undefined],
    ],
  )
  assert.deepEqual(rows.map((row) => row.label), items.map((item) => item.label))
})

test('one separator comes before the first action', () => {
  const rows = menuRowsOf(items, { preferences, appearance })
  assert.deepEqual(rows.filter((row) => row.separatorBefore).map((row) => row.id), ['pause'])
})

test('a switch with no value in either reply is refused', () => {
  assert.throws(() => menuRowsOf([{ id: 'sparkle', label: 'Sparkle', kind: 'switch' }], { preferences, appearance }), /no value for sparkle/)
})

test('an unknown kind is refused', () => {
  assert.throws(() => menuRowsOf([{ id: 'animate', label: 'Animate', kind: 'slider' }], { preferences, appearance }), /unknown kind slider/)
})
