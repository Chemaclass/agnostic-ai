// Run with `BENCH=1 npx vitest run`; `npm test` leaves it out.
// Three ways to group rows: flat (the state groups with no nesting, the lower
// bound), the first nesting alone (it walked every parent chain again and copied each
// family's array; kept here as it was, so the cost can be measured again), and
// groupConversations as it is now, state groups and nesting together.
import { test } from 'vitest'
import type { Conversation } from '../contract'
import { groupConversations } from './conversations'

const NOW = new Date(2026, 10, 7, 16, 30)
const key = (conversation: Conversation) => `${conversation.tool}:${conversation.id}`

function row(at: number, parent?: number): Conversation {
  return {
    tool: 'claude',
    app: 'cli',
    id: `row-${at}`,
    title: `Row ${at}`,
    updatedAt: new Date(NOW.getTime() - at * 1000).toISOString(),
    open: { kind: 'terminal', label: 'Open in Terminal' },
    ...(parent === undefined ? {} : { parent: { tool: 'claude', id: `row-${parent}` } }),
  }
}

/** Each row started from the one before it. */
const chain = (size: number) => Array.from({ length: size }, (_, at) => row(at, at === 0 ? undefined : at - 1))
/** Every row started from the first. */
const family = (size: number) => Array.from({ length: size }, (_, at) => row(at, at === 0 ? undefined : 0))

const GROUPS = ['needs-you', 'working', 'active', 'archived'] as const

function groupOf(conversation: Conversation): (typeof GROUPS)[number] {
  if (conversation.claimConflicts?.length || conversation.state === 'blocked' || conversation.state === 'unread') return 'needs-you'
  if (conversation.state === 'working') return 'working'
  if (conversation.state === 'active' || (conversation.state === undefined && conversation.live)) return 'active'
  return 'archived'
}

/** The grouping as it was before nesting: one pass per group. */
function flat(rows: Conversation[]) {
  return GROUPS.map((id) => ({ id, conversations: rows.filter((conversation) => groupOf(conversation) === id) })).filter((group) => group.conversations.length > 0)
}

function firstRootOf(conversation: Conversation, rows: ReadonlyMap<string, Conversation>): Conversation | undefined {
  const seen = new Set([key(conversation)])
  let root: Conversation | undefined
  for (let parent = conversation.parent; parent; parent = root?.parent) {
    const parentKey = `${parent.tool}:${parent.id}`
    const above = rows.get(parentKey)
    if (!above) break
    if (seen.has(parentKey)) return undefined
    seen.add(parentKey)
    root = above
  }
  return root
}

function firstNesting(rows: Conversation[]) {
  const byKey = new Map(rows.map((conversation) => [key(conversation), conversation]))
  const under = new Map<string, Conversation>()
  const children = new Map<string, Conversation[]>()
  for (const conversation of rows) {
    const root = firstRootOf(conversation, byKey)
    if (!root) continue
    under.set(key(conversation), root)
    children.set(key(root), [...(children.get(key(root)) ?? []), conversation])
  }
  const written = (conversation: Conversation) => Date.parse(conversation.updatedAt)
  const conversations = rows
    .filter((conversation) => !under.has(key(conversation)))
    .flatMap((conversation) => [conversation, ...(children.get(key(conversation)) ?? []).sort((a, b) => written(a) - written(b))])
  return { conversations, under }
}

/** Microseconds per call: runs `call` for about 200 ms, or once when one call takes longer. */
function perCall(call: () => unknown): number {
  let calls = 0
  const start = performance.now()
  do {
    call()
    calls += 1
  } while (performance.now() - start < 200)
  return ((performance.now() - start) * 1000) / calls
}

test('groups rows with and without nesting', () => {
  const table: Record<string, Record<string, number>> = {}
  for (const [shape, make] of [['chain', chain], ['family', family]] as const) {
    for (const size of [300, 2000, 20000]) {
      const rows = make(size)
      const micros = (call: () => unknown) => Number(perCall(call).toPrecision(3))
      table[`${shape} of ${size}`] = {
        'flat µs': micros(() => flat(rows)),
        'first nesting µs': micros(() => firstNesting(rows)),
        'groupConversations µs': micros(() => groupConversations(rows)),
      }
    }
  }
  console.table(table)
}, 600_000)
