import { test } from 'node:test'
import assert from 'node:assert/strict'
import { PLACES, ago, afterPage, confirmsCandidate, deleteConfirmOf, deletedOf, detailsOf, escapeTarget, freedOf, folderNamesOf, grouped, isDrag, keyOf, keycapsOf, nameOf, neighbour, openFailureOf, reasonOf, renameOf, replyOf, rowActionOf, rowMenuOf, shortcutOf, startedFromOf, stillWaits, summaryOf, titleOf, viewIn, withTitle } from '../frontend/model.js'

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

const child = (id, state, parent, extra = {}) => row(id, state, { parent: { tool: 'claude', id: parent }, ...extra })
const drawn = (groups) => groups.map((group) => [group.id, group.rows.map((r) => [r.id, group.under.get(keyOf(r))?.id ?? null])])

test('draws a child under its parent in the same group, whatever its tool, and keeps the group\'s count', () => {
  const groups = grouped([row('other', 'active'), child('kid', 'active', 'parent', { tool: 'codex' }), row('parent', 'active')])
  assert.deepEqual(drawn(groups), [['active', [['other', null], ['parent', null], ['kid', 'parent']]]])
})

test('a child whose parent sits in another group is a normal row in its own group: a waiting child stays under Needs you', () => {
  for (const parentState of ['active', 'working', 'completed', 'dormant']) {
    const groups = grouped([child('kid', 'blocked', 'parent'), row('parent', parentState)])
    assert.deepEqual(drawn(groups)[0], ['needs-you', [['kid', null]]])
    assert.equal(groups.find((group) => group.id === 'needs-you').rows.length, 1)
  }
})

test('a child is a normal row when its parent is not in the list: hidden, of another place, or not matching the search', () => {
  assert.deepEqual(drawn(grouped([child('kid', 'active', 'parent')])), [['active', [['kid', null]]]])
  const view = { conversations: [child('kid', 'active', 'parent'), row('parent', 'active', { app: 'cloud' })] }
  assert.deepEqual(drawn(grouped(viewIn(view, 'local').conversations)), [['active', [['kid', null]]]])
  assert.deepEqual(drawn(grouped(view.conversations)), [['active', [['parent', null], ['kid', 'parent']]]])
})

test('draws a chain at one level under the root in the group, oldest first', () => {
  const groups = grouped([
    child('grandchild', 'active', 'kid', { updatedAt: '2026-11-09T12:20:00Z' }),
    child('kid', 'active', 'root', { updatedAt: '2026-11-09T12:10:00Z' }),
    row('root', 'active', { updatedAt: '2026-11-09T11:00:00Z' }),
  ])
  assert.deepEqual(drawn(groups), [['active', [['root', null], ['kid', 'root'], ['grandchild', 'root']]]])
})

test('orders the children by when they started, then by when they were written, then by id, so a new message moves none', () => {
  const groups = grouped([
    child('late-start', 'active', 'root', { updatedAt: '2026-11-09T12:25:00Z', startedAt: '2026-11-09T12:05:00Z' }),
    child('early-start', 'active', 'root', { updatedAt: '2026-11-09T12:20:00Z', startedAt: '2026-11-09T11:30:00Z' }),
    child('b-no-start', 'active', 'root', { updatedAt: '2026-11-09T11:50:00Z' }),
    child('a-no-start', 'active', 'root', { updatedAt: '2026-11-09T11:50:00Z' }),
    row('root', 'active', { updatedAt: '2026-11-09T11:00:00Z', startedAt: '2026-11-09T11:00:00Z' }),
  ])
  assert.deepEqual(drawn(groups), [['active', [['root', null], ['early-start', 'root'], ['a-no-start', 'root'], ['b-no-start', 'root'], ['late-start', 'root']]]])
})

test('draws as many rows as it is given when two rows share a tool and id: the first owns the children', () => {
  const [group] = grouped([row('parent', 'active'), child('kid', 'active', 'parent'), row('parent', 'active', { title: 'A second copy' })])
  assert.deepEqual(group.rows.map((r) => [r.id, r.title]), [['parent', 'parent'], ['kid', 'kid'], ['parent', 'A second copy']])
})

test('draws every row of a loop made by hand as a normal row, and a row naming itself too', () => {
  const groups = grouped([child('a', 'active', 'b'), child('b', 'active', 'a'), child('c', 'active', 'a'), child('self', 'active', 'self')])
  assert.deepEqual(drawn(groups), [['active', [['a', null], ['b', null], ['c', null], ['self', null]]]])
})

test('the arrow keys move through the rows in the order they are drawn', () => {
  const rows = grouped([row('other', 'active'), child('kid', 'active', 'parent'), row('parent', 'active')]).flatMap((group) => group.rows.map(keyOf))
  assert.equal(neighbour(rows, 'claude:parent', 1), 'claude:kid')
  assert.equal(neighbour(rows, 'claude:kid', 1), 'claude:kid')
  assert.equal(neighbour(rows, 'claude:other', 1), 'claude:parent')
})

test('says which conversation a nested row was started from', () => {
  assert.equal(startedFromOf(row('parent', 'active', { title: 'Plan the release' })), 'Started from Plan the release')
  assert.equal(startedFromOf(row('parent', 'active', { title: '' })), 'Started from Untitled conversation')
})

test('offers Mark as read only on a row that needs the person, and Rename on every row', () => {
  assert.deepEqual(
    ['blocked', 'unread', 'working', 'active', 'completed', 'dormant'].map((state) => rowMenuOf(row('a', state))),
    [['read', 'rename'], ['read', 'rename'], ['rename'], ['rename'], ['rename', 'delete'], ['rename', 'delete']],
  )
})

test('offers Delete only on a chat under Archived that no process holds', () => {
  assert.deepEqual(rowMenuOf(row('a', 'completed')), ['rename', 'delete'])
  assert.deepEqual(rowMenuOf(row('a', 'completed', { live: true })), ['rename'])
  assert.deepEqual(rowMenuOf(row('a', 'dormant')), ['rename', 'delete'])
  assert.deepEqual(rowMenuOf(row('a', 'dormant', { live: true })), ['rename'])
  assert.deepEqual(rowMenuOf(row('a', 'active')), ['rename'])
})

test('the delete confirmation names the chat and the data it removes, or says why it cannot be deleted', () => {
  const now = Date.parse('2026-11-09T14:00:00Z')
  assert.deepEqual(deleteConfirmOf(row('a', 'completed', { title: 'Fix the parser', folder: '/Users/me/api' }), now), {
    title: 'Delete "Fix the parser" permanently?',
    details: 'Claude Code · api · 2h',
    body:
      'This removes the session from this computer: its transcript, its subagent transcripts, saved tool output, file backups and pasted images, and its name and marks in agnostic-ai. ' +
      "It cannot be undone. The prompts you typed stay in Claude Code's own prompt history.",
  })
  assert.equal(deleteConfirmOf(row('a', 'completed', { title: '' }), now).title, 'Delete "Untitled conversation" permanently?')
  const unsupported = 'Cursor keeps every chat in one shared database and has no command to delete one. Delete it in Cursor.'
  assert.deepEqual(deleteConfirmOf(row('a', 'completed', { tool: 'cursor', deleteUnsupported: unsupported }), now), { unsupported })
})

test('the delete confirmation for a chat that went quiet names it and says the delete cannot be undone', () => {
  const now = Date.parse('2026-11-09T14:00:00Z')
  const confirm = deleteConfirmOf(row('a', 'dormant', { title: 'Sketch the pricing page', folder: '/Users/me/site', updatedAt: '2026-11-01T14:00:00Z' }), now)
  assert.equal(confirm.title, 'Delete "Sketch the pricing page" permanently?')
  assert.match(confirm.details, /^Claude Code · site · /)
  assert.match(confirm.body, /^This removes the session from this computer: its transcript, .* It cannot be undone\./)
  const unsupported = 'Cursor keeps every chat in one shared database and has no command to delete one. Delete it in Cursor.'
  assert.deepEqual(deleteConfirmOf(row('a', 'dormant', { tool: 'cursor', deleteUnsupported: unsupported }), now), { unsupported })
})

test('says how much a delete freed', () => {
  assert.equal(freedOf(undefined), '')
  assert.equal(freedOf(0), '')
  assert.equal(freedOf(512), 'Freed 512 bytes')
  assert.equal(freedOf(1_500), 'Freed 1.5 KB')
  assert.equal(freedOf(12_345_678), 'Freed 12 MB')
  assert.equal(freedOf(2_400_000_000), 'Freed 2.4 GB')
  assert.deepEqual(deletedOf({ deleted: true, freedBytes: 12_345_678 }), { message: 'Deleted. Freed 12 MB.', note: undefined })
  assert.deepEqual(deletedOf({ deleted: true, note: 'The session data is gone.' }), { message: 'Deleted.', note: 'The session data is gone.' })
})

test('says why a row waits for the person, and nothing its group header already says', () => {
  assert.equal(reasonOf(row('a', 'blocked')), 'Waiting for you')
  assert.equal(reasonOf(row('a', 'unread')), undefined)
  assert.equal(reasonOf(row('a', 'working')), undefined)
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
  assert.equal(reasonOf(row('a', 'unread', { tool: 'codex', quietSince: '2026-11-09T11:53:00Z' }), now), undefined)
})

test('a row shown as needing the person is still marked read only while nothing new came in', () => {
  const shown = row('a', 'unread')
  const view = (...conversations) => ({ conversations })
  assert.equal(stillWaits(view(shown), shown), true)
  assert.equal(stillWaits(view(row('a', 'blocked')), shown), true)
  assert.equal(stillWaits(view(row('a', 'unread', { updatedAt: '2026-11-09T12:05:00Z' })), shown), false)
  assert.equal(stillWaits(view(row('a', 'active')), shown), false)
})

test('a row the latest list does not hold is not marked read: a new message may have taken it out of the search', () => {
  const shown = row('a', 'unread')
  assert.equal(stillWaits({ conversations: [row('a', 'unread', { tool: 'codex' })] }, shown), false)
  assert.equal(stillWaits({ conversations: [] }, shown), false)
  // A list that could not be read says nothing about the row.
  assert.equal(stillWaits(undefined, shown), false)
})

test('a row with no title shows a neutral label', () => {
  assert.equal(titleOf(row('a', 'active', { title: 'Fix the parser' })), 'Fix the parser')
  assert.equal(titleOf(row('a', 'active', { title: '' })), 'Untitled conversation')
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
  assert.equal(escapeTarget({ deleteConfirmOpen: true, rowMenuOpen: true, renaming: true, settingsOpen: true }), 'delete-confirm')
  assert.equal(escapeTarget({ rowMenuOpen: true, renaming: true, settingsOpen: true }), 'row-menu')
  assert.equal(escapeTarget({ rowMenuOpen: false, renaming: true, settingsOpen: true }), 'rename')
  assert.equal(escapeTarget({ rowMenuOpen: false, renaming: false, settingsOpen: true }), 'settings')
  assert.equal(escapeTarget({ rowMenuOpen: false, renaming: false, settingsOpen: false }), 'panel')
})

test('a failed open keeps its reason, and the resume command only when the server sent one', () => {
  assert.equal(openFailureOf({ opened: true, message: 'Switched to the Ghostty tab that runs it.' }), undefined)
  assert.deepEqual(openFailureOf({ opened: false, message: 'It runs in Warp on ttys021 (process 42).' }), { message: 'It runs in Warp on ttys021 (process 42).' })
  assert.deepEqual(openFailureOf({ opened: false, message: 'macOS blocked opening Terminal.', command: "cd '/tmp/app' && claude --resume x" }), {
    message: 'macOS blocked opening Terminal.',
    command: "cd '/tmp/app' && claude --resume x",
  })
})

test('shows the agent\'s latest reply the server sent, and nothing for a row without one', () => {
  assert.equal(replyOf(row('a', 'blocked', { reply: ' Should I run the migration first? ' })), 'Should I run the migration first?')
  assert.equal(replyOf(row('a', 'active')), '')
  assert.equal(replyOf(row('a', 'active', { reply: 42 })), '')
})

test('joins a later search page after the rows read so far, keeping the later copy of a row both pages hold', () => {
  const first = { next: '2026-11-02T12:00:00Z', matches: 3, conversations: [row('new', 'active'), row('old', 'unread')] }
  const later = { next: '2026-10-26T12:00:00Z', conversations: [row('old', 'active'), row('older', 'dormant')] }
  const joined = afterPage(first, later)
  assert.deepEqual(joined.conversations.map((r) => [r.id, r.state]), [['new', 'active'], ['old', 'active'], ['older', 'dormant']])
  assert.equal(joined.next, '2026-10-26T12:00:00Z')
  assert.equal(joined.matches, 3)
})
