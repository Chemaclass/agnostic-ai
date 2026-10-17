// Effect wording and tones. Every badge in the UI takes its text from
// here, so the product wording lives in one place:
//
//   allowed      Permitted
//   ask          Permitted, approval required
//   denied       Blocked
//   unavailable  Unavailable through this route (with the excluding list)
//   default      No rule; the tool's default applies
//   unresolved   Unresolved on this tool
//
// An effect the server marks hypothetical (the generated file is stale,
// missing, or unchecked) is always worded conditionally, so the UI never
// shows an unconditional "Permitted" for something that is not installed.

import type {
  Effect,
  EnforcementStatus,
  Explanations,
  GrantList,
  InstalledStateValue,
  Operation,
  Resolution,
  SecretRight,
} from '../contract'

export type Tone = 'permitted' | 'approval' | 'blocked' | 'info' | 'neutral' | 'accent'

export const EFFECTS: readonly Effect[] = ['allowed', 'ask', 'denied', 'unavailable', 'default', 'unresolved']

export const EFFECT_WORDING: Record<Effect, string> = {
  allowed: 'Permitted',
  ask: 'Permitted, approval required',
  denied: 'Blocked',
  unavailable: 'Unavailable through this route',
  default: "No rule; the tool's default applies",
  unresolved: 'Unresolved on this tool',
}

export const HYPOTHETICAL_WORDING: Record<Effect, string> = {
  allowed: 'Would be permitted once synced',
  ask: 'Would require approval once synced',
  denied: 'Would be blocked once synced',
  unavailable: 'Would be unavailable through this route once synced',
  default: "No rule; the tool's default would apply once synced",
  // Nothing is decided either way, so there is nothing to condition.
  unresolved: 'Unresolved on this tool',
}

const TONES: Record<Effect, Tone> = {
  allowed: 'permitted',
  ask: 'approval',
  denied: 'blocked',
  unavailable: 'neutral',
  default: 'neutral',
  unresolved: 'neutral',
}

export function isEffect(value: unknown): value is Effect {
  return typeof value === 'string' && (EFFECTS as readonly string[]).includes(value)
}

export function effectTone(effect: Effect): Tone {
  return TONES[effect] ?? 'neutral'
}

/** The badge text for an effective effect. */
export function effectWording(effect: Effect, hypothetical = false): string {
  const table = hypothetical ? HYPOTHETICAL_WORDING : EFFECT_WORDING
  return table[effect] ?? effect
}

/** The text for a declared effect: the specs' decision, never an effect claim. */
export function declaredWording(effect: Effect): string {
  return `Declared: ${lowerFirst(EFFECT_WORDING[effect] ?? effect)}`
}

export function lowerFirst(text: string): string {
  return text.length ? text[0].toLowerCase() + text.slice(1) : text
}

export function upperFirst(text: string): string {
  return text.length ? text[0].toUpperCase() + text.slice(1) : text
}

const AGENT_SCOPE_LISTS: readonly GrantList[] = ['can', 'tools', 'readonly', 'mcpServers']

export function isAgentScopeList(list: string | undefined): boolean {
  return !!list && (AGENT_SCOPE_LISTS as readonly string[]).includes(list)
}

const LIST_PHRASES: Record<GrantList, string> = {
  allow: 'the allow list',
  ask: 'the ask list',
  deny: 'the deny list',
  protected: 'the protected paths',
  can: "the agent's can list",
  tools: "the agent's tools list",
  readonly: "the agent's readonly flag",
  mcpServers: "the agent's mcpServers list",
}

/** "the agent's mcpServers list", "the deny list", ... */
export function listPhrase(list: string): string {
  return LIST_PHRASES[list as GrantList] ?? `the ${list} list`
}

const LIST_NAMES: Record<GrantList, string> = {
  allow: 'allow list',
  ask: 'ask list',
  deny: 'deny list',
  protected: 'protected paths',
  can: 'can list',
  tools: 'tools list',
  readonly: 'readonly flag',
  mcpServers: 'mcpServers list',
}

export function listName(list: string): string {
  return LIST_NAMES[list as GrantList] ?? `${list} list`
}

/** Which list excluded an unavailable operation, from its winning grant. */
export function excludedBy(op: Operation): string | undefined {
  if (op.effect !== 'unavailable' && op.declared !== 'unavailable') return undefined
  const winner = op.decided ?? op.grants.find((grant) => grant.wins)
  if (!winner) return "Excluded by the subject's own scope"
  return `Excluded by ${listPhrase(winner.list)}`
}

export interface EffectiveDisplay {
  /** The badge text. */
  text: string
  tone: Tone
  hypothetical: boolean
  /** For unavailable effects: the list that excluded the route. */
  excludedBy?: string
  /** For unresolved effects: the declared policy, shown muted beside it. */
  secondary?: { text: string; tone: Tone }
  /** The server's glossary entry for the effect, when it sent one. */
  explanation?: string
  /** The server's glossary entry for how the effect was reached. */
  resolutionNote?: string
}

/** Everything the "Effective on <tool>" cell needs for one operation. */
export function effectiveDisplay(op: Operation, explanations?: Explanations): EffectiveDisplay {
  const hypothetical = op.hypothetical === true
  let text = effectWording(op.effect, hypothetical)
  let secondary: EffectiveDisplay['secondary']
  if (op.effect === 'unresolved') {
    if (op.resolution === 'translated-unverified') text = `${text} · rule written, matching not verified`
    secondary = { text: declaredWording(op.declared), tone: 'neutral' }
  }
  return {
    text,
    tone: effectTone(op.effect),
    hypothetical,
    excludedBy: excludedBy(op),
    secondary,
    explanation: explanations?.effects?.[op.effect],
    resolutionNote: explanations?.resolution?.[op.resolution],
  }
}

const RESOLUTION_LABELS: Record<Resolution, string> = {
  translated: 'Translated',
  'translated-unverified': 'Rule written, matching not verified',
  hook: 'Enforced by a hook',
  'not-expressed': 'Not expressed',
  'agent-scope': 'Decided by agent scope',
  'no-rule': 'No rule',
  disabled: 'Provider disabled',
}

export function resolutionLabel(resolution: string): string {
  return RESOLUTION_LABELS[resolution as Resolution] ?? resolution
}

const ENFORCEMENT_LABELS: Record<EnforcementStatus, string> = {
  'native-rule': 'Native rule',
  hook: 'Hook',
  'not-expressed': 'Not expressed',
  'not-applicable': 'Not applicable',
}

export function enforcementLabel(status: string): string {
  return ENFORCEMENT_LABELS[status as EnforcementStatus] ?? status
}

const INSTALLED: Record<InstalledStateValue, { text: string; tone: Tone }> = {
  'in-sync': { text: 'In sync', tone: 'permitted' },
  stale: { text: 'Stale', tone: 'approval' },
  'not-generated': { text: 'Not generated', tone: 'approval' },
  'not-checked': { text: 'Not checked', tone: 'neutral' },
  'not-applicable': { text: 'Not applicable', tone: 'neutral' },
}

const INSTALLED_FALLBACK_NOTES: Record<InstalledStateValue, string> = {
  'in-sync': 'The generated file matches disk.',
  stale: 'The generated file differs from disk, so the tool runs on an older version.',
  'not-generated': 'Sync has not written the generated file yet.',
  'not-checked': 'The generated file was not compared with disk.',
  'not-applicable': 'No generated file carries this decision.',
}

export function installedDisplay(state: string, explanations?: Explanations): { text: string; tone: Tone; note: string } {
  const known = INSTALLED[state as InstalledStateValue]
  return {
    text: known?.text ?? state,
    tone: known?.tone ?? 'neutral',
    note: explanations?.installed?.[state] ?? INSTALLED_FALLBACK_NOTES[state as InstalledStateValue] ?? '',
  }
}

export const RUNTIME_TEXT = 'Not observed'

const RIGHTS: Record<SecretRight, { text: string; tone: Tone }> = {
  allowed: { text: 'Allowed', tone: 'permitted' },
  unavailable: { text: 'Unavailable', tone: 'neutral' },
  'not-readable': { text: 'Not readable', tone: 'info' },
  unknown: { text: 'Unknown', tone: 'neutral' },
}

export function rightDisplay(right: string): { text: string; tone: Tone } {
  return RIGHTS[right as SecretRight] ?? { text: right, tone: 'neutral' }
}

export const RIGHT_NAMES = {
  discover: 'Discover',
  readValue: 'Read value',
  use: 'Use',
  manage: 'Manage',
} as const

const PERMISSION_SUPPORT: Record<string, string> = {
  'native-rules': 'Writes native permission rules',
  'hooks-only': 'Enforces protected paths through a hook only',
  none: 'No permission support',
}

export function permissionSupportLabel(value: string): string {
  return PERMISSION_SUPPORT[value] ?? value
}

export type Direction = 'widened' | 'narrowed' | 'moved' | 'indeterminate'

export function directionDisplay(direction: string): { text: string; tone: Tone } {
  switch (direction) {
    case 'widened':
      return { text: 'Widened', tone: 'approval' }
    case 'narrowed':
      return { text: 'Narrowed', tone: 'info' }
    case 'moved':
      return { text: 'Moved', tone: 'neutral' }
    default:
      return { text: 'Indeterminate', tone: 'neutral' }
  }
}
