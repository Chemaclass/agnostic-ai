// Setup health: every finding in the overview on one page, errors first,
// with the same finding rows, fix prompts, and rechecks as the sections.

import type { Gap, OverviewView } from '../contract'
import { countFindings, gapLine, healthFixPrompt, healthGroups, healthSummary, placeLabel, type HealthEntry, type HealthSeverity } from '../model/health'
import { sectionEntryIds } from '../model/identity'
import { AppLink, PageHeader } from './app-context'
import { ErrorPanel } from './Banner'
import { RecheckSection, RecheckSectionEntries, useRechecks } from './Recheck'
import { ConfigurationRoot, Findings } from './Section'
import { SectionHeading } from './SectionHeading'
import type { Remote } from './Shell'
import type { ViewId } from '../model/url'
import { CopyButton, Icon } from './ui'

const TITLE = 'Setup health'
const SEVERITY_TITLES: Record<HealthSeverity, string> = { error: 'Errors', warning: 'Warnings' }

export function HealthPage({ overview, stale = false }: { overview: Remote<OverviewView>; stale?: boolean }) {
  const { rechecked } = useRechecks()
  const data = overview.loaded?.data
  if (!data) {
    return (
      <>
        <PageHeader title={TITLE} />
        {overview.error ? <ErrorPanel error={overview.error} /> : <p className="loading" aria-busy="true">Loading the overview…</p>}
      </>
    )
  }
  const count = countFindings(data)
  const total = count.errors + count.warnings
  const keep = rechecked?.place.startsWith('health:') ? { place: rechecked.place, before: rechecked.before } : undefined
  const groups = healthGroups(data, keep)
  const tone = count.errors ? 'is-error' : count.warnings ? 'is-warning' : 'is-clean'

  return (
    <>
      <PageHeader title={TITLE} subtitle="Every lint and doctor finding across the sections, errors first.">
        <div className="health-summary">
          <p className={`health-total ${tone}`}>
            <Icon name={total ? 'warning' : 'health'} />
            <span>{healthSummary(count)}</span>
          </p>
          {total > 0 && <CopyButton text={healthFixPrompt(data, data.snapshot.project.root)} label="Copy all fixes" copiedLabel="Copied" ariaLabel={total === 1 ? 'Copy the fix prompt for the one finding' : `Copy one fix prompt for all ${total} findings`} />}
        </div>
      </PageHeader>
      <fieldset className="stale-guard" disabled={stale} data-stale={stale || undefined}>
        <ConfigurationRoot.Provider value={data.snapshot.project.root}>
          {groups.length === 0 && <p className="muted">Lint and doctor found nothing to fix in this configuration.</p>}
          {groups.map((group) => {
            const headingId = `health-${group.severity}`
            const shown = group.sections.reduce((sum, s) => sum + s.entries.reduce((n, entry) => n + entry.findings.length, 0), 0)
            return (
              <section className="block health-group" aria-labelledby={headingId} key={group.severity}>
                <SectionHeading topic="findings" id={headingId} focusable>
                  {SEVERITY_TITLES[group.severity]} <span className="muted kind-count">{shown}</span>
                </SectionHeading>
                {group.sections.map(({ section, entries }) => (
                  <RecheckSection.Provider value={section.id} key={section.id}>
                    <RecheckSectionEntries.Provider value={sectionEntryIds(section)}>
                      <div className="health-section">
                        <h3 className="health-section-title">{section.label}</h3>
                        {entries.map((entry) => <HealthPlace key={entry.place} entry={entry} />)}
                      </div>
                    </RecheckSectionEntries.Provider>
                  </RecheckSection.Provider>
                ))}
              </section>
            )
          })}
          {data.health.gaps.length > 0 && <Gaps gaps={data.health.gaps} sections={data.sections} />}
        </ConfigurationRoot.Provider>
      </fieldset>
    </>
  )
}

function HealthPlace({ entry }: { entry: HealthEntry }) {
  const label = placeLabel(entry)
  const kind = entry.item ? entry.section.kinds.find((k) => k.kind === entry.item!.kind)?.label ?? entry.item.kind : undefined
  return (
    <div className="health-place">
      <p className="health-place-name">
        {entry.entry ? <AppLink to={{ view: entry.section.id as ViewId, entry: entry.entry }}>{label}</AppLink> : <span>{label}</span>}
        {kind && <span className="muted"> · {kind}</span>}
      </p>
      <Findings findings={entry.findings} ids={entry.ids} owner={entry.owner} place={entry.place} />
    </div>
  )
}

function Gaps({ gaps, sections }: { gaps: Gap[]; sections: OverviewView['sections'] }) {
  return (
    <section className="block health-gaps" aria-labelledby="health-gaps">
      <SectionHeading topic="support" id="health-gaps">
        Reach the best setup <span className="muted kind-count">{gaps.length}</span>
      </SectionHeading>
      <p className="muted">What the enabled tools support and this project does not use yet. These are suggestions, not findings.</p>
      <ul className="health-gap-list">
        {gaps.map((gap) => {
          const section = sections.find((s) => s.id === gap.section)
          return (
            <li key={gap.id} className="health-gap">
              <span>
                {gapLine(gap)}
                {section && <span className="muted"> · <AppLink to={{ view: section.id as ViewId }}>{section.label}</AppLink></span>}
              </span>
              <CopyButton text={gap.fix} label="Copy fix prompt" copiedLabel="Copied" ariaLabel={`Copy the fix prompt for ${gap.label}`} />
            </li>
          )
        })}
      </ul>
    </section>
  )
}
