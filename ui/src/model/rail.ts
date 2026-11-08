// The rail's live summaries: one line per section, from the loaded overview.

import type { Section } from '../contract'
import { builtinEnabled } from './audience-view'
import { kindNoun, liveCount, permissionCounts, plural, turnedOff } from './counts'
import type { FindingCount } from './health'

// Blocked first: it is what a reader checks before trusting an agent.
const PERMISSION_WORDS = [['permissions.deny', 'blocked'], ['permissions.ask', 'ask'], ['permissions.allow', 'allowed']] as const

/** "4 blocked, 3 ask, 11 allowed": the rules in every permission list. */
export function authoritySummary(section: Pick<Section, 'items'>): string {
  const counts = permissionCounts(section)
  const parts = PERMISSION_WORDS.filter(([key]) => counts[key] > 0).map(([key, word]) => `${counts[key]} ${word}`)
  return parts.length ? parts.join(', ') : 'No rules yet'
}

/** "2 built-ins on": the built-ins that run, else the preferences set. */
function settingsSummary(section: Pick<Section, 'kinds' | 'items'>): string | undefined {
  const on = section.items.filter((item) => item.kind === 'builtin' && builtinEnabled(item)).length
  if (on) return `${plural(on, 'built-in')} on`
  const preferences = section.kinds.find((kind) => kind.kind === 'settings')?.count ?? 0
  return preferences ? `${plural(preferences, 'preference')} set` : undefined
}

/** "1 rule, 3 agents": counts above zero only, or what is missing, as a short sentence. */
export function sectionSummary(section: Pick<Section, 'id' | 'kinds' | 'items'>): string {
  return sentence(countSummary(section))
}

function countSummary(section: Pick<Section, 'id' | 'kinds' | 'items'>): string {
  if (section.id === 'authority') return authoritySummary(section)
  const settings = section.id === 'settings' ? settingsSummary(section) : undefined
  if (settings) return settings
  const counted = section.kinds.map((kind) => ({ ...kind, count: liveCount(section, kind) })).filter((kind) => kind.count > 0)
  if (counted.length) return counted.map((kind) => `${kind.count} ${kindNoun(kind, kind.count)}`).join(', ')
  const off = turnedOff(section)
  if (off.length) return `${off.join(', ')} turned off`
  if (section.kinds.length === 1) return `no ${kindNoun(section.kinds[0], 0)} yet`
  return 'nothing set up yet'
}

function sentence(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1)
}

export type HealthTone = 'error' | 'warning' | 'clean'

export function healthTone(count: Pick<FindingCount, 'errors' | 'warnings'>): HealthTone {
  return count.errors ? 'error' : count.warnings ? 'warning' : 'clean'
}

/** A finding's severity in words, on finding cards and the Setup health groups. */
export const HEALTH_STATUS: Record<Exclude<HealthTone, 'clean'>, string> = {
  error: 'Needs a fix',
  warning: 'Worth a look',
}

/** The rail's Setup health row: "1 to fix, 12 you could add", plus a status for screen readers. */
export function healthRow(count: Pick<FindingCount, 'errors' | 'warnings'>, gaps: number): { status: string; summary: string } {
  const findings = count.errors + count.warnings
  const parts = [findings ? `${findings} to fix` : 'Nothing to fix', gaps ? `${gaps} you could add` : '']
  return { status: findings ? 'Needs attention' : 'All clear', summary: parts.filter(Boolean).join(', ') }
}
