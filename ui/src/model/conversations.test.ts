import { describe, expect, it } from 'vitest'
import type { Conversation, ConversationSource, ConversationsView } from '../contract'
import fixture from '../fixtures/conversations.mock.json'
import { appLabel, conversationsVerdict, filterConversations, groupConversations, liveBadge, mergePages, relativeTime, replyCommand, rowCommand, sameFirstPage, shownPrompt, stateBadge, chatText, toggleTool, toolChoices, withheldLabel } from './conversations'
import { normalizeConversationsView } from './normalize'

// Local times, so the day groups hold in any time zone the tests run in.
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
  it('groups by local calendar day and keeps the server order within a group', () => {
    const rows = [
      conversation({ id: 'late-today', updatedAt: local(11, 7, 16, 10).toISOString() }),
      conversation({ id: 'midnight', updatedAt: local(11, 7, 0, 0).toISOString() }),
      conversation({ id: 'before-midnight', updatedAt: local(11, 6, 23, 59).toISOString() }),
      conversation({ id: 'two-days', updatedAt: local(11, 5, 12).toISOString() }),
      conversation({ id: 'six-days', updatedAt: local(11, 1, 9).toISOString() }),
      conversation({ id: 'seven-days', updatedAt: local(10, 31, 23, 59).toISOString() }),
    ]
    const groups = groupConversations(rows, NOW)
    expect(groups.map((group) => [group.label, group.conversations.map((c) => c.id)])).toEqual([
      ['Today', ['late-today', 'midnight']],
      ['Yesterday', ['before-midnight']],
      ['This week', ['two-days', 'six-days']],
      ['Earlier', ['seven-days']],
    ])
  })

  it('counts a future time as today and an unreadable one as earlier, and leaves empty groups out', () => {
    const groups = groupConversations([conversation({ id: 'ahead', updatedAt: local(11, 8, 9).toISOString() }), conversation({ id: 'broken', updatedAt: 'not a date' })], NOW)
    expect(groups.map((group) => [group.label, group.conversations.map((c) => c.id)])).toEqual([['Today', ['ahead']], ['Earlier', ['broken']]])
    expect(groupConversations([], NOW)).toEqual([])
  })

  it('puts every live conversation under Open now first, in server order, and not again under its day', () => {
    const rows = [
      conversation({ id: 'live-new', live: true, activity: 'working', updatedAt: local(11, 7, 16, 20).toISOString() }),
      conversation({ id: 'today', updatedAt: local(11, 7, 16, 0).toISOString() }),
      conversation({ id: 'live-old', live: true, updatedAt: local(11, 2, 9).toISOString() }),
      conversation({ id: 'earlier', updatedAt: local(10, 1, 9).toISOString() }),
    ]
    expect(groupConversations(rows, NOW).map((group) => [group.label, group.conversations.map((c) => c.id)])).toEqual([
      ['Open now', ['live-new', 'live-old']],
      ['Today', ['today']],
      ['Earlier', ['earlier']],
    ])
    expect(groupConversations([conversation({ id: 'quiet' })], NOW).map((group) => group.label)).toEqual(['Today'])
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

  it('keeps an old open conversation from the first page under Open now, and an unreadable time last', () => {
    const firstWeek = { conversations: [conversation({ id: 'live-old', live: true, updatedAt: local(9, 1, 9).toISOString() }), conversation({ id: 'today', updatedAt: local(11, 7, 9).toISOString() })] }
    const weekBefore = { conversations: [conversation({ id: 'broken', updatedAt: 'not a date' }), conversation({ id: 'older', updatedAt: local(10, 28, 9).toISOString() })] }
    const merged = mergePages([firstWeek, weekBefore])
    expect(ids(merged)).toEqual(['claude:today', 'claude:older', 'claude:live-old', 'claude:broken'])
    expect(groupConversations(merged, NOW).map((group) => [group.label, ids(group.conversations)])).toEqual([
      ['Open now', ['claude:live-old']],
      ['Today', ['claude:today']],
      ['Earlier', ['claude:older', 'claude:broken']],
    ])
  })
})

describe('conversation search', () => {
  const rows = [
    conversation({ id: 'ui', title: 'Add a Conversations page', folder: '/Users/you/Products/agnostic-ai', branch: 'motechfr/conversations-tab' }),
    conversation({ id: 'sqlite', tool: 'codex', title: 'Speed up the sqlite reader', opening: ['The Cursor store read takes 900 ms.'], prompt: 'Try immutable=1.' }),
    conversation({ id: 'site', tool: 'cursor', title: '', folder: '/Users/you/Products/website' }),
  ]
  const ids = (list: Conversation[]) => list.map((c) => c.id)

  it('matches title, folder, branch, and prompt as the row shows them, ignoring case and outer spaces', () => {
    expect(ids(filterConversations(rows, 'CONVERSATIONS', 'all'))).toEqual(['ui'])
    expect(ids(filterConversations(rows, 'website', 'all'))).toEqual(['site'])
    expect(ids(filterConversations(rows, 'motechfr/', 'all'))).toEqual(['ui'])
    expect(ids(filterConversations(rows, ' Immutable=1 ', 'all'))).toEqual(['sqlite'])
    expect(ids(filterConversations(rows, '900 ms', 'all'))).toEqual([])
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

describe('conversation words', () => {
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
    expect([withheldLabel('title'), withheldLabel('opening'), withheldLabel('prompt')]).toEqual(['Title', 'Opening', 'Last prompt'])
  })

  it('counts each found tool\'s rows across the loaded weeks against everything it holds, and leaves out a tool not found', () => {
    const source = (over: Partial<ConversationSource>): ConversationSource => ({ tool: 'codex', label: 'Codex', found: true, count: 66, shown: 2, ...over })
    const firstWeek = { conversations: [conversation({ id: 'a', tool: 'codex' }), conversation({ id: 'b', tool: 'codex' }), conversation({ id: 'c' })] }
    const weekBefore = { conversations: [conversation({ id: 'b', tool: 'codex' }), conversation({ id: 'd', tool: 'codex' })] }
    expect(toolChoices([source({})], mergePages([firstWeek]))).toEqual([{ tool: 'codex', label: 'Codex', loaded: 2, description: '2 of 66 Codex chats loaded.' }])
    expect(toolChoices([source({})], mergePages([firstWeek, weekBefore]))[0].loaded).toBe(3)
    expect(toolChoices([source({ tool: 'cursor', label: 'Cursor', count: 1, note: 'Read from the state file.' })], mergePages([firstWeek]))[0].description).toBe('0 of 1 Cursor chat loaded. Read from the state file.')
    expect(toolChoices([source({ found: false, count: 0, note: 'No store' })], mergePages([firstWeek]))).toEqual([])
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

  it('reads the mock fixture with every app, route kind, live state, withheld field, and tool', () => {
    const view = normalizeConversationsView(fixture)
    expect(view.schemaVersion).toBe('conversations/v1')
    expect(new Set(view.conversations.map((c) => c.app))).toEqual(new Set(['cli', 'ide', 'desktop', 'background']))
    expect(new Set(view.conversations.map((c) => c.open.kind))).toEqual(new Set(['focus', 'terminal', 'link']))
    expect(view.conversations.filter((c) => c.live).map((c) => c.activity)).toEqual(['working', 'waiting', undefined])
    expect(new Set(view.conversations.flatMap((c) => c.withheld ?? []))).toEqual(new Set(['title', 'opening', 'prompt']))
    expect(view.tools.map((t) => t.tool)).toEqual(['claude', 'codex', 'cursor'])
    for (const source of view.tools) expect(view.conversations.filter((c) => c.tool === source.tool)).toHaveLength(source.shown)
  })
})

describe('conversations verdict', () => {
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
