// Loading views. Three sources share one interface:
//
//   api     the running server: GET /api/views/…, POST the impact
//   mock    the handwritten fixtures bundled with the client (?mock=1);
//           validation still POSTs to /api, so tests can stub it
//   static  a committed snapshot directory or URL (?source=…), read with
//           plain fetch; nothing can be validated without a server
//
// Every response is normalized before the UI reads it.

import {
  API,
  OVERVIEW_SCHEMA_VERSION,
  SCHEMA_VERSION,
  type AuthorityView,
  type ImpactResult,
  type OverviewView,
  type Proposal,
} from './contract'
import { normalizeAuthorityView, normalizeImpactResult, normalizeOverviewView } from './model/normalize'
import { staticAuthorityCandidates, staticOverviewUrl, type DataSource } from './model/url'

export interface Loaded<T> {
  data: T
  /** Where the document came from, as the JSON inspector shows it. */
  endpoint: string
  /** Set when the source could not give exactly what was asked for. */
  notice?: string
  /** Set when the document declares a schema this client does not render. */
  schemaWarning?: string
}

export const SERVER_HINT =
  'Start the server with `go run ./cmd/agnostic-ai serve --dir internal/authority/testdata/demo-project`, or open the bundled mock data with ?mock=1.'

export class LoadError extends Error {
  readonly hint?: string
  readonly status?: number
  constructor(message: string, hint?: string, status?: number) {
    super(message)
    this.name = 'LoadError'
    this.hint = hint
    this.status = status
  }
}

export interface AuthorityQuery {
  agentId?: string
  target?: string
}

function resolveUrl(path: string): string {
  return new URL(path, document.baseURI).toString()
}

async function readError(res: Response): Promise<string> {
  try {
    const text = await res.text()
    try {
      const body = JSON.parse(text) as { error?: unknown; message?: unknown }
      if (typeof body.error === 'string') return body.error
      if (typeof body.message === 'string') return body.message
    } catch {
      // Not JSON; fall through to the text.
    }
    return text.trim().slice(0, 300) || res.statusText
  } catch {
    return res.statusText
  }
}

async function getJson(url: string, signal: AbortSignal | undefined, unreachableHint: string): Promise<unknown> {
  let res: Response
  try {
    res = await fetch(url, { signal, headers: { Accept: 'application/json' } })
  } catch (err) {
    if ((err as Error).name === 'AbortError') throw err
    throw new LoadError(`Could not reach ${url}.`, unreachableHint)
  }
  if (!res.ok) {
    const message = await readError(res)
    const gateway = res.status === 502 || res.status === 503 || res.status === 504 || (res.status === 500 && /ECONNREFUSED|proxy/i.test(message))
    throw new LoadError(`${url} answered ${res.status}: ${message}`, gateway ? unreachableHint : undefined, res.status)
  }
  try {
    return await res.json()
  } catch {
    throw new LoadError(`${url} did not return JSON.`, unreachableHint, res.status)
  }
}

function schemaWarning(found: string, expected: string): string | undefined {
  if (found === expected) return undefined
  return `This client renders ${expected}; the source declares ${found || 'no schema version'}. Fields it does not know are left out.`
}

function mismatchNotice(view: AuthorityView, query: AuthorityQuery, holder: string): string | undefined {
  const wantAgent = query.agentId ?? view.agent.id
  const wantTarget = query.target ?? view.harness.target
  if (wantAgent === view.agent.id && wantTarget === view.harness.target) return undefined
  const agentLabel = view.agents.find((a) => a.id === wantAgent)?.label ?? wantAgent
  const toolLabel = view.harness.available.find((h) => h.target === wantTarget)?.label ?? wantTarget
  return `${holder} holds no view for ${agentLabel} on ${toolLabel}, so this page shows ${view.agent.label} on ${view.harness.label}.`
}

export async function loadAuthority(source: DataSource, query: AuthorityQuery, signal?: AbortSignal): Promise<Loaded<AuthorityView>> {
  switch (source.kind) {
    case 'invalid':
      throw new LoadError(source.reason, `Received: ${source.value}`)
    case 'mock': {
      const module = await import('./fixtures/authority.mock.json')
      const data = normalizeAuthorityView(module.default)
      return {
        data,
        endpoint: 'Mock fixture src/fixtures/authority.mock.json',
        notice: mismatchNotice(data, query, 'The mock fixture'),
        schemaWarning: schemaWarning(data.schemaVersion, SCHEMA_VERSION),
      }
    }
    case 'static': {
      const hint = 'Check that the snapshot directory holds authority.json and overview.json.'
      const fallback = staticAuthorityCandidates(source.base)[0]
      const [first] = staticAuthorityCandidates(source.base, query.agentId, query.target)
      if (first !== fallback) {
        const found = await tryStatic(first, signal)
        if (found) return staticLoaded(found, first)
      }
      const base = normalizeAuthorityView(await getJson(resolveUrl(fallback), signal, hint))
      // With only one of agentId and target asked for, fill the other from
      // the default view and look for the specific file once.
      const wantAgent = query.agentId ?? base.agent.id
      const wantTarget = query.target ?? base.harness.target
      if (first === fallback && (wantAgent !== base.agent.id || wantTarget !== base.harness.target)) {
        const [named] = staticAuthorityCandidates(source.base, wantAgent, wantTarget)
        if (named !== fallback) {
          const found = await tryStatic(named, signal)
          if (found) return staticLoaded(found, named)
        }
      }
      return { ...staticLoaded(base, fallback), notice: mismatchNotice(base, query, 'This snapshot') }
    }
    case 'api': {
      const params = new URLSearchParams()
      if (query.agentId) params.set('agentId', query.agentId)
      if (query.target) params.set('target', query.target)
      const path = params.size ? `${API.view}?${params}` : API.view
      const data = normalizeAuthorityView(await getJson(path, signal, SERVER_HINT))
      return { data, endpoint: `GET ${path}`, schemaWarning: schemaWarning(data.schemaVersion, SCHEMA_VERSION) }
    }
  }
}

function staticLoaded(data: AuthorityView, path: string): Loaded<AuthorityView> {
  return { data, endpoint: `Static file ${path}`, schemaWarning: schemaWarning(data.schemaVersion, SCHEMA_VERSION) }
}

/** A static file that may be absent: 404 returns undefined, anything else throws. */
async function tryStatic(path: string, signal: AbortSignal | undefined): Promise<AuthorityView | undefined> {
  try {
    return normalizeAuthorityView(await getJson(resolveUrl(path), signal, 'Check the snapshot directory.'))
  } catch (err) {
    if (err instanceof LoadError && err.status === 404) return undefined
    throw err
  }
}

export async function loadOverview(source: DataSource, signal?: AbortSignal): Promise<Loaded<OverviewView>> {
  switch (source.kind) {
    case 'invalid':
      throw new LoadError(source.reason, `Received: ${source.value}`)
    case 'mock': {
      const module = await import('./fixtures/overview.mock.json')
      const data = normalizeOverviewView(module.default)
      return { data, endpoint: 'Mock fixture src/fixtures/overview.mock.json', schemaWarning: schemaWarning(data.schemaVersion, OVERVIEW_SCHEMA_VERSION) }
    }
    case 'static': {
      const path = staticOverviewUrl(source.base)
      const data = normalizeOverviewView(await getJson(resolveUrl(path), signal, 'Check that the snapshot directory holds overview.json.'))
      return { data, endpoint: `Static file ${path}`, schemaWarning: schemaWarning(data.schemaVersion, OVERVIEW_SCHEMA_VERSION) }
    }
    case 'api': {
      const data = normalizeOverviewView(await getJson(API.overview, signal, SERVER_HINT))
      return { data, endpoint: `GET ${API.overview}`, schemaWarning: schemaWarning(data.schemaVersion, OVERVIEW_SCHEMA_VERSION) }
    }
  }
}

/** Whether "Validate on server" can run from this source. */
export function validationAvailability(source: DataSource): { available: boolean; note?: string } {
  if (source.kind === 'static' || source.kind === 'invalid') return { available: false, note: 'needs a running server' }
  return { available: true }
}

/** POSTs a proposal; the server computes its impact on a copy of the specs and writes nothing. */
export async function validateProposal(proposal: Proposal, signal?: AbortSignal): Promise<Loaded<ImpactResult>> {
  let res: Response
  try {
    res = await fetch(API.impact, {
      method: 'POST',
      signal,
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify(proposal),
    })
  } catch (err) {
    if ((err as Error).name === 'AbortError') throw err
    throw new LoadError('Could not reach the server to validate the proposal.', SERVER_HINT)
  }
  const text = await res.text()
  let body: unknown
  try {
    body = JSON.parse(text)
  } catch {
    body = undefined
  }
  // A refused proposal may come back as an ImpactResult with problems,
  // whatever the status code; show it as such.
  if (body && typeof body === 'object' && ('problems' in body || 'valid' in body)) {
    return { data: normalizeImpactResult(body), endpoint: `POST ${API.impact}` }
  }
  if (!res.ok) {
    const message = (body as { error?: string } | undefined)?.error ?? (text.trim().slice(0, 300) || res.statusText)
    throw new LoadError(`The server answered ${res.status}: ${message}`, res.status >= 500 ? SERVER_HINT : undefined, res.status)
  }
  throw new LoadError('The server did not return an impact result.', undefined, res.status)
}
