// Lists for the companion preview, in the shape GET /api/inbox answers. Each
// conversation was written some minutes before `now`; a test passes a fixed now
// to the preview and fixes the page clock to the same time.

const MINUTE = 60 * 1000

function conversation(now, tool, id, title, state, { folder = '/Users/you/Code/agnostic-ai', app = 'cli', minutesAgo = 5, branch = 'main', deleteUnsupported, reply, parent, shared } = {}) {
  const cloud = app === 'cloud'
  return {
    ...(deleteUnsupported ? { deleteUnsupported } : {}),
    ...(reply ? { reply } : {}),
    ...(parent ? { parent } : {}),
    ...(shared ? { live: true, cloud: { id: shared, link: 'https://claude.ai/code/' + shared.replace(/^cse_/, 'session_') } } : {}),
    tool,
    app,
    id,
    title,
    folder: cloud ? '' : folder,
    branch,
    updatedAt: new Date(now - minutesAgo * MINUTE).toISOString(),
    state,
    open: cloud ? { kind: 'browser', label: 'Open on claude.ai' } : { kind: 'focus', label: 'Open in Terminal' },
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
 * Each fixture is a list plus the ways its calls fail:
 * listProblem (the list cannot be read), cloudNote, marksNote, note (from the companion itself),
 * markProblem, renameProblem, deleteProblem, openProblem (a conversation that did not open) with its openCommand,
 * slowQueries (queries that contain the word wait that many milliseconds),
 * heldOpens (opens of these conversation ids wait until POST /dev/release-opens),
 * clipboard (the copied text; three lines when unset) and clipboardMarkdown (it came as HTML that goes as Markdown).
 */
export const FIXTURES = {
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
  nested: { label: 'Chats started from other chats', conversations: nested },
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
            helper('claude', 'agent-explore', 'Find rename call sites', 1, 'Explore'),
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
  'one-terminal': {
    label: 'One chat runs in a Ghostty tab, so pick mode starts on it',
    conversations: (now) => mixed(now).map((row) => (row.id === 'active-2' ? { ...row, live: true, open: { kind: 'focus', label: 'Switch to Ghostty' } } : row)),
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
      mixed(now).map((row) => (row.id === 'active-2' || row.id === 'work-1' ? { ...row, live: true, open: { kind: 'focus', label: 'Switch to Ghostty' } } : row)),
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
  'held-open-failure': {
    label: 'Open failure for "Plan the sync dry run", held until /dev/release-opens',
    conversations: mixed,
    openProblem: 'macOS blocked opening Terminal. Allow agnostic-ai companion to control Terminal in System Settings, or run this command yourself.',
    openCommand: "cd '/Users/you/Code/agnostic-ai' && claude --resume active-2",
    heldOpens: ['active-2'],
  },
}

/** A conversation that arrives while the list shows, for refresh tests. */
export function arrival(now) {
  return conversation(now, 'claude', 'arrived-1', 'Answer the review comments', 'unread', { minutesAgo: 0 })
}
