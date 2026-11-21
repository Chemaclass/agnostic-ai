import { test } from 'node:test'
import assert from 'node:assert/strict'
import { HELPERS_SHOWN, NOTHING_TO_SEND, NOTICES, PLACES, SWITCHED_TO_PASTE, ago, afterPage, approvalOf, approvalReasonOf, approvalsHeaderOf, approvedCallsOf, approvedLabelOf, approvedToggleOf, chipTextOf, circleLabelOf, cloudPageOf, confirmsCandidate, deleteConfirmOf, deletedOf, detailsOf, escapeTarget, freedOf, folderNamesOf, grouped, helperDetailsOf, helperOpenOf, helperReplyOf, helperRowsOf, helperTitleOf, helperTooltipOf, helpersToggleOf, isDrag, lateFailureReplaces, keyOf, keycapsOf, lateOpenFailureOf, nameOf, neighbour, openFailureOf, pickKeyOf, pickTitleOf, pickViewOf, pickableClip, quietNoteOf, renameOf, replyOf, rowActionOf, rowMenuOf, refusedShortcutOf, rowTooltipOf, roleOfTone, sendingTitleOf, shortcutOf, startsRename, startedFromOf, stillActive, stillWaits, straightSendOf, subagentNoticeOf, unreadLabelOf, unreadMarkOf, summaryOf, takesText, titleOf, togglesPick, typableRows, unreadableClipOf, unsentNoticeOf, viewIn, waitingOf, workingHelpersOf } from '../frontend/model.js'

import { currentShellOf, shellCommandOf, shellWarningOf, stopSessionConfirmOf, stopShellDetailsOf, stoppedSessionOf, stoppedShellOf } from '../frontend/model.js'

const row = (id, state, extra = {}) => ({
  tool: 'claude',
  id,
  state,
  title: id,
  updatedAt: '2026-11-09T12:00:00Z',
  ...(state === 'completed' ? { completedAt: '2026-11-09T13:00:00Z' } : {}),
  ...extra,
})

test('groups rows by state, in list order, and leaves empty groups out', () => {
  const groups = grouped([row('a', 'dormant'), row('b', 'unread'), row('c', 'working'), row('d', 'blocked'), row('e', 'completed')])
  assert.deepEqual(groups.map((group) => [group.id, group.rows.map((r) => r.id)]), [
    ['needs-you', ['d', 'b']],
    ['working', ['c']],
    ['archived', ['a', 'e']],
  ])
  assert.deepEqual(groups.map((group) => group.startsFolded), [false, false, true])
})

test('under Needs you a permission to give comes first, then another wait, then an unread reply, each kept in list order', () => {
  const asks = (id, kind) => row(id, 'blocked', kind ? { waiting: { for: kind } } : {})
  const rows = [row('reply-1', 'unread'), asks('question-1', 'answer'), asks('permission-1', 'permission'), asks('unknown-1'), row('reply-2', 'unread'), asks('permission-2', 'permission')]
  assert.deepEqual(grouped(rows)[0].rows.map((r) => r.id), ['permission-1', 'permission-2', 'question-1', 'unknown-1', 'reply-1', 'reply-2'])
})

test('under Needs you a family ranks by its most urgent row, and a child stays right after its parent', () => {
  const groups = grouped([row('parent', 'unread'), child('kid', 'blocked', 'parent', { waiting: { for: 'permission' } }), row('other', 'blocked')])
  assert.deepEqual(drawn(groups), [['needs-you', [['parent', null], ['kid', 'parent'], ['other', null]]]])
  const asking = grouped([row('asks', 'blocked', { waiting: { for: 'answer' } }), child('reply', 'unread', 'asks'), row('approve', 'blocked', { waiting: { for: 'permission' } }), row('read', 'unread')])
  assert.deepEqual(drawn(asking), [['needs-you', [['approve', null], ['asks', null], ['reply', 'asks'], ['read', null]]]])
})

test('the circle tells the chats that wait for the person apart from the unread ones', () => {
  const view = { needsYou: 4, conversations: [row('a', 'blocked'), row('b', 'unread'), row('c', 'working'), row('d', 'blocked'), row('e', 'unread')] }
  assert.deepEqual(waitingOf(view), { blocked: 2, unread: 2 })
  assert.deepEqual(waitingOf({ needsYou: 0, conversations: [] }), { blocked: 0, unread: 0 })
})

test('the circle\'s label names each count, and leaves out a count of zero', () => {
  assert.equal(circleLabelOf({ blocked: 2, unread: 4 }), 'Conversations: 2 waiting for you, 4 unread')
  assert.equal(circleLabelOf({ blocked: 1, unread: 1 }), 'Conversations: 1 waiting for you, 1 unread')
  assert.equal(circleLabelOf({ blocked: 1, unread: 0 }), 'Conversations: 1 waiting for you')
  assert.equal(circleLabelOf({ blocked: 0, unread: 3 }), 'Conversations: 3 unread')
  assert.equal(circleLabelOf({ blocked: 0, unread: 0 }), 'Conversations: none needs you')
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

test('offers Mark as read only on a row that needs the person, and Rename and Copy session ID on every row', () => {
  assert.deepEqual(
    ['blocked', 'unread', 'working', 'active', 'completed', 'dormant'].map((state) => rowMenuOf(row('a', state))),
    [['read', 'rename', 'copy-session-id', 'send-to', 'claim-files', 'complete'], ['read', 'rename', 'copy-session-id', 'send-to', 'claim-files', 'complete'], ['rename', 'copy-session-id', 'send-to', 'claim-files'], ['rename', 'copy-session-id', 'send-to', 'claim-files', 'complete'], ['rename', 'copy-session-id', 'send-to', 'claim-files', 'reopen', 'hide', 'delete'], ['rename', 'copy-session-id', 'send-to', 'claim-files', 'hide', 'delete']],
  )
})

test('offers Send to session only on local top-level Claude Code and Codex rows', () => {
  assert.ok(rowMenuOf(row('local-codex', 'active', { tool: 'codex' })).includes('send-to'))
  assert.ok(rowMenuOf(row('shared', 'active', { cloud: { id: 'cse_1' } })).includes('send-to'))
  for (const conversation of [
    row('cloud', 'active', { app: 'cloud' }),
    row('cowork', 'active', { app: 'cowork' }),
    row('chat', 'active', { app: 'chat' }),
    row('cursor', 'working', { tool: 'cursor' }),
    child('nested', 'active', 'parent'),
  ]) {
    assert.ok(!rowMenuOf(conversation).includes('send-to'))
  }
})

test('shows mailbox counts and only the thread controls available in its state', async () => {
  const { threadActionsOf, threadLabelOf, threadsOf } = await import('../frontend/model.js')
  const thread = { status: 'open', count: 3, cap: 8, waiting: 1 }
  assert.deepEqual(threadsOf(row('a', 'active')), [])
  assert.deepEqual(threadsOf(row('a', 'active', { threads: [thread] })), [thread])
  assert.equal(threadLabelOf(thread), '3 of 8 messages, 1 waiting')
  assert.equal(threadLabelOf({ ...thread, waiting: 0 }), '3 of 8 messages')
  assert.deepEqual(threadActionsOf(thread), ['reply', 'pause', 'close', 'stop'])
  assert.deepEqual(threadActionsOf({ ...thread, count: 8 }), ['reply', 'pause', 'close', 'raise-cap', 'stop'])
  assert.deepEqual(threadActionsOf({ ...thread, status: 'paused' }), ['reply', 'resume', 'stop'])
  assert.deepEqual(threadActionsOf({ ...thread, status: 'closing' }), ['stop'])
  assert.deepEqual(threadActionsOf({ ...thread, status: 'closed' }), [])
})

test('labels message states and offers approve, reject and queue only where they apply', async () => {
  const { messageActionsOf, messageStateLabelOf, threadLabelOf } = await import('../frontend/model.js')
  assert.deepEqual(['held', 'delivered', 'rejected', 'reserved'].map(messageStateLabelOf), ['Held for you', 'Delivered', 'Rejected', 'Delivering'])
  assert.equal(threadLabelOf({ count: 2, cap: 8, waiting: 0, held: 1 }), '2 of 8 messages, 1 held for you')
  const open = { status: 'open' }
  assert.deepEqual(messageActionsOf({ state: 'held' }, open), ['approve', 'reject'])
  assert.deepEqual(messageActionsOf({ state: 'held' }, { status: 'paused' }), ['approve', 'reject'])
  assert.deepEqual(messageActionsOf({ state: 'waiting' }, open), ['type', 'copy'])
  assert.deepEqual(messageActionsOf({ state: 'waiting' }, { status: 'paused' }), [])
  assert.deepEqual(messageActionsOf({ state: 'staged' }, open), ['queue'])
  assert.deepEqual(messageActionsOf({ state: 'staged' }, { status: 'closed' }), [])
  assert.deepEqual(messageActionsOf({ state: 'reserved' }, open), ['type', 'copy'])
  assert.deepEqual(messageActionsOf({ state: 'reserved' }, { status: 'closed' }), [])
  for (const state of ['delivered', 'rejected', 'cancelled']) assert.deepEqual(messageActionsOf({ state }, open), [])
})

test('offers Delete only on a chat under Archived that no process holds', () => {
  assert.deepEqual(rowMenuOf(row('a', 'completed')), ['rename', 'copy-session-id', 'send-to', 'claim-files', 'reopen', 'hide', 'delete'])
  assert.deepEqual(rowMenuOf(row('a', 'completed', { live: true })), ['rename', 'copy-session-id', 'send-to', 'claim-files', 'reopen', 'hide'])
  assert.deepEqual(rowMenuOf(row('a', 'dormant')), ['rename', 'copy-session-id', 'send-to', 'claim-files', 'hide', 'delete'])
  assert.deepEqual(rowMenuOf(row('a', 'dormant', { live: true })), ['rename', 'copy-session-id', 'send-to', 'claim-files', 'hide'])
  assert.deepEqual(rowMenuOf(row('a', 'active')), ['rename', 'copy-session-id', 'send-to', 'claim-files', 'complete'])
})

test('an archived chat has no read state: no Mark as read, and Delete without reading it first', () => {
  assert.deepEqual(rowMenuOf(row('a', 'dormant', { unread: true })), ['rename', 'copy-session-id', 'send-to', 'claim-files', 'hide', 'delete'])
  assert.deepEqual(rowMenuOf(row('a', 'completed', { unread: true, cloud: { id: 'cse_1' } })), ['rename', 'copy-session-id', 'send-to', 'claim-files', 'reopen', 'hide', 'delete', 'web'])
  assert.equal(rowActionOf(row('a', 'dormant', { unread: true })), 'hide')
  assert.equal(unreadMarkOf(row('a', 'dormant', { unread: true })), false)
  assert.equal(unreadMarkOf(row('a', 'active', { unread: true })), true)
})

test('the tooltip of an archived chat says nothing about a reply not read', () => {
  const open = { kind: 'terminal', label: 'Resume in Terminal' }
  assert.equal(rowTooltipOf(row('a', 'dormant', { open, folder: '/tmp/shop', unread: true }), Date.parse('2026-11-09T12:00:00Z')), 'Resume in Terminal\n/tmp/shop')
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

test('says how long a chat has been silent when its wait is read from that silence, not reported', () => {
  const now = Date.parse('2026-11-09T12:00:00Z')
  const quiet = (since) => row('a', 'blocked', { tool: 'codex', activity: 'waiting', quietSince: since })
  assert.equal(quietNoteOf(quiet('2026-11-09T11:55:00Z'), now), 'No activity for 5m')
  assert.equal(quietNoteOf(quiet('2026-11-09T11:53:00Z'), now), 'No activity for 7m')
  assert.equal(quietNoteOf(quiet('2026-11-09T09:00:00Z'), now), 'No activity for 3h')
  // A clock that runs behind the server's must not say "for now".
  assert.equal(quietNoteOf(quiet('2026-11-09T12:00:30Z'), now), 'No activity')
  // A wait the tool reports needs no note: Needs you already says it.
  assert.equal(quietNoteOf(row('a', 'blocked', { tool: 'codex', activity: 'waiting' }), now), undefined)
  assert.equal(quietNoteOf(quiet('not a time'), now), undefined)
  assert.equal(quietNoteOf(row('a', 'unread', { tool: 'codex', quietSince: '2026-11-09T11:53:00Z' }), now), undefined)
})

test('the meta line says only the tool, the place, the folder and the age, whatever the row waits for', () => {
  const now = Date.parse('2026-11-09T12:05:00Z')
  const rows = [
    row('a', 'blocked', { folder: '/tmp/shop', waiting: { for: 'permission', tool: 'Bash', detail: 'git push' } }),
    row('a', 'blocked', { folder: '/tmp/shop', waiting: { for: 'answer' } }),
    row('a', 'blocked', { folder: '/tmp/shop', tool: 'codex', activity: 'waiting', quietSince: '2026-11-09T11:53:00Z' }),
    row('a', 'blocked', { folder: '/tmp/shop', helpers: [{ tool: 'claude', id: 'h', title: 'h', state: 'working', updatedAt: '2026-11-09T12:00:00Z' }], workingSubagents: 1 }),
  ]
  for (const conversation of rows) {
    assert.match(detailsOf(conversation, now), /^(Claude Code|Codex) · shop · 5m$/)
  }
  assert.equal(detailsOf(row('a', 'blocked', { tool: 'codex', app: 'cloud', waiting: { for: 'permission' } }), now), 'Codex · Cloud · 5m')
})

test('a pending approval names what it approves, in the words each tool calls for', () => {
  const waiting = (tool, detail) => approvalOf(row('a', 'blocked', { waiting: { for: 'permission', ...(tool ? { tool } : {}), ...(detail ? { detail } : {}) } }))
  assert.deepEqual(waiting('Bash', 'git push origin…'), { icon: 'key', label: 'Approve', code: 'git push origin…', text: 'Approve git push origin…' })
  assert.deepEqual(waiting('Edit', 'dashboard.tsx'), { icon: 'key', label: 'Approve edit', code: 'dashboard.tsx', text: 'Approve edit dashboard.tsx' })
  assert.deepEqual(waiting('Write', 'notes.md'), { icon: 'key', label: 'Approve edit', code: 'notes.md', text: 'Approve edit notes.md' })
  assert.deepEqual(waiting('Read', '.env.example'), { icon: 'key', label: 'Approve read', code: '.env.example', text: 'Approve read .env.example' })
  assert.deepEqual(waiting('WebFetch', 'github.com'), { icon: 'key', label: 'Approve', code: 'github.com', text: 'Approve github.com' })
  // An MCP tool reads as words, not code; its server name never shows as an id.
  assert.deepEqual(waiting('mcp__bfe9f18c-b8fd-43ae-b28d-268779cd446a__notion-fetch', 'Notion fetch'), { icon: 'key', label: 'Approve Notion fetch', text: 'Approve Notion fetch' })
  // A withheld detail leaves the tool's name; an MCP tool's raw name is not shown.
  assert.deepEqual(waiting('Bash'), { icon: 'key', label: 'Approve Bash', text: 'Approve Bash' })
  assert.deepEqual(waiting('mcp__claude_ai_Notion__notion-fetch'), { icon: 'lock', label: 'Needs permission', text: 'Needs permission' })
  // A permission whose call is unknown, as a Codex cloud thread's.
  assert.deepEqual(waiting(), { icon: 'lock', label: 'Needs permission', text: 'Needs permission' })
})

test('a question, a wait of unknown kind and a row that waits for nothing show no approval', () => {
  assert.equal(approvalOf(row('a', 'blocked', { waiting: { for: 'answer' }, reply: 'Overwrite the files?' })), undefined)
  assert.equal(approvalOf(row('a', 'blocked', { activity: 'waiting' })), undefined)
  assert.equal(approvalOf(row('a', 'working')), undefined)
})

test('the tooltip keeps what the row no longer shows: the quiet time, and the reply an approval replaced', () => {
  const now = Date.parse('2026-11-09T12:00:00Z')
  const open = { kind: 'focus', label: 'Open in Terminal' }
  const approving = row('a', 'blocked', { open, folder: '/tmp/shop', reply: 'Pushing the branch.', waiting: { for: 'permission', tool: 'Bash', detail: 'git push' } })
  assert.equal(rowTooltipOf(approving, now), 'Open in Terminal\n/tmp/shop\nPushing the branch.')
  const asking = row('a', 'blocked', { open, folder: '/tmp/shop', reply: 'Overwrite the files?', waiting: { for: 'answer' } })
  assert.equal(rowTooltipOf(asking, now), 'Open in Terminal\n/tmp/shop')
  const quiet = row('a', 'blocked', { open, tool: 'codex', activity: 'waiting', quietSince: '2026-11-09T11:53:00Z' })
  assert.equal(rowTooltipOf(quiet, now), 'Open in Terminal\nNo activity for 7m')
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

test('mark all complete skips a row that changed since it was drawn, left Active, or left the list', () => {
  const shown = row('a', 'active')
  const view = (...rows) => ({ conversations: rows })
  assert.equal(stillActive(view(shown), shown), true)
  assert.equal(stillActive(view(row('a', 'active', { updatedAt: '2026-11-09T12:05:00Z' })), shown), false)
  assert.equal(stillActive(view(row('a', 'unread', { updatedAt: '2026-11-09T12:05:00Z' })), shown), false)
  assert.equal(stillActive(view(row('a', 'working')), shown), false)
  assert.equal(stillActive(view(), shown), false)
  assert.equal(stillActive(undefined, shown), false)
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

test('names a Claude chat and a Cowork task after Claude, not Claude Code', () => {
  const now = Date.parse('2026-11-09T12:05:00Z')
  assert.equal(detailsOf(row('6dad25fc-d7d3-413d-92de-b34e6d3aed07', 'unread', { app: 'chat' }), now), 'Claude · Chat · 5m')
  assert.equal(detailsOf(row('local_a', 'active', { app: 'cowork', folder: '/tmp/notes' }), now), 'Claude · Cowork · notes · 5m')
  assert.equal(detailsOf(row('6abcd7fe-837c-83ed-919e-a699ea6b4809', 'unread', { tool: 'chatgpt', app: 'chat' }), now), 'ChatGPT · Chat · 5m')
})

test('places chats with the cloud and Cowork tasks on this machine', () => {
  const view = { needsYou: 2, conversations: [row('chat', 'unread', { app: 'chat' }), row('task', 'unread', { app: 'cowork' }), row('session', 'active', { app: 'cli' })] }
  assert.deepEqual(viewIn(view, 'cloud').conversations.map((c) => c.id), ['chat'])
  assert.equal(viewIn(view, 'cloud').needsYou, 1)
  assert.deepEqual(viewIn(view, 'local').conversations.map((c) => c.id), ['task', 'session'])
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

test('refuses the keys every app uses to edit, quit or switch, and takes the rest', () => {
  for (const [shortcut, isMac] of [['Cmd+Q', true], ['Cmd+C', true], ['Cmd+V', true], ['Cmd+Shift+Z', true], ['Cmd+Space', true], ['Cmd+Tab', true], ['Ctrl+Tab', true], ['Ctrl+C', true], ['Ctrl+C', false], ['Ctrl+Shift+Tab', false], ['Alt+Tab', false], ['Alt+F4', false], ['Ctrl+Alt+Backspace', false], ['Ctrl+Alt+Delete', false]]) {
    assert.equal(refusedShortcutOf(shortcut, isMac), `${shortcut} is a shortcut every app uses; choose other keys`, shortcut)
  }
  for (const [shortcut, isMac] of [['Ctrl+Alt+Space', true], ['Ctrl+Alt+V', true], ['Cmd+Shift+K', true], ['Cmd+Shift+C', true], ['Super+C', false], ['Ctrl+Alt+Enter', false]]) {
    assert.equal(refusedShortcutOf(shortcut, isMac), undefined, shortcut)
  }
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

test('F2 alone starts a rename, as in a file manager', () => {
  assert.equal(startsRename({ key: 'F2' }), true)
  assert.equal(startsRename({ key: 'F2', metaKey: true }), false)
  assert.equal(startsRename({ key: 'F2', ctrlKey: true }), false)
  assert.equal(startsRename({ key: 'F2', altKey: true }), false)
  assert.equal(startsRename({ key: 'F2', shiftKey: true }), false)
  assert.equal(startsRename({ key: 'F2', repeat: true }), false)
  assert.equal(startsRename({ key: 'Enter' }), false)
})

test('closes the nearest thing on Escape: the row menu, a rename, the settings, then the panel', () => {
  assert.equal(escapeTarget({ deleteConfirmOpen: true, rowMenuOpen: true, renaming: true, settingsOpen: true }), 'delete-confirm')
  assert.equal(escapeTarget({ rowMenuOpen: true, renaming: true, settingsOpen: true }), 'row-menu')
  assert.equal(escapeTarget({ rowMenuOpen: false, renaming: true, settingsOpen: true }), 'rename')
  assert.equal(escapeTarget({ rowMenuOpen: false, renaming: false, settingsOpen: true }), 'settings')
  assert.equal(escapeTarget({ rowMenuOpen: false, renaming: false, settingsOpen: false }), 'panel')
})

test('Escape leaves pick mode before it closes the panel, and after the settings', () => {
  assert.equal(escapeTarget({ picking: true }), 'pick')
  assert.equal(escapeTarget({ picking: true, settingsOpen: true }), 'settings')
  assert.equal(escapeTarget({ picking: false }), 'panel')
})

test('pick mode never offers Archived, and archived rows never take the text', () => {
  const rows = [row('a', 'blocked'), row('b', 'working'), row('c', 'active'), row('d', 'completed'), row('e', 'dormant')]
  assert.deepEqual(grouped(pickViewOf({ conversations: rows, needsYou: 1 }).conversations).map((group) => group.id), ['needs-you', 'working', 'active'])
  assert.deepEqual(['blocked', 'unread', 'working', 'active', 'completed', 'dormant'].map((state) => takesText(row('x', state))), [true, true, true, true, false, false])
})

test('pick mode counts only the rows it offers, so a search foot never counts an archived match', () => {
  const view = { conversations: [row('a', 'unread'), row('d', 'completed'), row('e', 'dormant')], needsYou: 1, matches: 3 }
  assert.deepEqual(pickViewOf(view), { conversations: [row('a', 'unread')], needsYou: 1, matches: 1 })
  assert.deepEqual(pickViewOf({ conversations: [row('d', 'dormant')], needsYou: 0 }), { conversations: [], needsYou: 0 })
  assert.equal(summaryOf(pickViewOf({ conversations: [row('d', 'dormant')], needsYou: 0, matches: 1 }), 'go'), 'No conversation matches')
})

test('pick mode keeps the selected row while it is offered, else selects the first row the text can be typed into, else the first row', () => {
  assert.equal(pickKeyOf(['a', 'b', 'c'], ['c'], 'b'), 'b')
  assert.equal(pickKeyOf(['a', 'b', 'c'], ['c'], 'gone'), 'c')
  assert.equal(pickKeyOf(['a', 'b', 'c'], ['c'], undefined), 'c')
  assert.equal(pickKeyOf(['a', 'b'], ['folded'], 'gone'), 'a')
  assert.equal(pickKeyOf([], [], 'gone'), undefined)
})

test('only live rows in a Terminal or Ghostty tab, outside Archived, can be typed into', () => {
  const tab = (app) => ({ live: true, open: { kind: 'focus', label: 'Switch to ' + app } })
  const rows = [
    row('ghostty', 'unread', tab('Ghostty')),
    row('terminal', 'working', tab('Terminal')),
    row('iterm', 'active', tab('iTerm')),
    row('ended', 'active', { open: { kind: 'terminal', label: 'Open in Ghostty' } }),
    row('not-live', 'active', { ...tab('Ghostty'), live: false }),
    row('editor', 'active', { live: true, open: { kind: 'link', label: 'Open in Cursor' } }),
    row('archived', 'completed', tab('Ghostty')),
    row('asks', 'blocked', tab('Ghostty')),
    row('asks-seen', 'working', { ...tab('Ghostty'), activity: 'waiting' }),
    row('codex-works', 'working', { ...tab('Ghostty'), tool: 'codex' }),
    row('codex-idle', 'active', { ...tab('Ghostty'), tool: 'codex' }),
    row('cursor-idle', 'active', { ...tab('Ghostty'), tool: 'cursor' }),
  ]
  assert.deepEqual(typableRows(rows).map((r) => r.id), ['ghostty', 'terminal', 'codex-idle'])
  assert.deepEqual(typableRows([rows[1], rows[2]]).map((r) => r.id), ['terminal'])
  assert.deepEqual(typableRows([]), [])
})

test('the send shortcut sends straight only when exactly one row can be typed into', () => {
  const ghostty = { live: true, open: { kind: 'focus', label: 'Switch to Ghostty' } }
  const one = row('one', 'active', ghostty)
  const other = row('other', 'unread', ghostty)
  const ended = row('ended', 'active')
  assert.equal(straightSendOf([ended, one, row('done', 'completed', ghostty)]), one)
  assert.equal(straightSendOf([ended]), undefined)
  assert.equal(straightSendOf([one, other]), undefined)
  assert.equal(straightSendOf([]), undefined)
  // A folded group hides its rows: the one row the text could go to must show.
  assert.equal(straightSendOf([ended, one], ['claude:ended', 'claude:one']), one)
  assert.equal(straightSendOf([ended, one], ['claude:ended']), undefined)
  assert.equal(sendingTitleOf(one), 'Sending to one')
  assert.equal(sendingTitleOf({ ...one, title: '' }), 'Sending to Untitled conversation')
  assert.equal(sendingTitleOf(one, { format: 'markdown' }), 'Sending to one')
})

test('pick mode opens only on copied text, and names it by its words', () => {
  const clip = { preview: 'Fix the flaky login test. Then run it', words: 8, chars: 37, hash: 'abc' }
  assert.equal(pickableClip(clip), clip)
  assert.equal(pickableClip({ preview: '', words: 0, chars: 0, hash: '' }), undefined)
  assert.equal(pickableClip(undefined), undefined)
  assert.equal(pickTitleOf(clip), 'Click a chat or press Enter to paste these 8 words there')
  assert.equal(pickTitleOf({ ...clip, words: 1 }), 'Click a chat or press Enter to paste this word there')
  assert.equal(pickTitleOf({ ...clip, format: 'markdown', words: 412 }), 'Click a chat or press Enter to paste these 412 words there')
  assert.equal(unreadableClipOf(new Error('The companion could not reach its own shell.')), 'Could not read the copied text. The companion could not reach its own shell.')
  assert.equal(chipTextOf(clip), 'Fix the flaky login…')
  assert.equal(chipTextOf({ ...clip, preview: 'Two words' }), 'Two words')
  assert.equal(NOTHING_TO_SEND, 'Nothing to send: copy some text first.')
})

test('Cmd+Shift+V on a Mac, Ctrl+Shift+V elsewhere, toggles pick mode', () => {
  const press = (extra) => ({ code: 'KeyV', shiftKey: true, ...extra })
  assert.equal(togglesPick(press({ metaKey: true }), true), true)
  assert.equal(togglesPick(press({ ctrlKey: true }), false), true)
  assert.equal(togglesPick(press({ ctrlKey: true }), true), false)
  assert.equal(togglesPick(press({ metaKey: true, shiftKey: false }), true), false)
  assert.equal(togglesPick(press({ metaKey: true, altKey: true }), true), false)
  assert.equal(togglesPick(press({ metaKey: true, repeat: true }), true), false)
  assert.equal(togglesPick({ code: 'KeyC', metaKey: true, shiftKey: true }, true), false)
})

test('a failed open keeps its reason, and the resume command only when the server sent one', () => {
  assert.equal(openFailureOf({ opened: true, message: 'Switched to the Ghostty tab that runs it.' }), undefined)
  assert.deepEqual(openFailureOf({ opened: false, message: 'It runs in Warp on ttys021 (process 42).' }), { message: 'It runs in Warp on ttys021 (process 42).' })
  assert.deepEqual(openFailureOf({ opened: false, message: 'macOS blocked opening Terminal.', command: "cd '/tmp/app' && claude --resume x" }), {
    message: 'macOS blocked opening Terminal.',
    command: "cd '/tmp/app' && claude --resume x",
  })
})

test('a notice is an error when what was asked did not happen, and info when it happened with a detail left to say', () => {
  assert.deepEqual(
    Object.fromEntries(Object.entries(NOTICES).map(([name, notice]) => [name, notice.tone])),
    {
      notOpened: 'error',
      textChanged: 'error',
      notTyped: 'error',
      pasteByHand: 'info',
      cannotDelete: 'error',
      notDeleted: 'error',
      notTidied: 'info',
      shellNotStopped: 'error',
      shellProcessesLeft: 'info',
      sessionNotStopped: 'error',
      subagent: 'info',
    },
  )
  for (const notice of Object.values(NOTICES)) assert.notEqual(notice.label, '')
  assert.equal(new Set(Object.values(NOTICES).map((notice) => notice.label)).size, Object.keys(NOTICES).length)
})

test('an error is read out at once as an alert, and anything else waits its turn as a status', () => {
  assert.equal(roleOfTone('error'), 'alert')
  assert.equal(roleOfTone('info'), 'status')
  assert.equal(roleOfTone(undefined), 'status')
})

test('a send that typed nothing is info only when the server switched to the conversation for a paste by hand', () => {
  // The sentence pasteByHand in internal/conversations/send.go ends with when the switch worked.
  assert.equal(SWITCHED_TO_PASTE, 'Switched to it. Paste with Cmd+V.')
  assert.equal(unsentNoticeOf('Switched to it. Paste with Cmd+V.'), NOTICES.pasteByHand)
  assert.equal(unsentNoticeOf('It waits for your answer, and typed text could answer its question, so nothing was typed. Switched to it. Paste with Cmd+V.'), NOTICES.pasteByHand)
  assert.equal(unsentNoticeOf('Codex runs in an app that cannot be brought forward. The text is on the clipboard: paste it with Cmd+V.'), NOTICES.notTyped)
  assert.equal(unsentNoticeOf('It is not running, so nothing was typed. Open it to resume it, then paste with Cmd+V.'), NOTICES.notTyped)
  assert.equal(unsentNoticeOf('agnostic-ai serve did not answer.'), NOTICES.notTyped)
  // The server counts a window it brought forward without reaching the chat as not opened, as a click on the row does.
  assert.equal(unsentNoticeOf('Brought its Cursor window to the front, but the terminal inside it cannot be selected from outside. The text is on the clipboard: paste it with Cmd+V.'), NOTICES.notTyped)
  assert.equal(unsentNoticeOf(undefined), NOTICES.notTyped)
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

const shared = (id, state, extra = {}) =>
  row(id, state, { app: 'cli', live: true, cloud: { id: 'cse_a1', link: 'https://claude.ai/code/session_a1' }, ...extra })

test('says Shared for a local session that is also open on claude.ai', () => {
  const now = Date.parse('2026-11-09T12:05:00Z')
  assert.equal(detailsOf(shared('a', 'active', { folder: '/tmp/app' }), now), 'Claude Code · Shared · app · 5m')
})

test('a shared row shows under Local and under Cloud, and counts once in each', () => {
  const view = { needsYou: 2, conversations: [shared('a', 'blocked'), row('b', 'unread', { app: 'cloud' }), row('c', 'active', { app: 'cli' })] }
  assert.deepEqual(viewIn(view, 'local').conversations.map((r) => r.id), ['a', 'c'])
  assert.deepEqual(viewIn(view, 'cloud').conversations.map((r) => r.id), ['a', 'b'])
  assert.equal(viewIn(view, 'local').needsYou, 1)
  assert.equal(viewIn(view, 'cloud').needsYou, 2)
})

test('offers Open on claude.ai only on a shared row, and names its cloud session', () => {
  assert.deepEqual(rowMenuOf(shared('a', 'active')), ['rename', 'copy-session-id', 'send-to', 'claim-files', 'complete', 'web'])
  assert.deepEqual(rowMenuOf(shared('a', 'blocked')), ['read', 'rename', 'copy-session-id', 'send-to', 'claim-files', 'complete', 'web'])
  assert.deepEqual(rowMenuOf(row('a', 'active', { app: 'cloud' })), ['rename', 'copy-session-id', 'complete'])
  const sharedRow = shared('a', 'active')
  assert.deepEqual(cloudPageOf(sharedRow), { tool: 'claude', id: 'cse_a1', title: sharedRow.title })
  assert.equal(cloudPageOf(row('a', 'active')), undefined)
})

const helper = (id, state, minutesAgo, extra = {}) => ({ tool: 'claude', id, title: id, state, updatedAt: new Date(Date.parse('2026-11-09T12:00:00Z') - minutesAgo * 60000).toISOString(), ...extra })

test('the subagent toggle counts working subagents only, in plain words, and says whether it is unfolded', () => {
  assert.equal(helpersToggleOf(row('a', 'working', { helpers: [helper('h', 'working', 1)] }), false), '▸ 1 subagent')
  const mixed = row('a', 'working', { helpers: [helper('h1', 'working', 1), helper('h2', 'working', 2), helper('h3', 'finished', 3)] })
  assert.equal(helpersToggleOf(mixed, false), '▸ 2 subagents')
  assert.equal(helpersToggleOf(mixed, true), '▾ 2 subagents')
})

test('a finished subagent leaves the list; a chat whose subagents all finished lists none', () => {
  const done = row('a', 'working', { helpers: [helper('h1', 'finished', 1), helper('h2', 'finished', 2)] })
  assert.deepEqual(workingHelpersOf(done), [])
  assert.deepEqual(helperRowsOf(done, true), { rows: [], more: 0 })
  assert.deepEqual(workingHelpersOf(row('b', 'working')), [], 'a chat without the field lists none')
})

test('shows working subagents newest first, five at most until all are asked for', () => {
  const helpers = [
    helper('done-new', 'finished', 1),
    helper('work-old', 'working', 9),
    helper('work-older', 'working', 20),
    helper('work-new', 'working', 2),
    helper('work-mid', 'working', 5),
    helper('work-oldest', 'working', 40),
    helper('work-ancient', 'working', 50),
  ]
  const conversation = row('a', 'working', { helpers })
  const folded = helperRowsOf(conversation, false)
  assert.equal(HELPERS_SHOWN, 5)
  assert.deepEqual(folded.rows.map((h) => h.id), ['work-new', 'work-mid', 'work-old', 'work-older', 'work-oldest'])
  assert.equal(folded.more, 1)
  const all = helperRowsOf(conversation, true)
  assert.deepEqual(all.rows.map((h) => h.id), ['work-new', 'work-mid', 'work-old', 'work-older', 'work-oldest', 'work-ancient'])
  assert.equal(all.more, 0)
  assert.deepEqual(helpers.map((h) => h.id)[0], 'done-new', 'the server order is left as it came')
})

test('a subagent shows its latest message as one line, and none before it wrote one', () => {
  assert.equal(helperReplyOf(helper('h', 'working', 1, { reply: '  Looking in internal/cli first. ' })), 'Looking in internal/cli first.')
  assert.equal(helperReplyOf(helper('h', 'working', 1)), '')
  assert.equal(helperReplyOf(helper('h', 'working', 1, { reply: '   ' })), '')
})

test('a subagent names its tool, its type when it has a telling one, and its age', () => {
  const now = Date.parse('2026-11-09T12:00:00Z')
  assert.equal(helperDetailsOf(helper('h', 'working', 2, { tool: 'codex' }), now), 'Codex · 2m')
  assert.equal(helperDetailsOf(helper('h', 'working', 1, { kind: 'Explore' }), now), 'Claude Code · Explore · 1m')
  assert.equal(helperDetailsOf(helper('h', 'working', 1, { kind: 'general-purpose', tool: 'codex' }), now), 'Codex · 1m')
  assert.equal(helperTitleOf(helper('h', 'working', 4, { title: '' })), 'Untitled subagent')
})

test('a Codex subagent opens its own thread; a Claude Code subagent opens the chat it runs in', () => {
  const parent = row('parent', 'working')
  const run = helper('run-1', 'working', 1, { tool: 'codex' })
  assert.deepEqual(helperOpenOf(parent, run), { tool: 'codex', id: 'run-1', title: helperTitleOf(run) })
  assert.deepEqual(helperOpenOf(parent, helper('agent-1', 'working', 1)), { tool: 'claude', id: 'parent', title: 'parent', helper: 'agent-1' })
})

test('a subagent row says what its click does', () => {
  assert.equal(helperTooltipOf(helper('run-1', 'working', 1, { tool: 'codex' })), 'Open this Codex thread')
  assert.equal(helperTooltipOf(helper('agent-1', 'working', 1)), 'Open the chat it runs in, and show how to reach this subagent')
})

test('an open of a Claude Code subagent leaves the reply as a notice; any other open leaves none', () => {
  const said = 'Opened the chat it runs in. Its subagent is \'Find call sites\'. Use the agent rows below the prompt to switch to it.'
  assert.deepEqual(subagentNoticeOf({ opened: true, message: said, helper: { tool: 'claude', id: 'explore' } }), { message: said, hint: true })
  assert.equal(subagentNoticeOf({ opened: true, message: 'Switched to the Terminal tab that runs it.' }), undefined)
  assert.equal(subagentNoticeOf({ opened: false, message: 'That subagent no longer works in this chat.' }), undefined)
})

test('a late open failure replaces no notice or a subagent hint, and never another failure', () => {
  assert.equal(lateFailureReplaces(undefined), true)
  assert.equal(lateFailureReplaces(subagentNoticeOf({ opened: true, message: 'Opened.', helper: { tool: 'claude', id: 'x' } })), true)
  assert.equal(lateFailureReplaces({ message: 'Terminal could not be opened.' }), false)
})

test('a row with an unread marker may still be marked read while the list holds it unchanged', () => {
  const shown = row('a', 'active', { unread: true, updatedAt: '2026-11-09T12:00:00Z' })
  assert.equal(stillWaits({ conversations: [shown] }, shown), true)
  assert.equal(stillWaits({ conversations: [row('a', 'active', { updatedAt: '2026-11-09T12:00:00Z' })] }, shown), false)
  assert.equal(stillWaits({ conversations: [row('a', 'dormant', { unread: true, updatedAt: '2026-11-09T12:00:00Z' })] }, shown), false)
})

test('a group header names the unread chats it holds, and an archived one adds nothing', () => {
  assert.equal(unreadLabelOf([row('a', 'active', { unread: true }), row('b', 'active'), row('c', 'active', { unread: true })]), '2 unread')
  assert.equal(unreadLabelOf([row('a', 'dormant', { unread: true }), row('c', 'completed', { unread: true })]), undefined)
  assert.equal(unreadLabelOf([row('a', 'dormant')]), undefined)
})

test('a resumed chat with an unread marker offers Mark as read, and its tooltip says so', () => {
  const open = { kind: 'focus', label: 'Switch to Ghostty' }
  assert.deepEqual(rowMenuOf(row('a', 'active', { unread: true, live: true, open })), ['read', 'rename', 'copy-session-id', 'send-to', 'claim-files', 'complete'])
  assert.equal(rowTooltipOf(row('a', 'active', { unread: true, open }), Date.parse('2026-11-09T12:00:00Z')), 'Switch to Ghostty\nResumed with a reply from before that you have not read')
})

test('a late open failure the person dismissed while it was read does not show again', () => {
  const failure = { id: 3, title: 'Tidy the old branches', message: 'macOS blocked opening Terminal.' }
  assert.equal(lateOpenFailureOf(failure, 3), undefined)
  assert.equal(lateOpenFailureOf(failure, 2).id, 3)
})

test('a late open failure names its chat when the list holds it', () => {
  assert.deepEqual(lateOpenFailureOf({ id: 3, title: 'Tidy the old branches', message: 'macOS blocked opening Terminal.', command: 'claude --resume done-2' }), {
    message: 'Tidy the old branches: macOS blocked opening Terminal.',
    late: true,
    id: 3,
    command: 'claude --resume done-2',
  })
  assert.deepEqual(lateOpenFailureOf({ id: 4, message: 'agnostic-ai did not answer.' }), { message: 'agnostic-ai did not answer.', late: true, id: 4 })
})

// What the person sees, per mode (issue #3700). The server shapes each row from the decision log: a request the
// approvals hook approved, or may still answer, comes as working; one it left to the person comes blocked with a reason.
const permission = (id, extra = {}) => row(id, 'blocked', { live: true, activity: 'waiting', waiting: { for: 'permission', tool: 'Bash', detail: 'git push' }, ...extra })
const approvedRow = (id, approved) => row(id, 'working', { live: true, activity: 'working', approvals: { approved, recent: [{ time: '2026-11-09T11:59:00Z', call: 'Bash', detail: 'go test ./...' }] } })
const needsYouIds = (view) => grouped(view.conversations).find((group) => group.id === 'needs-you')?.rows.map((r) => r.id) ?? []

test('in ask every permission wait is a Needs permission row that counts, as today, with no reason', () => {
  const view = { needsYou: 1, conversations: [permission('a')] }
  assert.deepEqual(needsYouIds(view), ['a'])
  assert.deepEqual(waitingOf(view), { blocked: 1, unread: 0 })
  assert.equal(approvalReasonOf(view.conversations[0]), undefined)
  assert.equal(approvalsHeaderOf(view), undefined)
})

for (const mode of ['review', 'yolo']) {
  test(`in ${mode} an approved request never shows under Needs you nor counts, and its row says how many were approved`, () => {
    const view = { approvalsMode: mode, needsYou: 0, conversations: [approvedRow('a', 3)] }
    assert.deepEqual(needsYouIds(view), [])
    assert.deepEqual(waitingOf(view), { blocked: 0, unread: 0 })
    assert.equal(approvedLabelOf(view.conversations[0]), '3 approved for you')
    assert.equal(approvalOf(view.conversations[0]), undefined)
  })

  test(`in ${mode} a request the gate sends to the person shows under Needs you, counts, and names the rule`, () => {
    const asked = permission('a', { approvals: { reason: 'Always asks: publishing', approved: 2 } })
    const view = { approvalsMode: mode, needsYou: 1, conversations: [asked] }
    assert.deepEqual(needsYouIds(view), ['a'])
    assert.deepEqual(waitingOf(view), { blocked: 1, unread: 0 })
    assert.equal(approvalReasonOf(asked), 'Always asks: publishing')
    assert.equal(approvedLabelOf(asked), '2 approved for you')
  })

  test(`in ${mode} a tool with no route shows its prompt and says approvals do not reach it`, () => {
    const unsupported = permission('a', { tool: 'cursor', approvals: { reason: 'Approvals not supported for Cursor' } })
    const view = { approvalsMode: mode, needsYou: 1, conversations: [unsupported] }
    assert.deepEqual(waitingOf(view), { blocked: 1, unread: 0 })
    assert.equal(approvalReasonOf(unsupported), 'Approvals not supported for Cursor')
  })
}

test('the header names the mode only while approvals answer prompts', () => {
  assert.deepEqual(approvalsHeaderOf({ approvalsMode: 'yolo' }), { mode: 'yolo', label: 'Approvals: routine prompts are approved for you' })
  assert.deepEqual(approvalsHeaderOf({ approvalsMode: 'review' }), { mode: 'review', label: "Approvals: your tools' reviewers answer routine prompts" })
  assert.equal(approvalsHeaderOf({ approvalsMode: 'ask' }), undefined)
  assert.equal(approvalsHeaderOf({}), undefined)
  assert.equal(approvalsHeaderOf(undefined), undefined)
})

test('a reason shows only on a row that waits for a permission', () => {
  assert.equal(approvalReasonOf(row('a', 'working', { approvals: { reason: 'Always asks: publishing' } })), undefined)
  assert.equal(approvalReasonOf(permission('a', { approvals: { reason: '' } })), undefined)
})

test('the approved requests fold out newest first, with the detail when the server kept one', () => {
  const conversation = row('a', 'active', { approvals: { approved: 2, recent: [{ time: '2026-11-09T11:59:00Z', call: 'Bash', detail: 'go test ./...' }, { time: '2026-11-09T11:58:00Z', call: 'Edit' }] } })
  assert.deepEqual(approvedCallsOf(conversation), [{ text: 'Bash go test ./...', time: '2026-11-09T11:59:00Z' }, { text: 'Edit', time: '2026-11-09T11:58:00Z' }])
  assert.equal(approvedToggleOf(conversation, false), '▸ 2 approved for you')
  assert.equal(approvedToggleOf(conversation, true), '▾ 2 approved for you')
  assert.equal(approvedToggleOf(row('b', 'active'), false), undefined)
  assert.deepEqual(approvedCallsOf(row('b', 'active')), [])
})

test('the tooltip of a permission row names why approvals left it to the person', () => {
  const asked = permission('a', { folder: '/w/app', open: { label: 'Switch to Ghostty' }, approvals: { reason: 'Always asks: publishing' } })
  assert.equal(rowTooltipOf(asked, Date.parse('2026-11-09T12:00:00Z')), 'Switch to Ghostty\n/w/app\nAlways asks: publishing')
})

const shell = { pid: 42, procStart: 'shell-start', command: 'npm run dev', since: '2026-11-09T10:30:00Z', processCount: 3 }
const shellNow = Date.parse('2026-11-09T12:00:00Z')

test('a saved completion offers Undo complete, and an archived row can still be hidden', () => {
  const completedAt = '2026-11-09T13:00:00Z'
  assert.deepEqual(rowMenuOf(row('app-archive', 'dormant', { completedAt })), ['rename', 'copy-session-id', 'send-to', 'claim-files', 'reopen', 'hide', 'delete'])
  const waiting = row('shell-wait', 'blocked', { completedAt, live: true, waiting: { for: 'shell' }, stuckShells: [shell] })
  assert.deepEqual(rowMenuOf(waiting), ['read', 'rename', 'copy-session-id', 'send-to', 'claim-files', 'reopen', 'stop-shell'])
  assert.equal(rowActionOf(waiting), undefined)
})

test('offers Stop chat only on a live chat the server can stop, and says how it ended', () => {
  const stoppable = { pid: 42, procStart: 'chat-start' }
  assert.deepEqual(rowMenuOf(row('a', 'working', { live: true, stoppable })), ['rename', 'copy-session-id', 'send-to', 'claim-files', 'stop-session'])
  assert.deepEqual(rowMenuOf(row('a', 'working', { stoppable })), ['rename', 'copy-session-id', 'send-to', 'claim-files'])
  assert.deepEqual(rowMenuOf(row('a', 'working', { live: true })), ['rename', 'copy-session-id', 'send-to', 'claim-files'])
  assert.deepEqual(stopSessionConfirmOf(row('a', 'working', { title: 'Fix the build' })).title, 'Stop "Fix the build"?')
  assert.match(stopSessionConfirmOf(row('a', 'working')).body, /transcript stays/)
  assert.deepEqual(stoppedSessionOf({}), { message: 'Stopped.' })
  assert.deepEqual(stoppedSessionOf({ forced: true }), { message: 'Stopped. It did not end when asked, so it was forced to.' })
  assert.deepEqual(stoppedSessionOf({ stillRunning: true }), {
    message: 'The Claude Code process is still running after the stop request.',
    problem: true,
  })
  assert.deepEqual(stoppedSessionOf({ forced: true, stillRunning: true }), {
    message: 'The Claude Code process is still running after a force stop.',
    problem: true,
  })
  assert.equal(escapeTarget({ stopSessionOpen: true, rowMenuOpen: true }), 'stop-session')
})

test('warns about each shell with its age, command and owner when known', () => {
  const conversation = row('a', 'blocked', { live: true, stuckShells: [shell, { ...shell, pid: 45, command: 'npm run watch', helperName: 'Faramir' }] })
  assert.deepEqual(rowMenuOf(conversation), ['read', 'rename', 'copy-session-id', 'send-to', 'claim-files', 'complete', 'stop-shell'])
  assert.deepEqual(rowMenuOf({ ...conversation, live: false }), ['read', 'rename', 'copy-session-id', 'send-to', 'claim-files', 'complete'])
  assert.deepEqual(rowMenuOf({ ...conversation, stuckShells: [] }), ['read', 'rename', 'copy-session-id', 'send-to', 'claim-files', 'complete'])
  assert.deepEqual(rowMenuOf({ ...conversation, state: 'working' }), ['rename', 'copy-session-id', 'send-to', 'claim-files', 'stop-shell'])
  assert.deepEqual(conversation.stuckShells.map((entry) => shellWarningOf(entry, shellNow)), [
    'Shell running for 1h: npm run dev',
    'Faramir left a shell running for 1h: npm run watch',
  ])
  assert.equal(shellWarningOf({ ...shell, since: '2026-11-09T11:59:30Z' }, shellNow), 'Shell running for less than a minute: npm run dev')
})

test('shows a label for a credential-filtered shell command', () => {
  const filtered = { ...shell, command: '' }
  assert.equal(shellCommandOf(filtered), 'Command hidden')
  assert.equal(shellWarningOf(filtered, shellNow), 'Shell running for 1h: Command hidden')
})

test('confirms only an unchanged shell and names the processes still running after a stop', () => {
  const conversation = row('a', 'blocked', { live: true, stuckShells: [shell] })
  assert.equal(currentShellOf(conversation, shell), shell)
  assert.equal(currentShellOf(conversation, { ...shell, procStart: 'later' }), undefined)
  assert.equal(currentShellOf(conversation, { ...shell, processCount: 4 }), undefined)
  assert.equal(currentShellOf(conversation, { ...shell, command: 'npm run watch' }), undefined)
  assert.equal(currentShellOf({ ...conversation, live: false }, shell), undefined)
  assert.equal(stopShellDetailsOf(shell, shellNow), 'Running for 1h. This stops 3 processes.')
  assert.equal(stopShellDetailsOf({ ...shell, processCount: 1 }, shellNow), 'Running for 1h. This stops 1 process.')
  assert.equal(stoppedShellOf({}), 'Shell stopped.')
  assert.equal(stoppedShellOf({ stillRunning: [{ pid: 91 }, { pid: 92 }] }), 'Shell stop sent. Processes still running (PIDs 91, 92).')
})

test('Escape closes the stop shell confirmation before returning to the row menu', () => {
  assert.equal(escapeTarget({ stopShellOpen: true, rowMenuOpen: true }), 'stop-shell')
})

test('compares two stated model names without treating the tool as a model', async () => {
  const { keyOf, reviewModelsOf } = await import('../frontend/model.js')
  const sender = { tool: 'claude', id: 'a' }
  const reviewer = { tool: 'codex', id: 'b' }
  const models = {
    [keyOf(sender)]: { name: ' GPT-5.5 ', statedBy: 'human' },
    [keyOf(reviewer)]: { name: 'gpt-5.5', statedBy: 'human' },
  }
  assert.deepEqual(reviewModelsOf(models[keyOf(sender)].name, models[keyOf(reviewer)].name), {
    from: 'GPT-5.5', to: 'gpt-5.5', same: true,
  })
  assert.deepEqual(reviewModelsOf('Model A', 'Model B'), { from: 'Model A', to: 'Model B', same: false })
  assert.deepEqual(reviewModelsOf('Model A', ' '), { from: 'Model A', to: 'model unknown', same: false })
  assert.deepEqual(reviewModelsOf(undefined, undefined), { from: 'model unknown', to: 'model unknown', same: false })
})

test('names the handoff and claim statuses shown in the panel', async () => {
  const { claimCountOf, claimStatusOf, conflictLabelOf, handoffLabelOf } = await import('../frontend/model.js')
  assert.deepEqual(['offered', 'accepted', 'declined'].map(handoffLabelOf), ['Offered', 'Accepted', 'Declined'])
  assert.equal(claimStatusOf({ stale: true }), 'Stale claim')
  assert.equal(claimStatusOf({}), 'Active claim')
  assert.equal(claimCountOf(1), '1 claim')
  assert.equal(claimCountOf(2), '2 claims')
  assert.equal(conflictLabelOf({ path: 'src/a.go' }, 'Parser session'), 'Path conflict: src/a.go is held by Parser session.')
  assert.equal(conflictLabelOf({ path: 'src/a.go' }), 'Path conflict: no session holds src/a.go.')
})
