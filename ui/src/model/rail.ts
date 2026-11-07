// The rail's live summaries: one line per section, from the loaded overview.

import type { KindCount, Section } from '../contract'
import { isPermissionKey } from './permission-edit'
import type { FindingCount } from './health'

function noun(kind: KindCount, count: number): string {
  if (kind.kind === 'instructions') return count === 1 ? 'instructions file' : 'instructions files'
  const label = /^[A-Z]{2}/.test(kind.label) ? kind.label : kind.label.charAt(0).toLowerCase() + kind.label.slice(1)
  if (count !== 1) return label
  if (label.endsWith('ies')) return `${label.slice(0, -3)}y`
  return label.endsWith('s') ? label.slice(0, -1) : label
}

const PERMISSION_WORDS = { 'permissions.allow': 'allowed', 'permissions.ask': 'ask', 'permissions.deny': 'denied' } as const

/** "Authority: 11 allowed, 3 ask, 4 denied": the rules in every permission list. */
export function authoritySummary(section: Pick<Section, 'label' | 'items'>): string {
  const counts = { 'permissions.allow': 0, 'permissions.ask': 0, 'permissions.deny': 0 }
  for (const item of section.items) {
    for (const value of item.values) {
      if (isPermissionKey(value.key) && value.type === 'list') counts[value.key] += value.list?.length ?? 0
    }
  }
  const parts = (Object.keys(counts) as (keyof typeof counts)[]).filter((key) => counts[key] > 0).map((key) => `${counts[key]} ${PERMISSION_WORDS[key]}`)
  return `${section.label}: ${parts.length ? parts.join(', ') : 'no permissions yet'}`
}

/** "Context: 1 rule, 3 agents": counts above zero only, or what is missing. */
export function sectionSummary(section: Pick<Section, 'id' | 'label' | 'kinds' | 'items'>): string {
  if (section.id === 'authority') return authoritySummary(section)
  const counted = section.kinds.filter((kind) => kind.count > 0)
  if (counted.length) return `${section.label}: ${counted.map((kind) => `${kind.count} ${noun(kind, kind.count)}`).join(', ')}`
  if (section.kinds.length === 1) return `${section.label}: no ${noun(section.kinds[0], 0)} yet`
  return `${section.label}: nothing set up yet`
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
