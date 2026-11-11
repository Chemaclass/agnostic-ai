// What the list shows, worked out from the rows the shell sends. No DOM here.

/**
 * The groups, in the order the list shows them. Archived starts folded: it
 * holds what the person archived and what went quiet by itself.
 */
export const GROUPS = [
  { id: 'needs-you', title: 'Needs you', startsFolded: false },
  { id: 'working', title: 'Working', startsFolded: false },
  { id: 'active', title: 'Active', startsFolded: false },
  { id: 'archived', title: 'Archived', startsFolded: true },
]

/** Where conversations run, as the switch beside the search offers them. */
export const PLACES = [
  { id: 'all', label: 'All', title: 'Every conversation' },
  { id: 'local', label: 'Local', title: 'Conversations on this machine' },
  { id: 'cloud', label: 'Cloud', title: 'Conversations that run in a tool\'s cloud' },
]

/** Whether a conversation runs in `place`. */
const IN_PLACE = {
  all: () => true,
  local: (conversation) => conversation.app !== 'cloud',
  cloud: (conversation) => conversation.app === 'cloud',
}

const NEEDS_PERSON = { blocked: true, unread: true }

/**
 * The view narrowed to one place: its rows, how many of them need the
 * person and, in a search, how many match. `all` is the view as it came,
 * with the server's own counts, which also cover weeks the list has not read.
 */
export function viewIn(view, place) {
  if (place === 'all') return view
  const conversations = view.conversations.filter(IN_PLACE[place])
  const narrowed = { ...view, conversations, needsYou: conversations.filter((conversation) => NEEDS_PERSON[conversation.state]).length }
  if (view.matches !== undefined) narrowed.matches = conversations.length
  return narrowed
}

/** The group each state belongs to. */
const GROUP_OF_STATE = {
  blocked: 'needs-you',
  unread: 'needs-you',
  working: 'working',
  active: 'active',
  completed: 'archived',
  dormant: 'archived',
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

/** A typed name as one line, or undefined when nothing but spaces was typed. The server makes the other checks. */
export function nameOf(typed) {
  const name = typed.split(/\s+/).filter((word) => word !== '').join(' ')
  return name === '' ? undefined : name
}

/**
 * What Enter does with a typed name: `{ keep: true }` when it is the title the row already shows,
 * which may be cut short, so nothing is sent; `{ empty: true }` when nothing was typed; otherwise `{ title }`.
 */
export function renameOf(draft, shown) {
  const title = nameOf(draft)
  if (title === undefined) return { empty: true }
  if (title === shown) return { keep: true }
  return { title }
}

/** Whether a key press only confirms an input method's candidate, such as a Japanese conversion. */
export function confirmsCandidate(press) {
  return press.isComposing === true || press.keyCode === 229
}

/** What Escape closes: the row menu, then a rename, then the settings menu, then the panel. */
export function escapeTarget({ rowMenuOpen, renaming, settingsOpen }) {
  if (rowMenuOpen) return 'row-menu'
  if (renaming) return 'rename'
  if (settingsOpen) return 'settings'
  return 'panel'
}

/** The view with the row at `key` showing `title`, so a rename shows before the next list arrives. */
export function withTitle(view, key, title) {
  const conversations = view.conversations.map((conversation) => (keyOf(conversation) === key ? { ...conversation, title } : conversation))
  return { ...view, conversations }
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

/**
 * The one action a row offers: hide a chat that is archived, archive one that
 * is not. A working chat offers none, since the work would undo the archive.
 */
export function rowActionOf(conversation) {
  if (conversation.state === 'working') return undefined
  return GROUP_OF_STATE[conversation.state] === 'archived' ? 'hide' : 'archive'
}

/** Where a row runs, said only when it is not on this machine. */
const PLACE_LABELS = {
  cloud: 'Cloud',
}

/**
 * "Claude Code · agnostic-ai · 5m": the tool, where it runs when not here, the folder's name and how long ago it was written.
 * `names` holds what folderNamesOf gave for the folders in view; without one the folder shows its last name.
 */
export function detailsOf(conversation, now, names = new Map()) {
  const parts = [TOOL_LABELS[conversation.tool]]
  const place = PLACE_LABELS[conversation.app]
  if (place) parts.push(place)
  if (conversation.folder) parts.push(names.get(conversation.folder) ?? folderName(conversation.folder))
  parts.push(ago(conversation.updatedAt, now))
  return parts.join(' · ')
}

function namesOf(folder) {
  return folder.split(/[\\/]/).filter((name) => name !== '')
}

function tail(names, count) {
  return names.slice(-count).join('/')
}

function folderName(folder) {
  const names = namesOf(folder)
  return names.length === 0 ? folder : tail(names, 1)
}

/**
 * The name each folder shows: its last name, with as many parent folders as tell it apart
 * from the other folders in `folders` that end the same way, as in "shop/api" and "billing/api".
 */
export function folderNamesOf(folders) {
  const paths = new Map()
  for (const folder of folders) {
    const names = folder ? namesOf(folder) : []
    if (names.length > 0) paths.set(names.join('/'), names)
  }
  const shown = new Map([...paths.keys()].map((path) => [path, 1]))
  let growing = true
  while (growing) {
    growing = false
    const sharing = new Map()
    for (const [path, names] of paths) {
      const name = tail(names, shown.get(path))
      sharing.set(name, (sharing.get(name) ?? 0) + 1)
    }
    for (const [path, names] of paths) {
      const count = shown.get(path)
      if (count < names.length && sharing.get(tail(names, count)) > 1) {
        shown.set(path, count + 1)
        growing = true
      }
    }
  }
  const result = new Map()
  for (const folder of folders) {
    const names = folder ? namesOf(folder) : []
    if (names.length > 0) result.set(folder, tail(names, shown.get(names.join('/'))))
  }
  return result
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

/** The line at the foot: how the search went. Empty without a search. */
export function summaryOf(view, query) {
  if (query !== '') {
    if (view.matches === 0) return 'No conversation matches'
    // More matched than the list reads: say so instead of showing fewer rows than the count.
    if (view.conversations.length < view.matches) return `${view.matches} conversations match; the newest ${view.conversations.length} show`
    return view.matches === 1 ? '1 conversation matches' : `${view.matches} conversations match`
  }
  // Without a search the foot says nothing: the Needs you group already counts.
  return ''
}

/** The row after or before `key` among `keys`; the first row when `key` is not listed. step is 1 or -1. */
export function neighbour(keys, key, step) {
  if (keys.length === 0) return undefined
  const at = keys.indexOf(key)
  if (at === -1) return keys[0]
  return keys[Math.min(Math.max(at + step, 0), keys.length - 1)]
}

/** How far, in pixels, a press must travel before it drags the bubble. Less is a click with an unsteady hand. */
export const DRAG_DISTANCE = 4

/** Whether the pointer moved far enough from where it was pressed to be dragging. */
export function isDrag(pressed, pointer) {
  return Math.hypot(pointer.x - pressed.x, pointer.y - pressed.y) >= DRAG_DISTANCE
}

/** The key names Wails takes in a shortcut, by the browser's name for the physical key. Letters and digits are handled apart. */
const SHORTCUT_KEYS = {
  Space: 'Space',
  Enter: 'Enter',
  Tab: 'Tab',
  Backspace: 'Backspace',
  Delete: 'Delete',
  Home: 'Home',
  End: 'End',
  ArrowLeft: 'Left',
  ArrowRight: 'Right',
  ArrowUp: 'Up',
  ArrowDown: 'Down',
  F1: 'F1', F2: 'F2', F3: 'F3', F4: 'F4', F5: 'F5', F6: 'F6',
  F7: 'F7', F8: 'F8', F9: 'F9', F10: 'F10', F11: 'F11', F12: 'F12',
}

function shortcutKey(code) {
  const letter = /^Key([A-Z])$/.exec(code)
  if (letter) return letter[1]
  const digit = /^Digit([0-9])$/.exec(code)
  if (digit) return digit[1]
  return SHORTCUT_KEYS[code]
}

/**
 * The shortcut a key press spells, as Wails writes it ("Ctrl+Alt+Space"), or undefined while it is
 * not one yet: a shortcut needs a modifier other than Shift, so it cannot swallow plain typing, and a key
 * Wails can name. On a Mac the Command key is "Cmd"; elsewhere the Windows or Super key is "Super".
 */
export function shortcutOf(press, isMac) {
  const key = shortcutKey(press.code)
  if (key === undefined) return undefined
  const parts = []
  if (press.ctrlKey) parts.push('Ctrl')
  if (press.altKey) parts.push('Alt')
  if (press.metaKey) parts.push(isMac ? 'Cmd' : 'Super')
  if (parts.length === 0) return undefined
  if (press.shiftKey) parts.push('Shift')
  return [...parts, key].join('+')
}

/** How each part of a shortcut reads on a Mac, where the modifiers have symbols. */
const MAC_KEYCAPS = { Ctrl: '⌃', Alt: '⌥', Shift: '⇧', Cmd: '⌘', Super: '⌘' }

/** The keycaps that show a shortcut: ["⌃", "⌥", "Space"] on a Mac, ["Ctrl", "Alt", "Space"] elsewhere. */
export function keycapsOf(shortcut, isMac) {
  const parts = shortcut.split('+')
  if (!isMac) return parts
  return parts.map((part) => MAC_KEYCAPS[part] ?? part)
}
