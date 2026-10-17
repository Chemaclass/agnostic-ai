// The status pill on a resource card names its most consequential
// operation and that operation's effective effect, for example
// "Deploy would be permitted once synced". The server ranks nothing, so
// the client ranks actions by what they can change: destructive actions
// first, then actions that change state, then reads. The pill always
// states a real operation's real effect; only the choice of which
// operation to feature is the client's.

import type { Effect, Operation, Resource } from '../contract'
import { effectTone, effectWording, lowerFirst, type Tone } from './effects'

const DESTRUCTIVE = ['delete', 'destroy', 'drop', 'remove', 'rm', 'execute', 'exec', 'merge', 'purge', 'truncate', 'revoke', 'force']
const MUTATING = ['deploy', 'push', 'edit', 'write', 'create', 'update', 'variables', 'restart', 'run', 'shell', 'publish', 'upload', 'set', 'apply', 'commit', 'rollback', 'scale', 'send', 'post', 'insert']
const READING = ['read', 'get', 'list', 'logs', 'log', 'query', 'fetch', 'search', 'view', 'show', 'status', 'diff', 'describe', 'inspect', 'download', 'find']

/** 5 destructive, 4 changes state, 3 unknown, 1 reads. */
export function actionWeight(action: string): number {
  const words = action.toLowerCase().split(/[^a-z]+/).filter(Boolean)
  const first = words[0] ?? ''
  if (DESTRUCTIVE.includes(first)) return 5
  if (MUTATING.includes(first)) return 4
  if (READING.includes(first)) return 1
  if (words.some((w) => DESTRUCTIVE.includes(w))) return 5
  if (words.some((w) => MUTATING.includes(w))) return 4
  if (words.some((w) => READING.includes(w))) return 1
  return 3
}

export function isReadLike(action: string): boolean {
  return actionWeight(action) <= 1
}

// Among operations of equal weight, the one that needs no human is the
// more consequential.
const OPEN_RANK: Partial<Record<Effect, number>> = { allowed: 3, ask: 2, default: 1, unresolved: 1 }
const CLOSED_RANK: Partial<Record<Effect, number>> = { denied: 2, unavailable: 1 }

export interface ResourceStatus {
  text: string
  tone: Tone
  operationId?: string
  effect?: Effect
  hypothetical: boolean
}

function pick(ops: Operation[], rank: Partial<Record<Effect, number>>): Operation {
  return ops
    .map((op, index) => ({ op, index }))
    .sort(
      (a, b) =>
        actionWeight(b.op.action) - actionWeight(a.op.action) ||
        (rank[b.op.effect] ?? 0) - (rank[a.op.effect] ?? 0) ||
        a.index - b.index,
    )[0].op
}

/** "Deploy would be permitted once synced", "Delete blocked", "Edit files: no rule; ..." */
export function operationPhrase(op: Operation): string {
  const wording = effectWording(op.effect, op.hypothetical)
  const joiner = op.effect === 'allowed' || op.effect === 'ask' || op.effect === 'denied' ? ' ' : ': '
  return `${op.label}${joiner}${lowerFirst(wording)}`
}

export function resourceStatus(resource: Resource): ResourceStatus {
  const ops = resource.operations
  if (ops.length === 0) return { text: 'No operations known', tone: 'neutral', hypothetical: false }
  const hypothetical = ops.some((op) => op.hypothetical)

  // Operations that can happen: permitted, or left to a default nobody declared.
  const open = ops.filter((op) => op.effect in OPEN_RANK)
  const permitted = open.filter((op) => op.effect === 'allowed' || op.effect === 'ask')
  // Nothing is decided either way (every open operation is left to a
  // default or unresolved, all alike): the effect alone says it best.
  if (open.length > 0 && permitted.length === 0 && new Set(open.map((op) => op.effect)).size === 1) {
    const only = open[0]
    return { text: effectWording(only.effect, only.hypothetical), tone: effectTone(only.effect), effect: only.effect, operationId: only.id, hypothetical: only.hypothetical }
  }
  if (open.length > 0) {
    const top = pick(open, OPEN_RANK)
    if (isReadLike(top.action) && (top.effect === 'allowed' || top.effect === 'ask')) {
      const approval = open.some((op) => op.effect === 'ask')
      const hyp = open.some((op) => op.hypothetical)
      const text = hyp
        ? approval
          ? 'Would be read only, approval required, once synced'
          : 'Would be read only once synced'
        : approval
          ? 'Read only, approval required'
          : 'Read only'
      return { text, tone: 'info', operationId: top.id, effect: top.effect, hypothetical: hyp }
    }
    return { text: operationPhrase(top), tone: effectTone(top.effect), operationId: top.id, effect: top.effect, hypothetical: top.hypothetical }
  }

  // Nothing can happen through this route. One shared effect reads best alone.
  const effects = new Set(ops.map((op) => op.effect))
  if (effects.size === 1) {
    const only = ops[0]
    return { text: effectWording(only.effect, hypothetical), tone: effectTone(only.effect), effect: only.effect, operationId: only.id, hypothetical }
  }
  const top = pick(ops, CLOSED_RANK)
  return { text: operationPhrase(top), tone: effectTone(top.effect), operationId: top.id, effect: top.effect, hypothetical: top.hypothetical }
}
