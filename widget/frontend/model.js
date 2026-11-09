// What the list shows, worked out from the rows the shell sends. No DOM here.

/** The groups, in the order the list shows them. Done and Dormant start folded. */
export const GROUPS = [
  { id: 'needs-you', title: 'Needs you', startsFolded: false },
  { id: 'working', title: 'Working', startsFolded: false },
  { id: 'active', title: 'Active', startsFolded: false },
  { id: 'done', title: 'Done', startsFolded: true },
  { id: 'dormant', title: 'Dormant', startsFolded: true },
]

/** The group each state belongs to. */
const GROUP_OF_STATE = {
  blocked: 'needs-you',
  unread: 'needs-you',
  working: 'working',
  active: 'active',
  completed: 'done',
  dormant: 'dormant',
}

/** Why a row is where it is, said before the tool's name; states that need no words have none. */
const REASON_OF_STATE = {
  blocked: 'Waiting for you',
  unread: 'New reply',
  working: 'Working',
}

const TOOL_LABELS = {
  claude: 'Claude Code',
  codex: 'Codex',
  cursor: 'Cursor',
}

/** Ids are unique within a tool, not across tools. */
export function keyOf(conversation) {
  return conversation.tool + ':' + conversation.id
}

/** The rows of each group that has any, in list order. */
export function grouped(conversations) {
  return GROUPS.map((group) => ({
    ...group,
    rows: conversations.filter((conversation) => GROUP_OF_STATE[conversation.state] === group.id),
  })).filter((group) => group.rows.length > 0)
}

export function reasonOf(conversation) {
  return REASON_OF_STATE[conversation.state]
}

/** "Claude Code · agnostic-ai · 5m": the tool, the folder's name and how long ago it was written. */
export function detailsOf(conversation, now) {
  const parts = [TOOL_LABELS[conversation.tool]]
  if (conversation.folder) parts.push(folderName(conversation.folder))
  parts.push(ago(conversation.updatedAt, now))
  return parts.join(' · ')
}

function folderName(folder) {
  const names = folder.split(/[\\/]/).filter((name) => name !== '')
  return names.length === 0 ? folder : names[names.length - 1]
}

const MINUTE = 60 * 1000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

/** A short age: now, 5m, 3h, 2d, 6w. */
export function ago(stamp, now) {
  const age = now - Date.parse(stamp)
  if (age < MINUTE) return 'now'
  if (age < HOUR) return Math.floor(age / MINUTE) + 'm'
  if (age < DAY) return Math.floor(age / HOUR) + 'h'
  if (age < 14 * DAY) return Math.floor(age / DAY) + 'd'
  return Math.floor(age / (7 * DAY)) + 'w'
}

/** The line at the foot: what needs the person, or how the search went. */
export function summaryOf(view, query) {
  if (query !== '') {
    if (view.matches === 0) return 'No conversation matches'
    return view.matches === 1 ? '1 conversation matches' : `${view.matches} conversations match`
  }
  if (view.needsYou === 0) return 'All caught up'
  return view.needsYou === 1 ? '1 conversation needs you' : `${view.needsYou} conversations need you`
}

/** The row after or before `key` among `keys`; the first row when `key` is not listed. step is 1 or -1. */
export function neighbour(keys, key, step) {
  if (keys.length === 0) return undefined
  const at = keys.indexOf(key)
  if (at === -1) return keys[0]
  return keys[Math.min(Math.max(at + step, 0), keys.length - 1)]
}
