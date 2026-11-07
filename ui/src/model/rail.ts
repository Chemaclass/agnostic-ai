// The rail's live summaries: one line per section, from the loaded overview.

import type { KindCount, Section } from '../contract'
import type { FindingCount } from './health'

function noun(kind: KindCount, count: number): string {
  if (kind.kind === 'instructions') return count === 1 ? 'instructions file' : 'instructions files'
  const label = /^[A-Z]{2}/.test(kind.label) ? kind.label : kind.label.charAt(0).toLowerCase() + kind.label.slice(1)
  if (count !== 1) return label
  if (label.endsWith('ies')) return `${label.slice(0, -3)}y`
  return label.endsWith('s') ? label.slice(0, -1) : label
}

/** "Context: 1 rule, 3 agents": counts above zero only, or what is missing. */
export function sectionSummary(section: Pick<Section, 'label' | 'kinds'>): string {
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
