import type { ProtectedBlock, ProtectedEditPreview, SectionItem, SettingField } from '../contract'

export const PROTECT_DECISIONS = ['ask', 'deny'] as const

type Values = SettingField['set'][number]['values']

function fieldsFrom(values: Values | undefined): Partial<ProtectedBlock> {
  const fields: Partial<ProtectedBlock> = {}
  for (const value of values ?? []) {
    if (value.key === 'protected.paths' && value.type === 'list') fields.paths = value.list ?? []
    else if ((value.key === 'protected.decision' || value.key === 'protected.reason') && value.type === 'text') fields[value.key === 'protected.decision' ? 'decision' : 'reason'] = value.value === '""' || value.value === 'null' ? '' : value.value ?? ''
  }
  return fields
}

function blockOf(fields: Partial<ProtectedBlock>): ProtectedBlock | null {
  if (Object.keys(fields).length === 0) return null
  return { paths: fields.paths ?? [], decision: fields.decision ?? '', reason: fields.reason ?? '' }
}

/** Whether the overview hid part of block because it looks like a credential. */
export function hidesValue(block: ProtectedBlock | null) {
  return !!block && [...block.paths, block.reason].some((text) => text.includes('<redacted>'))
}

/** The project and local protected blocks of the spec at path, as each file writes them, and the block sync reads for you. */
export function protectedSelection(field: SettingField, files: SectionItem[], path: string) {
  const team = field.set.find((source) => source.path === path && !source.local)
  const target = field.editable?.sources?.find((source) => source.path === path)
  const spec = target?.spec ?? team?.spec ?? files.find((file) => file.path === path)?.name
  const localPath = target?.localPath ?? (path === field.editable?.path ? field.editable.localPath : undefined)
  const local = field.set.find((source) => source.local && (spec ? source.spec === spec : source.path === localPath))
  const project = fieldsFrom(team?.values)
  const own = fieldsFrom(local?.values)
  // A local `protected: null` turns the project block off for you.
  const personalCleared = local?.values.some((value) => value.key === 'protected' && value.value === 'null') ?? false
  // A local block merges over the project block key by key, as sync reads it.
  return { shared: blockOf(project), personal: blockOf(own), merged: personalCleared ? null : blockOf({ ...project, ...own }), personalSet: local !== undefined, personalCleared, personalPath: local?.path ?? localPath }
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
    || typeof reply.syncCommand !== 'string' || !reply.syncCommand.trim()
    || reply.cleared !== undefined && typeof reply.cleared !== 'boolean') throw invalid()
  const preview: ProtectedEditPreview = { schemaVersion: reply.schemaVersion, file: reply.file, audience: reply.audience, before: reply.before, protected: reply.protected, diff: reply.diff, expected: reply.expected, syncCommand: reply.syncCommand }
  if (typeof reply.cleared === 'boolean') preview.cleared = reply.cleared
  return preview
}
