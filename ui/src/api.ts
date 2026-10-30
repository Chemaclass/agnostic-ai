// Loading the overview. Three sources share one interface:
//
//   api     the running server: GET /api/views/overview
//   mock    the fixture bundled with the client (?mock=1)
//   static  a committed snapshot directory or URL (?source=…), read with
//           plain fetch
//
// Every response is normalized before the UI reads it.

import { API, CONTEXT_SCHEMA_VERSION, OVERVIEW_SCHEMA_VERSION, type ContextView, type MCPSpecPreview, type NativeHolder, type OverviewView, type PermissionEditRequest } from './contract'
import { normalizeContextView, normalizeOverviewView } from './model/normalize'
import { parsePermissionPreview } from './model/permission-edit'
import { staticOverviewUrl, type DataSource } from './model/url'

export interface Loaded<T> {
  data: T
  /** Where the document came from, as the JSON inspector shows it. */
  endpoint: string
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

/** Context size for one target, from the server or the mock fixture; a static snapshot has none. */
export async function loadContext(source: DataSource, target: string | undefined, file: string, signal?: AbortSignal): Promise<Loaded<ContextView>> {
  if (source.kind === 'mock') {
    const module = await import('./fixtures/context.mock.json')
    const data = normalizeContextView(module.default)
    return { data, endpoint: 'Mock fixture src/fixtures/context.mock.json', schemaWarning: schemaWarning(data.schemaVersion, CONTEXT_SCHEMA_VERSION) }
  }
  if (source.kind !== 'api') throw new LoadError('Context size needs a running agnostic-ai serve.')
  const params = new URLSearchParams()
  if (target) params.set('target', target)
  if (file) params.set('file', file)
  const query = params.toString()
  const path = query ? `${API.context}?${query}` : API.context
  const data = normalizeContextView(await getJson(path, signal, SERVER_HINT))
  return { data, endpoint: `GET ${path}`, schemaWarning: schemaWarning(data.schemaVersion, CONTEXT_SCHEMA_VERSION) }
}

/** An edit the server refused, with the validation problems it found. */
export class EditError extends Error {
  readonly problems: string[]
  /** 409 means the file changed after the page loaded; nothing was written. */
  readonly status?: number
  constructor(message: string, problems: string[] = [], status?: number) {
    super(message)
    this.name = 'EditError'
    this.problems = problems
    this.status = status
  }
}

/** POSTs one toggle edit; the server validates it and restores the file when it fails. */
export async function postEdit(path: string, body: unknown): Promise<void> {
  let res: Response
  try {
    res = await fetch(path, { method: 'POST', headers: { 'Content-Type': 'application/json', Accept: 'application/json' }, body: JSON.stringify(body) })
  } catch {
    throw new EditError('Could not reach the server to save the change.')
  }
  if (res.ok) return
  let reply: { error?: string; problems?: string[] } = {}
  try {
    reply = (await res.json()) as typeof reply
  } catch {
    // Not JSON; fall back to the status.
  }
  throw new EditError(reply.error ?? `The server answered ${res.status}.`, reply.problems ?? [], res.status)
}

export async function editPermissions(body: PermissionEditRequest) {
  let res: Response
  try {
    res = await fetch(API.editPermissions, {
      method: 'POST', headers: { 'Content-Type': 'application/json', Accept: 'application/json' }, body: JSON.stringify(body),
    })
  } catch {
    throw new EditError('Could not reach the server to save the rules.')
  }
  if (!res.ok) throw new EditError(await readError(res), [], res.status)
  return parsePermissionPreview(await res.json())
}

export async function importMCPSpec(action: 'preview' | 'apply', audience: 'shared' | 'personal', name: string, holder: NativeHolder, expected?: string): Promise<MCPSpecPreview> {
  let res: Response
  try {
    res = await fetch(API.editMCP, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ action, audience, name, target: holder.target, path: holder.path, scope: holder.scope, expected }),
    })
  } catch {
    throw new EditError('Could not reach the server. Try again when it is running.')
  }
  if (!res.ok) throw new EditError(await readError(res), [], res.status)
  return await res.json() as MCPSpecPreview
}
