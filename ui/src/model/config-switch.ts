import { API, type GlobalCreateReply, type SwitchReply } from '../contract'

/** What `/api/health` says this server reads. */
export interface ServedConfig {
  root: string
  global: boolean
  /** The project a global server started in. */
  projectRoot?: string
}

export interface SwitchOffer {
  label: string
  title: string
  /** The command that opens the same page when the server cannot. */
  command: string
}

export function parseServedConfig(value: unknown): ServedConfig | undefined {
  if (!value || typeof value !== 'object') return undefined
  const health = value as Record<string, unknown>
  if (typeof health.root !== 'string') return undefined
  const served: ServedConfig = { root: health.root, global: health.global === true }
  if (served.global && typeof health.projectRoot === 'string' && health.projectRoot) served.projectRoot = health.projectRoot
  return served
}

function shellQuote(path: string): string {
  return `'${path.replaceAll("'", "'\\''")}'`
}

export function baseName(path: string): string {
  return path.replace(/[\\/]+$/, '').split(/[\\/]/).pop() || path
}

export function switchOffer(served: ServedConfig): SwitchOffer | undefined {
  if (!served.global) {
    return {
      label: 'Open global settings',
      title: 'Open the user configuration in a new tab, starting its server when none runs',
      command: 'agnostic-ai ui --global',
    }
  }
  if (!served.projectRoot) return undefined
  return {
    label: `Open project ${baseName(served.projectRoot)}`,
    title: `Open ${served.projectRoot} in a new tab, starting its server when none runs`,
    command: `agnostic-ai ui --dir ${shellQuote(served.projectRoot)}`,
  }
}

const LOOPBACK_HOSTS = new Set(['127.0.0.1', '[::1]', 'localhost'])

export function parseSwitchReply(value: unknown): SwitchReply {
  const reply = (value && typeof value === 'object' ? value : {}) as Record<string, unknown>
  let url: URL | undefined
  try {
    url = typeof reply.url === 'string' ? new URL(reply.url) : undefined
  } catch {
    url = undefined
  }
  if (!url || url.protocol !== 'http:' || !LOOPBACK_HOSTS.has(url.hostname) || url.pathname !== '/' || url.search || url.hash || typeof reply.root !== 'string') {
    throw new Error('invalid switch reply')
  }
  return { url: url.toString(), root: reply.root, global: reply.global === true, started: reply.started === true }
}

export const GLOBAL_SETUP_TITLE = 'No global setup yet'
export const GLOBAL_SETUP_BODY =
  'One folder for your own rules, skills, agents, hooks, settings and MCP servers. Run "agnostic-ai sync --global" and every project gets them. Project specs never merge with it, so team defaults stay in the project. Put private files in "local/", which git ignores. Keep the folder in its own git repo to back it up.'

export function globalSetupCreated(root: string): string {
  return `Created ${root}. Add files, then run "agnostic-ai sync --global".`
}

/** The switch found no global setup; the page offers to create it. */
export class GlobalSetupError extends Error {
  readonly root: string
  constructor(root: string, message: string) {
    super(message)
    this.name = 'GlobalSetupError'
    this.root = root
  }
}

async function readJSON(res: Response): Promise<unknown> {
  try {
    return await res.json()
  } catch {
    return undefined
  }
}

function errorText(body: unknown, status: number): string {
  const error = (body as { error?: unknown } | undefined)?.error
  return typeof error === 'string' ? error : `The server answered ${status}.`
}

export async function requestSwitch(fetcher: typeof fetch = fetch): Promise<SwitchReply> {
  let res: Response
  try {
    res = await fetcher(API.switch, { method: 'POST', headers: { 'Content-Type': 'application/json', Accept: 'application/json' }, body: '{}' })
  } catch {
    throw new Error('Could not reach the server to open the other configuration.')
  }
  const body = await readJSON(res)
  if (!res.ok) {
    const missing = body as { missing?: unknown; root?: unknown } | undefined
    if (res.status === 409 && missing?.missing === true && typeof missing.root === 'string') {
      throw new GlobalSetupError(missing.root, errorText(body, res.status))
    }
    throw new Error(errorText(body, res.status))
  }
  return parseSwitchReply(body)
}

export async function createGlobalSetup(git: boolean, fetcher: typeof fetch = fetch): Promise<GlobalCreateReply> {
  let res: Response
  try {
    res = await fetcher(API.globalCreate, { method: 'POST', headers: { 'Content-Type': 'application/json', Accept: 'application/json' }, body: JSON.stringify({ git }) })
  } catch {
    throw new Error('Could not reach the server to create the global setup.')
  }
  const body = await readJSON(res)
  if (!res.ok) throw new Error(errorText(body, res.status))
  const reply = (body && typeof body === 'object' ? body : {}) as Record<string, unknown>
  if (typeof reply.root !== 'string') throw new Error('invalid create reply')
  const created = Array.isArray(reply.created) ? reply.created.filter((c): c is string => typeof c === 'string') : []
  return { root: reply.root, created, git: typeof reply.git === 'string' ? reply.git : '' }
}
