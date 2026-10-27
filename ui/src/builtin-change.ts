export const BUILTIN_CHANGE_PATH = '/api/changes/builtin'

export interface BuiltinFileChange {
  path: string
  action: 'create' | 'update' | 'delete'
  diff: string
}

export interface BuiltinChange {
  status: 'idle' | 'proposed' | 'applied' | 'undone'
  id?: string
  name?: string
  key?: 'model' | 'effort'
  /** The one tool a model or effort change sets; absent for every tool. */
  target?: string
  title?: string
  enabled: boolean
  reset?: boolean
  scope?: 'team' | 'local'
  config?: string
  targets: string[]
  changes: BuiltinFileChange[]
  findings: string[]
  undoId?: string
  undoName?: string
  undoTitle?: string
  undoAvailable: boolean
  supported: boolean
}

function object(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function strings(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === 'string')
}

function nonempty(value: unknown): value is string {
  return typeof value === 'string' && value.trim().length > 0
}

function fileChange(value: unknown): value is BuiltinFileChange {
  return object(value) && typeof value.path === 'string' && typeof value.diff === 'string'
    && (value.action === 'create' || value.action === 'update' || value.action === 'delete')
}

export function parseBuiltinChange(value: unknown): BuiltinChange {
  if (!object(value) || value.schemaVersion !== 'builtin-change/v1'
    || (value.status !== 'idle' && value.status !== 'proposed' && value.status !== 'applied' && value.status !== 'undone')
    || typeof value.enabled !== 'boolean' || typeof value.supported !== 'boolean' || typeof value.undoAvailable !== 'boolean'
    || !strings(value.targets) || !strings(value.findings) || !Array.isArray(value.changes) || !value.changes.every(fileChange)
    || (value.name !== undefined && !nonempty(value.name)) || (value.status !== 'idle' && !nonempty(value.name) && value.reset !== true)
    || (value.reset !== undefined && typeof value.reset !== 'boolean') || (value.scope !== undefined && value.scope !== 'team' && value.scope !== 'local')
    || (value.id !== undefined && !nonempty(value.id)) || (value.config !== undefined && typeof value.config !== 'string')
    || (value.undoId !== undefined && (!nonempty(value.undoId) || !value.undoAvailable))
    || (value.undoName !== undefined && (!nonempty(value.undoName) || !value.undoAvailable || !value.undoId))
    || (value.status === 'proposed' && !value.id) || (value.undoAvailable && !value.undoId)
    || (value.key !== undefined && value.key !== 'model' && value.key !== 'effort') || (value.key !== undefined && value.name !== value.key)
    || (value.title !== undefined && !nonempty(value.title)) || (value.key !== undefined && value.title === undefined)
    || (value.target !== undefined && (!nonempty(value.target) || value.key === undefined))
    || (value.undoTitle !== undefined && (!nonempty(value.undoTitle) || !value.undoId))) {
    throw new Error('The server returned an invalid change. Reload before trying again.')
  }
  return {
    status: value.status, id: value.id, name: value.name, key: value.key, target: value.target, title: value.title, enabled: value.enabled, reset: value.reset, scope: value.scope, config: value.config,
    targets: value.targets, changes: value.changes, findings: value.findings,
    undoId: value.undoId, undoName: value.undoName, undoTitle: value.undoTitle, undoAvailable: value.undoAvailable, supported: value.supported,
  }
}

export class BuiltinChangeError extends Error {
  constructor(message: string, readonly conflict = false) { super(message) }
}

export type BuiltinChangeAction = 'preview' | 'apply' | 'undo' | 'cancel'

export async function builtinChange(action?: BuiltinChangeAction, body?: unknown): Promise<BuiltinChange> {
  let response: Response
  try {
    response = await fetch(BUILTIN_CHANGE_PATH + (action ? `/${action}` : ''), action ? {
      method: 'POST', headers: { 'Content-Type': 'application/json', Accept: 'application/json' }, body: JSON.stringify(body),
    } : { headers: { Accept: 'application/json' } })
  } catch {
    throw new BuiltinChangeError('Connection lost. Reload to check the change before trying again.')
  }
  const value: unknown = await response.json().catch(() => null)
  if (!response.ok) {
    throw new BuiltinChangeError(object(value) && typeof value.error === 'string' ? value.error : `The server answered ${response.status}. Reload before trying again.`, response.status === 409)
  }
  return parseBuiltinChange(value)
}
