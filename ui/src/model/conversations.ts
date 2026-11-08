// The Conversations page: the Open now and day groups, search, the tool
// filter, and the words each app, activity, source, and withheld field
// shows as. The server sends the rows newest first; these helpers keep
// that order.

import type { Conversation, ConversationActivity, ConversationApp, ConversationOpenReply, ConversationRoute, ConversationSource, ConversationsView, ConversationTool } from '../contract'
import { plural } from './format'

const APP_LABELS = {
  cli: 'Terminal',
  ide: 'Editor',
  desktop: 'Desktop app',
  background: 'Background',
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
} satisfies Record<WithheldField, string>

export function withheldLabel(field: WithheldField): string {
  return WITHHELD_LABELS[field]
}

export const WITHHELD_NOTE = 'Hidden: may hold a credential'

/** A chip's count: the tool's rows loaded so far, of every conversation it holds; or that its store was not found. */
export function sourceLine(source: ConversationSource, loaded: Conversation[]): string {
  if (!source.found) return 'Not found'
  const rows = loaded.filter((conversation) => conversation.tool === source.tool).length
  return `${rows} of ${source.count}`
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

export type ToolFilter = ConversationTool | 'all'

function searchParts(conversation: Conversation): string[] {
  return [conversation.title, conversation.folder ?? '', conversation.branch ?? '', ...(conversation.opening ?? []), conversation.prompt ?? '']
}

/** Rows from the chosen tool whose shown text holds every word of the query, ignoring case: the rule the server applies, used for mock data. */
export function filterConversations(conversations: Conversation[], query: string, tool: ToolFilter): Conversation[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean)
  return conversations.filter((conversation) => {
    const toolMatches = tool === 'all' || conversation.tool === tool
    const text = searchParts(conversation).join('\n').toLowerCase()
    return toolMatches && words.every((word) => text.includes(word))
  })
}

/** A source chip toggles its tool: pressing the chosen one again shows every tool. */
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
  conversations: Conversation[]
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
    conversations: conversations.filter((conversation) => groupOf(conversation, now) === id),
  }))
  return groups.filter((group) => group.conversations.length > 0)
}

interface LiveBadge {
  label: string
  /** The badge tone: amber when the agent waits for the person. */
  tone: 'permitted' | 'approval' | 'neutral'
  /** The full sentence, for the title and screen readers. */
  description: string
}

const ACTIVITY_BADGES = {
  working: { label: 'Working', tone: 'permitted', description: 'Open now: the agent is working' },
  waiting: { label: 'Waiting for you', tone: 'approval', description: 'Open now: the agent is waiting for you' },
  idle: { label: 'Idle', tone: 'neutral', description: 'Open now: the agent is idle' },
} satisfies Record<ConversationActivity, LiveBadge>

const OPEN_BADGE: LiveBadge = { label: 'Open now', tone: 'permitted', description: 'Open now' }

/** What a live conversation is doing, when its tool says; a tool that does not say shows Open now. */
export function liveBadge(activity: ConversationActivity | undefined): LiveBadge {
  if (activity === undefined) return OPEN_BADGE
  return ACTIVITY_BADGES[activity]
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

/** The command a failed open offers to copy, unless the row already offers the same one. */
export function replyCommand(reply: ConversationOpenReply, route: ConversationRoute): string | undefined {
  if (reply.opened || !reply.command) return undefined
  if (reply.command === route.command) return undefined
  return reply.command
}
