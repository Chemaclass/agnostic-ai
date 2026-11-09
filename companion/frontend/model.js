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
    // More matched than the list reads: say so instead of showing fewer rows than the count.
    if (view.conversations.length < view.matches) return `${view.matches} conversations match; the newest ${view.conversations.length} show`
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
