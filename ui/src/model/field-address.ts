// Where a model or effort edit lands: the value for every tool, or one
// tool's entry in the field's per-tool map (model.codex). The server
// writes the file; this only reads what the overview already reports.

import type { TargetValue, Value } from '../contract'

/** The dotted key the address shows as: `model`, or `model.codex` for one tool. */
export function addressKey(key: string, target?: string): string {
  return target ? `${key}.${target}` : key
}

/** The value a spec gives the address: one tool's entry, or the field's own value or map default for every tool. */
export function valueAt(values: Value[], key: string, target?: string): string | undefined {
  const find = (k: string) => values.find((entry) => entry.key === k)?.value
  if (target) return find(addressKey(key, target))
  return find(key) ?? find(`${key}.default`)
}

/**
 * The value a personal file sets at the address itself. `localKeys` are the dotted keys that file sets; a value it
 * only inherits from the shared spec does not count. Without `localKeys`, every value counts.
 */
export function personalValueAt(values: Value[], localKeys: string[] | undefined, key: string, target?: string): string | undefined {
  const own = (k: string) => !localKeys || localKeys.includes(k)
  if (target) return own(addressKey(key, target)) ? valueAt(values, key, target) : undefined
  if (values.some((entry) => entry.key === key)) return own(key) ? valueAt(values, key) : undefined
  return own(`${key}.default`) ? valueAt(values, key) : undefined
}

/** Whether a personal file sets the field as one value for every tool, which replaces any per-tool team value. */
export function personalSetsWhole(localKeys: string[] | undefined, key: string): boolean {
  return !!localKeys?.includes(key)
}

/** Where the value a tool gets comes from, such as "personal · codex" or "shared · all tools". */
export function originLabel(r: TargetValue): string | undefined {
  if (!r.from?.spec) return undefined
  return `${r.from.local ? 'personal' : 'shared'} · ${r.from.perTool ? r.target : 'all tools'}`
}

/** The config key that replaces every spec value for this tool, when one does. */
export function configOverride(r: TargetValue): string | undefined {
  return r.from && !r.from.spec ? r.from.path : undefined
}
