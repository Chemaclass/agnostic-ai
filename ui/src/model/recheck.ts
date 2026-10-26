// Whether a finding still appears after the checks run again. Findings
// are matched by their link ids, which ignore line numbers, so a finding
// that only moved still counts as remaining. A lint run that stopped, or
// a refresh that failed, never counts as a fix.

import { LINT_INCOMPLETE, type ItemFinding, type OverviewView } from '../contract'
import { plural } from './format'
import { findingEntryIds, itemEntryId } from './identity'

export interface FoundFinding {
  id: string
  finding: ItemFinding
}

export interface RecheckTarget {
  id: string
  finding: ItemFinding
}

export interface RecheckResult {
  state: 'remaining' | 'resolved' | 'incomplete' | 'failed'
  /** For a remaining finding, the id it now has on the page. */
  id?: string
  /** New findings from the same check on the same file. */
  added: FoundFinding[]
  /** Identical findings before and after, when fewer remain. */
  copies?: { before: number; after: number }
}

export function overviewFindings(view: OverviewView): FoundFinding[] {
  const out: FoundFinding[] = []
  const add = (findings: ItemFinding[], owner?: string) => findingEntryIds(findings, owner).forEach((id, i) => out.push({ id, finding: findings[i] }))
  for (const section of view.sections) {
    for (const item of section.items) add(item.findings, itemEntryId(item))
    add(section.findings)
  }
  return out
}

const baseId = (id: string) => id.replace(/-\d+$/, '')
const stopped = (finding: ItemFinding) => finding.source === 'lint' && finding.code === LINT_INCOMPLETE

export function recheckResult(target: RecheckTarget, before: OverviewView, after: OverviewView): RecheckResult {
  const was = overviewFindings(before)
  const now = overviewFindings(after)
  const known = new Set(was.map((f) => f.id))
  const sameFile = (f: FoundFinding) => f.finding.source === target.finding.source && (f.finding.path ?? '') === (target.finding.path ?? '')
  const added = now.filter((f) => !known.has(f.id) && sameFile(f) && !stopped(f.finding))
  const copiesOf = (list: FoundFinding[]) => list.filter((f) => baseId(f.id) === baseId(target.id))
  const left = copiesOf(now)
  if (left.length) {
    const result: RecheckResult = { state: 'remaining', id: left.some((f) => f.id === target.id) ? target.id : left[0].id, added }
    const earlier = copiesOf(was).length
    if (left.length < earlier) result.copies = { before: earlier, after: left.length }
    return result
  }
  if (target.finding.source === 'lint' && !stopped(target.finding) && now.some((f) => stopped(f.finding))) return { state: 'incomplete', added: [] }
  return { state: 'resolved', added }
}

export function recheckMessage(result: RecheckResult, finding: ItemFinding): string {
  const parts: string[] = []
  switch (result.state) {
    case 'remaining':
      parts.push('This finding still appears after the recheck.')
      if (result.copies) parts.push(`${result.copies.after} of ${result.copies.before} identical findings still ${result.copies.after === 1 ? 'appears' : 'appear'}.`)
      break
    case 'resolved':
      parts.push(`This ${finding.source} finding no longer appears.`)
      break
    case 'incomplete':
      return 'Lint stopped before it finished, so this recheck cannot tell whether the finding is fixed. The last known finding is shown.'
    case 'failed':
      return 'The recheck did not finish, so this finding may still apply.'
  }
  const count = result.added.length
  if (count) parts.push(`${plural(count, `new ${finding.source} finding`)} ${count === 1 ? 'appears' : 'appear'}${finding.path ? ` in ${finding.path}` : ''}.`)
  return parts.join(' ')
}
