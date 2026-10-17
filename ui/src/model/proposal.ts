// Proposals and the client preview. The preview is an estimate drawn
// from the view the client already holds: it is always labelled
// "Proposed · client preview, not validated" and replaced by the
// server's ImpactResult once validated. The server alone decides.

import type { AuthorityView, Change, Effect, GrantList, Operation, Proposal, Resource } from '../contract'
import type { Direction } from './effects'

export type ProposalActionKind = 'revoke' | 'require-approval' | 'block'

const SETTINGS_LISTS: readonly string[] = ['allow', 'ask', 'deny']

function settingsGrant(op: Operation) {
  if (op.decided && SETTINGS_LISTS.includes(op.decided.list)) return op.decided
  return op.grants.find((g) => SETTINGS_LISTS.includes(g.list))
}

/**
 * The neutral rule that names this operation: the rule a settings grant
 * already uses when there is one, otherwise one built from the resource.
 */
export function ruleForOperation(resource: Resource, op: Operation): string {
  const existing = settingsGrant(op)
  if (existing) return existing.rule
  const { server, pattern } = resource.identity
  if (resource.kind === 'undeclared-scope' || resource.kind === 'declared-root' || server) {
    const name = server ?? resource.providerId.replace(/^mcp-/, '')
    return op.action && !['tools', 'any', '*'].includes(op.action) ? `mcp:${name}/${op.action}` : `mcp:${name}`
  }
  if (resource.kind === 'command-pattern') return !pattern || pattern === '*' ? 'shell' : `shell(${pattern})`
  if (resource.kind === 'file-pattern') return !pattern || pattern === '**' ? op.action : `${op.action}(${pattern})`
  if (resource.kind === 'web') return 'web'
  return op.action
}

export interface ActionAvailability {
  enabled: boolean
  reason?: string
  changes: Change[]
}

/** What each operation-row action proposes, and why it is unavailable when it is. */
export function proposalAction(kind: ProposalActionKind, resource: Resource, op: Operation): ActionAvailability {
  const rule = ruleForOperation(resource, op)
  switch (kind) {
    case 'revoke': {
      const winner = op.decided
      if (!winner || winner.list !== 'allow') {
        return { enabled: false, reason: 'No allow rule wins here, so there is nothing to revoke.', changes: [] }
      }
      return { enabled: true, changes: [{ op: 'remove', list: 'allow', rule: winner.rule }] }
    }
    case 'require-approval': {
      if (op.declared === 'ask') return { enabled: false, reason: 'Approval is already required.', changes: [] }
      if (op.declared === 'denied') return { enabled: false, reason: 'A deny rule wins; an ask rule would not change it.', changes: [] }
      return { enabled: true, changes: [{ op: 'add', list: 'ask', rule }] }
    }
    case 'block': {
      if (op.declared === 'denied') return { enabled: false, reason: 'A deny rule already wins.', changes: [] }
      return { enabled: true, changes: [{ op: 'add', list: 'deny', rule }] }
    }
  }
}

export function describeChange(change: Change): string {
  return change.op === 'add' ? `Add ${change.rule} to the ${change.list} list` : `Remove ${change.rule} from the ${change.list} list`
}

export type ParsedRule =
  | { kind: 'mcp'; server: string; tool?: string }
  | { kind: 'scoped'; scope: string; arg: string }
  | { kind: 'bare'; scope: string }
  | { kind: 'unknown'; raw: string }

export function parseRule(rule: string): ParsedRule {
  const text = rule.trim()
  let m = /^mcp:([^/\s]+)(?:\/(.+))?$/.exec(text)
  if (m) return { kind: 'mcp', server: m[1], ...(m[2] ? { tool: m[2] } : {}) }
  m = /^mcp__(.+?)(?:__(.+))?$/.exec(text)
  if (m) return { kind: 'mcp', server: m[1], ...(m[2] ? { tool: m[2] } : {}) }
  m = /^([A-Za-z][\w-]*)\((.+)\)$/.exec(text)
  if (m) return { kind: 'scoped', scope: m[1].toLowerCase(), arg: m[2] }
  if (/^[A-Za-z][\w-]*$/.test(text)) return { kind: 'bare', scope: text.toLowerCase() }
  return { kind: 'unknown', raw: text }
}

function commandShape(pattern: string): { prefix: string; wild: boolean } {
  const text = pattern.trim()
  if (text === '*' || text === '') return { prefix: '', wild: true }
  const wild = /(:\*|\s\*|\*)$/.test(text)
  const prefix = text.replace(/(:\*|\s\*|\*)$/, '').trim()
  return { prefix, wild }
}

/** Whether a shell rule argument covers every command a resource pattern names. */
export function commandCovers(ruleArg: string, resourcePattern: string): boolean {
  const rule = commandShape(ruleArg)
  const res = commandShape(resourcePattern)
  if (rule.wild) {
    if (rule.prefix === '') return true
    return res.prefix === rule.prefix || res.prefix.startsWith(`${rule.prefix} `)
  }
  return !res.wild && res.prefix === rule.prefix
}

function globToRegExp(glob: string): RegExp {
  let source = ''
  for (let i = 0; i < glob.length; i++) {
    const ch = glob[i]
    if (ch === '*') {
      if (glob[i + 1] === '*') {
        source += '.*'
        i++
      } else source += '[^/]*'
    } else if (ch === '?') source += '[^/]'
    else source += ch.replace(/[.+^${}()|[\]\\]/g, '\\$&')
  }
  return new RegExp(`^${source}$`)
}

/** Whether a file rule's glob covers a resource pattern (read literally). */
export function globCovers(ruleGlob: string, resourcePattern: string): boolean {
  if (ruleGlob === resourcePattern || ruleGlob === '**') return true
  return globToRegExp(ruleGlob).test(resourcePattern)
}

function scopeMatches(scope: string, arg: string | undefined, resource: Resource, op: Operation): boolean {
  if (scope === 'shell' || scope === 'bash') {
    if (resource.kind !== 'command-pattern') return false
    return arg === undefined || commandCovers(arg, resource.identity.pattern ?? '*')
  }
  if (scope === 'read' || scope === 'edit' || scope === 'write') {
    if (resource.kind !== 'file-pattern') return false
    const fits = scope === 'read' ? op.action === 'read' : op.action === 'edit' || op.action === 'write'
    if (!fits) return false
    return arg === undefined || globCovers(arg, resource.identity.pattern ?? '**')
  }
  if (scope === 'web' || scope === 'fetch' || scope === 'search') {
    if (resource.kind !== 'web') return false
    return scope === 'web' || op.action === scope
  }
  return false
}

/** Whether a rule would apply to an operation, by the neutral rule grammar. */
export function ruleMatchesOperation(rule: string, resource: Resource, op: Operation): boolean {
  if (op.grants.some((g) => g.rule === rule)) return true
  const parsed = parseRule(rule)
  switch (parsed.kind) {
    case 'mcp': {
      const server = resource.identity.server ?? resource.providerId.replace(/^mcp-/, '')
      if (!resource.providerId.startsWith('mcp-') && !resource.identity.server) return false
      return server === parsed.server && (!parsed.tool || parsed.tool === op.action)
    }
    case 'scoped':
      return scopeMatches(parsed.scope, parsed.arg, resource, op)
    case 'bare':
      return scopeMatches(parsed.scope, undefined, resource, op)
    default:
      return false
  }
}

const KNOWN_ORDER: Partial<Record<Effect, number>> = { allowed: 3, ask: 2, denied: 1, unavailable: 1 }
export const INDETERMINATE_REASON = "The tool's default and an unexpressed rule have no established order."

/** Same rule as the server: only known decisions are ranked. */
export function directionOf(before: Effect, after: Effect): { direction: Direction; reason?: string } {
  const a = KNOWN_ORDER[before]
  const b = KNOWN_ORDER[after]
  if (a === undefined || b === undefined) return { direction: 'indeterminate', reason: INDETERMINATE_REASON }
  if (b > a) return { direction: 'widened' }
  if (b < a) return { direction: 'narrowed' }
  return { direction: 'moved' }
}

function listFor(effect: Effect): GrantList | 'agent-scope' | undefined {
  switch (effect) {
    case 'allowed':
      return 'allow'
    case 'ask':
      return 'ask'
    case 'denied':
      return 'deny'
    case 'unavailable':
      return 'agent-scope'
    default:
      return undefined
  }
}

/** Whether the selected tool carries decisions from this list for any operation in the view. */
function harnessCarries(view: AuthorityView, list: GrantList | 'agent-scope'): boolean {
  return view.resources.some((r) =>
    r.operations.some((o) =>
      list === 'agent-scope'
        ? o.resolution === 'agent-scope'
        : (o.decided?.list === list || (list === 'ask' && o.decided?.list === 'protected')) &&
          (o.resolution === 'translated' || o.resolution === 'hook'),
    ),
  )
}

export interface PreviewChange {
  subjectId: string
  resourceId: string
  resourceLabel: string
  operationId: string
  operationLabel: string
  declaredBefore: Effect
  declaredAfter: Effect
  effectiveBefore: Effect
  effectiveAfter: Effect
  direction: Direction
  reason?: string
  hypothetical: boolean
}

export interface ProposalPreview {
  subjectId: string
  changes: PreviewChange[]
  /** Changes that matched no operation of the selected subject. */
  unmatched: Change[]
}

/** The client's estimate of what a proposal changes for the selected subject. */
export function previewProposal(view: AuthorityView, changes: Change[]): ProposalPreview {
  const matched = new Set<number>()
  const out: PreviewChange[] = []
  for (const resource of view.resources) {
    for (const op of resource.operations) {
      const settings = op.grants
        .filter((g) => g.list === 'allow' || g.list === 'ask' || g.list === 'deny' || g.list === 'protected')
        .map((g) => ({ list: g.list as string, rule: g.rule }))
      let touched = false
      changes.forEach((change, index) => {
        if (change.op === 'remove') {
          const at = settings.findIndex((s) => s.list === change.list && s.rule === change.rule)
          if (at >= 0) {
            settings.splice(at, 1)
            touched = true
            matched.add(index)
          }
        } else if (ruleMatchesOperation(change.rule, resource, op)) {
          if (!settings.some((s) => s.list === change.list && s.rule === change.rule)) settings.push({ list: change.list, rule: change.rule })
          touched = true
          matched.add(index)
        }
      })
      if (!touched) continue

      // The subject's own scope outranks every settings list: a route the
      // agent cannot take stays unavailable whatever the lists say.
      const protectedDecision: Effect = op.decided?.list === 'protected' ? op.declared : 'ask'
      let declaredAfter: Effect = 'default'
      if (op.declared === 'unavailable') declaredAfter = 'unavailable'
      else if (settings.some((s) => s.list === 'deny') || (protectedDecision === 'denied' && settings.some((s) => s.list === 'protected'))) declaredAfter = 'denied'
      else if (settings.some((s) => s.list === 'ask' || s.list === 'protected')) declaredAfter = 'ask'
      else if (settings.some((s) => s.list === 'allow')) declaredAfter = 'allowed'

      const list = listFor(declaredAfter)
      const effectiveAfter: Effect = list === undefined ? 'default' : harnessCarries(view, list) ? declaredAfter : 'unresolved'
      if (declaredAfter === op.declared && effectiveAfter === op.effect) continue

      const { direction, reason } =
        effectiveAfter === op.effect
          ? { direction: 'indeterminate' as Direction, reason: 'The effect on this tool stays the same; only the declaration changes.' }
          : directionOf(op.effect, effectiveAfter)
      out.push({
        subjectId: view.agent.id,
        resourceId: resource.id,
        resourceLabel: resource.label,
        operationId: op.id,
        operationLabel: op.label,
        declaredBefore: op.declared,
        declaredAfter,
        effectiveBefore: op.effect,
        effectiveAfter,
        direction,
        ...(reason ? { reason } : {}),
        hypothetical: op.hypothetical,
      })
    }
  }
  return { subjectId: view.agent.id, changes: out, unmatched: changes.filter((_, i) => !matched.has(i)) }
}

/** The body POSTed to the impact endpoint. */
export function buildProposal(view: AuthorityView, changes: Change[]): Proposal {
  return { agentId: view.agent.id, target: view.harness.target, revision: view.snapshot.revision, changes }
}

export function sameChange(a: Change, b: Change): boolean {
  return a.op === b.op && a.list === b.list && a.rule === b.rule
}
