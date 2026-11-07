// Setup health: every finding in the overview on one page, errors first,
// with the same finding rows, fix prompts, and rechecks as the sections.

import type { Improvements, OverviewView } from '../contract'
import { recheckAllMessage, type RecheckAllResult } from '../model/recheck'
import { countFindings, featureBars, gapTools, healthFixPrompt, healthGroups, healthSummary, healthVerdict, improvementsLine, placeLabel, type HealthEntry, type HealthSeverity } from '../model/health'
import { HEALTH_STATUS } from '../model/rail'
import { sectionEntryIds } from '../model/identity'
import { plural } from '../model/format'
import { AppLink, PageHeader, useApp } from './app-context'
import { ErrorPanel, Notice, type NoticeTone } from './Banner'
import { RecheckSection, RecheckSectionEntries, useRechecks } from './Recheck'
import { ConfigurationRoot } from './section-context'
import { Findings } from './Findings'
import { SectionHeading } from './SectionHeading'
import type { Remote } from './Shell'
import type { ViewId } from '../model/url'
import { CopyButton, Icon, toolName } from './ui'

const TITLE = 'Setup health'
const SEVERITY_TITLES: Record<HealthSeverity, string> = { error: HEALTH_STATUS.error, warning: HEALTH_STATUS.warning }

export function HealthPage({ overview, stale = false }: { overview: Remote<OverviewView>; stale?: boolean }) {
  const recheck = useRechecks()
  const { rechecked } = recheck
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
  const verdict = healthVerdict(count)

  return (
    <>
      <PageHeader title={TITLE} verdict={<>{verdict.key && <span className="health-verdict-key">{verdict.key}</span>}{verdict.rest}</>}>
        <HealthFacts errors={count.errors} warnings={count.warnings} gaps={data.health.gaps.length} />
        {(total > 0 || recheck.live) && (
          <div className="health-actions">
            {total > 0 && <CopyButton text={healthFixPrompt(data, data.snapshot.project.root)} label="Copy all fixes" copiedLabel="Copied" className="button-primary" ariaLabel={total === 1 ? 'Copy the fix prompt for the one finding' : `Copy one fix prompt for all ${total} findings`} />}
            {recheck.live && (total > 0 || recheck.all) && (
              <button type="button" className="button health-recheck-all" aria-disabled={recheck.pending !== undefined || undefined} onClick={() => recheck.pending === undefined && recheck.startAll()}>
                {recheck.pendingAll ? 'Rechecking…' : 'Recheck all'}
              </button>
            )}
          </div>
        )}
        {recheck.live && (
          <div className="health-recheck-result" role="status">
            {recheck.pendingAll ? <Notice><p>Running the checks again…</p></Notice> : recheck.all && <Notice tone={recheckAllTone(recheck.all)}><p>{recheckAllMessage(recheck.all)}</p></Notice>}
          </div>
        )}
        <FeatureTrack improvements={data.health.improvements} />
      </PageHeader>
      <fieldset className="stale-guard" disabled={stale} data-stale={stale || undefined}>
        <ConfigurationRoot.Provider value={data.snapshot.project.root}>
          {groups.map((group) => {
            const headingId = `health-${group.severity}`
            const shown = group.sections.reduce((sum, s) => sum + s.entries.reduce((n, entry) => n + entry.findings.length, 0), 0)
            return (
              <section className="block health-group" aria-labelledby={headingId} key={group.severity}>
                <SectionHeading topic="findings" id={headingId} focusable>
                  {SEVERITY_TITLES[group.severity]} {shown > 0 && <span className="kind-count">{shown}</span>}
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
          <Gaps gaps={data.health.gaps} sections={data.sections} />
        </ConfigurationRoot.Provider>
      </fieldset>
    </>
  )
}

function recheckAllTone(result: RecheckAllResult): NoticeTone {
  if (result.state === 'failed') return 'error'
  return result.remaining || result.unconfirmed || result.unknown ? 'warning' : 'success'
}

function HealthFacts({ errors, warnings, gaps }: { errors: number; warnings: number; gaps: number }) {
  const app = useApp()
  const tone = errors ? 'is-error' : warnings ? 'is-warning' : 'is-clean'
  return (
    <p className="page-facts">
      <span className={`page-fact page-fact-findings ${tone}`}>
        {errors + warnings > 0 && <Icon name="warning" />}
        {healthSummary({ errors, warnings })}
      </span>
      {gaps > 0 && <span className="page-fact">{plural(gaps, 'feature')} you could add</span>}
      <span className="page-fact">{app.source.kind === 'api' ? 'Checked just now' : 'From a saved snapshot'}</span>
    </p>
  )
}

/** One bar per feature the configured tools support, filled for those in use. */
function FeatureTrack({ improvements }: { improvements?: Improvements }) {
  const line = improvementsLine(improvements)
  const bars = featureBars(improvements)
  if (!line) return null
  return (
    <div className="health-track">
      {bars && (
        <span className="health-track-bars" aria-hidden="true">
          {bars.map((on, i) => <span key={i} className={`health-track-bar${on ? ' is-on' : ''}`} />)}
        </span>
      )}
      <p className="health-track-text">{line}</p>
    </div>
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
      <Findings findings={entry.findings} ids={entry.ids} owner={entry.owner} place={entry.place} sectionNamed />
    </div>
  )
}

function Gaps({ gaps, sections }: { gaps: OverviewView['health']['gaps']; sections: OverviewView['sections'] }) {
  if (gaps.length === 0) return null
  return (
    <section className="block health-gaps" aria-labelledby="health-gaps">
      <SectionHeading topic="support" id="health-gaps">
        Reach the best setup <span className="kind-count">{gaps.length}</span>
      </SectionHeading>
      <p className="health-gaps-intro">Features your tools support that this project does not use yet. Suggestions, not findings.</p>
      <ul className="health-gap-list">
        {gaps.map((gap) => {
          const section = sections.find((s) => s.id === gap.section)
          const every = gap.supported.map(toolName).join(', ')
          return (
            <li key={gap.id} className="health-gap">
              <p className="health-gap-name">{gap.label}</p>
              <p className="health-gap-text">
                {gap.missing.charAt(0).toUpperCase() + gap.missing.slice(1)}.{' '}
                <span title={every} aria-hidden="true">Supported by {gapTools(gap, toolName)}.</span>
                <span className="sr-only">Supported by {every}.</span>
                {section && <>{' '}<AppLink to={{ view: section.id as ViewId }}>{section.label}</AppLink></>}
              </p>
              <CopyButton text={gap.fix} label="Copy prompt" copiedLabel="Copied" className="health-gap-copy" ariaLabel={`Copy prompt for ${gap.label}`} />
            </li>
          )
        })}
      </ul>
    </section>
  )
}
