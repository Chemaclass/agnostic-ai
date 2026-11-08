// The Authority page's one rules table: every allow, ask and block rule, blocked first.

import type { PermissionKey, SettingField } from '../contract'
import { sourcesForAudience } from './audience-view'
import type { EditAudience } from './edit-audience'

export interface Decision {
  key: PermissionKey
  word: string
  tone: 'blocked' | 'approval' | 'permitted'
}

// Blocked first: it is what a reader checks before trusting an agent.
export const DECISIONS: readonly Decision[] = [
  { key: 'permissions.deny', word: 'Blocked', tone: 'blocked' },
  { key: 'permissions.ask', word: 'Ask first', tone: 'approval' },
  { key: 'permissions.allow', word: 'Allowed', tone: 'permitted' },
]

export interface RuleRow {
  rule: string
  /** The rule's place in its file's list, which an edit or removal addresses. */
  index: number
  path: string
  local: boolean
}

/** The rules a field lists for the audience shown, in file order. */
export function ruleRows(field: SettingField, audience?: EditAudience): RuleRow[] {
  return sourcesForAudience(field.set, audience).flatMap((source) => {
    const list = source.values.find((value) => value.key === field.key && value.type === 'list')?.list ?? []
    return list.map((rule, index) => ({ rule, index, path: source.path, local: source.local === true }))
  })
}

export function hasRules(fields: SettingField[] | undefined, audience?: EditAudience): boolean {
  return DECISIONS.some((decision) => {
    const field = fields?.find((candidate) => candidate.key === decision.key)
    return field !== undefined && ruleRows(field, audience).length > 0
  })
}

/** "2 paths, ask first": the protected paths the audience shown gets. */
export function protectedSummary(field: SettingField, audience?: EditAudience): string {
  const values = sourcesForAudience(field.set, audience).flatMap((source) => source.values)
  if (values.some((value) => value.key === 'protected' && value.value === 'null')) return 'Off for you'
  const paths = values.find((value) => value.key === 'protected.paths')?.list?.length ?? 0
  if (!paths) return 'None'
  const decision = values.find((value) => value.key === 'protected.decision')?.value
  const label = `${paths} path${paths === 1 ? '' : 's'}`
  return decision === 'deny' ? `${label}, blocked` : decision === 'ask' ? `${label}, ask first` : label
}

/** The default permission mode the audience shown gets, else "Not set". */
export function modeSummary(field: SettingField, audience?: EditAudience): string {
  const sources = [...sourcesForAudience(field.set, audience)].sort((a, b) => Number(Boolean(b.local)) - Number(Boolean(a.local)))
  return sources.flatMap((source) => source.values).find((value) => value.key === field.key && value.value && value.value !== 'null')?.value ?? 'Not set'
}

/** The protected paths and reason the audience shown gets. */
export function protectedPaths(field: SettingField, audience?: EditAudience): { paths: string[]; reason?: string } {
  const values = sourcesForAudience(field.set, audience).flatMap((source) => source.values)
  if (values.some((value) => value.key === 'protected' && value.value === 'null')) return { paths: [] }
  return { paths: values.find((value) => value.key === 'protected.paths')?.list ?? [], reason: values.find((value) => value.key === 'protected.reason')?.value || undefined }
}
