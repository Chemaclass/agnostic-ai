// The rail's live summaries: one line per section, from the loaded overview.

import type { Section } from '../contract'
import { kindNoun, liveCount, permissionCounts, turnedOff } from './counts'
import type { FindingCount } from './health'

const PERMISSION_WORDS = { 'permissions.allow': 'allowed', 'permissions.ask': 'ask', 'permissions.deny': 'denied' } as const

/** "Authority: 11 allowed, 3 ask, 4 denied": the rules in every permission list. */
export function authoritySummary(section: Pick<Section, 'items'>): string {
  const counts = permissionCounts(section)
  const parts = (Object.keys(counts) as (keyof typeof counts)[]).filter((key) => counts[key] > 0).map((key) => `${counts[key]} ${PERMISSION_WORDS[key]}`)
  return parts.length ? parts.join(', ') : 'no permissions yet'
}

/** "1 rule, 3 agents": counts above zero only, or what is missing. The rail shows it under the section's name and question. */
export function sectionSummary(section: Pick<Section, 'id' | 'kinds' | 'items'>): string {
  if (section.id === 'authority') return authoritySummary(section)
  const counted = section.kinds.map((kind) => ({ ...kind, count: liveCount(section, kind) })).filter((kind) => kind.count > 0)
  if (counted.length) return counted.map((kind) => `${kind.count} ${kindNoun(kind, kind.count)}`).join(', ')
  const off = turnedOff(section)
  if (off.length) return `${off.join(', ')} turned off`
  if (section.kinds.length === 1) return `no ${kindNoun(section.kinds[0], 0)} yet`
  return 'nothing set up yet'
}

export type HealthTone = 'error' | 'warning' | 'clean'

export function healthTone(count: Pick<FindingCount, 'errors' | 'warnings'>): HealthTone {
  return count.errors ? 'error' : count.warnings ? 'warning' : 'clean'
}

/** The status word beside the dot, with the same words as a finding card's severity. */
export const HEALTH_STATUS: Record<HealthTone, string> = {
  error: 'Needs a fix',
  warning: 'Worth a look',
  clean: 'All clear',
}
