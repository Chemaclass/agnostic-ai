import type { PermissionEditPreview, PermissionKey, SectionItem, SettingField } from '../contract'

export function isPermissionKey(key: unknown): key is PermissionKey {
  return key === 'permissions.allow' || key === 'permissions.ask' || key === 'permissions.deny'
}

export function permissionSelection(field: SettingField, files: SectionItem[], path: string) {
  const team = field.set.find((source) => source.path === path && !source.local)
  const target = field.editable?.sources?.find((source) => source.path === path)
  const spec = target?.spec ?? team?.spec ?? files.find((file) => file.path === path)?.name
  const localPath = target?.localPath ?? (path === field.editable?.path ? field.editable.localPath : undefined)
  const local = field.set.find((source) => source.local && (spec ? source.spec === spec : source.path === localPath))
  const rules = (source?: SettingField['set'][number]): string[] | null => source?.values.find((value) => value.key === field.key && value.type === 'list')?.list ?? null
  return { shared: rules(team), personal: rules(local), personalSet: local !== undefined, personalPath: local?.path ?? localPath }
}

export function rulesFromText(text: string): string[] {
  return text.split(/\r?\n/).map((rule) => rule.trim()).filter(Boolean)
}

export function parsePermissionPreview(value: unknown): PermissionEditPreview {
  const invalid = () => new Error('The server returned an invalid permission preview. Reload the page and try again.')
  if (!value || typeof value !== 'object') throw invalid()
  const list = (rules: unknown): rules is string[] | null => rules === null || Array.isArray(rules) && rules.every((rule) => typeof rule === 'string')
  if (!('schemaVersion' in value) || value.schemaVersion !== 'permission-edit/v1'
    || !('file' in value) || typeof value.file !== 'string' || !value.file.trim()
    || !('key' in value) || !isPermissionKey(value.key)
    || !('audience' in value) || value.audience !== 'shared' && value.audience !== 'personal'
    || !('before' in value) || !list(value.before)
    || !('rules' in value) || !list(value.rules)
    || !('diff' in value) || typeof value.diff !== 'string'
    || !('expected' in value) || typeof value.expected !== 'string' || !value.expected.trim()
    || !('syncCommand' in value) || typeof value.syncCommand !== 'string' || !value.syncCommand.trim()) throw invalid()
  return { schemaVersion: value.schemaVersion, file: value.file, key: value.key, audience: value.audience, before: value.before, rules: value.rules, diff: value.diff, expected: value.expected, syncCommand: value.syncCommand }
}
