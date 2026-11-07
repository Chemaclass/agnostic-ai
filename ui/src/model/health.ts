// Setup health: every finding in the overview, counted by severity and
// grouped for one view. Findings keep the link ids the section pages
// give them, so links and rechecks agree between the two places.

import type { Gap, Improvements, ItemFinding, OverviewView, Section, SectionItem } from '../contract'
import { FIX_GUIDANCE, findingFacts } from './finding-prompt'
import { joinAnd } from './counts'
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

/** The findings and suggestions picked on Setup health, by finding entry id and gap id. */
export interface HealthPick {
  findings: ReadonlySet<string>
  gaps: ReadonlySet<string>
}

/** One prompt for every finding, or for the picked findings and suggestions: errors first, then suggestions, with the shared guidance once at the end. */
export function healthFixPrompt(overview: OverviewView, root?: string, pick?: HealthPick): string {
  const places = countFindings(overview).byPlace
  const findings: { place: FindingPlace; finding: ItemFinding }[] = []
  for (const severity of HEALTH_SEVERITIES) {
    for (const place of places) {
      place.findings.forEach((finding, i) => {
        if (healthSeverity(finding) === severity && (!pick || pick.findings.has(place.ids[i]))) findings.push({ place, finding })
      })
    }
  }
  const gaps = pick ? overview.health.gaps.filter((gap) => pick.gaps.has(gap.id)) : []
  const total = findings.length + gaps.length
  const lines = [gaps.length
    ? `Work through ${total === 1 ? 'this agnostic-ai item' : `these ${total} agnostic-ai items`} in the project or global configuration${findings.length ? ', errors first, then suggestions' : ''}:`
    : total === 1 ? 'Fix this agnostic-ai finding in the project or global configuration:' : `Fix these ${total} agnostic-ai findings in the project or global configuration, errors first:`]
  if (root) lines.push(`Configuration root: ${root}`)
  let n = 0
  for (const { place, finding } of findings) lines.push('', `${++n}. ${place.section.label}: ${placeLabel(place)}`, ...findingFacts(finding))
  for (const gap of gaps) {
    const section = overview.sections.find((s) => s.id === gap.section)
    lines.push('', `${++n}. ${section ? `${section.label}: ` : ''}${gap.label} (suggestion)`, gap.fix)
  }
  lines.push('', 'Work through them in this order.')
  if (findings.length) lines.push('', ...FIX_GUIDANCE)
  return lines.join('\n')
}

const GAP_TOOLS_SHOWN = 3

/** The tools that support a gap, by name and cut to the first few, such as "Claude Code, Codex, Cursor, and 9 more". */
export function gapTools(gap: Gap, name: (target: string) => string = (target) => target): string {
  const names = gap.supported.map(name)
  if (names.length <= GAP_TOOLS_SHOWN + 1) return joinAnd(names)
  return joinAnd([...names.slice(0, GAP_TOOLS_SHOWN), `${names.length - GAP_TOOLS_SHOWN} more`])
}

/** How many features the enabled tools support are in use; nothing when they support none. */
export function improvementsLine(improvements: Improvements | undefined): string | undefined {
  if (!improvements || improvements.total === 0) return undefined
  const { applied, total } = improvements
  if (applied === total) return total === 1 ? 'The one feature your tools support is in use' : `All ${total} features your tools support are in use`
  return `${applied} of ${plural(total, 'feature')} your tools support ${total === 1 ? 'is' : 'are'} in use`
}

const TRACK_BARS_MAX = 40

/** One flag per feature bar, true for those in use; nothing when the counts are too large or do not add up, so the line alone says them. */
export function featureBars(improvements: Improvements | undefined): boolean[] | undefined {
  if (!improvements) return undefined
  const { applied, total } = improvements
  if (total < 1 || total > TRACK_BARS_MAX || applied < 0 || applied > total) return undefined
  return Array.from({ length: total }, (_, i) => i < applied)
}

/** The page's answer: what needs a fix and what is worth a look, with the leading count as its key phrase. */
export function healthVerdict({ errors, warnings }: Pick<FindingCount, 'errors' | 'warnings'>): { key?: string; rest: string } {
  const things = (n: number) => plural(n, 'thing')
  if (errors && warnings) return { key: things(errors), rest: ` ${errors === 1 ? 'needs' : 'need'} a fix, and ${warnings} more ${warnings === 1 ? 'is' : 'are'} worth a look.` }
  if (errors) return { key: things(errors), rest: ` ${errors === 1 ? 'needs' : 'need'} a fix.` }
  if (warnings) return { key: things(warnings), rest: ` ${warnings === 1 ? 'is' : 'are'} worth a look.` }
  return { rest: 'Nothing needs a fix.' }
}
