// Setup health: every finding in the overview, counted by severity and
// grouped for one view. Findings keep the link ids the section pages
// give them, so links and rechecks agree between the two places.

import type { Gap, Improvements, ItemFinding, OverviewView, Section, SectionItem } from '../contract'
import { FIX_GUIDANCE, findingFacts } from './finding-prompt'
import { plural } from './format'
import { findingEntryIds, itemEntryId } from './identity'

export type HealthSeverity = 'error' | 'warning'

export const HEALTH_SEVERITIES: HealthSeverity[] = ['error', 'warning']

/** Lint and doctor say `warn` or `warning`; anything but an error counts as a warning, as on the section pages. */
export function healthSeverity(finding: ItemFinding): HealthSeverity {
  return finding.severity === 'error' ? 'error' : 'warning'
}

/** The findings one owner carries: an item, or the section itself. */
export interface FindingPlace {
  section: Section
  item?: SectionItem
  owner?: string
  /** Section findings on the global setup, listed apart from the project's. */
  global?: boolean
  findings: ItemFinding[]
  ids: string[]
}

export interface FindingCount {
  errors: number
  warnings: number
  byPlace: FindingPlace[]
}

export function countFindings(overview: OverviewView): FindingCount {
  const byPlace: FindingPlace[] = []
  let errors = 0
  let warnings = 0
  const add = (place: FindingPlace) => {
    if (!place.findings.length) return
    byPlace.push(place)
    for (const finding of place.findings) {
      if (healthSeverity(finding) === 'error') errors++
      else warnings++
    }
  }
  for (const section of [...overview.sections].sort((a, b) => a.order - b.order)) {
    for (const item of section.items) {
      const owner = itemEntryId(item)
      add({ section, item, owner, findings: item.findings, ids: findingEntryIds(item.findings, owner) })
    }
    // Ids come from the whole list, as the section page gives them.
    const ids = findingEntryIds(section.findings)
    for (const global of [false, true]) {
      const picked = section.findings.map((finding, i) => ({ finding, id: ids[i] })).filter(({ finding }) => (finding.layer === 'global') === global)
      add({ section, global: global || undefined, findings: picked.map(({ finding }) => finding), ids: picked.map(({ id }) => id) })
    }
  }
  return { errors, warnings, byPlace }
}

export function healthSummary(count: Pick<FindingCount, 'errors' | 'warnings'>): string {
  const parts = [count.errors ? plural(count.errors, 'error') : '', count.warnings ? plural(count.warnings, 'warning') : ''].filter(Boolean)
  return parts.length ? parts.join(', ') : 'No findings'
}

/** The recheck place for one owner's findings of one severity in the health view. */
export function healthPlace(severity: HealthSeverity, place: Pick<FindingPlace, 'section' | 'owner' | 'global'>): string {
  return `health:${severity}:${place.owner ?? `${place.global ? 'global:' : ''}section:${place.section.id}`}`
}

/** One owner's findings of one severity. */
export interface HealthEntry extends FindingPlace {
  place: string
  /** Where its name links: the item, or the first finding for section findings. */
  entry?: string
}

export interface HealthGroup {
  severity: HealthSeverity
  sections: { section: Section; entries: HealthEntry[] }[]
}

/** A health place a recheck started from, kept on screen once its findings are gone. */
export interface KeptPlace {
  place: string
  before: OverviewView
}

/** Findings grouped by severity, then section, then owner, in section order. */
export function healthGroups(overview: OverviewView, keep?: KeptPlace): HealthGroup[] {
  const places = countFindings(overview).byPlace
  const sections = [...overview.sections].sort((a, b) => a.order - b.order)
  const entriesFor = (severity: HealthSeverity, place: FindingPlace): HealthEntry | undefined => {
    const picked = place.findings.map((finding, i) => ({ finding, id: place.ids[i] })).filter(({ finding }) => healthSeverity(finding) === severity)
    if (!picked.length) return undefined
    const ids = picked.map(({ id }) => id)
    return { ...place, findings: picked.map(({ finding }) => finding), ids, place: healthPlace(severity, place), entry: place.owner ?? ids[0] }
  }
  const groups = HEALTH_SEVERITIES.map((severity) => {
    const entries = places.map((place) => entriesFor(severity, place)).filter((entry): entry is HealthEntry => entry !== undefined)
    return { severity, entries }
  })
  if (keep && !groups.some((group) => group.entries.some((entry) => entry.place === keep.place))) {
    for (const severity of HEALTH_SEVERITIES) {
      const earlier = countFindings(keep.before).byPlace.find((place) => healthPlace(severity, place) === keep.place)
      if (!earlier) continue
      const section = sections.find((s) => s.id === earlier.section.id) ?? earlier.section
      const item = earlier.owner ? section.items.find((it) => itemEntryId(it) === earlier.owner) ?? earlier.item : undefined
      const group = groups.find((g) => g.severity === severity)!
      group.entries.push({ section, item, owner: earlier.owner, global: earlier.global, findings: [], ids: [], place: keep.place, entry: item ? earlier.owner : undefined })
      break
    }
  }
  const order = (section: Section) => sections.findIndex((s) => s.id === section.id)
  return groups
    .filter((group) => group.entries.length)
    .map(({ severity, entries }) => {
      const bySection: HealthGroup['sections'] = []
      for (const entry of [...entries].sort((a, b) => order(a.section) - order(b.section))) {
        const last = bySection[bySection.length - 1]
        if (last && last.section.id === entry.section.id) last.entries.push(entry)
        else bySection.push({ section: entry.section, entries: [entry] })
      }
      return { severity, sections: bySection }
    })
}

export function placeLabel(place: Pick<FindingPlace, 'item' | 'global'>): string {
  if (place.item) return place.item.label || place.item.name
  return place.global ? 'Global setup' : 'Other findings'
}

/** One prompt for every finding, errors first, with the shared guidance once at the end. */
export function healthFixPrompt(overview: OverviewView, root?: string): string {
  const count = countFindings(overview)
  const total = count.errors + count.warnings
  const lines = [total === 1 ? 'Fix this agnostic-ai finding in the project or global configuration:' : `Fix these ${total} agnostic-ai findings in the project or global configuration, errors first:`]
  if (root) lines.push(`Configuration root: ${root}`)
  let n = 0
  for (const severity of HEALTH_SEVERITIES) {
    for (const place of count.byPlace) {
      for (const finding of place.findings) {
        if (healthSeverity(finding) !== severity) continue
        n++
        lines.push('', `${n}. ${place.section.label}: ${placeLabel(place)}`, ...findingFacts(finding))
      }
    }
  }
  lines.push('', 'Work through them in this order.', '', ...FIX_GUIDANCE)
  return lines.join('\n')
}

/** One gap as the list shows it, such as "Skills: supported by claude, codex; no skill yet". */
export function gapLine(gap: Gap): string {
  return `${gap.label}: supported by ${gap.supported.join(', ')}; ${gap.missing}`
}

/** How many improvements the enabled tools support are in place; nothing when they support none. */
export function improvementsLine(improvements: Improvements | undefined): string | undefined {
  if (!improvements || improvements.total === 0) return undefined
  const { applied, total } = improvements
  if (applied === total) return total === 1 ? 'The one improvement is applied' : `All ${total} improvements applied`
  return `${applied} of ${total} ${total === 1 ? 'improvement' : 'improvements'} applied`
}
