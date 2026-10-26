// Whether the generated files of the configured tools match the source,
// summed up from the overview's per-target sync states.

import type { TargetSupport } from '../contract'
import { joinList, plural } from './format'

export interface SyncSummary {
  tone: 'match' | 'needs-sync' | 'unknown'
  headline: string
  detail?: string
}

const verb = (count: number, one: string, many: string) => (count === 1 ? one : many)

export function syncSummary(targets: TargetSupport[]): SyncSummary {
  if (targets.length === 0) return { tone: 'unknown', headline: 'No coding tools are configured, so no generated files were checked.' }
  const labels = (state: TargetSupport['sync']['state']) => targets.filter((t) => t.sync.state === state).map((t) => t.label || t.target)
  const drifted = labels('drifted')
  const unchecked = labels('not-checked')
  const notChecked = unchecked.length ? `${joinList(unchecked)} ${verb(unchecked.length, 'was', 'were')} not checked.` : undefined
  if (drifted.length) return { tone: 'needs-sync', headline: `${joinList(drifted)} ${verb(drifted.length, 'needs', 'need')} sync.`, detail: notChecked }
  if (notChecked) return { tone: 'unknown', headline: notChecked }
  return { tone: 'match', headline: `Generated files match the source for ${plural(targets.length, 'tool')}.` }
}
