// Counts shared by the rail summaries and the page verdicts.

import type { KindCount, PermissionKey, Section } from '../contract'
import { isPermissionKey } from './permission-edit'

/** A type's label as a noun in a sentence, singular for one: "1 rule", "2 instructions files". */
export function kindNoun(kind: Pick<KindCount, 'kind' | 'label'>, count: number): string {
  if (kind.kind === 'instructions') return count === 1 ? 'instructions file' : 'instructions files'
  if (kind.kind === 'review') return 'review instructions'
  const label = /^[A-Z]{2}/.test(kind.label) ? kind.label : kind.label.charAt(0).toLowerCase() + kind.label.slice(1)
  if (count !== 1) return label
  if (label.endsWith('ies')) return `${label.slice(0, -3)}y`
  return label.endsWith('s') ? label.slice(0, -1) : label
}

export function plural(count: number, one: string, many = `${one}s`): string {
  return `${count} ${count === 1 ? one : many}`
}

/** "a, b, and c": the last pair joined with "and". */
export function joinAnd(parts: string[]): string {
  if (parts.length < 3) return parts.join(' and ')
  return `${parts.slice(0, -1).join(', ')}, and ${parts[parts.length - 1]}`
}

export type PermissionCounts = Record<PermissionKey, number>

/** The rules in every permission list of a section. */
export function permissionCounts(section: Pick<Section, 'items'>): PermissionCounts {
  const counts: PermissionCounts = { 'permissions.allow': 0, 'permissions.ask': 0, 'permissions.deny': 0 }
  for (const item of section.items) {
    for (const value of item.values) {
      if (isPermissionKey(value.key) && value.type === 'list') counts[value.key] += value.list?.length ?? 0
    }
  }
  return counts
}
