// Lists for the companion preview, in the shape GET /api/inbox answers. Each
// conversation was written some minutes before `now`; a test passes a fixed now
// to the preview and fixes the page clock to the same time.

const MINUTE = 60 * 1000

function conversation(now, tool, id, title, state, { folder = '/Users/you/Code/agnostic-ai', app = 'cli', minutesAgo = 5, branch = 'main' } = {}) {
  const cloud = app === 'cloud'
  return {
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
    conversation(now, 'claude', 'waits-1', 'Approve the database migration', 'blocked', { folder: '/Users/you/Code/shop', minutesAgo: 2 }),
    conversation(now, 'codex', 'reply-1', 'Draft the release notes', 'unread', { minutesAgo: 12 }),
    conversation(now, 'claude', 'work-1', 'Refactor the search index', 'working', { minutesAgo: 0 }),
    conversation(now, 'cursor', 'active-1', 'Fix the login redirect', 'active', { folder: '/Users/you/Code/shop', minutesAgo: 180 }),
    conversation(now, 'claude', 'active-2', 'Plan the sync dry run', 'active', { minutesAgo: 300 }),
    conversation(now, 'claude', 'cloud-1', 'Write the onboarding guide', 'active', { app: 'cloud', minutesAgo: 60 }),
    conversation(now, 'codex', 'done-1', 'Bump the Go version', 'completed', { minutesAgo: 2 * 24 * 60 }),
    conversation(now, 'cursor', 'old-1', 'Sketch the pricing page', 'dormant', { folder: '/Users/you/Code/site', minutesAgo: 9 * 24 * 60 }),
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
 * Each fixture is a list plus the ways its calls fail:
 * listProblem (the list cannot be read), cloudNote, marksNote, note (from the companion itself),
 * markProblem, renameProblem, openProblem (a conversation that did not open),
 * and slowQueries (queries that contain the word wait that many milliseconds).
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
  'mark-failure': {
    label: 'Archive and hide failure',
    conversations: mixed,
    markProblem: 'agnostic-ai could not save the mark: the marks file is read-only.',
  },
  'open-failure': {
    label: 'Open failure',
    conversations: mixed,
    openProblem: 'Codex runs in an app that cannot be brought forward. Open it from the Codex app.',
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
}

/** A conversation that arrives while the list shows, for refresh tests. */
export function arrival(now) {
  return conversation(now, 'claude', 'arrived-1', 'Answer the review comments', 'unread', { minutesAgo: 0 })
}
