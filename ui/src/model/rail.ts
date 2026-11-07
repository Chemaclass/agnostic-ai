// The rail's live summaries: one line per section, from the loaded overview.

import type { Section } from '../contract'
import { kindNoun, liveCount, permissionCounts, turnedOff } from './counts'
import type { FindingCount } from './health'

const PERMISSION_WORDS = { 'permissions.allow': 'allowed', 'permissions.ask': 'ask', 'permissions.deny': 'denied' } as const

/** "Authority: 11 allowed, 3 ask, 4 denied": the rules in every permission list. */
export function authoritySummary(section: Pick<Section, 'label' | 'items'>): string {
  const counts = permissionCounts(section)
  const parts = (Object.keys(counts) as (keyof typeof counts)[]).filter((key) => counts[key] > 0).map((key) => `${counts[key]} ${PERMISSION_WORDS[key]}`)
  return `${section.label}: ${parts.length ? parts.join(', ') : 'no permissions yet'}`
}

/** "Context: 1 rule, 3 agents": counts above zero only, or what is missing. */
export function sectionSummary(section: Pick<Section, 'id' | 'label' | 'kinds' | 'items'>): string {
  if (section.id === 'authority') return authoritySummary(section)
  const counted = section.kinds.map((kind) => ({ ...kind, count: liveCount(section, kind) })).filter((kind) => kind.count > 0)
  if (counted.length) return `${section.label}: ${counted.map((kind) => `${kind.count} ${kindNoun(kind, kind.count)}`).join(', ')}`
  const off = turnedOff(section)
  if (off.length) return `${section.label}: ${off.join(', ')} turned off`
  if (section.kinds.length === 1) return `${section.label}: no ${kindNoun(section.kinds[0], 0)} yet`
  return `${section.label}: nothing set up yet`
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

/** The rail's Setup health row: "Needs attention" over "Setup health: 1 to fix, 12 you could add". */
export function healthRow(count: Pick<FindingCount, 'errors' | 'warnings'>, gaps: number): { status: string; summary: string } {
  const findings = count.errors + count.warnings
  const parts = [findings ? `${findings} to fix` : 'nothing to fix', gaps ? `${gaps} you could add` : '']
  return { status: findings ? 'Needs attention' : 'All clear', summary: `Setup health: ${parts.filter(Boolean).join(', ')}` }
}
