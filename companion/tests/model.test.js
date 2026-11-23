import { test } from 'node:test'
import assert from 'node:assert/strict'
import { HELPERS_SHOWN, NOTHING_TO_SEND, NOTICES, PLACES, SWITCHED_TO_PASTE, ago, afterPage, approvalOf, approvalReasonOf, approvalsHeaderOf, approvedCallsOf, approvedLabelOf, approvedToggleOf, chipTextOf, circleLabelOf, cloudPageOf, confirmsCandidate, deleteConfirmOf, deletedOf, detailsOf, escapeTarget, freedOf, folderNamesOf, grouped, helperDetailsOf, helperOpenOf, helperReplyOf, helperRowsOf, helperTitleOf, helperTooltipOf, helperTreeOf, helpersToggleOf, subtreeToggleOf, isDrag, lateFailureReplaces, keyOf, keycapsOf, lateOpenFailureOf, nameOf, neighbour, openFailureOf, pickKeyOf, pickTitleOf, pickViewOf, pickableClip, quietNoteOf, renameOf, renames, replyOf, rowActionOf, rowMenuOf, refusedShortcutOf, rowTooltipOf, roleOfTone, sendingTitleOf, shortcutOf, startsRename, startedFromOf, stillActive, stillWaits, straightSendOf, subagentNoticeOf, unreadLabelOf, unreadMarkOf, summaryOf, takesText, titleOf, togglesPick, typableRows, unreadableClipOf, unsentNoticeOf, viewIn, waitingOf, workingHelpersOf } from '../frontend/model.js'

import { missingToolsNoteOf } from '../frontend/model.js'

import { arrivalOf, handoffSentOf, messageLabelOf, receiversOf, sessionLabelOf, typeHelpOf, typedReceiverOf } from '../frontend/model.js'

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
  assert.deepEqual(waitingOf(view), { blocked: 2, unread: 2, icon: 'lock' })
  assert.deepEqual(waitingOf({ needsYou: 0, conversations: [] }), { blocked: 0, unread: 0, icon: 'terminal' })
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

test('offers Message another session only on local top-level Claude Code and Codex rows', () => {
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

test('says who acts next and keeps thread actions available by state', async () => {
  const { threadActionsOf, threadNextOf, threadsOf } = await import('../frontend/model.js')
  const thread = { status: 'open', count: 3, cap: 8, waiting: 1 }
  assert.deepEqual(threadsOf(row('a', 'active')), [])
  assert.deepEqual(threadsOf(row('a', 'active', { threads: [thread] })), [thread])
  assert.equal(threadNextOf(thread), 'The message is waiting. Open the thread for the recipient and delivery options.')
  assert.equal(threadNextOf({ ...thread, waiting: 0 }), 'You can reply to this thread.')
  assert.equal(threadNextOf({ ...thread, status: 'paused', held: 1 }), 'Resume the thread to send and deliver messages again.')
  assert.equal(threadNextOf({ ...thread, held: 1 }), 'Review the message that needs your approval.')
  assert.deepEqual(threadActionsOf(thread), ['reply', 'pause', 'close', 'stop'])
  assert.deepEqual(threadActionsOf({ ...thread, count: 8 }), ['reply', 'pause', 'close', 'raise-cap', 'stop'])
  assert.deepEqual(threadActionsOf({ ...thread, status: 'paused' }), ['reply', 'resume', 'stop'])
  assert.deepEqual(threadActionsOf({ ...thread, status: 'closing' }), ['stop'])
  assert.deepEqual(threadActionsOf({ ...thread, status: 'closed' }), [])
})

test('labels message states and offers approve, reject and queue only where they apply', async () => {
  const { messageActionsOf, messageStateLabelOf } = await import('../frontend/model.js')
  assert.deepEqual(['held', 'delivered', 'rejected', 'reserved'].map(messageStateLabelOf), ['Held for you', 'Delivered', 'Rejected', 'Delivering'])
  assert.equal(messageStateLabelOf('staged', 'copy'), 'Copied by you')
  assert.equal(messageStateLabelOf('staged', 'type'), 'Pasted by you')
  assert.equal(messageStateLabelOf('staged'), 'Copied or pasted by you')
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

test('only live rows in a Terminal, Ghostty, iTerm, WezTerm, kitty or Konsole tab, outside Archived, can be typed into', () => {
  const tab = (app) => ({ live: true, open: { kind: 'focus', label: 'Switch to ' + app } })
  const rows = [
    row('ghostty', 'unread', tab('Ghostty')),
    row('terminal', 'working', tab('Terminal')),
    row('iterm', 'active', tab('iTerm')),
    row('wezterm', 'active', tab('WezTerm')),
    row('kitty', 'active', tab('kitty')),
    row('konsole', 'active', tab('Konsole')),
    row('warp', 'active', tab('Warp')),
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
  assert.deepEqual(typableRows(rows).map((r) => r.id), ['ghostty', 'terminal', 'iterm', 'wezterm', 'kitty', 'konsole', 'codex-idle'])
  assert.deepEqual(typableRows([rows[1], rows[6]]).map((r) => r.id), ['terminal'])
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
  assert.deepEqual(folded.rows.map((node) => node.helper.id), ['work-new', 'work-mid', 'work-old', 'work-older', 'work-oldest'])
  assert.equal(folded.more, 1)
  const all = helperRowsOf(conversation, true)
  assert.deepEqual(all.rows.map((node) => node.helper.id), ['work-new', 'work-mid', 'work-old', 'work-older', 'work-oldest', 'work-ancient'])
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
  assert.equal(helperTooltipOf(helper('agent-1', 'working', 1, { reach: 'rows' })), 'Open the chat it runs in, and show how to reach this subagent')
  assert.equal(helperTooltipOf(helper('agent-1', 'working', 1, { reach: 'nested' })), 'Open the chat it runs in, and show how to reach this subagent')
  assert.equal(helperTooltipOf(helper('echo@team', 'working', 1, { reach: 'pane' })), 'Switch to its pane')
})

test('an open that reached the chat a subagent runs in leaves the reply as a notice; any other open leaves none', () => {
  const said = 'Switched to the Terminal tab that runs it. Press the down arrow to pick \'Find call sites\' below the prompt, then Enter.'
  const explore = { tool: 'claude', id: 'explore' }
  assert.deepEqual(subagentNoticeOf({ opened: true, message: said, helper: explore, reached: 'parent' }), { message: said, hint: true })
  assert.deepEqual(subagentNoticeOf({ opened: true, message: said, helper: explore }), { message: said, hint: true }, 'a reply without reached reads as the parent')
  const finished = 'That subagent has finished. Its chat is open.'
  assert.deepEqual(subagentNoticeOf({ opened: true, message: finished, reached: 'parent' }), { message: finished, hint: true })
  const codex = 'Switched to the Ghostty tab that runs it. Type /subagents to switch to \'Aquinas\'.'
  assert.deepEqual(subagentNoticeOf({ opened: true, message: codex, reached: 'parent' }), { message: codex, hint: true })
  assert.equal(subagentNoticeOf({ opened: true, message: 'Switched to the Terminal tab that runs it.', helper: explore, reached: 'subagent' }), undefined)
  assert.equal(subagentNoticeOf({ opened: true, message: 'Switched to the Terminal tab that runs it.' }), undefined)
  assert.equal(subagentNoticeOf({ opened: false, message: 'Terminal could not be opened.', helper: explore, reached: 'parent' }), undefined)
})

test('subagents show as a tree of any depth, each under its real parent, newest first on each level', () => {
  const helpers = [
    helper('mate', 'working', 5, { reach: 'rows' }),
    helper('one', 'working', 4, { parentId: 'mate', reach: 'nested' }),
    helper('two', 'working', 3, { parentId: 'one', reach: 'nested' }),
    helper('three', 'working', 2, { parentId: 'two', reach: 'nested' }),
    helper('one-b', 'working', 1, { parentId: 'mate', reach: 'nested' }),
    helper('done', 'finished', 1, { parentId: 'mate' }),
  ]
  const tree = helperTreeOf(row('a', 'working', { helpers }))
  const shape = (node) => [node.helper.id, node.children.map(shape)]
  assert.deepEqual(tree.roots.map(shape), [['mate', [['one-b', []], ['one', [['two', [['three', []]]]]]]]])
  assert.equal(tree.total, 5)
  assert.equal(tree.depth, 4)
  assert.equal(helpersToggleOf(row('a', 'working', { helpers }), false), '▸ 5 subagents, 4 levels')
  assert.equal(subtreeToggleOf(tree.roots[0], true), '▾ 4 subagents')
  assert.equal(subtreeToggleOf(tree.roots[0].children[1].children[0], false), '▸ 1 subagent')
})

test('a subagent whose parent is gone, or whose parents lead back to it, sits at the top', () => {
  const helpers = [
    helper('orphan', 'working', 1, { parentId: 'gone' }),
    helper('self', 'working', 2, { parentId: 'self' }),
    helper('loop-a', 'working', 3, { parentId: 'loop-b' }),
    helper('loop-b', 'working', 4, { parentId: 'loop-a' }),
    helper('under-loop', 'working', 5, { parentId: 'loop-a' }),
  ]
  const tree = helperTreeOf(row('a', 'working', { helpers }))
  const shape = (node) => [node.helper.id, node.children.map(shape)]
  assert.deepEqual(tree.roots.map(shape), [['orphan', []], ['self', []], ['loop-a', [['under-loop', []]]], ['loop-b', []]])
  assert.equal(tree.total, 5, 'no subagent is dropped')
  assert.equal(helpersToggleOf(row('a', 'working', { helpers }), true), '▾ 5 subagents, 2 levels')
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
  assert.deepEqual(waitingOf(view), { blocked: 1, unread: 0, icon: 'lock' })
  assert.equal(approvalReasonOf(view.conversations[0]), undefined)
  assert.equal(approvalsHeaderOf(view), undefined)
})

for (const mode of ['review', 'yolo']) {
  test(`in ${mode} an approved request never shows under Needs you nor counts, and its row says how many were approved`, () => {
    const view = { approvalsMode: mode, needsYou: 0, conversations: [approvedRow('a', 3)] }
    assert.deepEqual(needsYouIds(view), [])
    assert.deepEqual(waitingOf(view), { blocked: 0, unread: 0, icon: 'terminal' })
    assert.equal(approvedLabelOf(view.conversations[0]), '3 approved for you')
    assert.equal(approvalOf(view.conversations[0]), undefined)
  })

  test(`in ${mode} a request the gate sends to the person shows under Needs you, counts, and names the rule`, () => {
    const asked = permission('a', { approvals: { reason: 'Always asks: publishing', approved: 2 } })
    const view = { approvalsMode: mode, needsYou: 1, conversations: [asked] }
    assert.deepEqual(needsYouIds(view), ['a'])
    assert.deepEqual(waitingOf(view), { blocked: 1, unread: 0, icon: 'lock' })
    assert.equal(approvalReasonOf(asked), 'Always asks: publishing')
    assert.equal(approvedLabelOf(asked), '2 approved for you')
  })

  test(`in ${mode} a tool with no route shows its prompt and says approvals do not reach it`, () => {
    const unsupported = permission('a', { tool: 'cursor', approvals: { reason: 'Approvals not supported for Cursor' } })
    const view = { approvalsMode: mode, needsYou: 1, conversations: [unsupported] }
    assert.deepEqual(waitingOf(view), { blocked: 1, unread: 0, icon: 'lock' })
    assert.equal(approvalReasonOf(unsupported), 'Approvals not supported for Cursor')
  })
}

test('the header names the mode only while approvals answer prompts', () => {
  assert.deepEqual(approvalsHeaderOf({ approvalsMode: 'yolo' }), { mode: 'yolo', label: 'Auto-approve on', title: 'Approvals: routine prompts are approved for you' })
  assert.deepEqual(approvalsHeaderOf({ approvalsMode: 'review' }), { mode: 'review', label: 'Reviewers answer', title: "Approvals: your tools' reviewers answer routine prompts" })
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

test('names the handoff and conflict statuses shown in the panel', async () => {
  const { conflictLabelOf, handoffLabelOf } = await import('../frontend/model.js')
  assert.deepEqual(['offered', 'accepted', 'declined'].map(handoffLabelOf), ['Offered', 'Accepted', 'Declined'])
  assert.equal(conflictLabelOf({ path: 'src/a.go' }, 'Parser session'), 'Path conflict: src/a.go is held by Parser session.')
  assert.equal(conflictLabelOf({ path: 'src/a.go' }), 'Path conflict: no session holds src/a.go.')
})

test('a row chip counts the claimed paths, and none without a claim', async () => {
  const { claimChipOf } = await import('../frontend/model.js')
  assert.equal(claimChipOf({}), undefined)
  assert.equal(claimChipOf({ claimedPaths: 0 }), undefined)
  assert.equal(claimChipOf({ claimedPaths: 1 }), '1 path claimed')
  assert.equal(claimChipOf({ claimedPaths: 3 }), '3 paths claimed')
})

test('the footer button shows only with claims or conflicts, and warns about stale claims and conflicts', async () => {
  const { claimsButtonOf } = await import('../frontend/model.js')
  assert.deepEqual(claimsButtonOf({}), { shown: false })
  assert.deepEqual(claimsButtonOf({ mailboxClaims: { paths: 0, stale: 0, conflicts: 0 } }), { shown: false })
  assert.deepEqual(claimsButtonOf({ mailboxClaims: { paths: 3, stale: 0, conflicts: 0 } }), { shown: true, text: '3', label: 'File claims: 3 paths', warns: false })
  assert.deepEqual(claimsButtonOf({ mailboxClaims: { paths: 3, stale: 1, conflicts: 1 } }), { shown: true, text: '3', label: 'File claims: 3 paths, 1 stale, 1 conflict', warns: true })
  assert.deepEqual(claimsButtonOf({ mailboxClaims: { paths: 0, stale: 0, conflicts: 2 } }), { shown: true, text: '0', label: 'File claims: 0 paths, 2 conflicts', warns: true })
})

test('a claim says who made it and when', async () => {
  const { claimMetaOf } = await import('../frontend/model.js')
  const since = new Date(2026, 10, 22, 14, 2).toISOString()
  const sameDay = new Date(2026, 10, 22, 18, 0).getTime()
  assert.equal(claimMetaOf({ by: 'human', since }, sameDay), 'Claimed by you, 14:02')
  assert.equal(claimMetaOf({ by: 'agent', since }, sameDay), 'Claimed by the agent, 14:02')
  assert.equal(claimMetaOf({ by: 'human', since }, new Date(2026, 10, 24).getTime()), 'Claimed by you, Nov 22, 14:02')
  assert.equal(claimMetaOf({ by: 'agent' }, sameDay), 'Claimed by the agent')
})

test('the claims card names its owner, says what an empty card means, and lists gone owners first', async () => {
  const { claimsCardOf } = await import('../frontend/model.js')
  assert.deepEqual(claimsCardOf({ claims: [] }, 'Review the parser'), { title: 'Files claimed by Review the parser', empty: 'This session has no claims yet.', stale: [], active: [] })
  assert.deepEqual(claimsCardOf({ claims: [] }), { title: 'File claims', empty: 'No claims left.', stale: [], active: [] })
  assert.equal(claimsCardOf({ claims: [], conflicts: [{ path: 'a' }] }).empty, undefined)
  const live = { paths: ['a'] }
  const gone = { paths: ['b'], stale: true }
  const card = claimsCardOf({ claims: [live, gone] })
  assert.equal(card.empty, undefined)
  assert.deepEqual([card.stale, card.active], [[gone], [live]])
})

test('every help link names a page and points at the docs', async () => {
  const { HELP_TOPICS, helpOf } = await import('../frontend/model.js')
  for (const topic of Object.keys(HELP_TOPICS)) {
    const { path, title } = helpOf(topic)
    assert.match(path, /^\/docs\/[a-z0-9/-]*\/(#[a-z0-9-]+)?$/)
    assert.match(title, /^Open the docs: \S/)
  }
  assert.deepEqual(helpOf('file-claims'), { path: '/docs/mailbox/#hand-off-files', title: 'Open the docs: Mailbox: Hand off files' })
})

test('says which tools cannot be read without a command, and none for a tool that is just absent', () => {
  const view = {
    tools: [
      { tool: 'claude', label: 'Claude Code', found: true },
      { tool: 'codex', label: 'Codex', found: false, note: 'Reading Codex threads needs the sqlite3 command', needs: 'sqlite3' },
      { tool: 'cursor', label: 'Cursor', found: false, note: 'No Cursor chat index at C:\\x' },
      { tool: 'chatgpt', label: 'ChatGPT', found: false, note: 'Reading ChatGPT chats needs the sqlite3 command', needs: 'sqlite3' },
    ],
  }
  assert.equal(missingToolsNoteOf(view), 'Codex and ChatGPT chats are missing: reading them needs the sqlite3 command.')
  assert.equal(missingToolsNoteOf({ tools: [view.tools[0], view.tools[2]] }), undefined)
  assert.equal(missingToolsNoteOf({}), undefined)
})

test('says which messages an agent sent, the model it states and who answered a handoff', async () => {
  const { handoffAnswerOf, senderKindOf, statedModelOf } = await import('../frontend/model.js')
  const from = { tool: 'codex', id: 'a', by: 'agent' }
  assert.equal(senderKindOf({ from }), 'Sent by the agent')
  assert.equal(senderKindOf({ from: { ...from, by: 'human' } }), undefined)
  assert.equal(statedModelOf({ models: { 'codex:a': { name: 'gpt-5.5', statedBy: 'agent' } } }, { from }), 'Says it runs gpt-5.5')
  assert.equal(statedModelOf({ models: { 'codex:a': { name: 'gpt-5.5', statedBy: 'human' } } }, { from }), undefined)
  assert.equal(statedModelOf({}, { from }), undefined)
  assert.equal(statedModelOf({ models: { 'codex:a': { name: 'gpt-5.5', statedBy: 'agent' } } }, { from, model: 'gpt-5.6' }), 'Says it runs gpt-5.6')
  const thread = { events: [{ action: 'send', n: 2, by: 'agent' }, { action: 'accept', n: 2, by: 'agent' }, { action: 'decline', n: 3, by: 'human' }] }
  assert.equal(handoffAnswerOf(thread, { n: 2, handoff: 'accepted' }), 'Accepted by the agent')
  assert.equal(handoffAnswerOf(thread, { n: 3, handoff: 'declined' }), 'Declined by you')
  assert.equal(handoffAnswerOf(thread, { n: 4, handoff: 'offered' }), 'Offered')
  assert.equal(handoffAnswerOf({}, { n: 1 }), undefined)
})

test('a shell wait shows a terminal in the state mark, and every other wait the lock', async () => {
  const { stateIconOf } = await import('../frontend/model.js')
  assert.equal(stateIconOf(row('a', 'blocked', { waiting: { for: 'shell' } })), 'terminal')
  assert.equal(stateIconOf(row('a', 'blocked', { waiting: { for: 'permission' } })), 'lock')
  assert.equal(stateIconOf(row('a', 'blocked', { waiting: { for: 'answer' } })), 'lock')
  assert.equal(stateIconOf(row('a', 'blocked')), 'lock')
  assert.equal(stateIconOf(row('a', 'working', { waiting: { for: 'shell' } })), undefined)
})

test('Keep running waits 30 min, then doubles up to 4 h', async () => {
  const { keepOf, keepWaitOf, keptShellLineOf } = await import('../frontend/model.js')
  assert.deepEqual([0, 1, 2, 3, 4].map((keeps) => keepWaitOf(keeps) / 60000), [30, 60, 120, 240, 240])
  const conversation = row('a', 'blocked', { live: true, waiting: { for: 'shell' }, stuckShells: [shell] })
  const first = keepOf(conversation, shell, [], shellNow)
  assert.deepEqual(first, { tool: 'claude', id: 'a', pid: 42, procStart: 'shell-start', wakeAt: '2026-11-09T12:30:00.000Z', keeps: 1 })
  assert.equal(keepOf(conversation, shell, [first], shellNow).wakeAt, '2026-11-09T13:00:00.000Z')
  assert.equal(keepOf(conversation, shell, [{ ...first, procStart: 'other' }], shellNow).keeps, 1)
  const line = (minutes) => keptShellLineOf({ ...shell, keptUntil: new Date(shellNow + minutes * 60000).toISOString() }, shellNow)
  assert.equal(line(30), 'Kept running · asks again in 30 min: npm run dev')
  assert.equal(line(60), 'Kept running · asks again in 1 h: npm run dev')
  assert.equal(line(120), 'Kept running · asks again in 2 h: npm run dev')
  assert.equal(line(240), 'Kept running · asks again in 4 h: npm run dev')
  assert.equal(line(90), 'Kept running · asks again in 1 h 30 min: npm run dev')
})

test('a row waiting only on kept shells sits under Active until the wake time', async () => {
  const { keptViewOf } = await import('../frontend/model.js')
  const waitingRow = row('a', 'blocked', { live: true, waiting: { for: 'shell' }, stuckShells: [shell] })
  const view = { needsYou: 2, conversations: [waitingRow, row('b', 'unread')] }
  const record = { tool: 'claude', id: 'a', pid: 42, procStart: 'shell-start', wakeAt: '2026-11-09T12:30:00Z', keeps: 1 }

  const kept = keptViewOf(view, [record], shellNow)
  assert.equal(kept.conversations[0].state, 'active')
  assert.equal(kept.conversations[0].waiting, undefined)
  assert.equal(kept.conversations[0].stuckShells[0].keptUntil, record.wakeAt)
  assert.equal(kept.needsYou, 1)
  assert.deepEqual(waitingOf(kept), { blocked: 0, unread: 1, icon: 'terminal' })

  const due = keptViewOf(view, [record], Date.parse(record.wakeAt))
  assert.equal(due.conversations[0].state, 'blocked')
  assert.equal(due.needsYou, 2)

  const conflicted = { ...view, conversations: [{ ...waitingRow, claimConflicts: [{ id: 'c1', worktree: '/w', path: 'a.go' }] }, view.conversations[1]] }
  assert.equal(keptViewOf(conflicted, [record], shellNow).needsYou, 2, 'a path conflict keeps the row under Needs you')

  const another = keptViewOf(view, [{ ...record, procStart: 'later' }], shellNow)
  assert.equal(another.conversations[0].state, 'blocked')

  const twoShells = { ...waitingRow, stuckShells: [shell, { ...shell, pid: 45 }] }
  const oneNew = keptViewOf({ needsYou: 1, conversations: [twoShells] }, [record], shellNow)
  assert.equal(oneNew.conversations[0].state, 'blocked')
  assert.equal(oneNew.needsYou, 1)

  const working = keptViewOf({ needsYou: 0, conversations: [{ ...waitingRow, state: 'working', waiting: undefined }] }, [record], shellNow)
  assert.equal(working.conversations[0].state, 'working')
  assert.equal(working.conversations[0].stuckShells[0].keptUntil, record.wakeAt)
  assert.equal(keptViewOf(view, [], shellNow), view)
})

test('offers Ask about this shell again only on a row with a kept shell', async () => {
  const { keptShellsOfRow } = await import('../frontend/model.js')
  const conversation = row('a', 'active', { live: true, stuckShells: [{ ...shell, keptUntil: '2026-11-09T12:30:00Z' }] })
  assert.ok(rowMenuOf(conversation).includes('ask-shell-again'))
  assert.ok(!rowMenuOf({ ...conversation, stuckShells: [shell] }).includes('ask-shell-again'))
  const record = { tool: 'claude', id: 'a', pid: 42, procStart: 'shell-start', wakeAt: '2026-11-09T12:30:00Z', keeps: 1 }
  assert.deepEqual(keptShellsOfRow(conversation, [record, { ...record, pid: 7 }]), [record])
})

test('the stop confirm warns only while the agent still works', async () => {
  const { stopShellWarningOf } = await import('../frontend/model.js')
  assert.equal(stopShellWarningOf(row('a', 'working')), 'The agent is still working and may need this shell.')
  assert.equal(stopShellWarningOf(row('a', 'blocked', { waiting: { for: 'shell' } })), undefined)
})

test('a full list ends the records of shells gone from a live chat', async () => {
  const { finishedKeepsOf } = await import('../frontend/model.js')
  const record = { tool: 'claude', id: 'a', pid: 42, procStart: 'shell-start', wakeAt: '2026-11-09T12:30:00Z', keeps: 1 }
  const elsewhere = { ...record, id: 'gone' }
  const live = row('a', 'active', { live: true, stuckShells: [shell] })
  assert.deepEqual(finishedKeepsOf({ conversations: [live] }, [record, elsewhere]), [])
  assert.deepEqual(finishedKeepsOf({ conversations: [{ ...live, stuckShells: [{ ...shell, pid: 45 }] }] }, [record, elsewhere]), [record])
  assert.deepEqual(finishedKeepsOf({ conversations: [{ ...live, state: 'blocked', waiting: { for: 'shell' }, stuckShells: undefined }] }, [record]), [record])
  // Waiting on a permission or a question, or idle, the server lists no shells, so the shell may still run.
  assert.deepEqual(finishedKeepsOf({ conversations: [{ ...live, state: 'blocked', waiting: { for: 'permission' }, stuckShells: undefined }] }, [record]), [])
  assert.deepEqual(finishedKeepsOf({ conversations: [{ ...live, state: 'blocked', waiting: { for: 'answer' }, stuckShells: undefined }] }, [record]), [])
  assert.deepEqual(finishedKeepsOf({ conversations: [{ ...live, stuckShells: undefined }] }, [record, elsewhere]), [])
  assert.deepEqual(finishedKeepsOf({ conversations: [{ ...live, live: false, stuckShells: undefined }] }, [record]), [])
  assert.deepEqual(finishedKeepsOf({ conversations: [{ ...live, state: 'working', stuckShells: undefined }] }, [record]), [])
})

test('a review model label is only what the person entered, never what an agent states', async () => {
  const { modelLabelOf } = await import('../frontend/model.js')
  const ref = { tool: 'codex', id: 'a' }
  assert.equal(modelLabelOf({ 'codex:a': { name: 'opus', statedBy: 'human' } }, ref), 'opus')
  assert.equal(modelLabelOf({ 'codex:a': { name: 'gpt-5.5', statedBy: 'agent' } }, ref), '')
  assert.equal(modelLabelOf(undefined, ref), '')
  assert.equal(modelLabelOf({}, undefined), '')
})

const receiverNow = Date.parse('2026-11-09T12:00:00Z')
const flat = (groups) => groups.flatMap((group) => group.options)

test('lists local Claude Code and Codex sessions as receivers, without the sender, cloud, chat, Cowork or Cursor rows', () => {
  const from = row('sender', 'active')
  const groups = receiversOf([
    from,
    row('codex', 'active', { tool: 'codex' }),
    row('continued', 'active', { parent: { tool: 'claude', id: 'codex' } }),
    row('cloud', 'active', { app: 'cloud' }),
    row('chat', 'active', { app: 'chat' }),
    row('cowork', 'active', { app: 'cowork' }),
    row('cursor', 'active', { tool: 'cursor' }),
  ], from, receiverNow)
  assert.deepEqual(flat(groups).map((option) => option.to), [{ tool: 'codex', id: 'codex' }, { tool: 'claude', id: 'continued' }])
  assert.deepEqual(flat(groups).map((option) => option.value), ['codex:codex', 'claude:continued'])
})

test('groups receivers in list order, drops empty groups, labels Archived and puts the most recent first', () => {
  const from = row('sender', 'active')
  const groups = receiversOf([
    row('old-dormant', 'dormant', { updatedAt: '2026-11-01T12:00:00Z' }),
    row('busy', 'working'),
    row('waits', 'blocked', { updatedAt: '2026-11-08T12:00:00Z' }),
    row('replied', 'unread'),
    row('done', 'completed', { updatedAt: '2026-11-05T12:00:00Z' }),
  ], from, receiverNow)
  assert.deepEqual(groups.map((group) => [group.label, group.options.map((option) => option.to.id)]), [
    ['Needs you', ['replied', 'waits']],
    ['Working', ['busy']],
    ['Archived · gets it when resumed', ['done', 'old-dormant']],
  ])
})

test('labels a receiver by title, tool and folder, and adds the age, then the short ID, only where two read the same', () => {
  const from = row('sender', 'active')
  const folder = '/Users/you/Code/shop'
  const groups = receiversOf([
    row('plain', 'active', { title: 'Fix login', tool: 'codex', folder }),
    row('aaaaaaaa-1', 'active', { title: 'Same', folder, updatedAt: '2026-11-07T12:00:00Z' }),
    row('bbbbbbbb-1', 'active', { title: 'Same', folder, updatedAt: '2026-11-08T12:00:00Z' }),
    row('cccccccc-1', 'active', { title: 'Twin', folder, updatedAt: '2026-11-07T12:00:00Z' }),
    row('dddddddd-1', 'active', { title: 'Twin', folder, updatedAt: '2026-11-07T12:00:00Z' }),
  ], from, receiverNow)
  assert.deepEqual(flat(groups).map((option) => option.label), [
    'Fix login · Codex · shop',
    'Same · Claude Code · shop · 1d',
    'Same · Claude Code · shop · 2d',
    'Twin · Claude Code · shop · 2d · cccccccc',
    'Twin · Claude Code · shop · 2d · dddddddd',
  ])
  assert.equal(sessionLabelOf(row('x', 'active', { title: 'Companion rebuild' })), 'Companion rebuild · Claude Code')
})

test('the type help line names the receiver, or the other session without one', () => {
  assert.deepEqual(['question', 'finding', 'review_request', 'handoff'].map((type) => typeHelpOf(type, 'Draft the tests')), [
    'Ask Draft the tests something. Its reply comes back to this thread.',
    'Tell Draft the tests what you found. It can reply or close the thread.',
    "Ask Draft the tests to review the changes in this chat's worktree.",
    "Hand this session's task to Draft the tests. It gets the worktree, branch and handoff note, and resumes when you say so.",
  ])
  assert.equal(typeHelpOf('question', undefined), 'Ask the other session something. Its reply comes back to this thread.')
})

test('handoff receipts name the note or say how to ask for one', () => {
  assert.equal(
    handoffSentOf({ resume: { note: 'C:\\repo\\.agnostic-ai\\local\\handoffs\\feat-mailbox.md' } }, 'Draft the tests', 'Review the parser'),
    'Handoff waiting for Draft the tests, with the note feat-mailbox.md.',
  )
  assert.equal(
    handoffSentOf({ resume: { branch: 'feat/mailbox' } }, 'Draft the tests', 'Review the parser'),
    'Handoff waiting for Draft the tests. Review the parser has no handoff note for feat/mailbox yet. Ask it to "write a handoff".',
  )
  assert.equal(
    handoffSentOf({ resume: {} }),
    'Handoff waiting for the other session. The sender has no handoff note for its detached HEAD yet. Ask it to "write a handoff".',
  )
})

test('only a handoff labels its message as an optional note', () => {
  assert.equal(messageLabelOf('handoff'), 'Note (optional)')
  assert.equal(messageLabelOf('question'), 'Message')
  assert.equal(messageLabelOf('finding'), 'Message')
})

test('the arrival line follows the receiver group and is empty without a receiver', () => {
  const lead = 'This waits in the thread. With the mailbox built-in on, Draft gets it'
  assert.deepEqual(['needs-you', 'active', 'working', 'archived'].map((group) => arrivalOf(group, 'Draft')), [
    `${lead} with its next prompt.`,
    `${lead} with its next prompt.`,
    `${lead} with its next prompt, after its current work.`,
    `${lead} when it is resumed.`,
  ])
  assert.equal(arrivalOf('working', undefined), '')
  assert.equal(arrivalOf(undefined, 'Draft'), '')
})

test('a typed session ID finds one receiver, none, two tools or the sender', () => {
  const from = row('sender', 'active')
  const receivers = flat(receiversOf([from, row('one', 'active', { title: 'Draft' }), row('both', 'active'), row('both', 'active', { tool: 'codex' })], from, receiverNow))
  const found = typedReceiverOf(receivers, from, ' one ')
  assert.equal(found.status, 'found')
  assert.deepEqual(found.receiver.to, { tool: 'claude', id: 'one' })
  assert.equal(found.note, 'Found: Draft · Claude Code.')
  assert.deepEqual(typedReceiverOf(receivers, from, 'missing'), { status: 'unknown', note: 'No session in the list has this ID. It may not be loaded yet.' })
  assert.deepEqual(typedReceiverOf(receivers, from, 'both'), { status: 'two', note: 'Two sessions use this ID. Choose from the list.' })
  assert.deepEqual(typedReceiverOf(receivers, from, 'sender'), { status: 'sender', note: 'That is the sending session.' })
  assert.deepEqual(typedReceiverOf(receivers, from, '  '), { status: 'empty' })
})

test('a held request shows its folder and what it runs or writes, from the hook input', async () => {
  const { pendingOf } = await import('../frontend/model.js')
  const held = (call, input, extra = [], cwd = '/p') => row('a', 'blocked', {
    waiting: { for: 'permission', tool: call },
    approvals: { pending: [{ id: 'i1', digest: 'd1', call, input, cwd, expires: '2026-11-22T08:05:00Z' }, ...extra] },
  })
  const folder = { label: 'In', text: '/p' }
  assert.deepEqual(pendingOf(held('Bash', { command: 'touch a.txt' })), { id: 'i1', digest: 'd1', blocks: [folder, { label: 'Run', text: 'touch a.txt' }], more: 0, inTerminal: false })
  assert.deepEqual(pendingOf(held('Edit', { file_path: '/p/a.go', old: 'a', new: 'b' })).blocks, [folder, { label: 'Edit', text: '/p/a.go' }, { label: 'Replace', text: 'a' }, { label: 'With', text: 'b' }])
  assert.deepEqual(pendingOf(held('Write', { file_path: '/p/a.go', new: 'package a' })).blocks, [folder, { label: 'Write', text: '/p/a.go' }, { label: 'Content', text: 'package a' }])
  assert.deepEqual(pendingOf(held('Read', { file_path: '/p/a.go' })).blocks, [folder, { label: 'Read', text: '/p/a.go' }])
  assert.equal(pendingOf(held('Bash', { command: 'ls' }, [{ id: 'i2', digest: 'd2', call: 'Bash', input: { command: 'pwd' } }])).more, 1)
  assert.equal(pendingOf(held('Bash', { command: 'ls' }, [], '')), undefined, 'a request without its folder gets no buttons')
})

test('a held command shows how it runs, and one with an option the panel cannot read gets no buttons', async () => {
  const { pendingOf } = await import('../frontend/model.js')
  const held = (input) => row('a', 'blocked', {
    waiting: { for: 'permission', tool: 'Bash' },
    approvals: { pending: [{ id: 'i1', digest: 'd1', call: 'Bash', input: { command: 'make test', ...input }, cwd: '/p' }] },
  })
  assert.deepEqual(pendingOf(held({ background: true, timeout: 120000 })).blocks, [{ label: 'In', text: '/p' }, { label: 'Run', text: 'make test' }, { label: 'Options', text: 'In the background, time limit 120 s' }])
  assert.deepEqual(pendingOf(held({ timeout: 1500 })).blocks.at(-1), { label: 'Options', text: 'Time limit 1.5 s' })
  assert.equal(pendingOf(held({ background: 'yes' })), undefined)
  assert.equal(pendingOf(held({ timeout: -1 })), undefined)
  assert.equal(pendingOf(held({ timeout: '60' })), undefined)
})

test('a held Codex patch shows the whole patch and offers the terminal', async () => {
  const { pendingOf } = await import('../frontend/model.js')
  const patch = '*** Begin Patch\n*** Update File: a.go\n@@\n-a\n+b\n*** End Patch'
  const codex = { ...row('a', 'blocked', { waiting: { for: 'permission', tool: 'apply_patch' } }), tool: 'codex' }
  codex.approvals = { pending: [{ id: 'i1', digest: 'd1', call: 'apply_patch', input: { patch }, cwd: '/p/app' }] }
  assert.deepEqual(pendingOf(codex), { id: 'i1', digest: 'd1', blocks: [{ label: 'In', text: '/p/app' }, { label: 'Patch', text: patch }], more: 0, inTerminal: true })
})

test('a row without a held request, or that no longer waits, gets no buttons', async () => {
  const { pendingOf } = await import('../frontend/model.js')
  const pending = [{ id: 'i1', digest: 'd1', call: 'Bash', input: { command: 'ls' } }]
  assert.equal(pendingOf(row('a', 'blocked', { waiting: { for: 'permission', tool: 'Bash' } })), undefined)
  assert.equal(pendingOf(row('a', 'blocked', { waiting: { for: 'permission' }, approvals: { pending: [] } })), undefined)
  assert.equal(pendingOf(row('a', 'working', { approvals: { pending } })), undefined)
  assert.equal(pendingOf(row('a', 'blocked', { waiting: { for: 'permission' }, approvals: { pending: [{ id: 'i1', digest: 'd1', call: 'mcp__x__y', input: {} }] } })), undefined)
})

test('the circle counts the rows under Needs you and shows a terminal only when every wait is on a shell', () => {
  const shellWait = row('s', 'blocked', { live: true, waiting: { for: 'shell' }, stuckShells: [shell] })
  assert.deepEqual(waitingOf({ needsYou: 1, conversations: [shellWait] }), { blocked: 1, unread: 0, icon: 'terminal' })
  assert.deepEqual(waitingOf({ needsYou: 2, conversations: [shellWait, permission('p')] }), { blocked: 2, unread: 0, icon: 'lock' })
  assert.deepEqual(waitingOf({ needsYou: 9, conversations: [shellWait, row('u', 'unread'), row('w', 'working')] }), { blocked: 1, unread: 1, icon: 'terminal' })
})

test('a click on the circle opens the first row under Needs you', async () => {
  const { firstWaitingOf } = await import('../frontend/model.js')
  const view = { conversations: [row('u', 'unread'), row('w', 'working'), permission('p')] }
  assert.equal(firstWaitingOf(view), 'claude:p')
  assert.equal(firstWaitingOf({ conversations: [row('w', 'working')] }), undefined)
  assert.equal(firstWaitingOf(undefined), undefined)
})

test('every row under Needs you says why it waits, in one short line', async () => {
  const { needsReasonOf } = await import('../frontend/model.js')
  const now = Date.parse('2026-11-09T12:00:00Z')
  const reason = (conversation) => needsReasonOf(conversation, now)
  assert.deepEqual(reason(permission('p')), { kind: 'permission', text: 'Approve git push' })
  const shellWait = row('s', 'blocked', { live: true, waiting: { for: 'shell' }, stuckShells: [shell, { ...shell, pid: 43 }] })
  assert.deepEqual(reason(shellWait), { kind: 'shell', text: 'Waiting on 2 shells still running' })
  assert.deepEqual(reason({ ...shellWait, stuckShells: [shell] }), { kind: 'shell', text: 'Waiting on a shell still running' })
  assert.deepEqual(reason(row('q', 'blocked', { waiting: { for: 'answer' } })), { kind: 'question', text: 'Asks you a question' })
  assert.deepEqual(reason(row('z', 'blocked', { quietSince: '2026-11-09T11:53:00Z' })), { kind: 'quiet', text: 'No activity for 7m' })
  assert.deepEqual(reason(row('b', 'blocked')), { kind: 'waiting', text: 'Waiting for you' })
  assert.deepEqual(reason(row('u', 'unread', { reply: 'Done.' })), { kind: 'reply', text: 'New reply' })
  assert.deepEqual(reason(row('h', 'unread', { withheld: ['reply'] })), { kind: 'reply', text: 'New reply, hidden because it may hold a password or key' })
  assert.equal(reason(row('a', 'active')), undefined)
  assert.equal(reason(row('w', 'working', { waiting: { for: 'shell' } })), undefined)
  // Kept and stopped shells no longer count: the reason agrees with the lines under it.
  const keptOne = { ...shellWait, stuckShells: [shell, { ...shell, pid: 43, keptUntil: '2026-11-09T12:30:00Z' }] }
  assert.deepEqual(reason(keptOne), { kind: 'shell', text: 'Waiting on a shell still running' })
  const stoppedOne = (stopping) => stopping.pid === 43
  assert.deepEqual(needsReasonOf(shellWait, now, undefined, stoppedOne), { kind: 'shell', text: 'Waiting on a shell still running' })
  assert.equal(needsReasonOf(shellWait, now, undefined, () => true), undefined)
})

test('a row says its orange lines are shells still running, unless its reason already does or every shell is kept', async () => {
  const { shellsLeadOf } = await import('../frontend/model.js')
  const now = Date.parse('2026-11-09T12:00:00Z')
  const lead = (conversation) => shellsLeadOf(conversation, now)
  const other = { ...shell, pid: 43 }
  assert.equal(lead(row('w', 'working', { live: true, stuckShells: [shell] })), 'A shell is still running')
  assert.equal(lead(row('w', 'working', { live: true, stuckShells: [shell, other] })), '2 shells are still running')
  assert.equal(lead(row('s', 'blocked', { live: true, waiting: { for: 'shell' }, stuckShells: [shell, other] })), undefined)
  assert.equal(lead(row('s', 'blocked', { live: true, waiting: { for: 'shell' }, stuckShells: [shell] })), undefined)
  const kept = { ...other, keptUntil: '2026-11-09T12:30:00Z' }
  assert.equal(lead(row('k', 'working', { live: true, stuckShells: [{ ...shell, keptUntil: '2026-11-09T12:30:00Z' }, kept] })), undefined)
  assert.equal(lead(row('k', 'working', { live: true, stuckShells: [shell, kept] })), 'A shell is still running')
  const working = row('t', 'working', { live: true, stuckShells: [shell, other] })
  assert.equal(shellsLeadOf(working, now, (stopping) => stopping.pid === 43), 'A shell is still running')
  assert.equal(shellsLeadOf(working, now, () => true), undefined)
  const waits = row('s', 'blocked', { live: true, waiting: { for: 'shell' }, stuckShells: [shell, other] })
  assert.equal(shellsLeadOf(waits, now, (stopping) => stopping.pid === 43), undefined)
  assert.equal(shellsLeadOf(waits, now, () => true), undefined)
  assert.equal(lead(row('n', 'working', { live: true })), undefined)
  assert.equal(lead(row('n', 'working', { live: true, stuckShells: [] })), undefined)
  assert.equal(lead(row('q', 'blocked', { live: true, waiting: { for: 'answer' }, stuckShells: [shell] })), 'A shell is still running')
  assert.equal(lead(row('u', 'unread', { live: true, reply: 'Done.', stuckShells: [shell, other] })), '2 shells are still running')
})

test('shell labels tell apart shells whose commands share a long start', async () => {
  const { SHELL_COMMAND_SHOWN, shellLabelsOf } = await import('../frontend/model.js')
  const start = 'node scripts/watch.mjs ' + 'x'.repeat(240)
  const shells = [
    { ...shell, pid: 14476, command: start, since: '2026-11-09T11:16:00Z', processCount: 2 },
    { ...shell, pid: 38983, command: start + ' --port 4400', since: '2026-11-09T11:28:00Z', processCount: 2, helperName: 'Faramir' },
  ]
  const labels = shellLabelsOf(shells, shellNow)
  assert.notEqual(labels[0].command, labels[1].command)
  assert.ok(labels.every((label) => Array.from(label.command).length <= SHELL_COMMAND_SHOWN))
  assert.ok(labels[0].command.startsWith('node scripts/w') && labels[1].command.startsWith('node script'))
  assert.ok(labels[1].command.endsWith('--port 4400'))
  assert.deepEqual(labels.map((label) => label.age), ['44m', '32m'])
  assert.deepEqual(labels.map((label) => label.detail), ['', 'Faramir'])
  const clock = (stamp) => new Date(stamp).toTimeString().slice(0, 5)
  assert.equal(labels[0].full, `Running for 44 min, since ${clock(shells[0].since)}\nPID 14476 · 2 processes\n${start}`)
  assert.equal(labels[1].full.split('\n')[2], 'Left by Faramir')

  const hidden = shellLabelsOf([{ ...shell, pid: 14476, command: '' }, { ...shell, pid: 38983, command: '' }], shellNow)
  assert.deepEqual(hidden.map((label) => label.command), ['Command hidden · pid 14476', 'Command hidden · pid 38983'])
  assert.ok(hidden[0].full.endsWith('\nCommand hidden: it may hold a password or key'))

  const equal = shellLabelsOf([shell, { ...shell, pid: 43 }], shellNow)
  assert.deepEqual(equal.map((label) => label.command), ['npm run dev · pid 42', 'npm run dev · pid 43'])
  assert.deepEqual(shellLabelsOf([shell], shellNow).map((label) => label.command), ['npm run dev'])
  assert.equal(shellLabelsOf([{ ...shell, since: '2026-11-09T11:59:30Z', processCount: 1 }], shellNow)[0].full, `Running for less than a minute, since ${clock('2026-11-09T11:59:30Z')}\nPID 42 · 1 process\nnpm run dev`)
})

test('a kept shell names its wake time before its command', async () => {
  const { keptShellNoteOf } = await import('../frontend/model.js')
  assert.equal(keptShellNoteOf({ ...shell, keptUntil: new Date(shellNow + 30 * 60000).toISOString() }, shellNow), 'Kept running · asks again in 30 min')
})

const holderRef = { tool: 'claude', id: 'holder-1' }
const requesterRef = { tool: 'codex', id: 'requester-1' }
const pathConflict = { worktree: '/w', path: 'internal/mailbox/handoff.go', requester: requesterRef, holder: holderRef }
const names = { 'claude:holder-1': 'Fix the parser', 'codex:requester-1': 'Draft the tests' }
const nameIn = (ref) => names[`${ref.tool}:${ref.id}`]

test('a conflict line names the path and the other session on both rows', async () => {
  const { claimConflictOf } = await import('../frontend/model.js')
  assert.equal(claimConflictOf(row('x', 'active')), undefined)
  assert.equal(claimConflictOf({ ...holderRef, claimConflicts: [pathConflict] }, nameIn), 'Draft the tests wants internal/mailbox/handoff.go, which this session claims')
  assert.equal(claimConflictOf({ ...requesterRef, claimConflicts: [pathConflict] }, nameIn), 'internal/mailbox/handoff.go is claimed by Fix the parser')
  assert.equal(claimConflictOf({ ...requesterRef, claimConflicts: [pathConflict] }), 'internal/mailbox/handoff.go is claimed by Claude Code session holder-1')
  assert.equal(claimConflictOf({ ...requesterRef, claimConflicts: [{ ...pathConflict, holder: undefined }] }), 'No session holds internal/mailbox/handoff.go any more')
  const lostHandoff = { ...pathConflict, holder: requesterRef, sender: holderRef, thread: 't', n: 1 }
  assert.equal(claimConflictOf({ ...requesterRef, claimConflicts: [lostHandoff] }, nameIn), 'Fix the parser no longer holds internal/mailbox/handoff.go')
  assert.equal(claimConflictOf({ ...requesterRef, claimConflicts: [pathConflict, pathConflict] }, nameIn), 'internal/mailbox/handoff.go is claimed by Fix the parser, and 1 more conflict')
})

test('a conflict puts a row under Needs you whatever its state, after a permission and with the other waits', async () => {
  const { needsPersonOf, needsReasonOf } = await import('../frontend/model.js')
  const working = row('holder-1', 'working', { claimConflicts: [pathConflict] })
  assert.equal(needsPersonOf(working), true)
  assert.equal(needsPersonOf(row('y', 'working')), false)
  assert.deepEqual(needsReasonOf(working, Date.now(), nameIn), { kind: 'conflict', text: 'Draft the tests wants internal/mailbox/handoff.go, which this session claims' })
  const archived = row('holder-1', 'dormant', { claimConflicts: [pathConflict] })
  assert.equal(needsReasonOf(archived, Date.now(), nameIn).kind, 'conflict')
  const unread = row('holder-1', 'unread', { claimConflicts: [pathConflict] })
  assert.equal(needsReasonOf(unread, Date.now(), nameIn).kind, 'conflict')
  const permission = row('holder-1', 'blocked', {
    claimConflicts: [pathConflict], live: true, activity: 'waiting', waiting: { for: 'permission', tool: 'Bash', detail: 'npm test' },
  })
  assert.equal(needsReasonOf(permission, Date.now(), nameIn).kind, 'permission')
})

test('conflict rows group under Needs you, count as waits on the circle with a lock, and rank after a permission', async () => {
  const { firstWaitingOf, stateIconOf } = await import('../frontend/model.js')
  const conflicted = row('c', 'working', { claimConflicts: [pathConflict] })
  const permission = row('p', 'blocked', { live: true, activity: 'waiting', waiting: { for: 'permission', tool: 'Bash', detail: 'npm test' } })
  const unread = row('u', 'unread')
  const groups = grouped([unread, conflicted, row('w', 'working'), permission])
  assert.deepEqual(groups.map((group) => [group.id, group.rows.map((r) => r.id)]), [['needs-you', ['p', 'c', 'u']], ['working', ['w']]])
  assert.deepEqual(waitingOf({ conversations: [conflicted, unread] }), { blocked: 1, unread: 1, icon: 'lock' })
  assert.deepEqual(waitingOf({ conversations: [row('s', 'unread', { claimConflicts: [pathConflict] })] }), { blocked: 1, unread: 0, icon: 'lock' })
  assert.equal(firstWaitingOf({ conversations: [row('w', 'working'), conflicted] }), keyOf(conflicted))
  assert.equal(stateIconOf(conflicted), 'lock')
  assert.equal(stateIconOf(row('w', 'working')), undefined)
  assert.equal(viewIn({ conversations: [conflicted, row('w', 'working')] }, 'local').needsYou, 1)
})

test('a release splits into requests within the path and byte limits, one owner and worktree each', async () => {
  const { RELEASE_LIMITS, releaseBatchesOf } = await import('../frontend/model.js')
  const owner = { tool: 'codex', id: 'gone-1' }
  const many = Array.from({ length: 450 }, (_, i) => ({ owner, worktree: '/w', paths: [`p${i}.go`] }))
  const batches = releaseBatchesOf(many, { gone: true })
  assert.deepEqual(batches.map((batch) => batch.paths.length), [200, 200, 50])
  assert.ok(batches.every((batch) => batch.gone === true && batch.worktree === '/w' && batch.id === 'gone-1'))
  assert.deepEqual(batches.flatMap((batch) => batch.paths), many.map((claim) => claim.paths[0]))

  const long = Array.from({ length: 199 }, (_, i) => ({ owner, worktree: '/w', paths: [`${'d/'.repeat(190)}${i}.go`] }))
  const byBytes = releaseBatchesOf(long)
  assert.ok(byBytes.length > 1, 'long paths split before 200')
  for (const batch of byBytes) assert.ok(new TextEncoder().encode(JSON.stringify(batch)).length <= RELEASE_LIMITS.bytes)
  assert.equal(byBytes.reduce((sum, batch) => sum + batch.paths.length, 0), 199)

  const two = releaseBatchesOf([{ owner, worktree: '/a', paths: ['x'] }, { owner, worktree: '/b', paths: ['y'] }, { owner, worktree: '/a', paths: ['z'] }])
  assert.deepEqual(two.map((batch) => [batch.worktree, batch.paths]), [['/a', ['x', 'z']], ['/b', ['y']]])
})

test('a release that fails after an earlier batch reports how far it got', async () => {
  const { partialReleaseOf, releaseInBatches } = await import('../frontend/model.js')
  const batches = [{ paths: ['a', 'b'] }, { paths: ['c'] }, { paths: ['d'] }]
  const refusal = new Error('Those claims changed since the list was read.')
  const sent = []
  const result = await releaseInBatches(batches, async (batch) => {
    sent.push(batch)
    if (sent.length === 2) throw refusal
    return { version: 'v' + sent.length }
  })
  assert.deepEqual(result, { released: 2, total: 4, version: 'v1', error: refusal })
  assert.equal(sent.length, 2)
  assert.equal(partialReleaseOf(result, refusal.message), 'Released 2 of 4 paths. Those claims changed since the list was read.')
  assert.deepEqual(await releaseInBatches(batches, async () => ({ version: 'v' })), { released: 4, total: 4, version: 'v' })
})

test('the list offers a rename on every chat the server can rename, and on no other', () => {
  for (const [tool, app] of [['claude', 'cli'], ['claude', 'cloud'], ['codex', 'desktop'], ['cursor', 'ide'], ['chatgpt', 'chat']]) {
    assert.equal(renames({ tool, app }), true, `${tool} ${app}`)
  }
  for (const [tool, app] of [['claude', 'chat'], ['claude', 'cowork'], ['codex', 'cloud']]) {
    assert.equal(renames({ tool, app }), false, `${tool} ${app}`)
    assert.equal(rowMenuOf({ tool, app, state: 'active', open: {} }).includes('rename'), false, `${tool} ${app} menu`)
  }
})
