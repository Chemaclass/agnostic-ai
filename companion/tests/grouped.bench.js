// Run with `node tests/grouped.bench.js` from companion/; `make companion-test` leaves it out.
// Three ways to group rows that all wait as active: flat (the state groups with no
// nesting, the lower bound), the first nesting (it walked every parent chain again
// and copied each family's array; kept here as it was, so the cost can be measured
// again), and grouped as it is now.
import { GROUPS, grouped, keyOf } from '../frontend/model.js'

const NOW = Date.parse('2026-11-09T12:00:00Z')

function row(at, parent) {
  return {
    tool: 'claude',
    id: `row-${at}`,
    state: 'active',
    title: `Row ${at}`,
    updatedAt: new Date(NOW - at * 1000).toISOString(),
    ...(parent === undefined ? {} : { parent: { tool: 'claude', id: `row-${parent}` } }),
  }
}

/** Each row started from the one before it. */
const chain = (size) => Array.from({ length: size }, (_, at) => row(at, at === 0 ? undefined : at - 1))
/** Every row started from the first. */
const family = (size) => Array.from({ length: size }, (_, at) => row(at, at === 0 ? undefined : 0))

const GROUP_OF_STATE = { blocked: 'needs-you', unread: 'needs-you', working: 'working', active: 'active', completed: 'archived', dormant: 'archived' }

/** The grouping as it was before nesting: one pass per group. */
function flat(conversations) {
  return GROUPS.map((group) => ({
    ...group,
    rows: conversations.filter((conversation) => GROUP_OF_STATE[conversation.state] === group.id),
  })).filter((group) => group.rows.length > 0)
}

function firstRootOf(conversation, rows) {
  const seen = new Set([keyOf(conversation)])
  let root
  for (let parent = conversation.parent; parent; parent = root.parent) {
    const key = parent.tool + ':' + parent.id
    const above = rows.get(key)
    if (!above) break
    if (seen.has(key)) return undefined
    seen.add(key)
    root = above
  }
  return root
}

function firstNesting(rows) {
  const byKey = new Map(rows.map((conversation) => [keyOf(conversation), conversation]))
  const under = new Map()
  const children = new Map()
  for (const conversation of rows) {
    const root = firstRootOf(conversation, byKey)
    if (!root) continue
    under.set(keyOf(conversation), root)
    children.set(keyOf(root), [...(children.get(keyOf(root)) ?? []), conversation])
  }
  const written = (conversation) => Date.parse(conversation.updatedAt) || 0
  const drawn = rows
    .filter((conversation) => !under.has(keyOf(conversation)))
    .flatMap((conversation) => [conversation, ...(children.get(keyOf(conversation)) ?? []).sort((a, b) => written(a) - written(b))])
  return { rows: drawn, under }
}

/** Microseconds per call: runs `call` for about 200 ms, or once when one call takes longer. */
function perCall(call) {
  let calls = 0
  const start = performance.now()
  do {
    call()
    calls += 1
  } while (performance.now() - start < 200)
  return Number((((performance.now() - start) * 1000) / calls).toPrecision(3))
}

const table = {}
for (const [shape, make] of [['chain', chain], ['family', family]]) {
  for (const size of [300, 2000, 20000]) {
    const rows = make(size)
    table[`${shape} of ${size}`] = {
      'flat µs': perCall(() => flat(rows)),
      'first nesting µs': perCall(() => firstNesting(rows)),
      'grouped µs': perCall(() => grouped(rows)),
    }
  }
}
console.table(table)
