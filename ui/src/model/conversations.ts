// The Conversations page: the Open now and day groups, search, the tool
// filter, and the words each app, activity, source, and withheld field
// shows as. The server sends the rows newest first; these helpers keep
// that order.

import type { Conversation, ConversationActivity, ConversationApp, ConversationOpenReply, ConversationRoute, ConversationSource, ConversationState, ConversationTool, ConversationsView } from '../contract'
import { plural } from './format'

const APP_LABELS = {
  cli: 'Terminal',
  ide: 'Editor',
  desktop: 'Desktop app',
  background: 'Background',
  cloud: 'Cloud',
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

/** One button of the tool filter: a tool whose store was found, with its rows loaded so far. */
interface ToolChoice {
  tool: ConversationTool
  label: string
  loaded: number
  /** How many rows are loaded of every conversation it holds, and the server's note. */
  description: string
}

/** The tool filter's buttons. A tool whose store was not found has nothing to filter, so it is left out. */
export function toolChoices(sources: ConversationSource[], loaded: Conversation[]): ToolChoice[] {
  return sources.filter((source) => source.found).map((source) => {
    const rows = loaded.filter((conversation) => conversation.tool === source.tool).length
    const description = `${rows} of ${plural(source.count, `${source.label} chat`)} loaded.${source.note ? ` ${source.note}` : ''}`
    return { tool: source.tool, label: source.label, loaded: rows, description }
  })
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

// A time that does not parse sorts last, as it groups under Earlier.
function writtenAt(conversation: Conversation): number {
  const time = Date.parse(conversation.updatedAt)
  return Number.isNaN(time) ? Number.NEGATIVE_INFINITY : time
}

function newestFirst(a: Conversation, b: Conversation): number {
  const first = writtenAt(a)
  const second = writtenAt(b)
  if (first === second) return 0
  return first > second ? -1 : 1
}

/** Every loaded week as one list: one row per tool and id, the later page's copy winning, newest first. groupConversations puts the open ones on top. */
export function mergePages(pages: Pick<ConversationsView, 'conversations'>[]): Conversation[] {
  const rows = new Map<string, Conversation>()
  for (const page of pages) {
    for (const conversation of page.conversations) rows.set(conversationKey(conversation), conversation)
  }
  return [...rows.values()].sort(newestFirst)
}

/** Whether two first pages show the same rows, sources and end of list; their times of reading may differ. */
export function sameFirstPage(a: ConversationsView, b: ConversationsView): boolean {
  const shown = (view: ConversationsView) => JSON.stringify([view.tools, view.conversations, view.next !== undefined])
  return shown(a) === shown(b)
}

export type ToolFilter = ConversationTool | 'all'

function searchParts(conversation: Conversation): string[] {
  return [chatText(conversation.title).text, conversation.folder ?? '', conversation.branch ?? '', chatText(conversation.prompt ?? '').text, conversation.reply ?? '']
}

/** Rows from the chosen tool whose title, folder, branch, last prompt, or latest reply, as the row shows them, holds every word of the query, ignoring case: the server's rule, applied here to mock data. */
export function filterConversations(conversations: Conversation[], query: string, tool: ToolFilter): Conversation[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean)
  return conversations.filter((conversation) => {
    const toolMatches = tool === 'all' || conversation.tool === tool
    const text = searchParts(conversation).join('\n').toLowerCase()
    return toolMatches && words.every((word) => text.includes(word))
  })
}

/** A tool button toggles its tool: pressing the chosen one again shows every tool. */
export function toggleTool(current: ToolFilter, tool: ConversationTool): ToolFilter {
  return current === tool ? 'all' : tool
}

type GroupId = 'live' | DayGroupId
type DayGroupId = 'today' | 'yesterday' | 'week' | 'earlier'

const GROUP_LABELS = {
  live: 'Open now',
  today: 'Today',
  yesterday: 'Yesterday',
  week: 'This week',
  earlier: 'Earlier',
} satisfies Record<GroupId, string>

const GROUP_ORDER: GroupId[] = ['live', 'today', 'yesterday', 'week', 'earlier']

interface ConversationGroup {
  id: GroupId
  label: string
  /** In the order the list draws them: each nested row right after the row it sits under. */
  conversations: Conversation[]
  /** The row each nested row sits under, by tool and id. */
  under: ReadonlyMap<string, Conversation>
}

const DAY_MS = 24 * 60 * 60 * 1000

function localMidnight(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate())
}

/** Local calendar days between the two dates; rounding absorbs the 23 or 25 hour days of a clock change. */
function calendarDaysAgo(date: Date, now: Date): number {
  return Math.round((localMidnight(now).getTime() - localMidnight(date).getTime()) / DAY_MS)
}

// This week means the five days before yesterday, so a chat from last
// Saturday still counts on a Monday. A time in the future (another
// machine's clock) counts as today; one that does not parse, as earlier.
function dayGroupOf(updatedAt: string, now: Date): DayGroupId {
  const date = new Date(updatedAt)
  if (Number.isNaN(date.getTime())) return 'earlier'
  const days = calendarDaysAgo(date, now)
  if (days <= 0) return 'today'
  if (days === 1) return 'yesterday'
  if (days < 7) return 'week'
  return 'earlier'
}

function groupOf(conversation: Conversation, now: Date): GroupId {
  if (conversation.live) return 'live'
  return dayGroupOf(conversation.updatedAt, now)
}

/** Open now for every live conversation, then Today, Yesterday, This week, and Earlier in local time, from updatedAt; empty groups are left out. */
export function groupConversations(conversations: Conversation[], now: Date): ConversationGroup[] {
  const groups = GROUP_ORDER.map((id) => ({
    id,
    label: GROUP_LABELS[id],
    ...nestRows(conversations.filter((conversation) => groupOf(conversation, now) === id)),
  }))
  return groups.filter((group) => group.conversations.length > 0)
}

/** The row a conversation sits under: the last of its parents, walking up, that is in `rows`. Undefined for a row with none, and for one whose walk meets a loop. */
function rootOf(conversation: Conversation, rows: ReadonlyMap<string, Conversation>): Conversation | undefined {
  const seen = new Set([conversationKey(conversation)])
  let root: Conversation | undefined
  for (let parent = conversation.parent; parent; parent = root?.parent) {
    const key = `${parent.tool}:${parent.id}`
    const above = rows.get(key)
    if (!above) break
    if (seen.has(key)) return undefined
    seen.add(key)
    root = above
  }
  return root
}

/** One group's rows as the list draws them: every descendant right after the root it sits under, oldest first, one level deep. */
function nestRows(rows: Conversation[]): Pick<ConversationGroup, 'conversations' | 'under'> {
  const byKey = new Map(rows.map((conversation) => [conversationKey(conversation), conversation]))
  const under = new Map<string, Conversation>()
  const children = new Map<string, Conversation[]>()
  for (const conversation of rows) {
    const root = rootOf(conversation, byKey)
    if (!root) continue
    under.set(conversationKey(conversation), root)
    const key = conversationKey(root)
    children.set(key, [...(children.get(key) ?? []), conversation])
  }
  const conversations = rows
    .filter((conversation) => !under.has(conversationKey(conversation)))
    .flatMap((conversation) => [conversation, ...(children.get(conversationKey(conversation)) ?? []).sort((a, b) => newestFirst(b, a))])
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

/** The status chip of an open chat: only Working or Waiting, since the Open now group already says it is open. A wait read from silence says how long nothing came instead. */
export function liveBadge(conversation: Conversation, now: Date): LiveBadge | undefined {
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

interface StateBadge {
  label: string
  /** Amber for a reply that waits to be read, neutral for a finished chat. */
  tone: 'approval' | 'neutral'
  /** The full sentence, for the title and screen readers. */
  description: string
}

/** The chip for the person's own marks: a reply not yet read, or a chat marked as done. Waiting and working already have the live chip, and an active or dormant chat needs none. */
export function stateBadge(state: ConversationState | undefined): StateBadge | undefined {
  if (state === 'unread' || state === 'completed') return STATE_BADGES[state]
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

/** The command a failed open offers to copy, unless the row already offers the same one. */
export function replyCommand(reply: ConversationOpenReply, route: ConversationRoute): string | undefined {
  if (reply.opened || !reply.command) return undefined
  if (reply.command === rowCommand(route)) return undefined
  return reply.command
}

/** The page's answer to "Where did you leave off?": the open chats and how many wait, else when the last one ran. */
export function conversationsVerdict(conversations: Conversation[], now: Date, olderWeeks: boolean): string {
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
