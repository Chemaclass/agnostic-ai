// The Conversations page groups and nests rows by what needs attention.

import type { Conversation, ConversationActivity, ConversationApp, ConversationHelper, ConversationOpenReply, ConversationOpenRequest, ConversationProcessRef, ConversationRoute, ConversationSource, ConversationState, ConversationTool, ConversationsView, StuckShell } from '../contract'
import { plural } from './format'

const APP_LABELS = {
  cli: 'Terminal',
  ide: 'Editor',
  desktop: 'Desktop app',
  background: 'Background',
  cloud: 'Cloud',
  chat: 'Chat',
  cowork: 'Cowork',
} satisfies Record<ConversationApp, string>

/** Where the chat lives, in words; an app this client does not know shows its raw name. */
export function appLabel(app: ConversationApp): string {
  return Object.hasOwn(APP_LABELS, app) ? APP_LABELS[app] : app
}

type WithheldField = NonNullable<Conversation['withheld']>[number]

const WITHHELD_LABELS = {
  title: 'Title',
  opening: 'Opening',
  prompt: 'Last prompt',
  reply: 'Latest reply',
} satisfies Record<WithheldField, string>

export function withheldLabel(field: WithheldField): string {
  return WITHHELD_LABELS[field]
}

export const WITHHELD_NOTE = 'Hidden: may hold a credential'

interface ToolChoice {
  tool: ConversationTool
  label: string
  description: string
}

export function toolChoices(sources: ConversationSource[]): ToolChoice[] {
  return sources.filter((source) => source.found).map((source) => ({
    tool: source.tool,
    label: source.label,
    description: `Show ${source.label} conversations.${source.note ? ` ${source.note}` : ''}`,
  }))
}

export const CONVERSATION_PLACES = [
  { id: 'all', label: 'All', title: 'Every conversation' },
  { id: 'local', label: 'Local', title: 'Conversations on this machine' },
  { id: 'cloud', label: 'Cloud', title: "Conversations that run in a tool's cloud" },
] as const

export type PlaceFilter = (typeof CONVERSATION_PLACES)[number]['id']

function inPlace(conversation: Conversation, place: PlaceFilter): boolean {
  const online = conversation.app === 'cloud' || conversation.app === 'chat'
  if (place === 'local') return !online
  if (place === 'cloud') return online || conversation.cloud !== undefined
  return true
}

export function conversationSearchStatus(searching: boolean, total: number | undefined, kept: number, loaded: number, filtered: boolean): string {
  if (searching) return 'Searching…'
  if (filtered) return `${kept} of ${plural(loaded, 'loaded conversation')} ${kept === 1 ? 'matches' : 'match'}.`
  if (total !== undefined) return `${plural(total, 'conversation')} ${total === 1 ? 'matches' : 'match'}.`
  return ''
}

// The server cuts long titles and prompts, so the closing tag may be gone.
const SHELL_INPUT = /^\s*<bash-input>([\s\S]*?)(?:<\/bash-input>\s*)?$/

/** A title or prompt as the row shows it: a shell command the person typed reads as "! command". */
export interface ChatText {
  text: string
  shell: boolean
}

export function chatText(raw: string): ChatText {
  const shell = SHELL_INPUT.exec(raw)
  if (shell) return { text: `! ${shell[1].trim()}`, shell: true }
  return { text: raw.trim(), shell: raw.trim().startsWith('! ') }
}

/** The last prompt to show under the title, unless it says the same thing. */
export function shownPrompt(title: ChatText, prompt: string | undefined): ChatText | undefined {
  if (!prompt) return undefined
  const shown = chatText(prompt)
  if (shown.text === '' || shown.text === title.text) return undefined
  return shown
}

function conversationKey(conversation: Conversation): string {
  return `${conversation.tool}:${conversation.id}`
}

// A time that does not parse sorts last.
function writtenAt(row: Pick<Conversation, 'updatedAt'>): number {
  const time = Date.parse(row.updatedAt)
  return Number.isNaN(time) ? Number.NEGATIVE_INFINITY : time
}

function newestFirst(a: Pick<Conversation, 'updatedAt'>, b: Pick<Conversation, 'updatedAt'>): number {
  const first = writtenAt(a)
  const second = writtenAt(b)
  if (first === second) return 0
  return first > second ? -1 : 1
}

/** Every loaded week as one list: one row per tool and id, the later page's copy winning, newest first. groupConversations puts the rows needing attention on top. */
export function mergePages(pages: Pick<ConversationsView, 'conversations'>[]): Conversation[] {
  const rows = new Map<string, Conversation>()
  for (const page of pages) {
    for (const conversation of page.conversations) rows.set(conversationKey(conversation), conversation)
  }
  return [...rows.values()].sort(newestFirst)
}

/** Whether two first pages have the same visible content, ignoring when they were read. */
export function sameFirstPage(a: ConversationsView, b: ConversationsView): boolean {
  if (a.cloudNote !== b.cloudNote) return false
  const shown = (view: ConversationsView) => JSON.stringify([view.tools, view.conversations, view.needsYou, view.next !== undefined])
  return shown(a) === shown(b)
}

export type ToolFilter = ConversationTool | 'all'

function searchParts(conversation: Conversation): string[] {
  return [chatText(conversation.title).text, conversation.folder ?? '', conversation.branch ?? '', chatText(conversation.prompt ?? '').text, conversation.reply ?? '']
}

/** Matches the selected place and tool, and every search word in the displayed title, folder, branch, prompt or reply. */
export function filterConversations(conversations: Conversation[], query: string, tool: ToolFilter, place: PlaceFilter = 'all'): Conversation[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean)
  return conversations.filter((conversation) => {
    const toolMatches = tool === 'all' || conversation.tool === tool
    const text = searchParts(conversation).join('\n').toLowerCase()
    return toolMatches && inPlace(conversation, place) && words.every((word) => text.includes(word))
  })
}

/** A tool button toggles its tool: pressing the chosen one again shows every tool. */
export function toggleTool(current: ToolFilter, tool: ConversationTool): ToolFilter {
  return current === tool ? 'all' : tool
}

type GroupId = 'needs-you' | 'working' | 'active' | 'archived'

const GROUP_LABELS = {
  'needs-you': 'Needs you',
  working: 'Working',
  active: 'Active',
  archived: 'Archived',
} satisfies Record<GroupId, string>

const GROUP_ORDER: GroupId[] = ['needs-you', 'working', 'active', 'archived']

interface ConversationGroup {
  id: GroupId
  label: string
  conversations: Conversation[]
  under: ReadonlyMap<string, Conversation>
}

const GROUP_OF_STATE = {
  blocked: 'needs-you',
  unread: 'needs-you',
  working: 'working',
  active: 'active',
  completed: 'archived',
  dormant: 'archived',
} satisfies Record<ConversationState, GroupId>

export function conversationGroupOf(conversation: Conversation): GroupId {
  if (conversation.claimConflicts?.length) return 'needs-you'
  if (conversation.state !== undefined) return GROUP_OF_STATE[conversation.state]
  return conversation.live ? 'active' : 'archived'
}

function urgencyOf(conversation: Conversation): number {
  if (conversation.state === 'blocked' && conversation.waiting?.for === 'permission') return 0
  return conversation.state === 'blocked' || conversation.claimConflicts?.length ? 1 : 2
}

function mostUrgentFirst(group: Pick<ConversationGroup, 'conversations' | 'under'>): Pick<ConversationGroup, 'conversations' | 'under'> {
  const families: { conversations: Conversation[]; urgency: number }[] = []
  for (const conversation of group.conversations) {
    if (!group.under.has(conversationKey(conversation))) families.push({ conversations: [], urgency: Infinity })
    const family = families[families.length - 1]
    family.conversations.push(conversation)
    family.urgency = Math.min(family.urgency, urgencyOf(conversation))
  }
  families.sort((a, b) => a.urgency - b.urgency)
  return { conversations: families.flatMap((family) => family.conversations), under: group.under }
}

export function groupConversations(conversations: Conversation[], _now?: Date): ConversationGroup[] {
  const ordered = [...conversations].sort(newestFirst)
  return GROUP_ORDER.map((id) => {
    const nested = nestRows(ordered.filter((conversation) => conversationGroupOf(conversation) === id))
    return { id, label: GROUP_LABELS[id], ...(id === 'needs-you' ? mostUrgentFirst(nested) : nested) }
  }).filter((group) => group.conversations.length > 0)
}

// A walk that meets a loop gives no top, so every row on it stays a normal row.
const LOOP = Symbol('loop')

/**
 * The top of each row's family in `rows`: the last of its parents, walking up, that is in `rows`,
 * or the row itself. Each walk stops at a row already worked out, so the whole list costs one pass.
 */
function topsOf(rows: ReadonlyMap<string, Conversation>): (conversation: Conversation) => Conversation | typeof LOOP {
  const tops = new Map<string, Conversation | typeof LOOP>()
  return (conversation) => {
    const path: string[] = []
    const onPath = new Set<string>()
    let at = conversation
    let top: Conversation | typeof LOOP
    for (;;) {
      const key = conversationKey(at)
      const known = tops.get(key)
      if (known !== undefined) {
        top = known
        break
      }
      path.push(key)
      onPath.add(key)
      const above = at.parent && rows.get(`${at.parent.tool}:${at.parent.id}`)
      if (!above) {
        top = at
        break
      }
      if (onPath.has(conversationKey(above))) {
        top = LOOP
        break
      }
      at = above
    }
    for (const key of path) tops.set(key, top)
    return top
  }
}

// When a child started: startedAt, else its last write. A time that does not parse counts as the oldest.
function startedAt(conversation: Conversation): number {
  const time = Date.parse(conversation.startedAt ?? conversation.updatedAt)
  return Number.isNaN(time) ? Number.NEGATIVE_INFINITY : time
}

/** Oldest start first, then by tool and id, so a new message moves no child. Each time is read once. */
function byStart(family: Conversation[]): Conversation[] {
  return family
    .map((conversation) => ({ conversation, time: startedAt(conversation), key: conversationKey(conversation) }))
    .sort((a, b) => (a.time !== b.time ? (a.time < b.time ? -1 : 1) : a.key === b.key ? 0 : a.key < b.key ? -1 : 1))
    .map((entry) => entry.conversation)
}

/**
 * One group's rows as the list draws them: every descendant right after the root it sits under,
 * oldest first, one level deep. The first row with a tool and id owns its children, so as many rows come out as go in.
 */
function nestRows(rows: Conversation[]): Pick<ConversationGroup, 'conversations' | 'under'> {
  const byKey = new Map<string, Conversation>()
  for (const conversation of rows) {
    if (!byKey.has(conversationKey(conversation))) byKey.set(conversationKey(conversation), conversation)
  }
  const topOf = topsOf(byKey)
  const under = new Map<string, Conversation>()
  const children = new Map<Conversation, Conversation[]>()
  for (const conversation of rows) {
    const top = topOf(conversation)
    if (top === LOOP || conversationKey(top) === conversationKey(conversation)) continue
    under.set(conversationKey(conversation), top)
    const family = children.get(top)
    if (family) family.push(conversation)
    else children.set(top, [conversation])
  }
  const conversations: Conversation[] = []
  for (const conversation of rows) {
    if (under.has(conversationKey(conversation))) continue
    conversations.push(conversation)
    for (const child of byStart(children.get(conversation) ?? [])) conversations.push(child)
  }
  return { conversations, under }
}

interface LiveBadge {
  label: string
  /** The badge tone: amber when the agent waits for the person. */
  tone: 'permitted' | 'approval'
  /** The full sentence, for the title and screen readers. */
  description: string
}

const ACTIVITY_BADGES = {
  working: { label: 'Working', tone: 'permitted', description: 'Open now: the agent is working' },
  waiting: { label: 'Waiting for you', tone: 'approval', description: 'Open now: the agent is waiting for you' },
} satisfies Partial<Record<ConversationActivity, LiveBadge>>

/** A live activity chip. A wait read from silence says how long nothing came instead. */
export function liveBadge(conversation: Conversation, now: Date): LiveBadge | undefined {
  if (conversation.waiting?.for === 'shell') return undefined
  const { activity } = conversation
  if (activity === 'waiting') return quietBadge(conversation, now) ?? ACTIVITY_BADGES.waiting
  if (activity === 'working') return ACTIVITY_BADGES[activity]
  return undefined
}

function quietBadge(conversation: Conversation, now: Date): LiveBadge | undefined {
  const quiet = quietFor(conversation, now)
  if (!quiet) return undefined
  if (quiet.minutes < 1) return { label: 'No activity', tone: 'approval', description: 'Open now: nothing written lately, so it may be waiting for you' }
  return { label: `No activity for ${quiet.short}`, tone: 'approval', description: `Open now: nothing written for ${quiet.long}, so it may be waiting for you` }
}

/** How long a wait read from silence has lasted: "7m" and "7 minutes". Undefined for a wait the tool reported. */
function quietFor(conversation: Conversation, now: Date): { minutes: number; short: string; long: string } | undefined {
  if (!conversation.quietSince) return undefined
  const since = new Date(conversation.quietSince).getTime()
  if (Number.isNaN(since)) return undefined
  const minutes = Math.floor((now.getTime() - since) / 60_000)
  if (minutes < 60) return { minutes, short: `${minutes}m`, long: plural(minutes, 'minute') }
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return { minutes, short: `${hours}h`, long: plural(hours, 'hour') }
  const days = Math.floor(hours / 24)
  return { minutes, short: `${days}d`, long: plural(days, 'day') }
}

const STATE_BADGES = {
  unread: { label: 'New reply', tone: 'approval', description: 'Something was written since you last opened it' },
  completed: { label: 'Archived', tone: 'neutral', description: 'You archived it; it comes back with a new message or when it starts working' },
} satisfies Partial<Record<ConversationState, StateBadge>>

const RESUMED_UNREAD: StateBadge = { label: 'Unread', tone: 'approval', description: 'It was resumed with a reply from before that you have not read. Opening it here clears this.' }

interface StateBadge {
  label: string
  /** Amber for a reply that waits to be read, neutral for a finished chat. */
  tone: 'approval' | 'neutral'
  /** The full sentence, for the title and screen readers. */
  description: string
}

/** The chip for the person's own marks: a reply not yet read, a chat resumed with one, or a chat marked as done. Waiting and working already have the live chip, and any other active or dormant chat needs none. */
export function stateBadge(state: ConversationState | undefined, unread?: boolean): StateBadge | undefined {
  if (state === 'unread' || state === 'completed') return STATE_BADGES[state]
  if (state === 'active' && unread) return RESUMED_UNREAD
  return undefined
}

/** "just now", "12 min ago", "3 h ago", "2 days ago", then the date. */
export function relativeTime(iso: string, now: Date): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  const minutes = Math.floor((now.getTime() - date.getTime()) / 60_000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes} min ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} h ago`
  const days = Math.floor(hours / 24)
  if (days < 7) return `${plural(days, 'day')} ago`
  const sameYear = date.getFullYear() === now.getFullYear()
  return date.toLocaleDateString(undefined, sameYear ? { day: 'numeric', month: 'short' } : { day: 'numeric', month: 'short', year: 'numeric' })
}

/** The resume command a row offers to copy. A running session gets none: running it would start a second copy. */
export function rowCommand(route: ConversationRoute): string | undefined {
  if (route.kind === 'focus') return undefined
  return route.command
}

/** The command left after an open could not reach its target, unless the row already offers it. */
export function replyCommand(reply: ConversationOpenReply, route: ConversationRoute): string | undefined {
  if (!reply.command || (reply.opened && reply.reached !== 'parent')) return undefined
  if (reply.command === rowCommand(route)) return undefined
  return reply.command
}

/** The page's answer to "Where did you leave off?": the open chats and how many wait, else when the last one ran. */
export function conversationsVerdict(conversations: Conversation[], now: Date, olderWeeks: boolean, needsYou?: number): string {
  if (needsYou !== undefined) {
    if (needsYou === 0) return 'No chats need you.'
    return needsYou === 1 ? 'One chat needs you.' : `${needsYou} chats need you.`
  }
  const open = conversations.filter((conversation) => conversation.live)
  if (open.length > 0) {
    // A wait read from silence is not said as a certain wait.
    const waits = open.filter((conversation) => conversation.activity === 'waiting')
    const quiet = waits.filter((conversation) => quietFor(conversation, now)).length
    const waiting = waits.length - quiet
    if (open.length === 1) return waiting ? 'One chat is open and waits for you.' : quiet ? 'One chat is open with no recent activity.' : 'One chat is open now.'
    const parts = [`${open.length} chats are open now`]
    if (waiting) parts.push(`${waiting} ${waiting === 1 ? 'waits' : 'wait'} for you`)
    if (quiet) parts.push(`${quiet} ${quiet === 1 ? 'has' : 'have'} no recent activity`)
    return `${parts.join('; ')}.`
  }
  const newest = conversations[0]
  if (newest) {
    const when = relativeTime(newest.updatedAt, now)
    return /ago$|^just now$/.test(when) ? `Your last chat was ${when}.` : `Your last chat was on ${when}.`
  }
  return olderWeeks ? 'No chat in the past week.' : 'No chats to show.'
}

/** How many subagents show under a chat before "Show N more". */
export const HELPERS_SHOWN = 5

const TOOL_LABELS = {
  claude: 'Claude Code',
  codex: 'Codex',
  cursor: 'Cursor',
  chatgpt: 'ChatGPT',
} satisfies Record<ConversationTool, string>

export function conversationToolLabel(conversation: Conversation): string {
  if (conversation.tool === 'claude' && (conversation.app === 'chat' || conversation.app === 'cowork')) return 'Claude'
  return TOOL_LABELS[conversation.tool]
}

export function conversationPlaceLabel(conversation: Conversation): string | undefined {
  if (conversation.cloud) return 'Shared'
  if (conversation.app === 'cloud' || conversation.app === 'chat' || conversation.app === 'cowork') return APP_LABELS[conversation.app]
  return undefined
}

type ConversationRef = Pick<Conversation, 'tool' | 'id'>
type ConversationName = (ref: ConversationRef) => string | undefined

function claimConflictReason(conversation: Conversation, nameOf: ConversationName = () => undefined): string | undefined {
  const conflicts = conversation.claimConflicts ?? []
  if (conflicts.length === 0) return undefined
  const [first] = conflicts
  const name = (ref: ConversationRef) => nameOf(ref) || `${TOOL_LABELS[ref.tool]} session ${ref.id.slice(0, 8)}`
  const self = conversationKey(conversation)
  let text: string
  if (`${first.requester.tool}:${first.requester.id}` !== self) text = `${name(first.requester)} wants ${first.path}, which this session claims`
  else if (first.holder && `${first.holder.tool}:${first.holder.id}` !== self) text = `${first.path} is claimed by ${name(first.holder)}`
  else if (first.sender) text = `${name(first.sender)} no longer holds ${first.path}`
  else text = `No session holds ${first.path} any more`
  return conflicts.length > 1 ? `${text}, and ${plural(conflicts.length - 1, 'more conflict')}` : text
}

const APPROVAL_VERBS: Readonly<Record<string, string>> = { Edit: 'edit', MultiEdit: 'edit', Write: 'edit', NotebookEdit: 'edit', Read: 'read' }

export function needsYouReason(conversation: Conversation, now: Date, nameOf?: ConversationName): string | undefined {
  if (conversationGroupOf(conversation) !== 'needs-you') return undefined
  const waiting = conversation.waiting
  if (conversation.state === 'blocked' && waiting?.for === 'permission') {
    if (!waiting.tool) return 'Needs permission'
    if (waiting.tool.startsWith('mcp__')) return waiting.detail ? `Approve ${waiting.detail}` : 'Needs permission'
    if (!waiting.detail) return `Approve ${waiting.tool}`
    const verb = APPROVAL_VERBS[waiting.tool]
    return `${verb ? `Approve ${verb}` : 'Approve'} ${waiting.detail}`
  }
  const conflict = claimConflictReason(conversation, nameOf)
  if (conflict) return conflict
  if (conversation.state === 'unread') return conversation.withheld?.includes('reply') ? 'New reply, hidden because it may hold a password or key' : 'New reply'
  if (waiting?.for === 'shell') {
    const count = conversation.live ? conversation.stuckShells?.length ?? 0 : 0
    return count > 1 ? `Waiting on ${count} shells still running` : count === 1 ? 'Waiting on a shell still running' : undefined
  }
  if (waiting?.for === 'answer') return 'Asks you a question'
  const since = Date.parse(conversation.quietSince ?? '')
  if (!Number.isNaN(since)) {
    const minutes = Math.floor((now.getTime() - since) / 60_000)
    if (minutes < 1) return 'No activity'
    const age = minutes < 60 ? `${minutes}m` : minutes < 1440 ? `${Math.floor(minutes / 60)}h` : minutes < 20160 ? `${Math.floor(minutes / 1440)}d` : `${Math.floor(minutes / 10080)}w`
    return `No activity for ${age}`
  }
  return 'Waiting for you'
}

/** Agent types that say nothing beyond "a subagent", so the details leave them out. */
const PLAIN_KINDS = new Set(['general-purpose', 'default'])

/** The working subagents a chat shows, newest first, and how many more a "Show N more" button hides: HELPERS_SHOWN at most, or every one once the person asked for all. */
export function helperRows(conversation: Conversation, showAll: boolean): { rows: ConversationHelper[]; more: number } {
  const ordered = (conversation.helpers ?? []).filter((helper) => helper.state === 'working').sort(newestFirst)
  if (showAll || ordered.length <= HELPERS_SHOWN) return { rows: ordered, more: 0 }
  return { rows: ordered.slice(0, HELPERS_SHOWN), more: ordered.length - HELPERS_SHOWN }
}

/** A subagent's title; one whose tool gave none, or whose title may hold a credential, gets a neutral label. */
export function helperTitle(helper: ConversationHelper): string {
  return helper.title === '' ? 'Untitled subagent' : helper.title
}

/** "Claude Code · Explore · 12 min ago": the subagent's tool, its agent type when that says something, and its age. */
export function helperDetails(helper: ConversationHelper, now: Date): string {
  const parts = [TOOL_LABELS[helper.tool]]
  if (helper.kind && !PLAIN_KINDS.has(helper.kind)) parts.push(helper.kind)
  parts.push(relativeTime(helper.updatedAt, now))
  return parts.join(' · ')
}

/**
 * What a click on a subagent opens: a Codex thread opens itself, as a Codex chat does.
 * A Claude Code subagent runs inside its chat and has no place of its own, so its chat opens, naming the subagent.
 */
export function helperOpenRequest(parent: Conversation, helper: ConversationHelper, editor: string): ConversationOpenRequest {
  if (helper.tool === 'codex') return { tool: 'codex', id: helper.id, editor }
  return { tool: parent.tool, id: parent.id, editor, helper: helper.id }
}

export function helperTooltip(helper: ConversationHelper): string {
  return helper.tool === 'codex' ? 'Open this Codex thread' : 'Open the chat it runs in, and show how to reach this subagent'
}

export function isConversationProcessRef(value: unknown): value is ConversationProcessRef {
  if (!value || typeof value !== 'object') return false
  const { pid, procStart } = value as Record<string, unknown>
  return typeof pid === 'number' && Number.isSafeInteger(pid) && pid > 1 && typeof procStart === 'string' && procStart.trim() !== ''
}

export function selectedStuckShell(shells: readonly StuckShell[], selected: StuckShell): StuckShell | undefined {
  return shells.find((shell) => shell.pid === selected.pid &&
    shell.procStart === selected.procStart &&
    shell.command === selected.command &&
    shell.since === selected.since &&
    shell.helperName === selected.helperName &&
    shell.processCount === selected.processCount)
}

export function shellWarning(shell: StuckShell, now: Date): string {
  const minutes = Math.max(0, Math.floor((now.getTime() - Date.parse(shell.since)) / 60_000))
  const hours = Math.floor(minutes / 60)
  const age = hours >= 24 ? plural(Math.floor(hours / 24), 'day')
    : hours > 0 ? plural(hours, 'hour')
    : minutes > 0 ? plural(minutes, 'minute')
    : 'less than a minute'
  const subject = shell.helperName ? `${shell.helperName} left a shell running` : 'Shell running'
  return `${subject} for ${age}`
}
