import type { ProtectedBlock, ProtectedEditPreview, SectionItem, SettingField } from '../contract'

export const PROTECT_DECISIONS = ['ask', 'deny'] as const

function blockFrom(values: SettingField['set'][number]['values'] | undefined): ProtectedBlock | null {
  const text = (key: string) => values?.find((value) => value.key === `protected.${key}` && value.type === 'text')?.value ?? ''
  const paths = values?.find((value) => value.key === 'protected.paths' && value.type === 'list')?.list
  if (!paths && !text('decision') && !text('reason')) return null
  return { paths: paths ?? [], decision: text('decision'), reason: text('reason') }
}

/** The project and local protected blocks of the spec at path, as each file writes them. */
export function protectedSelection(field: SettingField, files: SectionItem[], path: string) {
  const team = field.set.find((source) => source.path === path && !source.local)
  const target = field.editable?.sources?.find((source) => source.path === path)
  const spec = target?.spec ?? team?.spec ?? files.find((file) => file.path === path)?.name
  const localPath = target?.localPath ?? (path === field.editable?.path ? field.editable.localPath : undefined)
  const local = field.set.find((source) => source.local && (spec ? source.spec === spec : source.path === localPath))
  return { shared: blockFrom(team?.values), personal: blockFrom(local?.values), personalSet: local !== undefined, personalPath: local?.path ?? localPath }
}

function isBlock(value: unknown): value is ProtectedBlock | null {
  if (value === null) return true
  if (!value || typeof value !== 'object') return false
  const block = value as Record<string, unknown>
  return Array.isArray(block.paths) && block.paths.every((path) => typeof path === 'string') && typeof block.decision === 'string' && typeof block.reason === 'string'
}

export function parseProtectedPreview(value: unknown): ProtectedEditPreview {
  const invalid = () => new Error('The server returned an invalid protected paths preview. Reload the page and try again.')
  if (!value || typeof value !== 'object') throw invalid()
  const reply = value as Record<string, unknown>
  if (reply.schemaVersion !== 'protected-edit/v1'
    || typeof reply.file !== 'string' || !reply.file.trim()
    || reply.audience !== 'shared' && reply.audience !== 'personal'
    || !isBlock(reply.before) || !isBlock(reply.protected)
    || typeof reply.diff !== 'string'
    || typeof reply.expected !== 'string' || !reply.expected.trim()
    || typeof reply.syncCommand !== 'string' || !reply.syncCommand.trim()) throw invalid()
  return { schemaVersion: reply.schemaVersion, file: reply.file, audience: reply.audience, before: reply.before, protected: reply.protected, diff: reply.diff, expected: reply.expected, syncCommand: reply.syncCommand }
}
