import { test } from 'node:test'
import assert from 'node:assert/strict'
import { PLACES, ago, confirmsCandidate, detailsOf, escapeTarget, folderNamesOf, grouped, isDrag, keycapsOf, nameOf, neighbour, reasonOf, renameOf, rowActionOf, rowMenuOf, shortcutOf, summaryOf, viewIn, withTitle } from '../frontend/model.js'

const row = (id, state, extra = {}) => ({ tool: 'claude', id, state, title: id, updatedAt: '2026-11-09T12:00:00Z', ...extra })

test('groups rows by state, in list order, and leaves empty groups out', () => {
  const groups = grouped([row('a', 'dormant'), row('b', 'unread'), row('c', 'working'), row('d', 'blocked'), row('e', 'completed')])
  assert.deepEqual(groups.map((group) => [group.id, group.rows.map((r) => r.id)]), [
    ['needs-you', ['b', 'd']],
    ['working', ['c']],
    ['archived', ['a', 'e']],
  ])
  assert.deepEqual(groups.map((group) => group.startsFolded), [false, false, true])
})

test('offers Mark as read only on a row that needs the person, and Rename on every row', () => {
  assert.deepEqual(
    ['blocked', 'unread', 'working', 'active', 'completed', 'dormant'].map((state) => rowMenuOf(row('a', state))),
    [['read', 'rename'], ['read', 'rename'], ['rename'], ['rename'], ['rename'], ['rename']],
  )
})

test('says why a row needs the person, and nothing for a quiet one', () => {
  assert.equal(reasonOf(row('a', 'blocked')), 'Waiting for you')
  assert.equal(reasonOf(row('a', 'unread')), 'New reply')
  assert.equal(reasonOf(row('a', 'working')), 'Working')
  assert.equal(reasonOf(row('a', 'active')), undefined)
})

test('says how long a chat has been silent when its wait is read from that silence, not reported', () => {
  const now = Date.parse('2026-11-09T12:00:00Z')
  const quiet = (since) => row('a', 'blocked', { tool: 'codex', activity: 'waiting', quietSince: since })
  assert.equal(reasonOf(quiet('2026-11-09T11:55:00Z'), now), 'No activity for 5m')
  assert.equal(reasonOf(quiet('2026-11-09T11:53:00Z'), now), 'No activity for 7m')
  assert.equal(reasonOf(quiet('2026-11-09T09:00:00Z'), now), 'No activity for 3h')
  // A clock that runs behind the server's must not say "for now".
  assert.equal(reasonOf(quiet('2026-11-09T12:00:30Z'), now), 'No activity')
  assert.equal(reasonOf(row('a', 'blocked', { tool: 'codex', activity: 'waiting' }), now), 'Waiting for you')
  assert.equal(reasonOf(quiet('not a time'), now), 'Waiting for you')
  assert.equal(reasonOf(row('a', 'unread', { tool: 'codex', quietSince: '2026-11-09T11:53:00Z' }), now), 'New reply')
})

test('offers archive on a chat that is not archived, hide on one that is, and nothing on a working one', () => {
  assert.deepEqual(
    ['blocked', 'unread', 'active', 'working', 'completed', 'dormant'].map((state) => rowActionOf(row('a', state))),
    ['archive', 'archive', 'archive', undefined, 'hide', 'hide'],
  )
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
  assert.equal(detailsOf(row('a', 'active', { folder: '/Users/me/code/agnostic-ai' }), now), 'Claude Code · agnostic-ai · 5m')
  assert.equal(detailsOf(row('a', 'active', { tool: 'cursor', folder: 'C:\\work\\app\\' }), now), 'Cursor · app · 5m')
  assert.equal(detailsOf(row('a', 'active', { tool: 'codex' }), now), 'Codex · 5m')
})

test('says Cloud for a session that runs in the cloud, and nothing for one on this machine', () => {
  const now = Date.parse('2026-11-09T12:05:00Z')
  assert.equal(detailsOf(row('cse_a1', 'active', { app: 'cloud' }), now), 'Claude Code · Cloud · 5m')
  assert.equal(detailsOf(row('a', 'active', { app: 'cli', folder: '/tmp/app' }), now), 'Claude Code · app · 5m')
  assert.equal(detailsOf(row('a', 'active', { app: 'desktop' }), now), 'Claude Code · 5m')
})

test('adds parent folders only when two folders in view share a last name', () => {
  const now = Date.parse('2026-11-09T12:05:00Z')
  const folders = ['/Users/me/a/api', '/Users/me/b/api', '/Users/me/web', '/Users/me/b/api/']
  assert.equal(detailsOf(row('a', 'active', { folder: '/Users/me/a/api' }), now, folderNamesOf(folders)), 'Claude Code · a/api · 5m')
  assert.equal(detailsOf(row('b', 'active', { folder: '/Users/me/b/api/' }), now, folderNamesOf(folders)), 'Claude Code · b/api · 5m')
  assert.equal(detailsOf(row('c', 'active', { folder: '/Users/me/web' }), now, folderNamesOf(folders)), 'Claude Code · web · 5m')
  assert.equal(detailsOf(row('d', 'active', { folder: '/Users/me/a/api' }), now, folderNamesOf(['/Users/me/a/api', '/Users/me/a/api'])), 'Claude Code · api · 5m')
  assert.equal(detailsOf(row('e', 'active', { folder: '/x/shop/api' }), now, folderNamesOf(['/x/shop/api', '/y/shop/api'])), 'Claude Code · x/shop/api · 5m')
  const nested = folderNamesOf(['/x/shop/api', '/y/shop/api', '/z/web/api', '/api', '/'])
  assert.deepEqual([...nested.values()], ['x/shop/api', 'y/shop/api', 'web/api', 'api'])
})

test('tells same-named folders apart with their own separators and skips rows with no folder', () => {
  const now = Date.parse('2026-11-09T12:05:00Z')
  const folders = ['C:\\work\\api', 'D:\\home\\api\\', '', undefined]
  assert.equal(detailsOf(row('a', 'active', { tool: 'cursor', folder: 'C:\\work\\api' }), now, folderNamesOf(folders)), 'Cursor · work\\api · 5m')
  assert.equal(detailsOf(row('b', 'active', { tool: 'cursor', folder: 'D:\\home\\api\\' }), now, folderNamesOf(folders)), 'Cursor · home\\api · 5m')
  assert.equal(detailsOf(row('c', 'active', { app: 'cloud' }), now, folderNamesOf(folders)), 'Claude Code · Cloud · 5m')
  // On macOS a backslash can be part of a folder's name, so these are two folders.
  const names = folderNamesOf(['/Users/me/a\\b/api', '/Users/me/a/b/api'])
  assert.deepEqual([...names.values()], ['a\\b/api', 'a/b/api'])
})

test('sums up a search, and says nothing without one', () => {
  assert.equal(summaryOf({ needsYou: 3 }, ''), '')
  const found = (matches, shown) => ({ needsYou: 3, matches, conversations: Array.from({ length: shown }, (_, i) => row(String(i), 'active')) })
  assert.equal(summaryOf(found(0, 0), 'parser'), 'No conversation matches')
  assert.equal(summaryOf(found(1, 1), 'parser'), '1 conversation matches')
  assert.equal(summaryOf(found(12, 12), 'parser'), '12 conversations match')
  assert.equal(summaryOf(found(40, 25), 'parser'), '40 conversations match; the newest 25 show')
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

test('narrows the list to one place and counts what needs the person there', () => {
  const view = {
    needsYou: 9,
    conversations: [row('a', 'unread'), row('b', 'blocked', { app: 'cloud' }), row('c', 'active', { app: 'cloud' }), row('d', 'working', { app: 'cli' })],
  }
  assert.equal(viewIn(view, 'all'), view)
  const local = viewIn(view, 'local')
  assert.deepEqual([local.conversations.map((r) => r.id), local.needsYou, local.matches], [['a', 'd'], 1, undefined])
  const cloud = viewIn({ ...view, matches: 4 }, 'cloud')
  assert.deepEqual([cloud.conversations.map((r) => r.id), cloud.needsYou, cloud.matches], [['b', 'c'], 1, 2])
  assert.deepEqual(PLACES.map((place) => place.id), ['all', 'local', 'cloud'])
})

test('takes a typed name as one line and refuses an empty one', () => {
  assert.equal(nameOf('  Parser \n\t fix  '), 'Parser fix')
  assert.equal(nameOf(''), undefined)
  assert.equal(nameOf(' \n\t '), undefined)
})

test('shows a new name on its row at once and leaves the rest of the view as it was', () => {
  const view = { needsYou: 1, conversations: [row('a', 'unread'), row('a', 'active', { tool: 'codex' }), row('b', 'completed')] }
  const renamed = withTitle(view, 'claude:a', 'Parser fix')
  assert.deepEqual(renamed.conversations.map((r) => [r.tool, r.id, r.title, r.state]), [
    ['claude', 'a', 'Parser fix', 'unread'],
    ['codex', 'a', 'a', 'active'],
    ['claude', 'b', 'b', 'completed'],
  ])
  assert.equal(renamed.needsYou, 1)
  assert.equal(view.conversations[0].title, 'a')
})

test('sends a typed name, refuses an empty one and keeps a name that did not change', () => {
  assert.deepEqual(renameOf(' Parser  fix ', '/clear'), { title: 'Parser fix' })
  assert.deepEqual(renameOf('  ', '/clear'), { empty: true })
  assert.deepEqual(renameOf('/clear', '/clear'), { keep: true })
  // A long title shows cut short; saving it as it shows would lose its end.
  const cut = 'a'.repeat(119) + '…'
  assert.deepEqual(renameOf(cut, cut), { keep: true })
})

test('lets an input method confirm its candidate without saving', () => {
  assert.equal(confirmsCandidate({ key: 'Enter', isComposing: true, keyCode: 13 }), true)
  assert.equal(confirmsCandidate({ key: 'Enter', isComposing: false, keyCode: 229 }), true)
  assert.equal(confirmsCandidate({ key: 'Enter', isComposing: false, keyCode: 13 }), false)
})

test('closes the nearest thing on Escape: the row menu, a rename, the settings, then the panel', () => {
  assert.equal(escapeTarget({ rowMenuOpen: true, renaming: true, settingsOpen: true }), 'row-menu')
  assert.equal(escapeTarget({ rowMenuOpen: false, renaming: true, settingsOpen: true }), 'rename')
  assert.equal(escapeTarget({ rowMenuOpen: false, renaming: false, settingsOpen: true }), 'settings')
  assert.equal(escapeTarget({ rowMenuOpen: false, renaming: false, settingsOpen: false }), 'panel')
})
