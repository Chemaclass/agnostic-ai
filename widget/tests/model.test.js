import { test } from 'node:test'
import assert from 'node:assert/strict'
import { ago, detailsOf, grouped, isDrag, keycapsOf, neighbour, reasonOf, shortcutOf, summaryOf } from '../frontend/model.js'

const row = (id, state, extra = {}) => ({ tool: 'claude', id, state, title: id, updatedAt: '2026-11-09T12:00:00Z', ...extra })

test('groups rows by state, in list order, and leaves empty groups out', () => {
  const groups = grouped([row('a', 'dormant'), row('b', 'unread'), row('c', 'working'), row('d', 'blocked'), row('e', 'completed')])
  assert.deepEqual(groups.map((group) => [group.id, group.rows.map((r) => r.id)]), [
    ['needs-you', ['b', 'd']],
    ['working', ['c']],
    ['done', ['e']],
    ['dormant', ['a']],
  ])
  assert.deepEqual(groups.map((group) => group.startsFolded), [false, false, true, true])
})

test('says why a row needs the person, and nothing for a quiet one', () => {
  assert.equal(reasonOf(row('a', 'blocked')), 'Waiting for you')
  assert.equal(reasonOf(row('a', 'unread')), 'New reply')
  assert.equal(reasonOf(row('a', 'working')), 'Working')
  assert.equal(reasonOf(row('a', 'active')), undefined)
})

test('writes a short age', () => {
  const now = Date.parse('2026-11-09T12:00:00Z')
  const at = (stamp) => ago(stamp, now)
  assert.deepEqual(
    [at('2026-11-09T11:59:30Z'), at('2026-11-09T11:55:00Z'), at('2026-11-09T09:00:00Z'), at('2026-11-07T12:00:00Z'), at('2026-10-12T12:00:00Z')],
    ['now', '5m', '3h', '2d', '4w'],
  )
})

test('names the tool, the folder and the age, on either kind of path', () => {
  const now = Date.parse('2026-11-09T12:05:00Z')
  assert.equal(detailsOf(row('a', 'active', { folder: '/Users/mo/Products/agnostic-ai' }), now), 'Claude Code · agnostic-ai · 5m')
  assert.equal(detailsOf(row('a', 'active', { tool: 'cursor', folder: 'C:\\work\\app\\' }), now), 'Cursor · app · 5m')
  assert.equal(detailsOf(row('a', 'active', { tool: 'codex' }), now), 'Codex · 5m')
})

test('sums up what needs the person, or the search', () => {
  assert.equal(summaryOf({ needsYou: 0 }, ''), 'All caught up')
  assert.equal(summaryOf({ needsYou: 1 }, ''), '1 conversation needs you')
  assert.equal(summaryOf({ needsYou: 3 }, ''), '3 conversations need you')
  assert.equal(summaryOf({ needsYou: 3, matches: 0 }, 'parser'), 'No conversation matches')
  assert.equal(summaryOf({ needsYou: 3, matches: 1 }, 'parser'), '1 conversation matches')
  assert.equal(summaryOf({ needsYou: 3, matches: 12 }, 'parser'), '12 conversations match')
})

test('moves through the rows and stops at the ends', () => {
  const keys = ['a', 'b', 'c']
  assert.equal(neighbour(keys, 'a', 1), 'b')
  assert.equal(neighbour(keys, 'c', 1), 'c')
  assert.equal(neighbour(keys, 'a', -1), 'a')
  assert.equal(neighbour(keys, 'gone', 1), 'a')
  assert.equal(neighbour([], 'a', 1), undefined)
})

test('takes a press that barely moves as a click, and a longer one as a drag', () => {
  const pressed = { x: 30, y: 30 }
  assert.equal(isDrag(pressed, { x: 30, y: 30 }), false)
  assert.equal(isDrag(pressed, { x: 32, y: 32 }), false)
  assert.equal(isDrag(pressed, { x: 34, y: 30 }), true)
  assert.equal(isDrag(pressed, { x: 27, y: 26 }), true)
})

test('spells a key press as a shortcut once it has a modifier and a key', () => {
  const press = (code, modifiers = {}) => ({ code, ctrlKey: false, altKey: false, metaKey: false, shiftKey: false, ...modifiers })
  assert.equal(shortcutOf(press('Space', { ctrlKey: true, altKey: true }), true), 'Ctrl+Alt+Space')
  assert.equal(shortcutOf(press('KeyK', { metaKey: true, shiftKey: true }), true), 'Cmd+Shift+K')
  assert.equal(shortcutOf(press('KeyK', { metaKey: true }), false), 'Super+K')
  assert.equal(shortcutOf(press('Digit1', { altKey: true }), false), 'Alt+1')
  assert.equal(shortcutOf(press('F5', { ctrlKey: true }), false), 'Ctrl+F5')
  assert.equal(shortcutOf(press('KeyK'), true), undefined, 'a plain key is typing')
  assert.equal(shortcutOf(press('KeyK', { shiftKey: true }), true), undefined, 'Shift alone is typing a capital')
  assert.equal(shortcutOf(press('ControlLeft', { ctrlKey: true }), true), undefined, 'a modifier on its own is not a shortcut yet')
  assert.equal(shortcutOf(press('Comma', { ctrlKey: true }), true), undefined, 'a key Wails cannot name')
})

test('shows a shortcut as keycaps, with symbols on a Mac', () => {
  assert.deepEqual(keycapsOf('Ctrl+Alt+Space', true), ['⌃', '⌥', 'Space'])
  assert.deepEqual(keycapsOf('Cmd+Shift+K', true), ['⌘', '⇧', 'K'])
  assert.deepEqual(keycapsOf('Ctrl+Alt+Space', false), ['Ctrl', 'Alt', 'Space'])
})
