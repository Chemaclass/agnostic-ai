// URL state. Every navigable choice lives in the query string, so a link
// reproduces the screen: `?view=lifecycle`. No router library: the app
// parses location.search and pushes history.

export const SECTION_VIEWS = ['context', 'authority', 'tools', 'lifecycle', 'settings'] as const
export type ViewId = (typeof SECTION_VIEWS)[number]

export interface UrlState {
  view: ViewId
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
  return {
    view: isView(view) ? view : DEFAULT_STATE.view,
    mock: params.get('mock') === '1' || params.get('mock') === 'true',
    source: source && source.length <= 2048 ? source : undefined,
  }
}

/** Serializes in a fixed order and leaves defaults out, so links stay short and stable. */
export function serializeUrlState(state: UrlState): string {
  const params = new URLSearchParams()
  if (state.view !== DEFAULT_STATE.view) params.set('view', state.view)
  if (state.source) params.set('source', state.source)
  else if (state.mock) params.set('mock', '1')
  const query = params.toString()
  return query ? `?${query}` : ''
}

/** Applies a patch; the data source (mock or static snapshot) carries over. */
export function nextState(state: UrlState, patch: Partial<UrlState>): UrlState {
  return { ...state, ...patch }
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

export function staticOverviewUrl(base: string): string {
  return `${base}overview.json`
}
