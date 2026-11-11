// Normalizes server output before the UI reads it. Go encodes a nil
// slice as `null`, and an older or newer server may omit a field the
// client expects; both would crash a renderer that maps over arrays.
// Normalizing never invents a decision: a missing effect stays missing
// and renders as its raw value, a missing list becomes an empty list.

import type { ContextLoad, ContextReport, ContextSize, ContextView, Conversation, ConversationRoute, ConversationSource, ConversationsView, Gap, Improvements, OverviewView, Section, Snapshot, ToolCatalog } from '../contract'

type Loose = Record<string, unknown>

function arr<T>(value: unknown): T[] {
  return Array.isArray(value) ? (value as T[]) : []
}

function obj(value: unknown): Loose {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as Loose) : {}
}

export function normalizeSnapshot(raw: unknown): Snapshot {
  const s = obj(raw)
  const evidence = obj(s.evidence)
  return {
    ...(s as unknown as Snapshot),
    revision: String(s.revision ?? ''),
    generatedAt: String(s.generatedAt ?? ''),
    project: { root: '', name: '', ...(obj(s.project) as object) } as Snapshot['project'],
    targets: arr<string>(s.targets),
    configFiles: arr<string>(s.configFiles),
    specs: arr(s.specs),
    evidence: {
      declared: 'loaded',
      installed: 'not-checked',
      runtime: 'not-observed',
      note: '',
      ...(evidence as object),
    } as Snapshot['evidence'],
  }
}

function normalizeSection(raw: unknown): Section {
  const s = obj(raw)
  return {
    ...(s as unknown as Section),
    kinds: arr(s.kinds),
    items: arr(s.items).map((i) => {
      const item = obj(i)
      return {
        ...(item as unknown as Section['items'][number]),
        findings: arr(item.findings),
        ...(item.tools ? { tools: { ...(obj(item.tools) as unknown as NonNullable<Section['items'][number]['tools']>), tools: arr(obj(item.tools).tools) } } : {}),
        values: arr<Loose>(item.values).map((v) => ({ ...(v as unknown as Section['items'][number]['values'][number]), ...(v.list ? { list: arr(v.list) } : {}) })),
      }
    }),
    support: arr(s.support).map((k) => {
      const kind = obj(k)
      return { ...(kind as unknown as Section['support'][number]), supported: arr(kind.supported), unsupported: arr(kind.unsupported) }
    }),
    findings: arr(s.findings),
    ...(s.fields
      ? {
          fields: arr<Loose>(s.fields).map((f) => ({
            ...(f as unknown as NonNullable<Section['fields']>[number]),
            set: arr<Loose>(f.set).map((src) => ({ ...(src as unknown as NonNullable<Section['fields']>[number]['set'][number]), values: arr(src.values) })),
          })),
        }
      : {}),
    ...(s.stages ? { stages: arr<Loose>(s.stages).map((st) => ({ ...(st as unknown as NonNullable<Section['stages']>[number]), targets: arr(st.targets) })) } : {}),
  }
}

// A static or ?source= overview can come from anywhere, so the top bar links only a GitHub page.
function normalizeRepository(raw: unknown): OverviewView['repository'] {
  const r = obj(raw)
  if (typeof r.url !== 'string' || typeof r.name !== 'string') return undefined
  if (!/^https:\/\/github\.com\/[\w.-]+\/[\w.-]+$/.test(r.url)) return undefined
  return { url: r.url, name: r.name }
}

export function normalizeOverviewView(raw: unknown): OverviewView {
  const v = obj(raw)
  return {
    ...(v as unknown as OverviewView),
    repository: normalizeRepository(v.repository),
    schemaVersion: String(v.schemaVersion ?? ''),
    snapshot: normalizeSnapshot(v.snapshot),
    sections: arr(v.sections).map(normalizeSection),
    targets: arr(v.targets).map((t) => {
      const target = obj(t)
      const sync = obj(target.sync)
      return {
        ...(target as unknown as OverviewView['targets'][number]),
        kinds: arr(target.kinds),
        sync: { state: sync.state === 'in-sync' || sync.state === 'drifted' ? sync.state : 'not-checked', files: arr(sync.files) },
      }
    }),
    tools: normalizeToolCatalog(v.tools),
    health: normalizeHealth(v.health),
  }
}

function normalizeHealth(raw: unknown): OverviewView['health'] {
  const v = obj(raw)
  const health: OverviewView['health'] = { gaps: arr(v.gaps).map(normalizeGap).filter((gap) => gap.id && gap.fix) }
  const improvements = normalizeImprovements(v.improvements)
  if (improvements) health.improvements = improvements
  return health
}

function normalizeImprovements(raw: unknown): Improvements | undefined {
  const { applied, total } = obj(raw)
  if (!Number.isInteger(applied) || !Number.isInteger(total)) return undefined
  const a = applied as number, t = total as number
  return a >= 0 && a <= t ? { applied: a, total: t } : undefined
}

function normalizeGap(raw: unknown): Gap {
  const v = obj(raw)
  return { id: String(v.id ?? ''), section: String(v.section ?? ''), label: String(v.label ?? ''), supported: arr(v.supported).map(String), missing: String(v.missing ?? ''), fix: String(v.fix ?? '') }
}

function normalizeToolCatalog(raw: unknown): ToolCatalog {
  const v = obj(raw)
  const source = v.source === 'shared' || v.source === 'personal' ? v.source : 'default'
  return {
    source,
    items: arr(v.items)
      .map((t) => {
        const tool = obj(t)
        return { target: String(tool.target ?? ''), label: String(tool.label ?? tool.target ?? ''), description: String(tool.description ?? ''), enabled: tool.enabled === true, local: tool.local === true, detected: tool.detected === true }
      })
      .filter((tool) => tool.target),
  }
}

function count(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

function size(raw: unknown): ContextSize {
  const s = obj(raw)
  return { words: count(s.words), bytes: count(s.bytes) }
}

function text(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function load(value: unknown): ContextLoad {
  return value === 'file-scope' || value === 'on-demand' ? value : 'startup'
}

function normalizeContextReport(raw: unknown): ContextReport {
  const r = obj(raw)
  return {
    version: text(r.version),
    command: text(r.command),
    target: text(r.target),
    ...(typeof r.file === 'string' && r.file ? { file: r.file } : {}),
    note: text(r.note),
    startup: size(r.startup),
    file_scope: size(r.file_scope),
    on_demand: size(r.on_demand),
    contributions: arr<Loose>(r.contributions).map((c) => ({ source: text(c.source), category: text(c.category), load: load(c.load), ...size(c) })),
    opportunities: arr<Loose>(r.opportunities).map((o) => ({
      kind: text(o.kind),
      target: text(o.target),
      load: load(o.load),
      passage: text(o.passage),
      copies: count(o.copies),
      size: size(o.size),
      reduction: size(o.reduction),
      sources: arr<Loose>(o.sources).map((src) => ({ source: text(src.source), category: text(src.category) })),
      suggestion: text(o.suggestion),
    })),
  }
}

export function normalizeContextView(raw: unknown): ContextView {
  const v = obj(raw)
  return {
    schemaVersion: text(v.schemaVersion),
    supported: v.supported === true,
    reason: text(v.reason),
    ...(typeof v.root === 'string' ? { root: v.root } : {}),
    targets: arr<unknown>(v.targets).filter((t): t is string => typeof t === 'string'),
    fileTargets: arr<unknown>(v.fileTargets).filter((t): t is string => typeof t === 'string'),
    target: text(v.target),
    ...(typeof v.startupWordBudget === 'number' && v.startupWordBudget > 0 ? { startupWordBudget: v.startupWordBudget } : {}),
    ...(v.report ? { report: normalizeContextReport(v.report) } : {}),
  }
}

const WITHHELD_FIELDS = ['title', 'opening', 'prompt', 'reply'] as const

function isWithheldField(value: unknown): value is NonNullable<Conversation['withheld']>[number] {
  return (WITHHELD_FIELDS as readonly unknown[]).includes(value)
}

const ACTIVITIES = ['working', 'waiting', 'idle'] as const

function isActivity(value: unknown): value is NonNullable<Conversation['activity']> {
  return (ACTIVITIES as readonly unknown[]).includes(value)
}

// An activity this client does not know is left out, so the row shows Open now.
function normalizeConversation(raw: unknown): Conversation {
  const c = obj(raw)
  const route = obj(c.open)
  return {
    ...(c as unknown as Conversation),
    activity: isActivity(c.activity) ? c.activity : undefined,
    // Anything but a time string reads as a wait the tool reported.
    quietSince: typeof c.quietSince === 'string' && !Number.isNaN(Date.parse(c.quietSince)) ? c.quietSince : undefined,
    deleteUnsupported: typeof c.deleteUnsupported === 'string' && c.deleteUnsupported !== '' ? c.deleteUnsupported : undefined,
    id: text(c.id),
    title: text(c.title),
    updatedAt: text(c.updatedAt),
    opening: arr<unknown>(c.opening).filter((line): line is string => typeof line === 'string'),
    reply: typeof c.reply === 'string' && c.reply.trim() !== '' ? c.reply : undefined,
    withheld: arr<unknown>(c.withheld).filter(isWithheldField),
    live: c.live === true,
    open: { ...(route as unknown as ConversationRoute), label: text(route.label) },
  }
}

function normalizeConversationSource(raw: unknown): ConversationSource {
  const s = obj(raw)
  return { ...(s as unknown as ConversationSource), label: text(s.label), found: s.found === true, count: count(s.count), shown: count(s.shown) }
}

export function normalizeConversationsView(raw: unknown): ConversationsView {
  const v = obj(raw)
  return {
    // Another version still renders; loadConversations reports it as a schema warning.
    schemaVersion: text(v.schemaVersion) as ConversationsView['schemaVersion'],
    generatedAt: text(v.generatedAt),
    from: text(v.from),
    to: text(v.to),
    // A missing or empty next means no older conversation remains.
    ...(typeof v.next === 'string' && v.next !== '' ? { next: v.next } : {}),
    ...(typeof v.matches === 'number' ? { matches: v.matches } : {}),
    tools: arr(v.tools).map(normalizeConversationSource),
    conversations: arr(v.conversations).map(normalizeConversation),
  }
}
