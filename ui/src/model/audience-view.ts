import type { SectionItem, SettingField, Value } from '../contract'
import type { EditAudience } from './edit-audience'

type FieldSource = SettingField['set'][number]

export function builtinEnabled(item: Pick<SectionItem, 'enabled' | 'local'>, audience?: EditAudience): boolean {
  // The API marks a built-in local only when its state differs from shared.
  return audience === 'shared' && item.local ? !item.enabled : item.enabled === true
}

export function sourcesForAudience(sources: FieldSource[], audience?: EditAudience): FieldSource[] {
  if (!audience) return sources
  if (audience === 'shared') return sources.filter((source) => !source.local)
  return sources.flatMap((source) => {
    if (source.local) return [source]
    const overrides = sources.filter((other) => other.local && other.spec === source.spec).flatMap((other) => other.values)
    const values = source.values.filter((value) => !overrides.some((own) => value.key === own.key || value.key.startsWith(`${own.key}.`)))
    return values.length ? [{ ...source, values }] : []
  })
}

export function comparisonValue(values: Value[], key: string): string | undefined {
  const value = values.find((entry) => entry.key === key) ?? values.find((entry) => key.startsWith(`${entry.key}.`) && entry.value === 'null')
  if (!value) return values.some((entry) => entry.key.startsWith(`${key}.`)) ? 'Configured' : undefined
  if (value.type === 'bool') return value.value === 'true' ? 'On' : 'Off'
  if (value.type === 'list') return value.list?.length ? value.list.join(', ') : 'Empty'
  return value.value === 'null' ? 'Cleared' : value.value
}
