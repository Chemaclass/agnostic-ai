import { API, type SwitchReply } from '../contract'

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

function baseName(path: string): string {
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
    command: `agnostic-ai ui --dir ${served.projectRoot}`,
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

export async function requestSwitch(fetcher: typeof fetch = fetch): Promise<SwitchReply> {
  let res: Response
  try {
    res = await fetcher(API.switch, { method: 'POST', headers: { 'Content-Type': 'application/json', Accept: 'application/json' }, body: '{}' })
  } catch {
    throw new Error('Could not reach the server to open the other configuration.')
  }
  let body: unknown
  try {
    body = await res.json()
  } catch {
    body = undefined
  }
  if (!res.ok) {
    const error = (body as { error?: unknown } | undefined)?.error
    throw new Error(typeof error === 'string' ? error : `The server answered ${res.status}.`)
  }
  return parseSwitchReply(body)
}
