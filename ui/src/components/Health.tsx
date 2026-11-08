// Setup health: every finding in the overview on one page, errors first,
// with the same finding rows, fix prompts, and rechecks as the sections.

import { useState } from 'react'
import type { Improvements, OverviewView } from '../contract'
import { recheckAllMessage, type RecheckAllResult } from '../model/recheck'
import { countFindings, featureBars, gapLine, healthFixPrompt, healthGroups, healthSummary, healthVerdict, improvementsLine, placeLabel, type HealthEntry, type HealthSeverity } from '../model/health'
import { HEALTH_STATUS } from '../model/rail'
import { sectionEntryIds } from '../model/identity'
import { joinAnd } from '../model/counts'
import { AppLink, PageHeader, useApp } from './app-context'
import { ErrorPanel, Notice, type NoticeTone } from './Banner'
import { RecheckSection, RecheckSectionEntries, useRechecks } from './Recheck'
import { ConfigurationRoot } from './section-context'
import { Findings } from './Findings'
import { DocLink, SectionHeading } from './SectionHeading'
import type { Remote } from './Shell'
import type { ViewId } from '../model/url'
import { CopyButton, Icon, RowActions, toolName } from './ui'
import { PickBox, PickTray, PicksContext, type Picks } from './pick'

const TITLE = 'Setup health'
const SEVERITY_TITLES: Record<HealthSeverity, string> = { error: 'Fix first', warning: HEALTH_STATUS.warning }
// Phones show this many suggestions until the person asks for the rest.
const GAPS_SHOWN_ON_PHONE = 4

export function HealthPage({ overview, stale = false }: { overview: Remote<OverviewView>; stale?: boolean }) {
  const recheck = useRechecks()
  const { rechecked } = recheck
  const [picked, setPicked] = useState<ReadonlySet<string>>(new Set())
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
  // A pick whose finding a recheck fixed drops out of the count and the prompt.
  const pick = {
    findings: new Set(count.byPlace.flatMap((place) => place.ids).filter((id) => picked.has(id))),
    gaps: new Set(data.health.gaps.map((gap) => gap.id).filter((id) => picked.has(id))),
  }
  const pickedCount = pick.findings.size + pick.gaps.size
  const picks: Picks = {
    has: (id) => picked.has(id),
    set: (id, on) => setPicked((now) => {
      const next = new Set(now)
      if (on) next.add(id)
      else next.delete(id)
      return next
    }),
  }

  return (
    <>
      <PageHeader title={TITLE} verdict={<>{verdict.key && <span className="health-verdict-key">{verdict.key}</span>}{verdict.rest}</>}>
        <HealthFacts errors={count.errors} warnings={count.warnings} gaps={data.health.gaps.length} stale={stale} recheck={recheck.live && (total > 0 || recheck.all !== undefined)} />
        <FeatureTrack improvements={data.health.improvements} />
        {recheck.live && (
          // A failed result is an error Notice, which announces itself as an alert.
          <div className="health-recheck-result" role={recheck.all?.state === 'failed' ? undefined : 'status'}>
            {recheck.pendingAll ? <Notice><p>Running the checks again…</p></Notice> : recheck.all && <Notice tone={recheckAllTone(recheck.all)}><p>{recheckAllMessage(recheck.all)}</p></Notice>}
          </div>
        )}
      </PageHeader>
      <fieldset className="stale-guard" disabled={stale} data-stale={stale || undefined}>
        <ConfigurationRoot.Provider value={data.snapshot.project.root}>
          <PicksContext.Provider value={picks}>
            {groups.map((group, index) => {
              const headingId = `health-${group.severity}`
              const shown = group.sections.reduce((sum, s) => sum + s.entries.reduce((n, entry) => n + entry.findings.length, 0), 0)
              return (
                <section className="block health-group" aria-labelledby={headingId} key={group.severity}>
                  <SectionHeading id={headingId} focusable actions={index === 0 && total > 1 && (
                    <div className="health-actions">
                      <CopyButton text={healthFixPrompt(data, data.snapshot.project.root)} label="Copy all fixes" copiedLabel="Copied" className="button-primary" ariaLabel={total === 1 ? 'Copy the fix prompt for the one finding' : `Copy one fix prompt for all ${total} findings`} />
                    </div>
                  )}>
                    {SEVERITY_TITLES[group.severity]} {shown > 0 && <span className="kind-count">{shown}</span>}
                  </SectionHeading>
                  {group.sections.map(({ section, entries }) => (
                    <RecheckSection.Provider value={section.id} key={section.id}>
                      <RecheckSectionEntries.Provider value={sectionEntryIds(section)}>
                        <div className="health-section">
                          {entries.map((entry) => <HealthPlace key={entry.place} entry={entry} />)}
                        </div>
                      </RecheckSectionEntries.Provider>
                    </RecheckSection.Provider>
                  ))}
                </section>
              )
            })}
            <Gaps gaps={data.health.gaps} sections={data.sections} />
          </PicksContext.Provider>
        </ConfigurationRoot.Provider>
        <PickTray count={pickedCount} prompt={pickedCount ? healthFixPrompt(data, data.snapshot.project.root, pick) : ''} onClear={() => setPicked(new Set())} />
      </fieldset>
    </>
  )
}

function recheckAllTone(result: RecheckAllResult): NoticeTone {
  if (result.state === 'failed') return 'error'
  return result.remaining || result.unconfirmed || result.unknown ? 'warning' : 'success'
}

// The verdict already says when nothing needs a fix, so the facts skip "No findings".
function HealthFacts({ errors, warnings, gaps, stale, recheck }: { errors: number; warnings: number; gaps: number; stale: boolean; recheck: boolean }) {
  const app = useApp()
  const rechecks = useRechecks()
  const tone = errors ? 'is-error' : 'is-warning'
  return (
    <div className="page-facts health-facts">
      {errors + warnings > 0 && (
        <span className={`page-fact page-fact-findings ${tone}`}>
          <Icon name="warning" />
          {healthSummary({ errors, warnings })}
        </span>
      )}
      {gaps > 0 && <span className="page-fact"><span>{gaps} <span className="health-fact-long">{gaps === 1 ? 'feature ' : 'features '}</span>you could add</span></span>}
      <span className="page-fact">
        {stale ? 'Last check failed; showing earlier results' : app.source.kind === 'api' ? 'Checked just now' : 'From a saved snapshot'}
        {recheck && (
          <button type="button" className="link-button health-recheck-all" aria-disabled={rechecks.pending !== undefined || undefined} onClick={() => rechecks.pending === undefined && rechecks.startAll()}>
            {rechecks.pendingAll ? 'Rechecking…' : 'Recheck'}
          </button>
        )}
      </span>
      <span className="page-facts-end"><DocLink topic="findings" /></span>
    </div>
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

// Section findings name their section on the card, so only items and the global setup get a name above.
function HealthPlace({ entry }: { entry: HealthEntry }) {
  const label = placeLabel(entry)
  const kind = entry.item ? entry.section.kinds.find((k) => k.kind === entry.item!.kind)?.label ?? entry.item.kind : undefined
  return (
    <div className="health-place">
      {(entry.item || entry.global) && (
        <p className="health-place-name">
          {entry.entry ? <AppLink to={{ view: entry.section.id as ViewId, entry: entry.entry }}>{label}</AppLink> : <span>{label}</span>}
          {kind && <span className="muted"> · {kind}</span>}
        </p>
      )}
      <Findings findings={entry.findings} ids={entry.ids} owner={entry.owner} place={entry.place} />
    </div>
  )
}

function Gaps({ gaps, sections }: { gaps: OverviewView['health']['gaps']; sections: OverviewView['sections'] }) {
  const [all, setAll] = useState(false)
  if (gaps.length === 0) return null
  const hidden = gaps.length - GAPS_SHOWN_ON_PHONE
  return (
    <section className="block health-gaps" aria-labelledby="health-gaps">
      <SectionHeading id="health-gaps">
        You could add <span className="kind-count">{gaps.length}</span>
      </SectionHeading>
      <p className="health-gaps-intro">Features your tools support that this project does not use yet.</p>
      <ul className={`health-gap-list${all ? ' is-all' : ''}`}>
        {gaps.map((gap) => {
          const section = sections.find((s) => s.id === gap.section)
          return (
            <li key={gap.id} className="health-gap row-hover">
              <PickBox id={gap.id} name={gap.label} className="health-gap-pick" />
              <p className="health-gap-name">{section ? <AppLink to={{ view: section.id as ViewId }} title={`Open ${section.label}`}>{gap.label}</AppLink> : gap.label}</p>
              <p className="health-gap-text">
                {gapLine(gap)}
                <span className="sr-only"> Supported by {joinAnd(gap.supported.map(toolName))}.</span>
              </p>
              <RowActions label={gap.label}>
                <CopyButton text={gap.fix} label="Copy prompt" copiedLabel="Copied" className="health-gap-copy" ariaLabel={`Copy prompt for ${gap.label}`} />
              </RowActions>
            </li>
          )
        })}
      </ul>
      {hidden > 0 && !all && <button type="button" className="button health-gaps-more" onClick={() => setAll(true)}>Show {hidden} more</button>}
    </section>
  )
}
