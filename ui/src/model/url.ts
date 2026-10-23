// URL state. Every navigable choice lives in the query string, so a link
// reproduces the screen: `?view=authority&domain=cloud-deployment&resource=…`.
// No router library: the app parses location.search and pushes history.

import type { Change, Proposal } from '../contract'

export const SECTION_VIEWS = ['context', 'authority', 'tools', 'lifecycle', 'settings'] as const
export const VIEW_IDS = SECTION_VIEWS
export type ViewId = (typeof VIEW_IDS)[number]
export type SectionViewId = (typeof SECTION_VIEWS)[number]
export type AuthorityTab = 'resources' | 'circuit'

export interface UrlState {
  view: ViewId
  target?: string
  agentId?: string
  domain?: string
  resource?: string
  tab: AuthorityTab
  /** A proposal draft shared as a link. */
  proposal?: Proposal
  mock: boolean
  /** Static snapshot directory or URL. */
  source?: string
}

export const DEFAULT_STATE: UrlState = { view: 'context', tab: 'resources', mock: false }

const MAX_PARAM = 200
const MAX_PROPOSAL = 8000
const ID = /^[A-Za-z0-9][A-Za-z0-9._:|/-]*$/

function cleanId(value: string | null): string | undefined {
  if (!value) return undefined
  const trimmed = value.trim()
  if (!trimmed || trimmed.length > MAX_PARAM || !ID.test(trimmed)) return undefined
  return trimmed
}

function isView(value: string | null): value is ViewId {
  return !!value && (VIEW_IDS as readonly string[]).includes(value)
}

const LISTS = ['allow', 'ask', 'deny'] as const
const OPS = ['add', 'remove'] as const

/** Accepts only a well-formed proposal; anything else is dropped. */
export function sanitizeProposal(raw: unknown): Proposal | undefined {
  if (!raw || typeof raw !== 'object') return undefined
  const p = raw as Record<string, unknown>
  if (!Array.isArray(p.changes)) return undefined
  const changes: Change[] = []
  for (const c of p.changes) {
    if (!c || typeof c !== 'object') return undefined
    const change = c as Record<string, unknown>
    if (!(OPS as readonly unknown[]).includes(change.op)) return undefined
    if (!(LISTS as readonly unknown[]).includes(change.list)) return undefined
    if (typeof change.rule !== 'string' || !change.rule.trim() || change.rule.length > MAX_PARAM) return undefined
    changes.push({
      op: change.op as Change['op'],
      list: change.list as Change['list'],
      rule: change.rule.trim(),
      ...(typeof change.source === 'string' && change.source ? { source: change.source } : {}),
    })
  }
  return {
    agentId: typeof p.agentId === 'string' ? p.agentId : '',
    ...(typeof p.target === 'string' && p.target ? { target: p.target } : {}),
    ...(typeof p.revision === 'string' && p.revision ? { revision: p.revision } : {}),
    changes,
  }
}

export function parseUrlState(search: string): UrlState {
  const params = new URLSearchParams(search.startsWith('?') ? search.slice(1) : search)
  const view = params.get('view')
  let proposal: Proposal | undefined
  const rawProposal = params.get('proposal')
  if (rawProposal && rawProposal.length <= MAX_PROPOSAL) {
    try {
      proposal = sanitizeProposal(JSON.parse(rawProposal))
    } catch {
      proposal = undefined
    }
  }
  const source = params.get('source')?.trim()
  return {
    view: isView(view) ? view : DEFAULT_STATE.view,
    target: cleanId(params.get('target')),
    agentId: cleanId(params.get('agentId')),
    domain: cleanId(params.get('domain')),
    resource: cleanId(params.get('resource')),
    tab: params.get('tab') === 'circuit' ? 'circuit' : 'resources',
    proposal,
    mock: params.get('mock') === '1' || params.get('mock') === 'true',
    source: source && source.length <= 2048 ? source : undefined,
  }
}

/** Serializes in a fixed order and leaves defaults out, so links stay short and stable. */
export function serializeUrlState(state: UrlState): string {
  const params = new URLSearchParams()
  if (state.view !== DEFAULT_STATE.view) params.set('view', state.view)
  if (state.target) params.set('target', state.target)
  if (state.agentId) params.set('agentId', state.agentId)
  if (state.domain) params.set('domain', state.domain)
  if (state.resource) params.set('resource', state.resource)
  if (state.tab !== 'resources') params.set('tab', state.tab)
  if (state.proposal) params.set('proposal', JSON.stringify(state.proposal))
  if (state.source) params.set('source', state.source)
  else if (state.mock) params.set('mock', '1')
  const query = params.toString()
  return query ? `?${query}` : ''
}

/**
 * Applies a patch and clears what the patch invalidates: a new view
 * leaves the Authority selection behind, a new domain drops the selected
 * resource, and a new subject or tool drops a proposal made for another.
 * The data source (mock or static snapshot) always carries over.
 */
export function nextState(state: UrlState, patch: Partial<UrlState>): UrlState {
  const next: UrlState = { ...state, ...patch }
  if (patch.view && patch.view !== state.view) {
    if (patch.view !== 'authority') {
      if (!('domain' in patch)) next.domain = undefined
      if (!('resource' in patch)) next.resource = undefined
      if (!('tab' in patch)) next.tab = 'resources'
      if (!('proposal' in patch)) next.proposal = undefined
    }
  }
  if ('domain' in patch && patch.domain !== state.domain && !('resource' in patch)) next.resource = undefined
  const subjectChanged = ('agentId' in patch && patch.agentId !== state.agentId) || ('target' in patch && patch.target !== state.target)
  if (subjectChanged && !('proposal' in patch)) next.proposal = undefined
  return next
}

export type DataSource =
  | { kind: 'api' }
  | { kind: 'mock' }
  | { kind: 'static'; base: string }
  | { kind: 'invalid'; value: string; reason: string }

const SCHEME = /^[a-z][a-z0-9+.-]*:/i

/**
 * Decides where views come from. `?source=` names a static snapshot
 * directory (relative to the page) or an http(s) URL; `?mock=1` loads the
 * bundled fixtures; otherwise a build-time default source applies, and
 * without one the client asks the server's /api.
 */
export function resolveDataSource(state: Pick<UrlState, 'mock' | 'source'>, defaultSource?: string): DataSource {
  const raw = state.source ?? (state.mock ? undefined : defaultSource?.trim() || undefined)
  if (raw !== undefined) {
    if (SCHEME.test(raw) && !/^https?:/i.test(raw)) {
      return { kind: 'invalid', value: raw, reason: 'A snapshot source must be a relative directory or an http(s) URL.' }
    }
    return { kind: 'static', base: raw.endsWith('/') ? raw : `${raw}/` }
  }
  if (state.mock) return { kind: 'mock' }
  return { kind: 'api' }
}

const FILE_PART = /^[A-Za-z0-9._-]+$/

/**
 * The files a static snapshot is read from, most specific first:
 * `authority.<agentId>.<target>.json` when both are known, then
 * `authority.json`.
 */
export function staticAuthorityCandidates(base: string, agentId?: string, target?: string): string[] {
  const candidates: string[] = []
  if (agentId && target && FILE_PART.test(agentId) && FILE_PART.test(target)) {
    candidates.push(`${base}authority.${agentId}.${target}.json`)
  }
  candidates.push(`${base}authority.json`)
  return candidates
}

export function staticOverviewUrl(base: string): string {
  return `${base}overview.json`
}
