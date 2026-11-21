// Lists for the companion preview, in the shape GET /api/inbox answers. Each
// conversation was written some minutes before `now`; a test passes a fixed now
// to the preview and fixes the page clock to the same time.

const MINUTE = 60 * 1000

/** The states the server gives only a chat a running process holds. */
const LIVE_STATES = new Set(['blocked', 'working'])

/** The command that resumes a local chat, as internal/conversations writes it. */
const RESUME = { claude: 'claude --resume', codex: 'codex resume', cursor: 'cursor-agent --resume' }

/** How the server reopens a chat (internal/conversations, Route): a link for a cloud one, the running tab for a live one, a new tab otherwise. */
function openOf(tool, app, id, folder, live) {
  if (app === 'cloud') {
    return tool === 'codex'
      ? { kind: 'link', label: 'Open in Codex app', link: `codex://threads/${id}?hostId=durable` }
      : { kind: 'link', label: 'Open on claude.ai', link: 'https://claude.ai/code/session_' + id.replace(/^cse_/, '') }
  }
  const command = `cd '${folder}' && ${RESUME[tool]} ${id}`
  return live ? { kind: 'focus', label: 'Open in Terminal', command } : { kind: 'terminal', label: 'Resume in Terminal', command }
}

function conversation(now, tool, id, title, state, { folder = '/Users/you/Code/agnostic-ai', app = 'cli', minutesAgo = 5, branch = 'main', deleteUnsupported, reply, parent, shared, live = LIVE_STATES.has(state) || shared !== undefined } = {}) {
  const cloud = app === 'cloud'
  return {
    ...(deleteUnsupported ? { deleteUnsupported } : {}),
    ...(reply ? { reply } : {}),
    ...(parent ? { parent } : {}),
    ...(live ? { live: true } : {}),
    ...(state === 'completed' ? { completedAt: new Date(now).toISOString() } : {}),
    // Like the server: a live Claude Code session in a terminal can be stopped.
    ...(live && tool === 'claude' && app === 'cli' ? { stoppable: { pid: 4000 + id.length, procStart: 'Fri Nov 20 09:00:00 2026' } } : {}),
    ...(shared ? { cloud: { id: shared, link: 'https://claude.ai/code/' + shared.replace(/^cse_/, 'session_') } } : {}),
    tool,
    app,
    id,
    title,
    folder: cloud ? '' : folder,
    branch,
    updatedAt: new Date(now - minutesAgo * MINUTE).toISOString(),
    state,
    open: openOf(tool, app, id, folder, live),
  }
}

function mixed(now) {
  return [
    conversation(now, 'claude', 'waits-1', 'Approve the database migration', 'blocked', {
      folder: '/Users/you/Code/shop',
      minutesAgo: 2,
      reply: 'Should I run the migration on the staging copy first?',
    }),
    conversation(now, 'codex', 'reply-1', 'Draft the release notes', 'unread', { minutesAgo: 12, reply: 'The draft is ready in CHANGES.md, with one section per area.' }),
    conversation(now, 'claude', 'work-1', 'Refactor the search index', 'working', { minutesAgo: 0, reply: 'Moving the tokenizer into its own package.' }),
    conversation(now, 'cursor', 'active-1', 'Fix the login redirect', 'active', { folder: '/Users/you/Code/shop', minutesAgo: 180 }),
    conversation(now, 'claude', 'active-2', 'Plan the sync dry run', 'active', { minutesAgo: 300 }),
    conversation(now, 'claude', 'cloud-1', 'Write the onboarding guide', 'active', { app: 'cloud', minutesAgo: 60 }),
    conversation(now, 'codex', 'done-1', 'Bump the Go version', 'completed', { minutesAgo: 2 * 24 * 60 }),
    conversation(now, 'cursor', 'old-1', 'Sketch the pricing page', 'dormant', { folder: '/Users/you/Code/site', minutesAgo: 9 * 24 * 60 }),
  ]
}

/** Archived chats: Claude Code sessions that can be deleted, archived by hand or quiet for days, and others that cannot. */
function archived(now) {
  return [
    conversation(now, 'claude', 'active-3', 'Write the migration guide', 'active', { minutesAgo: 30 }),
    conversation(now, 'claude', 'done-2', 'Tidy the old branches', 'completed', { minutesAgo: 3 * 24 * 60 }),
    conversation(now, 'codex', 'done-3', 'Bump the Go version', 'completed', {
      minutesAgo: 2 * 24 * 60,
      deleteUnsupported: 'Deleting Codex threads from here is not supported: Codex deletes a thread even while it is open. Close it, then run this in a terminal: codex delete done-3',
    }),
    conversation(now, 'claude', 'old-2', 'Sketch the pricing page', 'dormant', { folder: '/Users/you/Code/site', minutesAgo: 9 * 24 * 60 }),
    conversation(now, 'cursor', 'old-3', 'Rename the config keys', 'dormant', {
      minutesAgo: 12 * 24 * 60,
      deleteUnsupported: 'Cursor keeps every chat in one shared database and has no command to delete one. Delete it in Cursor.',
    }),
  ]
}

/** A local session that Remote Control also shows on claude.ai, beside a plain cloud row and a plain local one. */
function shared(now) {
  return [
    conversation(now, 'claude', 'rc-1', 'Review the payment retries', 'blocked', { minutesAgo: 3, shared: 'cse_rc1' }),
    conversation(now, 'claude', 'cloud-3', 'Write the onboarding guide', 'active', { app: 'cloud', minutesAgo: 60 }),
    conversation(now, 'claude', 'local-3', 'Plan the sync dry run', 'active', { minutesAgo: 300 }),
  ]
}

/** Four chats that need the person, three here and one in the cloud, two of them about the parser. */
function needsYou(now) {
  return [
    conversation(now, 'claude', 'wait-2', 'Approve the schema change', 'blocked', { minutesAgo: 3, reply: 'Should I add the new column as nullable first?' }),
    conversation(now, 'codex', 'reply-2', 'Split the parser module', 'unread', { minutesAgo: 8 }),
    conversation(now, 'cursor', 'reply-3', 'Review the parser tests', 'unread', { folder: '/Users/you/Code/shop', minutesAgo: 20 }),
    conversation(now, 'claude', 'cloud-2', 'Write the support guide', 'unread', { app: 'cloud', minutesAgo: 40 }),
    conversation(now, 'claude', 'active-5', 'Plan the next release', 'active', { minutesAgo: 90 }),
  ]
}

const STATES = ['blocked', 'unread', 'working', 'active', 'active', 'completed', 'dormant']
const TOOLS = ['claude', 'codex', 'cursor']

function many(now) {
  return Array.from({ length: 60 }, (_, at) =>
    conversation(now, TOOLS[at % TOOLS.length], 'many-' + at, `Conversation number ${at + 1}`, STATES[at % STATES.length], {
      folder: `/Users/you/Code/project-${at % 9}`,
      minutesAgo: at * 47,
    }),
  )
}

function longNames(now) {
  return [
    conversation(now, 'claude', 'long-1', 'Investigate why the nightly integration run fails only on the second Tuesday of each month when the cache is cold and the network is slow', 'blocked', {
      folder: '/Users/you/Code/an-organisation-with-a-long-name/a-monorepo-with-an-even-longer-name/packages/the-service-that-has-the-longest-name-of-all',
    }),
    conversation(now, 'codex', 'long-2', 'Rewrite_the_configuration_loader_so_that_it_accepts_nested_environment_overrides_without_breaking', 'working'),
    conversation(now, 'cursor', 'long-3', '', 'active', { minutesAgo: 90 }),
  ]
}

function duplicateProjects(now) {
  return [
    conversation(now, 'claude', 'api-1', 'Add rate limits', 'unread', { folder: '/Users/you/Code/shop/api' }),
    conversation(now, 'codex', 'api-2', 'Split the user routes', 'working', { folder: '/Users/you/Code/billing/api' }),
    conversation(now, 'cursor', 'api-3', 'Fix the health check', 'active', { folder: '/Users/you/Work/api', minutesAgo: 240 }),
  ]
}

/**
 * Conversations started from others: a child under its parent in Needs you and in Active, a waiting child
 * whose parent is in Active, a local child of a cloud parent, and a child whose parent is not in the list.
 */
function nested(now) {
  const from = (id) => ({ tool: 'claude', id })
  return [
    conversation(now, 'claude', 'nest-blocked', 'Check the docs links', 'blocked', { minutesAgo: 4, parent: from('nest-active') }),
    conversation(now, 'codex', 'nest-reply', 'Update the API docs', 'unread', { minutesAgo: 10, parent: from('nest-wait') }),
    conversation(now, 'claude', 'nest-wait', 'Approve the API change', 'blocked', { minutesAgo: 30, reply: 'Should the old field stay for one release?' }),
    conversation(now, 'cursor', 'nest-active-kid', 'Write the docs index', 'active', { minutesAgo: 60, parent: from('nest-active') }),
    conversation(now, 'claude', 'nest-active', 'Plan the docs site', 'active', { minutesAgo: 120 }),
    conversation(now, 'claude', 'nest-local', 'Try the API guide examples', 'active', { minutesAgo: 150, parent: from('nest-cloud') }),
    conversation(now, 'claude', 'nest-cloud', 'Draft the API guide', 'active', { app: 'cloud', minutesAgo: 200 }),
    conversation(now, 'codex', 'nest-orphan', 'Tidy the docs styles', 'active', { minutesAgo: 240, parent: from('gone-1') }),
  ]
}

/**
 * Chats that wait on the person, as the server says what for: a command, an edit and an MCP tool to approve,
 * a question, and a Codex cloud thread whose approval names no call. Two have working subagents.
 */
function approvals(now) {
  const helper = (id, title, minutesAgo) => ({ tool: 'claude', id, title, kind: 'Explore', state: 'working', updatedAt: new Date(now - minutesAgo * MINUTE).toISOString() })
  const permission = (tool, detail) => ({ activity: 'waiting', live: true, waiting: { for: 'permission', ...(tool ? { tool } : {}), ...(detail ? { detail } : {}) } })
  return [
    {
      ...conversation(now, 'claude', 'approve-bash', 'Refactor PRs', 'blocked', { folder: '/Users/you/Code/shop', minutesAgo: 2, reply: 'The seed fix and the flag rename are still running.' }),
      ...permission('Bash', 'git push origin…'),
    },
    { ...conversation(now, 'claude', 'approve-edit', 'Webhooks v2 dashboard', 'blocked', { folder: '/Users/you/Code/kombo', minutesAgo: 30 }), ...permission('Edit', 'dashboard.tsx') },
    {
      ...conversation(now, 'claude', 'approve-mcp', 'Rider lifecycle gaps in Notion', 'blocked', { folder: '/Users/you/Code/glue', minutesAgo: 4, reply: 'Pulling the rider pages to compare stages.' }),
      ...permission('mcp__bfe9f18c-b8fd-43ae-b28d-268779cd446a__notion-fetch', 'Notion fetch'),
      workingSubagents: 2,
      helpers: [helper('agent-pages', 'Read the rider pages', 1), helper('agent-stages', 'List the stages', 2)],
    },
    {
      ...conversation(now, 'claude', 'question', 'SFTP write: discussion', 'blocked', { folder: '/Users/you/Code/shop', minutesAgo: 6, reply: 'Should the upload overwrite existing files?' }),
      activity: 'waiting',
      live: true,
      waiting: { for: 'answer' },
      workingSubagents: 1,
      helpers: [helper('agent-sftp', 'Check the SFTP folder', 3)],
    },
    { ...conversation(now, 'codex', 'approve-cloud', 'Release notes draft', 'blocked', { app: 'cloud', minutesAgo: 12 }), ...permission() },
  ]
}

/**
 * Rows as the server shapes them while approvals answer prompts (issue #3700): a chat whose requests the hook
 * approved, so it works on with no row under Needs you, a request the gate left to the person with its rule, and a
 * tool approvals do not reach.
 */
function autoApprovals(now) {
  const permission = (detail, reason) => ({ activity: 'waiting', live: true, waiting: { for: 'permission', tool: 'Bash', detail }, approvals: { reason } })
  const approved = (minutesAgo, detail) => ({ time: new Date(now - minutesAgo * MINUTE).toISOString(), call: 'Bash', detail })
  return [
    {
      ...conversation(now, 'claude', 'auto-push', 'Ship the parser fix', 'blocked', { minutesAgo: 1 }),
      ...permission('git push origin main', 'Always asks: publishing'),
      approvals: { approved: 2, reason: 'Always asks: publishing', recent: [approved(2, 'go test ./...'), approved(4, 'go vet ./...')] },
    },
    {
      ...conversation(now, 'claude', 'auto-tests', 'Fix the flaky login test', 'working', { minutesAgo: 0, reply: 'Running the suite again.' }),
      activity: 'working',
      approvals: { approved: 3, recent: [approved(0, 'go test ./internal/auth/...'), approved(1, 'make lint'), approved(3)] },
    },
    { ...conversation(now, 'cursor', 'auto-cursor', 'Rename the config keys', 'blocked', { folder: '/Users/you/Code/shop', minutesAgo: 6 }), ...permission('npm run build', 'Approvals not supported for Cursor') },
  ]
}

function stuckShells(now) {
  const makeShell = (pid, minutes, command, processCount) => {
    const since = new Date(now - minutes * MINUTE).toISOString()
    return { pid, procStart: since, since, command, processCount }
  }
  return [
    {
      ...conversation(now, 'claude', 'shell-blocked', 'Preview the reporting changes', 'blocked'),
      waiting: { for: 'shell' },
      stuckShells: [{ ...makeShell(531, 15 * 60, 'bash scripts/run-preview.bash', 3), helperId: 'faramir', helperName: 'Faramir' }],
    },
    {
      ...conversation(now, 'claude', 'shells-working', 'Refactor the search index', 'working'),
      stuckShells: [makeShell(840, 90, 'npm run dev', 3), makeShell(910, 45, 'npm run watch', 2)],
    },
  ]
}

/**
 * Each fixture is a list plus the ways its calls fail:
 * listProblem (the list cannot be read), cloudNote, marksNote, note (from the companion itself),
 * markProblem, renameProblem, deleteProblem, deleteNote (deleted, with something left to tidy),
 * openProblem (a conversation that did not open) with its openCommand,
 * sendProblem (the send call fails), unsent (nothing was typed and the conversation was not switched to),
 * slowQueries (queries that contain the word wait that many milliseconds),
 * searchPages (a search answers {size} matches per page, each after {ms} milliseconds),
 * rereadMs (a mark answers that many milliseconds later, once the list read after it arrives),
 * marksIgnored (a mark is kept, but the list read after it shows the chat where it was),
 * knownClosed and appArchived (ids known to be closed or archived by their tools),
 * heldOpens (opens of these conversation ids wait until POST /dev/release-opens),
 * openPending (the server did not answer an open in time, so nobody knows yet whether it opened),
 * lateOpenProblem (such an open fails later, with openCommand, once POST /dev/settle-late-opens),
 * clipboard (the copied text; three lines when unset) and clipboardMarkdown (it came as HTML that goes as Markdown),
 * approvalsMode (the loosest approvals mode the home config sets, until Ask me).
 */
const MAILBOX_SOURCE = { tool: 'claude', id: '84fc6d21-820c-4c88-9729-ff8ee40de1f3' }
const MAILBOX_TARGET = { tool: 'codex', id: '0199a2b4-5c1e-7d30-9f2a-3b6c8d1e4f50' }
const MAILBOX_THIRD = { tool: 'codex', id: 'e50eac57-0dcd-4089-bd24-6c95d4171790' }

function mailboxSessions(now) {
  return [
    conversation(now, MAILBOX_SOURCE.tool, MAILBOX_SOURCE.id, 'Review the parser', 'unread'),
    conversation(now, MAILBOX_TARGET.tool, MAILBOX_TARGET.id, 'Draft the tests', 'working'),
  ]
}

const reviewRefs = {
  worktree: '/Users/you/Code/agnostic-ai',
  commit: '1'.repeat(40),
  base: '2'.repeat(40),
  branch: 'feat/mailbox',
  paths: ['internal/mailbox/store.go', 'internal/mailbox/store_test.go'],
  uncommitted: 2,
}

function mailboxThread(now, { count, status, state, id }) {
  const at = new Date(now).toISOString()
  return {
    schemaVersion: 'mailbox/v1', id, status, cap: 8, between: [MAILBOX_SOURCE, MAILBOX_TARGET], refusals: [], events: [],
    messages: Array.from({ length: count }, (_, i) => ({
      n: i + 1, type: 'question', from: { ...MAILBOX_SOURCE, by: 'human' }, to: MAILBOX_TARGET,
      body: 'Parser question ' + (i + 1), state, at, queuedAt: at,
    })),
  }
}

export const FIXTURES = {
  mailbox: { label: 'Mailbox sessions', conversations: mailboxSessions, mailboxTypingRefused: true },
  'mailbox-handoff': {
    label: 'Sessions for a handoff',
    conversations: (now) => [
      ...mailboxSessions(now),
      conversation(now, MAILBOX_THIRD.tool, MAILBOX_THIRD.id, 'Handle the conflict', 'active'),
    ],
  },
  'mailbox-stale-claim': {
    label: 'File claim with a missing session',
    conversations: mailboxSessions,
    mailboxClaims: (now) => [{
      owner: MAILBOX_THIRD, worktree: '/Users/you/Code/agnostic-ai', paths: ['internal/mailbox/handoff.go'],
      since: new Date(now).toISOString(), by: 'human',
    }],
  },
  'mailbox-review': {
    label: 'Review request with a default base', conversations: mailboxSessions, reviewRefs,
  },
  'mailbox-review-no-base': {
    label: 'Review request with no default base', conversations: mailboxSessions, reviewRefs, mailboxNoDefaultBase: true,
  },
  'mailbox-cap': {
    label: 'A mailbox thread at its cap', conversations: mailboxSessions,
    mailbox: (now) => [mailboxThread(now, { count: 8, status: 'open', state: 'staged', id: 'th_20261107_00000001' })],
  },
  'mailbox-statuses': {
    label: 'Mailbox thread states', conversations: mailboxSessions,
    mailbox: (now) => [
      mailboxThread(now, { count: 1, status: 'open', state: 'waiting', id: 'th_20261107_00000001' }),
      mailboxThread(now, { count: 1, status: 'paused', state: 'waiting', id: 'th_20261107_00000002' }),
      mailboxThread(now, { count: 1, status: 'closing', state: 'waiting', id: 'th_20261107_00000003' }),
      mailboxThread(now, { count: 1, status: 'closed', state: 'cancelled', id: 'th_20261107_00000004' }),
    ],
  },
  'mailbox-warning': {
    label: 'An unreadable mailbox thread', conversations: mailboxSessions,
    mailboxWarning: 'One mailbox thread could not be read.',
  },

  mixed: { label: 'Mixed waiting and working', conversations: mixed },
  empty: { label: 'Empty inbox', conversations: () => [] },
  many: { label: 'Many sessions', conversations: many },
  'long-names': { label: 'Long names', conversations: longNames },
  'duplicate-projects': { label: 'Duplicate project names', conversations: duplicateProjects },
  'cloud-failure': {
    label: 'Cloud failure',
    conversations: (now) => mixed(now).filter((row) => row.app !== 'cloud'),
    cloudNote: 'Claude Code cloud sessions could not be read: the sign-in expired.',
  },
  'rename-failure': {
    label: 'Rename failure',
    conversations: mixed,
    renameProblem: 'agnostic-ai could not save the name: the marks file is read-only.',
  },
  'needs-you': { label: 'Four chats that need you, one in the cloud', conversations: needsYou },
  'stuck-shells': { label: 'Forgotten shells on waiting and working Claude Code chats', conversations: stuckShells },
  nested: { label: 'Chats started from other chats', conversations: nested },
  approvals: { label: 'Chats waiting for an approval or an answer', conversations: approvals },
  'auto-approvals': { label: 'Approvals in yolo: approved for you, always asks, and a tool approvals do not reach', conversations: autoApprovals, approvalsMode: 'yolo' },
  shared: { label: 'A local chat also open on claude.ai', conversations: shared },
  workers: {
    label: 'Native sub-agents: three working, two while waiting, unknown',
    conversations: (now) => [
      { ...conversation(now, 'claude', 'workers-1', 'Build the companion', 'working'), activity: 'working', workingSubagents: 3 },
      { ...conversation(now, 'codex', 'workers-2', 'Review the implementation', 'blocked'), workingSubagents: 2 },
      { ...conversation(now, 'claude', 'workers-idle', 'Idle parent with a worker', 'working'), activity: 'idle', workingSubagents: 1 },
      { ...conversation(now, 'codex', 'workers-wait-read', 'Waiting parent already read', 'working'), activity: 'waiting', workingSubagents: 1 },
      { ...conversation(now, 'codex', 'workers-codex-done', 'Codex parent after its turn', 'working'), workingSubagents: 1 },
      conversation(now, 'claude', 'workers-unknown', 'Activity not available', 'working'),
    ],
  },
  helpers: {
    label: 'Working subagents under their chat, Claude Code and Codex; a chat whose subagents all finished shows none',
    conversations: (now) => {
      const helper = (tool, id, title, minutesAgo, kind, state = 'working') => ({ tool, id, title, ...(kind ? { kind } : {}), state, updatedAt: new Date(now - minutesAgo * MINUTE).toISOString() })
      return [
        {
          ...conversation(now, 'claude', 'helpers-1', 'Rename chats', 'working', { minutesAgo: 0, reply: 'All four Codex reviews are running.' }),
          workingSubagents: 4,
          helpers: [
            helper('codex', 'run-3511', 'Review #3511: local app chats', 2),
            { ...helper('claude', 'agent-explore', 'Find rename call sites', 1, 'Explore'), reply: 'Twelve call sites so far, most in internal/cli.' },
            helper('claude', 'agent-size', 'Size budget check', 1, 'general-purpose'),
            helper('codex', 'run-3512', 'Review #3512: rename in each app', 4),
            helper('claude', 'agent-plan', 'Helper rows design', 6, 'Plan'),
            helper('codex', 'run-3513', 'Review #3513: working orbits', 9),
            helper('claude', 'agent-old', '', 30, 'general-purpose'),
          ],
        },
        {
          // The server sends only working subagents; a finished one that still came is not shown.
          ...conversation(now, 'codex', 'helpers-2', 'Plan the migration', 'active', { minutesAgo: 20 }),
          workingSubagents: 0,
          helpers: [helper('codex', 'sub-1', 'Aquinas', 25, 'explorer', 'finished')],
        },
      ]
    },
  },
  'needs-you-mark-failure': {
    label: 'Mark all as read failure',
    conversations: needsYou,
    markProblem: 'agnostic-ai could not save the mark: the marks file is read-only.',
  },
  'mark-failure': {
    label: 'Archive and hide failure',
    conversations: mixed,
    markProblem: 'agnostic-ai could not save the mark: the marks file is read-only.',
  },
  archived: { label: 'Archived chats to delete', conversations: archived },
  completions: {
    label: 'Completed chats in a terminal and the cloud, and one archived in its app',
    conversations: (now) => [
      conversation(now, 'claude', 'recent-complete', 'Check the new config', 'completed'),
      conversation(now, 'claude', 'closed-complete', 'Finish the install', 'completed'),
      conversation(now, 'claude', 'cloud-complete', 'Draft the release guide', 'completed', { app: 'cloud', minutesAgo: 3 * 24 * 60 }),
      { ...conversation(now, 'claude', 'app-archive', 'Review next week', 'dormant', { app: 'cloud' }), completedAt: new Date(now).toISOString() },
    ],
    knownClosed: ['closed-complete'],
    appArchived: ['app-archive'],
  },
  'archived-shared': {
    label: 'An archived chat that was also on claude.ai, so its menu offers both the page and delete',
    conversations: (now) => [...archived(now), conversation(now, 'claude', 'done-rc', 'Ship the retry queue', 'completed', { minutesAgo: 2 * 24 * 60, shared: 'cse_done', live: false })],
  },
  'one-terminal': {
    label: 'One chat runs in a Ghostty tab, so pick mode starts on it',
    conversations: (now) => mixed(now).map((row) => (row.id === 'active-2' ? { ...row, live: true, open: { ...row.open, kind: 'focus', label: 'Switch to Ghostty' } } : row)),
  },
  'one-terminal-markdown': {
    label: 'A copied web page and one chat in a Ghostty tab, so the send shortcut sends Markdown straight there',
    conversations: (now) => FIXTURES['one-terminal'].conversations(now),
    clipboard: 'Release notes\nFixed two bugs in the parser.\nSee the migration guide.',
    clipboardMarkdown: true,
  },
  'two-terminals': {
    label: 'Two chats run in Ghostty tabs, so the send shortcut asks which',
    conversations: (now) =>
      mixed(now).map((row) => (row.id === 'active-2' || row.id === 'work-1' ? { ...row, live: true, open: { ...row.open, kind: 'focus', label: 'Switch to Ghostty' } } : row)),
  },
  'empty-clipboard': { label: 'Nothing copied, so pick mode does not open', conversations: mixed, clipboard: '' },
  'markdown-clipboard': {
    label: 'A copied web page that goes as Markdown',
    conversations: mixed,
    clipboard: 'Release notes\nFixed two bugs in the parser.\nSee the migration guide.',
    clipboardMarkdown: true,
  },
  'long-clipboard': {
    label: 'A long copied page to send',
    conversations: mixed,
    clipboard: Array.from({ length: 40 }, (_, at) => `Line ${at + 1} of the copied page, with enough words to wrap the banner.`).join('\n'),
  },
  'delete-failure': {
    label: 'Delete failure',
    conversations: archived,
    deleteProblem:
      'Deleted 1 of 4 locations, then stopped: permission denied. Still on disk: /Users/you/.claude/file-history/done-2, /Users/you/.claude/projects/-Users-you-Code-agnostic-ai/done-2.jsonl. Its marks were kept.',
  },
  'delete-note': {
    label: 'Deleted, with something left to tidy',
    conversations: archived,
    deleteNote: 'Its marks could not be removed: the marks file is read-only.',
  },
  'send-failure': {
    label: 'Send failure',
    conversations: mixed,
    sendProblem: 'agnostic-ai serve did not answer in time.',
  },
  'switch-failure': {
    label: 'Send that neither typed nor switched',
    conversations: mixed,
    unsent: 'Cursor runs in an app that cannot be brought forward. The text is on the clipboard: paste it with Cmd+V.',
  },
  'open-failure': {
    label: 'Open failure',
    conversations: mixed,
    openProblem: 'Codex runs in an app that cannot be brought forward. Open it from the Codex app.',
  },
  'resume-failure': {
    label: 'Resume failure with a command',
    conversations: mixed,
    openProblem: 'macOS blocked opening Terminal. Allow agnostic-ai companion to control Terminal in System Settings, or run this command yourself.',
    openCommand: "cd '/Users/you/Code/agnostic-ai' && claude --resume active-2",
  },
  'list-failure': {
    label: 'List cannot be read',
    conversations: mixed,
    listProblem: 'agnostic-ai serve stopped before it listened: address already in use',
  },
  'version-note': {
    label: 'Version mismatch',
    conversations: mixed,
    note: 'This companion was built for agnostic-ai 0.80.0 and 0.81.0 is installed. Run: agnostic-ai companion install',
  },
  'slow-search': {
    label: 'Slow search for "release"',
    conversations: mixed,
    slowQueries: { release: 1500 },
  },
  'paged-search': {
    label: 'Search answers two matches per page, each 0.6 seconds later',
    conversations: mixed,
    searchPages: { size: 2, ms: 600 },
  },
  'slow-reread': {
    label: 'A mark answers 2.5 seconds later, with the list read after it',
    conversations: mixed,
    rereadMs: 2500,
  },
  'marks-ignored': {
    label: 'The list read after a mark still shows the chat where it was',
    conversations: mixed,
    marksIgnored: true,
  },
  'slow-open': {
    label: 'Open the server is slow to answer',
    conversations: mixed,
    openPending: true,
  },
  'late-open-failure': {
    label: 'Open that fails after the server was slow to answer, once /dev/settle-late-opens',
    conversations: archived,
    openPending: true,
    lateOpenProblem: 'macOS blocked opening Terminal. Allow agnostic-ai companion to control Terminal in System Settings, or run this command yourself.',
    openCommand: "cd '/Users/you/Code/agnostic-ai' && claude --resume done-2",
  },
  'held-late-open-failure': {
    label: 'Open of "Tidy the old branches" held until /dev/release-opens, then still opening, then failing',
    conversations: archived,
    openPending: true,
    heldOpens: ['done-2'],
    lateOpenProblem: 'macOS blocked opening Terminal. Allow agnostic-ai companion to control Terminal in System Settings, or run this command yourself.',
    openCommand: "cd '/Users/you/Code/agnostic-ai' && claude --resume done-2",
  },
  'held-open-failure': {
    label: 'Open failure for "Plan the sync dry run", held until /dev/release-opens',
    conversations: mixed,
    openProblem: 'macOS blocked opening Terminal. Allow agnostic-ai companion to control Terminal in System Settings, or run this command yourself.',
    openCommand: "cd '/Users/you/Code/agnostic-ai' && claude --resume active-2",
    heldOpens: ['active-2'],
  },
  'closed-unread': {
    label: 'A terminal chat that closed with an unread reply, under a folded Archived, and one resumed by hand with one',
    conversations: (now) => [
      { ...conversation(now, 'claude', 'active-6', 'Write the migration guide', 'active', { minutesAgo: 30, live: true }), unread: true },
      { ...conversation(now, 'claude', 'closed-1', 'Port the billing job', 'dormant', { minutesAgo: 9 * 24 * 60, reply: 'The job now runs on the new queue.' }), unread: true },
      conversation(now, 'claude', 'old-4', 'Sketch the pricing page', 'dormant', { folder: '/Users/you/Code/site', minutesAgo: 12 * 24 * 60 }),
    ],
  },
}

/** A conversation that arrives while the list shows, for refresh tests. */
export function arrival(now) {
  return conversation(now, 'claude', 'arrived-1', 'Answer the review comments', 'unread', { minutesAgo: 0 })
}
