import { describe, expect, it } from 'vitest'
import type { Conversation, ConversationHelper, ConversationSource, ConversationsView, StuckShell } from '../contract'
import fixture from '../fixtures/conversations.mock.json'
import { appLabel, CONVERSATION_PLACES, conversationSearchStatus, conversationPlaceLabel, conversationToolLabel, needsYouReason, conversationsVerdict, helperDetails, helperOpenRequest, helperRows, helperTitle, helperTooltip, HELPERS_SHOWN, filterConversations, groupConversations, liveBadge, mergePages, relativeTime, replyCommand, rowCommand, sameFirstPage, selectedStuckShell, shellWarning, shownPrompt, stateBadge, chatText, toggleTool, toolChoices, withheldLabel } from './conversations'
import { normalizeConversationsView } from './normalize'

// Fixed local time for ages in the list.
const local = (month: number, day: number, hour: number, minute = 0) => new Date(2026, month - 1, day, hour, minute)
const NOW = local(11, 7, 16, 30)

const conversation = (over: Partial<Conversation>): Conversation => ({
  tool: 'claude',
  app: 'cli',
  id: 'a',
  title: 'A chat',
  updatedAt: NOW.toISOString(),
  open: { kind: 'terminal', label: 'Open in Terminal', command: 'claude --resume a' },
  ...over,
})

describe('conversation groups', () => {
  const groupsOf = (rows: Conversation[]) => groupConversations(rows).map((group) => [group.id, group.label, group.conversations.map((row) => row.id)])
  const conflict = { id: 'c', worktree: '/repo', path: 'app.ts', requester: { tool: 'codex' as const, id: 'requester' } }

  it('groups all six states by need, in declared order, and leaves empty groups out', () => {
    const rows = (['dormant', 'active', 'completed', 'working', 'unread', 'blocked'] as const).map((state) => conversation({ id: state, state }))
    expect(groupsOf(rows)).toEqual([
      ['needs-you', 'Needs you', ['blocked', 'unread']],
      ['working', 'Working', ['working']],
      ['active', 'Active', ['active']],
      ['archived', 'Archived', ['dormant', 'completed']],
    ])
    expect(groupConversations([])).toEqual([])
  })

  it('keeps missing states visible, live under Active and others under Archived, newest first', () => {
    expect(groupsOf([
      conversation({ id: 'older', updatedAt: local(10, 1, 9).toISOString() }),
      conversation({ id: 'live', live: true, activity: 'working' }),
      conversation({ id: 'broken', updatedAt: 'not a date' }),
      conversation({ id: 'future', updatedAt: local(11, 8, 9).toISOString() }),
    ])).toEqual([
      ['active', 'Active', ['live']],
      ['archived', 'Archived', ['future', 'older', 'broken']],
    ])
  })

  it('gives file conflicts precedence over every state and ignores resumed Active unread marks', () => {
    const rows = (['blocked', 'unread', 'working', 'active', 'completed', 'dormant'] as const).map((state) => conversation({ id: state, state, claimConflicts: [conflict] }))
    rows.push(conversation({ id: 'resumed', state: 'active', unread: true }))
    expect(groupsOf(rows)).toEqual([
      ['needs-you', 'Needs you', ['blocked', 'unread', 'working', 'active', 'completed', 'dormant']],
      ['active', 'Active', ['resumed']],
    ])
  })

  it('sorts roots newest first, ranking Needs you by permission, other waits or conflicts, then unread', () => {
    const at = (hour: number) => local(11, 7, hour).toISOString()
    expect(groupsOf([
      conversation({ id: 'reply', state: 'unread', updatedAt: at(16) }),
      conversation({ id: 'old-permission', state: 'blocked', waiting: { for: 'permission' }, updatedAt: at(12) }),
      conversation({ id: 'conflict', state: 'active', claimConflicts: [conflict], updatedAt: at(15) }),
      conversation({ id: 'question', state: 'blocked', waiting: { for: 'answer' }, updatedAt: at(14) }),
      conversation({ id: 'new-permission', state: 'blocked', waiting: { for: 'permission' }, updatedAt: at(13) }),
    ])).toEqual([['needs-you', 'Needs you', ['new-permission', 'old-permission', 'conflict', 'question', 'reply']]])
  })

  it('ranks each family by its most urgent member without separating children', () => {
    const [group] = groupConversations([
      conversation({ id: 'reply', state: 'unread' }),
      conversation({ id: 'wait', state: 'blocked', updatedAt: local(11, 7, 15).toISOString() }),
      conversation({ id: 'parent', state: 'unread', updatedAt: local(11, 7, 13).toISOString() }),
      conversation({ id: 'permission-child', state: 'blocked', waiting: { for: 'permission' }, parent: { tool: 'claude', id: 'parent' } }),
    ])
    expect(group.conversations.map((row) => row.id)).toEqual(['parent', 'permission-child', 'wait', 'reply'])
    expect(group.under.get('claude:permission-child')?.id).toBe('parent')
  })

  it('keeps a waiting child in Needs you when its archived parent loads', () => {
    const child = conversation({ id: 'child', state: 'blocked', parent: { tool: 'claude', id: 'parent' } })
    const parent = conversation({ id: 'parent', state: 'completed' })
    expect(groupsOf([child, parent])).toEqual([['needs-you', 'Needs you', ['child']], ['archived', 'Archived', ['parent']]])
    expect(groupConversations([child, parent])[0].under.size).toBe(0)
  })
})

describe('nested rows', () => {
  const at = (hour: number, minute = 0) => local(11, 7, hour, minute).toISOString()
  const drawn = (rows: Conversation[]) => groupConversations(rows, NOW).map((group) => [group.label, group.conversations.map((c) => [c.id, group.under.get(`${c.tool}:${c.id}`)?.id ?? null])])

  it('draws a child under its parent in the same group, whatever its tool, and keeps the group\'s count', () => {
    const rows = [
      conversation({ id: 'other', updatedAt: at(16, 20) }),
      conversation({ id: 'child', tool: 'codex', updatedAt: at(16, 10), parent: { tool: 'claude', id: 'parent' } }),
      conversation({ id: 'parent', updatedAt: at(15) }),
    ]
    expect(drawn(rows)).toEqual([['Archived', [['other', null], ['parent', null], ['child', 'parent']]]])
  })

  it('draws a child whose parent sits in another group as a normal row in its own group', () => {
    const rows = [
      conversation({ id: 'child', live: true, updatedAt: at(16), parent: { tool: 'claude', id: 'parent' } }),
      conversation({ id: 'parent', updatedAt: at(15) }),
    ]
    expect(drawn(rows)).toEqual([['Active', [['child', null]]], ['Archived', [['parent', null]]]])
  })

  it('draws a child as a normal row when its parent is not in the list: hidden, older, of another tool, or not matching the search', () => {
    const rows = [
      conversation({ id: 'child', tool: 'codex', updatedAt: at(16), parent: { tool: 'claude', id: 'parent' } }),
      conversation({ id: 'parent', updatedAt: at(15) }),
    ]
    expect(drawn([rows[0]])).toEqual([['Archived', [['child', null]]]])
    expect(drawn(filterConversations(rows, '', 'codex'))).toEqual([['Archived', [['child', null]]]])
    expect(drawn([conversation({ id: 'child', parent: { tool: 'codex', id: 'parent' } }), conversation({ id: 'parent', updatedAt: at(15) })])).toEqual([['Archived', [['child', null], ['parent', null]]]])
  })

  it('draws a chain at one level under the root that is in the group, oldest first', () => {
    const rows = [
      conversation({ id: 'grandchild', updatedAt: at(16, 20), parent: { tool: 'claude', id: 'child' } }),
      conversation({ id: 'child', updatedAt: at(16, 10), parent: { tool: 'claude', id: 'root' } }),
      conversation({ id: 'root', updatedAt: at(15) }),
    ]
    expect(drawn(rows)).toEqual([['Archived', [['root', null], ['child', 'root'], ['grandchild', 'root']]]])
  })

  it('draws every row of a loop made by hand as a normal row, and a row naming itself too', () => {
    const rows = [
      conversation({ id: 'a', updatedAt: at(16), parent: { tool: 'claude', id: 'b' } }),
      conversation({ id: 'b', updatedAt: at(15), parent: { tool: 'claude', id: 'a' } }),
      conversation({ id: 'c', updatedAt: at(14), parent: { tool: 'claude', id: 'a' } }),
      conversation({ id: 'self', updatedAt: at(13), parent: { tool: 'claude', id: 'self' } }),
    ]
    expect(drawn(rows)).toEqual([['Archived', [['a', null], ['b', null], ['c', null], ['self', null]]]])
  })

  it('orders the children by when they started, then by when they were written, then by id, so a new message moves none', () => {
    const rows = [
      conversation({ id: 'late-start', updatedAt: at(16, 25), startedAt: at(16, 5), parent: { tool: 'claude', id: 'root' } }),
      conversation({ id: 'early-start', updatedAt: at(16, 20), startedAt: at(15, 30), parent: { tool: 'claude', id: 'root' } }),
      conversation({ id: 'b-no-start', updatedAt: at(15, 50), parent: { tool: 'claude', id: 'root' } }),
      conversation({ id: 'a-no-start', updatedAt: at(15, 50), parent: { tool: 'claude', id: 'root' } }),
      conversation({ id: 'root', updatedAt: at(15), startedAt: at(15) }),
    ]
    expect(drawn(rows)).toEqual([['Archived', [['root', null], ['early-start', 'root'], ['a-no-start', 'root'], ['b-no-start', 'root'], ['late-start', 'root']]]])
  })

  it('draws as many rows as it is given when two rows share a tool and id: the first owns the children', () => {
    const rows = [
      conversation({ id: 'parent', updatedAt: at(16) }),
      conversation({ id: 'child', updatedAt: at(15, 30), parent: { tool: 'claude', id: 'parent' } }),
      conversation({ id: 'parent', title: 'A second copy', updatedAt: at(15) }),
    ]
    const [group] = groupConversations(rows, NOW)
    expect(group.conversations).toHaveLength(3)
    expect(group.conversations.map((c) => [c.id, c.title])).toEqual([['parent', 'A chat'], ['child', 'A chat'], ['parent', 'A second copy']])
  })

  it('moves a child under its parent once the week that holds the parent loads', () => {
    const firstWeek = { conversations: [conversation({ id: 'child', live: true, updatedAt: at(16), parent: { tool: 'claude', id: 'parent' } })] }
    const weekBefore = { conversations: [conversation({ id: 'parent', live: true, updatedAt: local(10, 20, 9).toISOString() })] }
    expect(drawn(mergePages([firstWeek]))).toEqual([['Active', [['child', null]]]])
    expect(drawn(mergePages([firstWeek, weekBefore]))).toEqual([['Active', [['parent', null], ['child', 'parent']]]])
  })

  it('leaves the verdict as it was without nesting', () => {
    const rows = [
      conversation({ id: 'parent', live: true, updatedAt: at(16) }),
      conversation({ id: 'child', live: true, activity: 'waiting', updatedAt: at(16, 10), parent: { tool: 'claude', id: 'parent' } }),
    ]
    const flat = rows.map(({ parent: _parent, ...rest }) => rest)
    expect(conversationsVerdict(rows, NOW, false)).toBe(conversationsVerdict(flat, NOW, false))
    expect(conversationsVerdict(rows, NOW, false)).toBe('2 chats are open now; 1 waits for you.')
  })
})

describe('loaded weeks', () => {
  it('treats a fresh first page read later as the same when its rows, sources and end match', () => {
    const page = (overrides: Partial<ConversationsView>): ConversationsView => ({
      schemaVersion: 'conversations/v1', generatedAt: '2026-11-07T12:00:00Z', from: '2026-10-31T12:00:00Z', to: '2026-11-07T12:00:00Z', next: '2026-10-31T12:00:00Z',
      tools: [{ tool: 'claude', label: 'Claude Code', found: true, count: 1, shown: 1 }], conversations: [conversation({ id: 'a' })], ...overrides,
    })
    const kept = page({})
    expect(sameFirstPage(kept, page({ generatedAt: '2026-11-07T12:01:00Z', from: '2026-10-31T12:01:00Z', to: '2026-11-07T12:01:00Z', next: '2026-10-31T12:01:00Z' }))).toBe(true)
    expect(sameFirstPage(kept, page({ conversations: [conversation({ id: 'a', live: true })] }))).toBe(false)
    expect(sameFirstPage(kept, page({ tools: [{ tool: 'claude', label: 'Claude Code', found: true, count: 2, shown: 1 }] }))).toBe(false)
    expect(sameFirstPage(kept, page({ next: undefined }))).toBe(false)
    expect(sameFirstPage(kept, page({ needsYou: 2 }))).toBe(false)
    expect(sameFirstPage(page({ needsYou: 2 }), page({ needsYou: 3 }))).toBe(false)
  })

  const ids = (list: Conversation[]) => list.map((c) => `${c.tool}:${c.id}`)

  it('appends an older week after the newer one', () => {
    const firstWeek = { conversations: [conversation({ id: 'new', updatedAt: local(11, 7, 9).toISOString() }), conversation({ id: 'mid', updatedAt: local(11, 3, 9).toISOString() })] }
    const weekBefore = { conversations: [conversation({ id: 'old', updatedAt: local(10, 28, 9).toISOString() })] }
    expect(ids(mergePages([firstWeek, weekBefore]))).toEqual(['claude:new', 'claude:mid', 'claude:old'])
  })

  it('keeps one row per tool and id, the later page\'s copy winning, in newest-first order', () => {
    const firstWeek = { conversations: [conversation({ id: 'a', title: 'First copy', updatedAt: local(11, 6, 9).toISOString() }), conversation({ id: 'a', tool: 'codex', updatedAt: local(11, 5, 9).toISOString() })] }
    const weekBefore = { conversations: [conversation({ id: 'b', updatedAt: local(10, 30, 9).toISOString() }), conversation({ id: 'a', title: 'Later copy', updatedAt: local(10, 29, 9).toISOString() })] }
    const merged = mergePages([firstWeek, weekBefore])
    expect(ids(merged)).toEqual(['codex:a', 'claude:b', 'claude:a'])
    expect(merged.find((c) => c.tool === 'claude' && c.id === 'a')?.title).toBe('Later copy')
  })

  it('keeps an old live conversation from an older server under Active, and an unreadable time last', () => {
    const firstWeek = { conversations: [conversation({ id: 'live-old', live: true, updatedAt: local(9, 1, 9).toISOString() }), conversation({ id: 'today', updatedAt: local(11, 7, 9).toISOString() })] }
    const weekBefore = { conversations: [conversation({ id: 'broken', updatedAt: 'not a date' }), conversation({ id: 'older', updatedAt: local(10, 28, 9).toISOString() })] }
    const merged = mergePages([firstWeek, weekBefore])
    expect(ids(merged)).toEqual(['claude:today', 'claude:older', 'claude:live-old', 'claude:broken'])
    expect(groupConversations(merged, NOW).map((group) => [group.label, ids(group.conversations)])).toEqual([
      ['Active', ['claude:live-old']],
      ['Archived', ['claude:today', 'claude:older', 'claude:broken']],
    ])
  })
})

describe('conversation search', () => {
  const rows = [
    conversation({ id: 'ui', title: 'Add a Conversations page', folder: '/Users/you/Products/agnostic-ai', branch: 'motechfr/conversations-tab' }),
    conversation({ id: 'sqlite', tool: 'codex', title: 'Speed up the sqlite reader', opening: ['The Cursor store read takes 900 ms.'], prompt: 'Try immutable=1.', reply: 'I measured 7 ms after the change.' }),
    conversation({ id: 'site', tool: 'cursor', title: '', folder: '/Users/you/Products/website' }),
  ]
  const ids = (list: Conversation[]) => list.map((c) => c.id)

  it('matches title, folder, branch, prompt, and reply as the row shows them, ignoring case and outer spaces', () => {
    expect(ids(filterConversations(rows, 'CONVERSATIONS', 'all'))).toEqual(['ui'])
    expect(ids(filterConversations(rows, 'website', 'all'))).toEqual(['site'])
    expect(ids(filterConversations(rows, 'motechfr/', 'all'))).toEqual(['ui'])
    expect(ids(filterConversations(rows, ' Immutable=1 ', 'all'))).toEqual(['sqlite'])
    expect(ids(filterConversations(rows, '900 ms', 'all'))).toEqual([])
    expect(ids(filterConversations(rows, 'Measured', 'all'))).toEqual(['sqlite'])
    expect(ids(filterConversations([conversation({ id: 'gl', title: '<bash-input>gl</bash-input>' })], '! gl', 'all'))).toEqual(['gl'])
    expect(ids(filterConversations(rows, 'products', 'all'))).toEqual(['ui', 'site'])
    expect(ids(filterConversations(rows, 'nothing like this', 'all'))).toEqual([])
  })

  it('narrows to one tool, alone or with a query', () => {
    expect(ids(filterConversations(rows, '', 'all'))).toEqual(['ui', 'sqlite', 'site'])
    expect(ids(filterConversations(rows, '', 'codex'))).toEqual(['sqlite'])
    expect(ids(filterConversations(rows, 'products', 'cursor'))).toEqual(['site'])
    expect(ids(filterConversations(rows, 'sqlite', 'claude'))).toEqual([])
  })

  it('does not search the id or the resume command', () => {
    expect(ids(filterConversations(rows, 'resume', 'all'))).toEqual([])
  })

  it('toggles a tool chip on, onto another tool, and back off', () => {
    expect(toggleTool('all', 'codex')).toBe('codex')
    expect(toggleTool('codex', 'cursor')).toBe('cursor')
    expect(toggleTool('cursor', 'cursor')).toBe('all')
  })
})

describe('Needs you reasons', () => {
  const waiting = (over: Partial<Conversation>) => conversation({ state: 'blocked', ...over })
  const conflict = { id: 'c', worktree: '/repo', path: 'app.ts', requester: { tool: 'codex' as const, id: 'requester' }, holder: { tool: 'claude' as const, id: 'holder' } }

  it('matches permission wording and gives permission precedence over file conflicts', () => {
    expect(needsYouReason(waiting({ waiting: { for: 'permission' }, claimConflicts: [conflict] }), NOW)).toBe('Needs permission')
    expect(needsYouReason(waiting({ waiting: { for: 'permission', tool: 'Edit', detail: 'app.ts' } }), NOW)).toBe('Approve edit app.ts')
    expect(needsYouReason(waiting({ waiting: { for: 'permission', tool: 'Bash' } }), NOW)).toBe('Approve Bash')
    expect(needsYouReason(waiting({ waiting: { for: 'permission', tool: 'Bash', detail: 'git status' } }), NOW)).toBe('Approve git status')
    expect(needsYouReason(waiting({ waiting: { for: 'permission', tool: 'mcp__service__read' } }), NOW)).toBe('Needs permission')
    expect(needsYouReason(waiting({ waiting: { for: 'permission', tool: 'mcp__service__read', detail: 'Service read' } }), NOW)).toBe('Approve Service read')
  })

  it('names both sides of a conflict, including unknown sessions, missing claims and extra conflicts', () => {
    const nameOf = (ref: { tool: string; id: string }) => ref.id === 'requester' ? 'The requester' : 'The holder'
    expect(needsYouReason(conversation({ id: 'holder', state: 'working', claimConflicts: [conflict] }), NOW, nameOf)).toBe('The requester wants app.ts, which this session claims')
    expect(needsYouReason(conversation({ id: 'requester', tool: 'codex', state: 'unread', claimConflicts: [conflict, conflict] }), NOW, nameOf)).toBe('app.ts is claimed by The holder, and 1 more conflict')
    expect(needsYouReason(conversation({ id: 'holder', claimConflicts: [conflict] }), NOW)).toBe('Codex session requeste wants app.ts, which this session claims')
    expect(needsYouReason(conversation({ id: 'requester', tool: 'codex', claimConflicts: [{ ...conflict, holder: undefined, sender: { tool: 'claude', id: 'holder' } }] }), NOW, nameOf)).toBe('The holder no longer holds app.ts')
    expect(needsYouReason(conversation({ id: 'requester', tool: 'codex', claimConflicts: [{ ...conflict, holder: undefined }] }), NOW)).toBe('No session holds app.ts any more')
  })

  it('matches replies, questions, silence and unknown waits and leaves resumed unread in Active', () => {
    expect(needsYouReason(conversation({ state: 'unread' }), NOW)).toBe('New reply')
    expect(needsYouReason(conversation({ state: 'unread', withheld: ['reply'] }), NOW)).toBe('New reply, hidden because it may hold a password or key')
    expect(needsYouReason(waiting({ waiting: { for: 'answer' } }), NOW)).toBe('Asks you a question')
    expect(needsYouReason(waiting({ quietSince: local(11, 7, 16, 23).toISOString() }), NOW)).toBe('No activity for 7m')
    expect(needsYouReason(waiting({ quietSince: NOW.toISOString() }), NOW)).toBe('No activity')
    expect(needsYouReason(waiting({ quietSince: 'broken' }), NOW)).toBe('Waiting for you')
    expect(needsYouReason(conversation({ state: 'active', unread: true }), NOW)).toBeUndefined()
  })

  it('names only live shells with singular and plural wording', () => {
    const shell: StuckShell = { pid: 23, procStart: 'a', command: 'sleep 90', since: NOW.toISOString(), processCount: 1 }
    expect(needsYouReason(waiting({ live: true, waiting: { for: 'shell' }, stuckShells: [shell] }), NOW)).toBe('Waiting on a shell still running')
    expect(needsYouReason(waiting({ live: true, waiting: { for: 'shell' }, stuckShells: [shell, { ...shell, pid: 24 }] }), NOW)).toBe('Waiting on 2 shells still running')
    expect(needsYouReason(waiting({ waiting: { for: 'shell' }, stuckShells: [shell] }), NOW)).toBeUndefined()
  })
})

describe('conversation places and result scope', () => {
  const rows = [
    conversation({ id: 'local', app: 'cli' }),
    conversation({ id: 'cloud', tool: 'codex', app: 'cloud' }),
    conversation({ id: 'chat', app: 'chat' }),
    conversation({ id: 'cowork', app: 'cowork' }),
    conversation({ id: 'shared', tool: 'codex', app: 'cli', cloud: { id: 'remote', link: 'https://example.com/shared' } }),
    conversation({ id: 'shared-cowork', app: 'cowork', cloud: { id: 'remote-cowork', link: 'https://example.com/shared' } }),
  ]
  const ids = (rows: Conversation[]) => rows.map((row) => row.id)

  it('matches Companion labels, tooltips and overlapping Local and Cloud rules', () => {
    expect(CONVERSATION_PLACES).toEqual([
      { id: 'all', label: 'All', title: 'Every conversation' },
      { id: 'local', label: 'Local', title: 'Conversations on this machine' },
      { id: 'cloud', label: 'Cloud', title: "Conversations that run in a tool's cloud" },
    ])
    expect(ids(filterConversations(rows, '', 'all', 'all'))).toEqual(ids(rows))
    expect(ids(filterConversations(rows, '', 'all', 'local'))).toEqual(['local', 'cowork', 'shared', 'shared-cowork'])
    expect(ids(filterConversations(rows, '', 'all', 'cloud'))).toEqual(['cloud', 'chat', 'shared', 'shared-cowork'])
  })

  it('combines place and the selected tool with every search word', () => {
    expect(ids(filterConversations(rows, '', 'codex', 'local'))).toEqual(['shared'])
    expect(ids(filterConversations(rows, '', 'codex', 'cloud'))).toEqual(['cloud', 'shared'])
    expect(ids(filterConversations(rows.map((row) => ({ ...row, title: row.id })), 'SHARED', 'claude', 'cloud'))).toEqual(['shared-cowork'])
  })

  it('keeps unfiltered search totals global and calls filtered results loaded matches', () => {
    expect(conversationSearchStatus(false, 45, 2, 9, false)).toBe('45 conversations match.')
    expect(conversationSearchStatus(false, 45, 2, 9, true)).toBe('2 of 9 loaded conversations match.')
    expect(conversationSearchStatus(false, 45, 1, 12, true)).toBe('1 of 12 loaded conversations matches.')
    expect(conversationSearchStatus(false, undefined, 0, 0, true)).toBe('0 of 0 loaded conversations match.')
    expect(conversationSearchStatus(true, 45, 0, 0, true)).toBe('Searching…')
  })
})

describe('conversation words', () => {
  it('uses Companion product and place labels', () => {
    expect(conversationToolLabel(conversation({ app: 'chat' }))).toBe('Claude')
    expect(conversationToolLabel(conversation({ app: 'cowork' }))).toBe('Claude')
    expect(conversationToolLabel(conversation({ app: 'cli' }))).toBe('Claude Code')
    expect(conversationToolLabel(conversation({ tool: 'codex', app: 'cloud' }))).toBe('Codex')
    expect((['cloud', 'chat', 'cowork'] as const).map((app) => conversationPlaceLabel(conversation({ app })))).toEqual(['Cloud', 'Chat', 'Cowork'])
    expect(conversationPlaceLabel(conversation({ cloud: { id: 'cloud', link: 'https://claude.ai/code/cloud' } }))).toBe('Shared')
    expect(conversationPlaceLabel(conversation({}))).toBeUndefined()
  })

  it('names every app', () => {
    expect((['cli', 'ide', 'desktop', 'background', 'cloud'] as const).map(appLabel)).toEqual(['Terminal', 'Editor', 'Desktop app', 'Background', 'Cloud'])
    expect(appLabel('watch' as Conversation['app'])).toBe('watch')
  })

  it('shows a status chip only while the agent works or waits, in amber when it waits for the person', () => {
    expect(liveBadge(conversation({ activity: 'working' }), NOW)).toEqual({ label: 'Working', tone: 'permitted', description: 'Open now: the agent is working' })
    expect(liveBadge(conversation({ activity: 'waiting' }), NOW)).toEqual({ label: 'Waiting for you', tone: 'approval', description: 'Open now: the agent is waiting for you' })
    expect(liveBadge(conversation({ activity: 'idle' }), NOW)).toBeUndefined()
    expect(liveBadge(conversation({}), NOW)).toBeUndefined()
  })

  it('shows the shell warning in place of Working when only the shell is active', () => {
    expect(liveBadge(conversation({ activity: 'working', state: 'blocked', waiting: { for: 'shell' } }), NOW)).toBeUndefined()
  })

  it('says how long a chat has been silent when its wait is read from that silence, not reported', () => {
    const since = (minutes: number) => new Date(NOW.getTime() - minutes * 60_000).toISOString()
    expect(liveBadge(conversation({ tool: 'codex', activity: 'waiting', quietSince: since(7) }), NOW)).toEqual({ label: 'No activity for 7m', tone: 'approval', description: 'Open now: nothing written for 7 minutes, so it may be waiting for you' })
    expect(liveBadge(conversation({ tool: 'codex', activity: 'waiting', quietSince: since(180) }), NOW)?.description).toBe('Open now: nothing written for 3 hours, so it may be waiting for you')
    expect(liveBadge(conversation({ tool: 'codex', activity: 'waiting', quietSince: since(60 * 24 * 2 + 5) }), NOW)?.label).toBe('No activity for 2d')
    expect(liveBadge(conversation({ tool: 'codex', activity: 'waiting', quietSince: since(1) }), NOW)?.description).toBe('Open now: nothing written for 1 minute, so it may be waiting for you')
    expect(liveBadge(conversation({ tool: 'codex', activity: 'waiting', quietSince: new Date(NOW.getTime() + 30_000).toISOString() }), NOW)?.label).toBe('No activity')
    expect(liveBadge(conversation({ tool: 'codex', activity: 'waiting', quietSince: 'not a time' }), NOW)?.label).toBe('Waiting for you')
    expect(liveBadge(conversation({ tool: 'codex', activity: 'waiting' }), NOW)?.label).toBe('Waiting for you')
  })

  it('shows a chip for a reply not yet read and for a chat marked as done, and none otherwise', () => {
    expect(stateBadge('unread')).toEqual({ label: 'New reply', tone: 'approval', description: 'Something was written since you last opened it' })
    expect(stateBadge('completed')).toEqual({ label: 'Archived', tone: 'neutral', description: 'You archived it; it comes back with a new message or when it starts working' })
    for (const state of ['blocked', 'working', 'active', 'dormant', undefined] as const) expect(stateBadge(state)).toBeUndefined()
    expect(stateBadge('active', true)).toEqual({ label: 'Unread', tone: 'approval', description: 'It was resumed with a reply from before that you have not read. Opening it here clears this.' })
    for (const state of ['blocked', 'working', 'dormant', undefined] as const) expect(stateBadge(state, true)).toBeUndefined()
  })

  it('shows a typed shell command as "! command" and hides a last prompt that repeats the title', () => {
    expect(chatText('<bash-input>gl</bash-input>')).toEqual({ text: '! gl', shell: true })
    expect(chatText(' Plan the release ')).toEqual({ text: 'Plan the release', shell: false })
    expect(chatText('<bash-input>docker compose up --build web…')).toEqual({ text: '! docker compose up --build web…', shell: true })
    expect(shownPrompt(chatText('<bash-input>gl</bash-input>'), '! gl')).toBeUndefined()
    expect(shownPrompt(chatText('/mcp'), '/mcp')).toBeUndefined()
    expect(shownPrompt(chatText('Plan'), undefined)).toBeUndefined()
    expect(shownPrompt(chatText('Plan'), '<bash-input>make test</bash-input>')).toEqual({ text: '! make test', shell: true })
    expect(shownPrompt(chatText('Plan'), 'Go on')).toEqual({ text: 'Go on', shell: false })
  })

  it('names every withheld field', () => {
    expect([withheldLabel('title'), withheldLabel('opening'), withheldLabel('prompt'), withheldLabel('reply')]).toEqual(['Title', 'Opening', 'Last prompt', 'Latest reply'])
  })

  it('names found tools without per-tool counts and preserves their source notes', () => {
    const source = (over: Partial<ConversationSource>): ConversationSource => ({ tool: 'codex', label: 'Codex', found: true, count: 66, shown: 2, ...over })
    expect(toolChoices([source({})])).toEqual([{ tool: 'codex', label: 'Codex', description: 'Show Codex conversations.' }])
    expect(toolChoices([source({ tool: 'cursor', label: 'Cursor', note: 'Read from the state file.' })])[0].description).toBe('Show Cursor conversations. Read from the state file.')
    expect(toolChoices([source({ found: false, count: 0, note: 'No store' })])).toEqual([])
  })

  it('says how long ago, then the date', () => {
    const ago = (minutes: number) => new Date(NOW.getTime() - minutes * 60_000).toISOString()
    expect(relativeTime(ago(0), NOW)).toBe('just now')
    expect(relativeTime(ago(-5), NOW)).toBe('just now')
    expect(relativeTime(ago(12), NOW)).toBe('12 min ago')
    expect(relativeTime(ago(3 * 60 + 5), NOW)).toBe('3 h ago')
    expect(relativeTime(ago(24 * 60), NOW)).toBe('1 day ago')
    expect(relativeTime(ago(6 * 24 * 60), NOW)).toBe('6 days ago')
    const lastMonth = local(10, 21, 19)
    expect(relativeTime(lastMonth.toISOString(), NOW)).toBe(lastMonth.toLocaleDateString(undefined, { day: 'numeric', month: 'short' }))
    const lastYear = new Date(2025, 11, 30, 9)
    expect(relativeTime(lastYear.toISOString(), NOW)).toContain('2025')
    expect(relativeTime('not a date', NOW)).toBe('not a date')
  })

  it('offers the reply command only when opening failed and the row does not offer the same one', () => {
    const terminal = { kind: 'terminal', label: 'Open in Terminal', command: 'claude --resume a' } as const
    const link = { kind: 'link', label: 'Open in Claude app', link: 'claude://resume?session=a' } as const
    expect(replyCommand({ opened: false, message: 'No terminal app found.', command: 'claude --resume a' }, link)).toBe('claude --resume a')
    expect(replyCommand({ opened: false, message: 'No terminal app found.', command: 'claude --resume a' }, terminal)).toBeUndefined()
    expect(replyCommand({ opened: false, message: 'Not found.' }, link)).toBeUndefined()
    expect(replyCommand({ opened: true, message: 'Opened.', command: 'claude --resume a' }, link)).toBeUndefined()
  })

  it('offers no resume command on a row that still runs, since running it starts a second copy', () => {
    const focus = { kind: 'focus', label: 'Open in Terminal', command: 'claude --resume a' } as const
    const terminal = { kind: 'terminal', label: 'Open in Terminal', command: 'claude --resume a' } as const
    expect(rowCommand(focus)).toBeUndefined()
    expect(rowCommand(terminal)).toBe('claude --resume a')
    expect(rowCommand({ kind: 'terminal', label: 'Open in Terminal' })).toBeUndefined()
    expect(replyCommand({ opened: false, message: 'Its folder name holds control characters.' }, { kind: 'terminal', label: 'Open in Terminal' })).toBeUndefined()
    expect(replyCommand({ opened: false, message: 'The session ended; no terminal app found.', command: 'claude --resume a' }, focus)).toBe('claude --resume a')
  })
})

describe('conversations normalization', () => {
  it('turns null lists into empty ones and drops withheld names and activities it does not know', () => {
    const view = normalizeConversationsView({
      schemaVersion: 'conversations/v1',
      generatedAt: '2026-11-07T15:30:00Z',
      tools: null,
      conversations: [{ tool: 'codex', app: 'cli', id: 'x', title: null, updatedAt: '2026-11-07T15:00:00Z', opening: null, withheld: ['prompt', 'transcript'], live: true, activity: 'compacting', open: { kind: 'terminal', label: 'Open in Terminal' } }],
    })
    expect(view.tools).toEqual([])
    expect(view).toMatchObject({ from: '', to: '' })
    expect(view.next).toBeUndefined()
    expect(normalizeConversationsView({ from: '2026-10-31T15:30:00Z', to: '2026-11-07T15:30:00Z', next: '2026-10-31T15:30:00Z' })).toMatchObject({ from: '2026-10-31T15:30:00Z', to: '2026-11-07T15:30:00Z', next: '2026-10-31T15:30:00Z' })
    expect(normalizeConversationsView({ next: '' }).next).toBeUndefined()
    expect(view.conversations[0]).toMatchObject({ title: '', opening: [], withheld: ['prompt'], live: true, activity: undefined, open: { label: 'Open in Terminal' } })
  })

  it('keeps when a silent wait began and drops a value that is not a time string', () => {
    const quiet = (quietSince: unknown) => normalizeConversationsView({ conversations: [{ tool: 'codex', app: 'cli', id: 'x', updatedAt: '2026-11-07T15:00:00Z', live: true, activity: 'waiting', quietSince, open: {} }] }).conversations[0].quietSince
    expect(quiet('2026-11-07T15:00:00Z')).toBe('2026-11-07T15:00:00Z')
    expect(quiet(42)).toBeUndefined()
    expect(quiet('')).toBeUndefined()
    expect(quiet('not a time')).toBeUndefined()
  })

  it('keeps the agent\'s latest reply when it is text and drops anything else', () => {
    const reply = (value: unknown) => normalizeConversationsView({ conversations: [{ tool: 'claude', app: 'cli', id: 'x', updatedAt: '2026-11-07T15:00:00Z', reply: value, withheld: ['reply', 'other'], open: {} }] }).conversations[0]
    expect(reply('The tests pass.').reply).toBe('The tests pass.')
    expect(reply(42).reply).toBeUndefined()
    expect(reply('  ').reply).toBeUndefined()
    expect(reply(undefined).withheld).toEqual(['reply'])
  })

  it('keeps why an archived row cannot be deleted and drops a value that is not a sentence', () => {
    const reason = (deleteUnsupported: unknown, state = 'completed') => normalizeConversationsView({ conversations: [{ tool: 'cursor', app: 'ide', id: 'x', updatedAt: '2026-11-07T15:00:00Z', state, deleteUnsupported, open: {} }] }).conversations[0].deleteUnsupported
    expect(reason('Delete it in Cursor.')).toBe('Delete it in Cursor.')
    expect(reason('Delete it in Cursor.', 'dormant')).toBe('Delete it in Cursor.')
    expect(reason(42)).toBeUndefined()
    expect(reason('')).toBeUndefined()
  })

  it('keeps a parent with a known tool and an id, and drops any other', () => {
    const parent = (value: unknown) => normalizeConversationsView({ conversations: [{ tool: 'claude', app: 'cli', id: 'x', updatedAt: '2026-11-07T15:00:00Z', parent: value, open: {} }] }).conversations[0].parent
    expect(parent({ tool: 'codex', id: 'abc' })).toEqual({ tool: 'codex', id: 'abc' })
    expect(parent({ tool: 'codex', id: 'abc', extra: 1 })).toEqual({ tool: 'codex', id: 'abc' })
    expect(parent({ tool: 'other', id: 'abc' })).toBeUndefined()
    expect(parent({ tool: 'claude', id: '' })).toBeUndefined()
    expect(parent({ tool: 'claude', id: 42 })).toBeUndefined()
    expect(parent('claude:abc')).toBeUndefined()
    expect(parent(null)).toBeUndefined()
  })

  it('reads the mock fixture with every app, route kind, live state, withheld field, and tool', () => {
    const view = normalizeConversationsView(fixture)
    expect(view.schemaVersion).toBe('conversations/v1')
    expect(new Set(view.conversations.map((c) => c.app))).toEqual(new Set(['cli', 'ide', 'desktop', 'background']))
    expect(new Set(view.conversations.map((c) => c.open.kind))).toEqual(new Set(['focus', 'terminal', 'link']))
    expect(view.conversations.filter((c) => c.live).map((c) => c.activity)).toEqual(['working', 'waiting', undefined])
    expect(new Set(view.conversations.flatMap((c) => c.withheld ?? []))).toEqual(new Set(['title', 'opening', 'prompt', 'reply']))
    expect(view.tools.map((t) => t.tool)).toEqual(['claude', 'codex', 'cursor'])
    for (const source of view.tools) expect(view.conversations.filter((c) => c.tool === source.tool)).toHaveLength(source.shown)
  })
})

describe('conversations verdict', () => {
  it('uses the server-wide count even when loaded or filtered rows show fewer', () => {
    const rows = [conversation({ live: true, state: 'blocked', activity: 'waiting' })]
    expect(conversationsVerdict(rows, NOW, false, 12)).toBe('12 chats need you.')
    expect(conversationsVerdict([], NOW, true, 1)).toBe('One chat needs you.')
    expect(conversationsVerdict(rows, NOW, false, 0)).toBe('No chats need you.')
  })

  it('counts the open chats and those that wait for you', () => {
    const open = [conversation({ id: 'a', live: true, activity: 'working' }), conversation({ id: 'b', live: true, activity: 'waiting' }), conversation({ id: 'c', live: true })]
    expect(conversationsVerdict(open, NOW, false)).toBe('3 chats are open now; 1 waits for you.')
    expect(conversationsVerdict(open.slice(0, 1), NOW, false)).toBe('One chat is open now.')
    expect(conversationsVerdict(open.slice(1, 2), NOW, false)).toBe('One chat is open and waits for you.')
    expect(conversationsVerdict([open[0], conversation({ id: 'd' })], NOW, false)).toBe('One chat is open now.')
  })

  it('does not say a chat waits for you when only its silence says so', () => {
    const quiet = conversation({ id: 'q', tool: 'codex', live: true, activity: 'waiting', quietSince: new Date(NOW.getTime() - 7 * 60_000).toISOString() })
    const waiting = conversation({ id: 'w', live: true, activity: 'waiting' })
    expect(conversationsVerdict([quiet], NOW, false)).toBe('One chat is open with no recent activity.')
    expect(conversationsVerdict([waiting, quiet], NOW, false)).toBe('2 chats are open now; 1 waits for you; 1 has no recent activity.')
    expect(conversationsVerdict([quiet, { ...quiet, id: 'r' }, conversation({ id: 'c', live: true })], NOW, false)).toBe('3 chats are open now; 2 have no recent activity.')
    expect(conversationsVerdict([{ ...quiet, quietSince: 'not a time' }], NOW, false)).toBe('One chat is open and waits for you.')
  })

  it('says when the last chat ran when none is open, or that there is none', () => {
    const rows = [conversation({ id: 'new', updatedAt: local(11, 7, 14, 30).toISOString() }), conversation({ id: 'old', updatedAt: local(11, 5, 9).toISOString() })]
    expect(conversationsVerdict(rows, NOW, false)).toBe('Your last chat was 2 h ago.')
    const older = local(10, 25, 9).toISOString()
    expect(conversationsVerdict([conversation({ updatedAt: older })], NOW, false)).toBe(`Your last chat was on ${relativeTime(older, NOW)}.`)
    expect(conversationsVerdict([], NOW, true)).toBe('No chat in the past week.')
    expect(conversationsVerdict([], NOW, false)).toBe('No chats to show.')
  })
})

describe('subagents', () => {
  const minutesAgo = (minutes: number) => new Date(NOW.getTime() - minutes * 60_000).toISOString()
  const helper = (over: Partial<ConversationHelper>): ConversationHelper => ({ tool: 'claude', id: 'agent1', title: 'Find the loader', state: 'working', updatedAt: minutesAgo(1), ...over })

  it('lists working subagents newest first, a time that does not parse last, and hides those past the first five', () => {
    const helpers = [helper({ id: 'old', updatedAt: minutesAgo(9) }), helper({ id: 'odd', updatedAt: 'not a time' }), helper({ id: 'new', updatedAt: minutesAgo(1) }), { ...helper({ id: 'done' }), state: 'finished' as 'working' }]
    expect(helperRows(conversation({ helpers }), false).rows.map((row) => row.id)).toEqual(['new', 'old', 'odd'])
    expect(helperRows(conversation({}), false)).toEqual({ rows: [], more: 0 })
    const many = Array.from({ length: HELPERS_SHOWN + 2 }, (_, index) => helper({ id: `a${index}`, updatedAt: minutesAgo(index) }))
    expect(helperRows(conversation({ helpers: many }), false)).toMatchObject({ more: 2 })
    expect(helperRows(conversation({ helpers: many }), false).rows.map((row) => row.id)).toEqual(['a0', 'a1', 'a2', 'a3', 'a4'])
    expect(helperRows(conversation({ helpers: many }), true)).toMatchObject({ more: 0, rows: { length: HELPERS_SHOWN + 2 } })
  })

  it('names a subagent with no title and leaves plain agent types out of its details', () => {
    expect(helperTitle(helper({ title: '' }))).toBe('Untitled subagent')
    expect(helperTitle(helper({}))).toBe('Find the loader')
    expect(helperDetails(helper({ kind: 'Explore', updatedAt: minutesAgo(12) }), NOW)).toBe('Claude Code · Explore · 12 min ago')
    expect(helperDetails(helper({ kind: 'general-purpose' }), NOW)).toBe('Claude Code · 1 min ago')
    expect(helperDetails(helper({ kind: 'default', tool: 'codex' }), NOW)).toBe('Codex · 1 min ago')
  })

  it('opens a Claude Code subagent through its chat, naming it, and a Codex one as its own thread', () => {
    const parent = conversation({ id: 'parent' })
    expect(helperOpenRequest(parent, helper({ id: 'agent1' }), 'zed')).toEqual({ tool: 'claude', id: 'parent', editor: 'zed', helper: 'agent1' })
    expect(helperOpenRequest(parent, helper({ tool: 'codex', id: 'thread' }), 'zed')).toEqual({ tool: 'codex', id: 'thread', editor: 'zed' })
    expect(helperTooltip(helper({}))).toBe('Open the chat it runs in, and show how to reach this subagent')
    expect(helperTooltip(helper({ tool: 'codex' }))).toBe('Open this Codex thread')
  })

  it('keeps only subagents that name a known tool and an id', () => {
    const view = normalizeConversationsView({ conversations: [{ tool: 'claude', app: 'cli', id: 'x', updatedAt: NOW.toISOString(), open: {}, helpers: [{ tool: 'claude', id: 'a', state: 'working', updatedAt: NOW.toISOString() }, { tool: 'gemini', id: 'b' }, { tool: 'codex', id: '' }, 'nope'] }, { tool: 'claude', app: 'cli', id: 'y', updatedAt: NOW.toISOString(), open: {}, helpers: 'nope' }] })
    expect(view.conversations[0].helpers).toEqual([{ tool: 'claude', id: 'a', title: '', state: 'working', updatedAt: NOW.toISOString() }])
    expect(view.conversations[1].helpers).toBeUndefined()
  })
})

describe('stuck shells', () => {
  const shell: StuckShell = { pid: 37, procStart: 'start', since: local(11, 7, 15).toISOString(), command: 'npm run dev', processCount: 2 }

  it('shows the shell age with the helper name when present', () => {
    expect(shellWarning(shell, NOW)).toBe('Shell running for 1 hour')
    expect(shellWarning({ ...shell, helperName: 'Faramir', since: local(11, 7, 16).toISOString() }, NOW)).toBe('Faramir left a shell running for 30 minutes')
  })

  it('keeps a selection only while its process and confirmation details match', () => {
    expect(selectedStuckShell([shell], { ...shell })).toBe(shell)
    for (const change of [{ procStart: 'replacement' }, { processCount: 3 }, { command: 'npm run preview' }, { since: NOW.toISOString() }, { helperName: 'Faramir' }]) {
      expect(selectedStuckShell([{ ...shell, ...change }], shell)).toBeUndefined()
    }
    expect(selectedStuckShell([], shell)).toBeUndefined()
  })

  it('drops malformed process details when reading the view', () => {
    const bad = [{ ...shell, pid: 0 }, { ...shell, procStart: '' }, { ...shell, processCount: -1 }, { ...shell, since: 'never' }]
    const view = normalizeConversationsView({ conversations: [{ ...conversation({ live: true }), stuckShells: [...bad, shell] }] })
    expect(view.conversations[0].stuckShells).toEqual([shell])
  })
})
