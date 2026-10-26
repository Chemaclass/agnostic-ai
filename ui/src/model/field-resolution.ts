// Plain-language answers for the value sync writes for each tool, from
// the server's resolution. The client never decides which value wins.

import type { SyncState, TargetValue } from '../contract'
import { joinList } from './format'

export interface ValueGroup {
  value?: string
  labels: string[]
}

/** Tools grouped by the value sync writes for them, in configured order. */
export function valueGroups(resolved: TargetValue[]): ValueGroup[] {
  const groups: ValueGroup[] = []
  for (const r of resolved) {
    const value = r.dropped ? undefined : r.value
    const group = groups.find((g) => g.value === value)
    if (group) group.labels.push(r.label)
    else groups.push({ value, labels: [r.label] })
  }
  return groups
}

/** What sync writes for one tool, without claiming a runtime default. */
export function generatedAnswer(r: TargetValue): string {
  if (r.dropped) return `Sync writes no value: ${r.dropped}. ${r.label} keeps its own default.`
  if (!r.value) return `No settings spec sets a value for ${r.label}, so sync writes none and ${r.label} keeps its own default.`
  const via = [r.alias && `resolved from ${r.alias}`, r.tier && `from model tier ${r.tier}`].filter(Boolean)
  const unchecked = r.checked ? '' : ` Sync did not render ${r.label} here, so whether its files take this value was not checked.`
  return `Sync writes ${r.value}${via.length ? ` (${joinList(via as string[])})` : ''}.${unchecked}`
}

/** What the installed files say, from the generated-file status. */
export function installedAnswer(label: string, state: SyncState | undefined): string {
  switch (state) {
    case 'in-sync': return `Installed ${label} files match what sync would write.`
    case 'drifted': return `Installed ${label} files differ from what sync would write. Run sync to install this value.`
    default: return `Installed ${label} files were not checked.`
  }
}
