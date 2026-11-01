// URL state. Every navigable choice lives in the query string, so a link
// reproduces the screen: `?view=lifecycle`. No router library: the app
// parses location.search and pushes history.

export const SECTION_VIEWS = ['context', 'authority', 'tools', 'lifecycle', 'settings'] as const
export type ViewId = (typeof SECTION_VIEWS)[number]

export interface UrlState {
  view: ViewId
  /** The selected group within a section. */
  tab?: string
  /** The section search. */
  q?: string
  /** The linked item, field, or finding (see identity.ts). */
  entry?: string
  /** The coding tool chosen in Context size. */
  target?: string
  /** The file Context size counts scoped rules for. */
  file?: string
  mock: boolean
  /** Static snapshot directory or URL. */
  source?: string
}

export const DEFAULT_STATE: UrlState = { view: 'context', mock: false }

function isView(value: string | null): value is ViewId {
  return !!value && (SECTION_VIEWS as readonly string[]).includes(value)
}

export function parseUrlState(search: string): UrlState {
  const params = new URLSearchParams(search.startsWith('?') ? search.slice(1) : search)
  const view = params.get('view')
  const source = params.get('source')?.trim()
  const q = params.get('q') ?? ''
  const entry = params.get('entry') ?? ''
  const tab = params.get('tab') ?? ''
  const target = params.get('target') ?? ''
  const file = params.get('file') ?? ''
  return {
    view: isView(view) ? view : DEFAULT_STATE.view,
    tab: /^[a-z][a-z0-9-]{0,47}$/.test(tab) ? tab : undefined,
    q: q && q.length <= 512 ? q : undefined,
    entry: entry && entry.length <= 2048 ? entry : undefined,
    target: /^[a-z][a-z0-9-]{0,47}$/.test(target) ? target : undefined,
    file: file && file.length <= 1024 ? file : undefined,
    mock: params.get('mock') === '1' || params.get('mock') === 'true',
    source: source && source.length <= 2048 ? source : undefined,
  }
}

/** Serializes in a fixed order and leaves defaults out, so links stay short and stable. */
export function serializeUrlState(state: UrlState): string {
  const params = new URLSearchParams()
  if (state.view !== DEFAULT_STATE.view) params.set('view', state.view)
  if (state.tab) params.set('tab', state.tab)
  if (state.q) params.set('q', state.q)
  if (state.entry) params.set('entry', state.entry)
  if (state.target) params.set('target', state.target)
  if (state.file) params.set('file', state.file)
  if (state.source) params.set('source', state.source)
  else if (state.mock) params.set('mock', '1')
  const query = params.toString()
  return query ? `?${query}` : ''
}

/** Applies a patch; the data source carries over, and a new section starts without the old search or link. */
export function nextState(state: UrlState, patch: Partial<UrlState>): UrlState {
  const base = patch.view && patch.view !== state.view ? { ...state, tab: undefined, q: undefined, entry: undefined, target: undefined, file: undefined } : state
  return { ...base, ...patch }
}

export type DataSource =
  | { kind: 'api' }
  | { kind: 'mock' }
  | { kind: 'static'; base: string }
  | { kind: 'invalid'; value: string; reason: string }

const SCHEME = /^[a-z][a-z0-9+.-]*:/i

/**
 * Decides where the overview comes from. `?source=` names a static
 * snapshot directory (relative to the page) or an http(s) URL; `?mock=1`
 * loads the bundled fixture; otherwise a build-time default source
 * applies, and without one the client asks the server's /api.
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

/** Context size can load its report: a running server or the bundled fixture, never a static snapshot. */
export function contextUsageAvailable(source: DataSource): boolean {
  return source.kind === 'api' || source.kind === 'mock'
}

export function staticOverviewUrl(base: string): string {
  return `${base}overview.json`
}
