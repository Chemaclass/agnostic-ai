// The user settings routes of a user-level server, and the session key
// that lets this page loosen approvals. Only a page that view opened
// through its unlock file gets the key.

import { API, type SettingKey, type SettingsSyncPlan, type SettingsSyncReply, type SettingsView, type SettingsWriteReply } from './contract'

const SESSION_STORAGE = 'agnostic-ai.session'
const SESSION_HEADER = 'X-Agnostic-Ai-Session'
const UNLOCK_FRAGMENT = /^#unlock=([0-9a-f]+)$/

/** A refused settings call: the reason, and what the server holds now. */
export class SettingsError extends Error {
  readonly status?: number
  /** The current document, on a 409 or a refused write. */
  readonly settings?: SettingsView
  /** The new plan, when the pending changes moved. */
  readonly plan?: SettingsSyncPlan
  /** The command to run by hand, when a sync failed. */
  readonly command?: string
  constructor(message: string, details: { status?: number; settings?: SettingsView; plan?: SettingsSyncPlan; command?: string } = {}) {
    super(message)
    this.name = 'SettingsError'
    this.status = details.status
    this.settings = details.settings
    this.plan = details.plan
    this.command = details.command
  }
}

function sessionKey(): string | undefined {
  try {
    return window.sessionStorage.getItem(SESSION_STORAGE) ?? undefined
  } catch {
    return undefined
  }
}

/**
 * Trades the one-time code in the address for the session key, then
 * removes the code from the address. A used or expired code leaves the
 * page locked. Resolves once done, or at once when there is no code.
 */
export function unlockSession(): Promise<void> {
  const match = UNLOCK_FRAGMENT.exec(window.location.hash)
  if (!match) return unlocking
  window.history.replaceState(window.history.state, '', `${window.location.pathname}${window.location.search}`)
  // A service worker left on this address by an earlier program could
  // watch the exchange, so the page removes it and stays locked.
  if (navigator.serviceWorker?.controller) {
    unlockRefused = SERVICE_WORKER_REFUSAL
    unlocking = navigator.serviceWorker.getRegistrations().then((all) => Promise.all(all.map((one) => one.unregister()))).then(() => undefined, () => undefined)
    return unlocking
  }
  unlocking = exchange(match[1])
  return unlocking
}

export const SERVICE_WORKER_REFUSAL = 'A service worker controlled this page, so it stayed locked. It is removed now: close this tab and run agnostic-ai view --scope user again.'

let unlockRefused: string | undefined

/** Why this tab did not unlock, when the reason is worth showing. */
export function unlockProblem(): string | undefined {
  return unlockRefused
}

/** Settings calls wait for a code exchange in flight, so the first read already counts as unlocked. */
let unlocking: Promise<void> = Promise.resolve()

async function exchange(code: string): Promise<void> {
  try {
    const res = await fetch(API.sessionUnlock, { method: 'POST', headers: { 'Content-Type': 'application/json', Accept: 'application/json' }, body: JSON.stringify({ code }) })
    if (!res.ok) return
    const reply = (await res.json()) as { key?: unknown }
    if (typeof reply.key === 'string') window.sessionStorage.setItem(SESSION_STORAGE, reply.key)
  } catch {
    // The page stays locked; tightening still works.
  }
}

/** The session key header, when this tab has the key; hook and MCP server saves at user level need it. */
export function sessionHeaders(): Record<string, string> {
  const key = sessionKey()
  return key ? { [SESSION_HEADER]: key } : {}
}

function headers(): Record<string, string> {
  const key = sessionKey()
  return { 'Content-Type': 'application/json', Accept: 'application/json', ...(key ? { [SESSION_HEADER]: key } : {}) }
}

async function call<T>(path: string, init: () => RequestInit): Promise<{ body: T; res: Response }> {
  await unlocking
  let res: Response
  try {
    res = await fetch(path, init())
  } catch {
    throw new SettingsError('Could not reach the server.')
  }
  if (res.ok) return { body: (await res.json()) as T, res }
  let reply: { error?: string; settings?: SettingsView; plan?: SettingsSyncPlan; command?: string } = {}
  try {
    reply = (await res.json()) as typeof reply
  } catch {
    // Not JSON; the status says enough.
  }
  throw new SettingsError(reply.error ?? `The server answered ${res.status}.`, { status: res.status, settings: reply.settings, plan: reply.plan, command: reply.command })
}

/** Reads the settings document; undefined when it still matches tag. */
export async function readSettings(tag?: string, signal?: AbortSignal): Promise<{ settings: SettingsView; tag?: string } | undefined> {
  await unlocking
  const init: RequestInit = { signal, headers: { ...headers(), ...(tag ? { 'If-None-Match': tag } : {}) } }
  let res: Response
  try {
    res = await fetch(API.settingsView, init)
  } catch (err) {
    if ((err as Error).name === 'AbortError') throw err
    throw new SettingsError('Could not reach the server.')
  }
  if (res.status === 304) return undefined
  if (!res.ok) throw new SettingsError(`The settings could not be read (${res.status}).`, { status: res.status })
  return { settings: (await res.json()) as SettingsView, tag: res.headers.get('ETag') ?? undefined }
}

/** Saves one setting; expected is the value the page showed. */
export async function saveSetting(key: SettingKey, value: string | boolean, expected: string | boolean | undefined): Promise<SettingsWriteReply> {
  const body = expected === undefined ? { key, value } : { key, value, expected }
  return (await call<SettingsWriteReply>(API.settingsWrite, () => ({ method: 'POST', headers: headers(), body: JSON.stringify(body) }))).body
}

export async function planSync(): Promise<SettingsSyncPlan> {
  return (await call<SettingsSyncPlan>(API.settingsSync, () => ({ method: 'POST', headers: headers(), body: JSON.stringify({ action: 'plan' }) }))).body
}

export async function applySync(id: string): Promise<SettingsSyncReply> {
  return (await call<SettingsSyncReply>(API.settingsSync, () => ({ method: 'POST', headers: headers(), body: JSON.stringify({ action: 'apply', id }) }))).body
}
