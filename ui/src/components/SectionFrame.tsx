import { createContext, useCallback, useContext, useEffect, useId, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { API, type OverviewView, type Section } from '../contract'
import { postEdit } from '../api'
import { PageHeader, useApp } from './app-context'
import { ErrorPanel } from './Banner'
import { CodingTools } from './CodingTools'
import { EditorSettings } from './EditorSettings'
import { ContextUsage } from './ContextUsage'
import { BuiltinChangeControls, BuiltinChangeProvider, ChangeStatus } from './BuiltinChange'
import { EditAudienceProvider, EditAudienceSwitch, ViewPersonalButton, useLevelName, useViewAudience } from './EditAudience'
import { MCPSpecNotice, type MCPSpecSaved } from './MCPSpecAction'
import { SpecMovedNotice, SpecMovedSaver, useSpecMovedNotice } from './SpecMoveAction'
import { PermissionEditProvider, PermissionSaveNotice } from './PermissionEditor'
import { SectionTabs } from './SectionTabs'
import { DocLink, SectionHeading } from './SectionHeading'
import { RecheckSection, RecheckSectionEntries, useRechecks } from './Recheck'
import { builtinEnabled, sourcesForAudience } from '../model/audience-view'
import { fieldEntryId, findingEntryIds, itemEntryId, kindEntryId, sectionEntryIds } from '../model/identity'
import { fieldSearchText, itemSearchText, matchesSearch } from '../model/search'
import { filterTools, toolCount } from '../model/coding-tools'
import { docsTopic, entryTab, landingTab, nativeSearchText, sectionTabs, tabTitle } from '../model/section-tabs'
import { contextLanding } from '../model/url'
import type { Remote } from './Shell'
import { Badge, Code, Icon } from './ui'
import { EmptyState } from './EmptyState'
import { findingTallyText, sectionFacts, sectionFindings, sectionVerdict, tallyFindings } from '../model/verdict'
import { ConfigurationRoot, ConfiguredTools, PersonalBuiltins } from './section-context'
import { SettingsFields } from './Fields'
import { AuthorityFields, AuthorityToolbarAction } from './Authority'
import { ConfigurationFiles, GroupedItems, sharedUnlistedCommand } from './Items'
import { NativeServers } from './NativeServers'
import { Lifecycle } from './Lifecycle'
import { Findings } from './Findings'
import { SupportLink } from './SupportTable'
import { ResetAction } from './EditSwitch'

export function SectionPage({ sectionId, overview, stale = false }: { sectionId: string; overview: Remote<OverviewView>; stale?: boolean }) {
  const data = overview.loaded?.data
  const [docs, setDocs] = useState<{ section: string; topic: string }>()
  const reportDocs = useCallback((section: string, topic: string) => setDocs((shown) => (shown?.section === section && shown.topic === topic ? shown : { section, topic })), [])
  if (!data) {
    return (
      <>
        <PageHeader title={sectionId.charAt(0).toUpperCase() + sectionId.slice(1)} />
        {overview.error ? <ErrorPanel error={overview.error} /> : <p className="loading" aria-busy="true">Loading the overview…</p>}
      </>
    )
  }
  const section = data.sections.find((s) => s.id === sectionId)
  if (!section) {
    return (
      <>
        <PageHeader title="Section not found" />
        <p>The overview has no section named {sectionId}.</p>
      </>
    )
  }
  const toolLabel = (target: string) => data.targets.find((t) => t.target === target)?.label ?? target

  const topic = docs?.section === section.id ? docs.topic : docsTopic(section, undefined)

  return (
    <DocsTopic.Provider value={reportDocs}>
      <PageHeader title={section.label} verdict={sectionVerdict(section)}>
        <SectionFacts key={section.id} section={section} docs={topic} support={<SupportLink key={section.id} section={section} toolLabel={toolLabel} />} />
      </PageHeader>
      <fieldset className="stale-guard" disabled={stale} data-stale={stale || undefined}>
        <ConfigurationRoot.Provider value={data.snapshot.project.root}>
          <ConfiguredTools.Provider value={data.targets}>
            <EditAudienceProvider key={`${data.snapshot.project.root}:${section.id}`} enabled={section.id === 'settings' || section.id === 'tools' || section.id === 'authority'}>
              <SectionContents section={section} />
            </EditAudienceProvider>
          </ConfiguredTools.Provider>
        </ConfigurationRoot.Provider>
      </fieldset>
    </DocsTopic.Provider>
  )
}

/** Opens the folded findings list that holds this heading, then moves focus to it. */
export function revealFindings(findingsId: string) {
  const heading = document.getElementById(findingsId)
  if (!heading) return
  const disclosure = heading.closest('details')
  if (disclosure) disclosure.open = true
  heading.scrollIntoView({ block: 'start' })
  heading.focus({ preventScroll: true })
}

// The Docs link in the facts line follows the open tab, which the page body works out.
const DocsTopic = createContext<(section: string, topic: string) => void>(() => {})

function SectionFacts({ section, docs, support }: { section: Section; docs: string; support: ReactNode }) {
  const app = useApp()
  const tally = tallyFindings(sectionFindings(section))
  const findings = findingTallyText(tally)
  const total = tally.errors + tally.warnings
  const facts = sectionFacts(section)
  const tone = tally.errors ? 'is-error' : 'is-warning'
  const shownAbove = section.findings.some((finding) => finding.severity === 'error')
  // Review leads to the first finding: the section's own list, else the first row that has one.
  const review = () => {
    if (section.findings.length) return revealFindings(`${section.id}-findings`)
    const item = section.items.find((entry) => entry.findings.length)
    if (item) app.navigate({ view: section.id, tab: undefined, q: undefined, entry: findingEntryIds(item.findings, itemEntryId(item))[0] })
  }
  return (
      <div className="page-facts">
        {facts.map((fact) => <span key={fact} className="page-fact">{fact}</span>)}
        {findings && (
          <span className={`page-fact page-fact-findings ${tone}`} role={shownAbove ? 'alert' : undefined}>
            <Icon name="warning" />
            {findings}
          </span>
        )}
        {total > 0 && <button type="button" className="button page-facts-review" onClick={review}>Review {total} finding{total === 1 ? '' : 's'}</button>}
        {support}
        <span className="page-facts-end"><DocLink topic={docs} /></span>
      </div>
  )
}

// Built-ins and model or effort fields share one preview, apply, and undo flow.
function hasChanges(section: Section) {
  return section.items.some((item) => item.kind === 'builtin') || (section.fields ?? []).some((field) => (field.key === 'model' || field.key === 'effort') && field.editable && !field.editable.disabledReason)
}

export function SectionContents({ section }: { section: Section }) {
  const changes = hasChanges(section)
  const personalBuiltins = section.kinds.some((kind) => kind.kind === 'builtin' && kind.local) || section.items.some((item) => item.kind === 'builtin' && item.local)
  return <PersonalBuiltins.Provider value={personalBuiltins}><BuiltinChangeProvider active={changes}><PermissionEditProvider><SectionContentsBody section={section} changes={changes} /></PermissionEditProvider></BuiltinChangeProvider></PersonalBuiltins.Provider>
}

function SectionContentsBody({ section, changes }: { section: Section; changes: boolean }) {
  const app = useApp()
  const audience = useViewAudience()
  const [warningsExpanded, setWarningsExpanded] = useState(false)
  const personalBuiltins = useContext(PersonalBuiltins)
  const level = useLevelName()
  const root = useContext(ConfigurationRoot)
  const noticeKey = `agnostic-ai.mcp-spec-saved:${root ?? ''}`
  const [mcpSaved, setMCPSaved] = useState<MCPSpecSaved | undefined>(() => {
    try {
      const value: unknown = JSON.parse(sessionStorage.getItem(noticeKey) ?? 'null')
      if (value && typeof value === 'object' && 'file' in value && typeof value.file === 'string' && 'syncCommand' in value && typeof value.syncCommand === 'string') return { file: value.file, syncCommand: value.syncCommand }
    } catch { /* A saved notice is optional. */ }
    return undefined
  })
  const saveMCPNotice = (saved?: MCPSpecSaved) => {
    setMCPSaved(saved)
    try {
      if (saved) sessionStorage.setItem(noticeKey, JSON.stringify(saved))
      else sessionStorage.removeItem(noticeKey)
    } catch { /* Keep the notice until this page reloads. */ }
    if (saved) app.reload(true)
  }
  const [specMoved, saveSpecMoved] = useSpecMovedNotice(root)
  const search = app.state.q ?? ''
  const entry = app.state.entry
  const { rechecked, pendingPlace } = useRechecks()
  const sectionPlace = `section:${section.id}`
  const remainingHere = rechecked?.result.state === 'remaining' && sectionEntryIds(section).has(rechecked.result.id ?? rechecked.id)
  const orphan = rechecked?.section === section.id && !rechecked.place.startsWith('health:') && rechecked.place !== sectionPlace && !remainingHere && !section.items.some((item) => itemEntryId(item) === rechecked.place) ? rechecked.place : undefined
  const searchInput = useRef<HTMLInputElement>(null)
  const searchId = useId()
  const query = search.trim().toLowerCase()
  const matches = (parts: (string | undefined)[]) => matchesSearch(parts, query)
  const matchingItems = new Set(section.items.filter((item) => matches(itemSearchText(item))))
  const fieldMatches = section.fields?.filter((field) => matches(fieldSearchText(field)))
  const matchingFields = section.fields?.filter((field) => fieldMatches?.includes(field) || fieldEntryId(field) === entry)
  const visibleItems = section.items.filter((item) => matchingItems.has(item) || item.findings.length > 0 || itemEntryId(item) === entry || itemEntryId(item) === pendingPlace || itemEntryId(item) === rechecked?.place)
  const tabs = sectionTabs(section).filter((tab) => tab.extra !== 'editor' || app.source.kind === 'api')
  const nativeMatches = section.native?.filter((server) => matches(nativeSearchText(server))) ?? []
  const toolMatches = query ? filterTools(app.overview?.tools.items ?? [], query, false, audience ?? 'shared').length : 0
  const hasMatches = (tab: typeof tabs[number]) => tab.kinds.some((kind) => section.items.some((item) => item.kind === kind && matchingItems.has(item)) || kind === 'settings' && (fieldMatches?.length ?? 0) > 0) || tab.extra === 'native' && nativeMatches.length > 0 || tab.extra === 'targets' && toolMatches > 0
  const active = entryTab(section, tabs, entry)
    ?? tabs.find((tab) => tab.id === app.state.tab)?.id
    ?? (query ? tabs.find(hasMatches)?.id : undefined)
    ?? landingTab(section, tabs, contextLanding(app.source, app.overview))
  const setSearch = (value: string) => app.navigate({ q: value || undefined, tab: value.trim() ? undefined : active }, { replace: true })
  const selectTab = (tab: string) => app.navigate({ tab, entry: undefined, q: app.state.q })
  const activePanel = tabs.find((tab) => tab.id === active)
  // One selector serves every block on the page; Config files and Editor show the combined setup.
  const scoped = (section.id === 'settings' || section.id === 'authority') && (activePanel ? activePanel.extra === 'targets' || activePanel.kinds.some((kind) => kind === 'settings' || kind === 'builtin') : section.kinds.some((k) => k.kind === 'settings' || k.kind === 'builtin'))
  const panels = tabs.length > 0 ? tabs : [{ id: 'all', label: section.label, kinds: section.kinds.map((kind) => kind.kind), extra: undefined }]
  const tabLabels = tabs.map((tab) => {
    const items = section.items.filter((item) => tab.kinds.includes(item.kind))
    const fields = tab.kinds.includes('settings') ? section.fields ?? [] : []
    let count: string | number | undefined
    if (tab.extra === 'usage' || tab.extra === 'editor') count = undefined
    else if (query) {
      const matched = tab.extra === 'native' ? nativeMatches.length : tab.extra === 'targets' ? toolMatches : items.filter((item) => matchingItems.has(item)).length + fields.filter((field) => fieldMatches?.includes(field)).length
      count = `${matched} match${matched === 1 ? '' : 'es'}`
    }
    else if (tab.extra === 'targets') { const c = toolCount(app.overview?.tools.items ?? [], audience ?? 'shared'); count = `${c.on} of ${c.total} on` }
    else if (tab.extra === 'native') count = section.native?.length ?? 0
    else if (tab.kinds.includes('builtin')) { const on = items.filter((item) => builtinEnabled(item, audience)).length; count = on ? `${on} on` : undefined }
    else if (tab.kinds.includes('settings')) { const set = fields.filter((field) => sourcesForAudience(field.set, audience).length > 0).length; count = set ? `${set} set` : undefined }
    else count = items.length
    if (count === 0) count = undefined
    return { id: tab.id, label: tab.label, title: tabTitle(section, tab), count, findings: items.reduce((sum, item) => sum + item.findings.length, 0) }
  })
  const setDocsTopic = useContext(DocsTopic)
  const topic = docsTopic(section, activePanel)
  useLayoutEffect(() => setDocsTopic(section.id, topic), [setDocsTopic, section.id, topic])
  const entryMissing = entry !== undefined && !sectionEntryIds(section).has(entry)
  useEffect(() => {
    if (!entry) return
    const target = document.querySelector<HTMLElement>(`[data-entry="${CSS.escape(entry)}"]`)
    if (!target) return
    const details = target.closest('details')
    if (details) details.open = true
    target.scrollIntoView({ block: 'center' })
    target.focus({ preventScroll: true })
  }, [entry, section.id, app.reveals])
  const matchCount = matchingItems.size + (fieldMatches?.length ?? 0) + nativeMatches.length + toolMatches
  const retainedFindings = section.items.some((item) => item.findings.length > 0 && !matchingItems.has(item))
  const errors = section.findings.filter((finding) => finding.severity === 'error').length
  const warnings = section.findings.length - errors
  const findingsId = `${section.id}-findings`
  const otherFindings = (section.findings.length > 0 || rechecked?.place === sectionPlace || orphan !== undefined) && (
    <section className="block" aria-labelledby={findingsId}>
      <SectionHeading id={findingsId} focusable>Other findings</SectionHeading>
      <Findings findings={section.findings} place={sectionPlace} orphanPlace={orphan} />
    </section>
  )
  const warningDetails = otherFindings && (
    <details className="section-warnings" open={warningsExpanded || rechecked?.place === sectionPlace || orphan !== undefined || undefined} onToggle={(event) => setWarningsExpanded(event.currentTarget.open)}>
      <summary>{warnings ? `${warnings} warning${warnings === 1 ? '' : 's'}` : 'Check results'}</summary>
      {otherFindings}
    </details>
  )
  return (
    <RecheckSection.Provider value={section.id}>
      <RecheckSectionEntries.Provider value={sectionEntryIds(section)}>
      <SpecMovedSaver.Provider value={saveSpecMoved}>
      {app.source.kind === 'api' && specMoved && <SpecMovedNotice moved={specMoved} dismiss={() => saveSpecMoved()} />}
      {entryMissing && (
        <div className="entry-missing" role="status">
          <p>The linked {entry.startsWith('finding:') ? 'finding' : entry.startsWith('field:') ? 'setting' : 'item'} is not in this {section.label.toLowerCase()} section. It may have been removed, renamed, or fixed, or the link may come from another configuration.</p>
          <button type="button" className="button button-quiet" onClick={() => app.navigate({ entry: undefined }, { replace: true })}>Dismiss</button>
        </div>
      )}
      {errors > 0 && otherFindings}
      <div className="section-search" role="search" aria-label={`Search ${section.label}`}>
        <label htmlFor={searchId} className="sr-only">Search {section.label.toLowerCase()}</label>
        <div className="section-search-input">
          <Icon name="search" />
          <input ref={searchInput} id={searchId} type="search" value={search} onChange={(event) => setSearch(event.target.value)} placeholder={`Filter ${section.label.toLowerCase()}`} aria-describedby={`${searchId}-status`} />
        </div>
        {search && <button type="button" className="button" onClick={() => {
          setSearch('')
          searchInput.current?.focus()
        }}>Clear search</button>}
        <p id={`${searchId}-status`} className="section-search-status" role="status">
          {query ? (matchCount ? `${matchCount} ${matchCount === 1 ? 'match' : 'matches'}.` : 'No matches.') : ''}
          {query && retainedFindings && ' Items with findings stay visible.'}
        </p>
        {scoped && <EditAudienceSwitch />}
        {section.id === 'authority' && section.fields && <AuthorityToolbarAction fields={section.fields} />}
      </div>
      {tabs.length > 1 && <SectionTabs section={section.id} label={section.label} active={active!} select={selectTab} tabs={tabLabels} />}
      {section.id === 'authority' && <PermissionSaveNotice />}
      {panels.map((panel) => <div
        key={panel.id}
        className="section-panel"
        id={`panel-${section.id}-${panel.id}`}
        role={tabs.length > 1 ? 'tabpanel' : undefined}
        aria-labelledby={tabs.length > 1 ? `tab-${section.id}-${panel.id}` : undefined}
        tabIndex={tabs.length > 1 ? 0 : undefined}
        hidden={tabs.length > 1 && panel.id !== active}
      >
      {section.kinds.filter((kind) => panel.kinds.includes(kind.kind)).map((k) => {
        const items = visibleItems.filter((it) => it.kind === k.kind)
        if (k.kind === 'hook' && section.stages) return <Lifecycle key={`${k.kind}:${query}:${entry ?? ''}`} stages={section.stages} hooks={items} count={k.count} filtering={Boolean(query)} matchingHooks={matchingItems} />
        if (k.kind === 'settings' && section.id === 'authority' && section.fields) return <AuthorityFields key={k.kind} title={k.label} headingEntry={kindEntryId(k.kind)} fields={section.fields} files={items} query={query} />
        if (k.kind === 'settings' && matchingFields) return <SettingsFields key={k.kind} headingEntry={kindEntryId(k.kind)} title={k.label} fields={matchingFields} files={items} filtering={Boolean(query)} support={section.support.find((entry) => entry.kind === k.kind)} status={changes && <ChangeStatus labels={section.items} settings builtins={section.kinds.some((other) => other.kind === 'builtin')} />} />
        const titleId = `${section.id}-${k.kind}`
        const unlisted = sharedUnlistedCommand(section.items.filter((it) => it.kind === k.kind))
        const sharedBlocked = k.kind === 'builtin' && audience === 'shared' && personalBuiltins
        const count = k.kind === 'builtin' ? section.items.filter((item) => item.kind === 'builtin' && builtinEnabled(item, audience)).length : k.count
        return (
          <section className="block" aria-labelledby={titleId} key={k.kind}>
            <SectionHeading id={titleId} focusable entry={kindEntryId(k.kind)}>
              {k.label}{tabs.length < 2 && count > 0 && <span className="muted kind-count">{count}{k.kind === 'builtin' ? ' enabled' : ''}</span>}
              {k.local && audience !== 'shared' && <Badge tone="accent" title={`Your local config replaces the ${level} list`}>local</Badge>}
            </SectionHeading>
            {k.kind === 'builtin' && <ChangeStatus labels={section.items} settings={false} builtins />}
            {sharedBlocked && <div className="audience-notice"><p>These are the {level} defaults. Your local built-in list replaces them for you. Reset that list before changing {level} built-ins.</p><ViewPersonalButton /></div>}
            {k.kind === 'builtin' && audience === 'personal' && !personalBuiltins && <p className="muted">Inherited from {level}. Changing a built-in creates your local list.</p>}
            {k.kind === 'builtin' && !sharedBlocked && <BuiltinChangeControls local={audience === 'personal' && personalBuiltins} resetSource={<ResetAction label={`Reset built-ins to the ${level} list`} reset={() => postEdit(API.editBuiltin, { audience: 'personal', reset: true, expected: section.items.filter((item) => item.kind === 'builtin' && item.enabled).map((item) => item.name) })} />} />}
            {unlisted !== undefined && items.length > 0 && (
              <p className="muted tool-state tools-unlisted">
                Tools not listed yet. Run <Code>{unlisted}</Code> to list them.
              </p>
            )}
            {items.length === 0 ? <EmptyState kind={k} support={section.support.find((entry) => entry.kind === k.kind)} filtering={Boolean(query)} /> : k.kind === 'config' ? <ConfigurationFiles items={items} /> : <GroupedItems items={items} sharedUnlisted={unlisted !== undefined} />}
          </section>
        )
      })}
      {panel.extra === 'native' && (section.native?.length
        ? <NativeServers servers={section.native} matches={matches} filtering={Boolean(query)} onSaved={saveMCPNotice} />
        : <section className="block"><SectionHeading>Found in tools</SectionHeading><p className="muted">No additional servers are listed in your tools’ own configuration.</p></section>)}
      {panel.extra === 'usage' && <ContextUsage />}
      {panel.extra === 'targets' && <CodingTools />}
      {panel.extra === 'editor' && <EditorSettings />}
      </div>)}
      {section.id === 'tools' && app.source.kind === 'api' && mcpSaved && <MCPSpecNotice saved={mcpSaved} dismiss={() => saveMCPNotice()} />}
      {errors === 0 && warningDetails}
      </SpecMovedSaver.Provider>
      </RecheckSectionEntries.Provider>
    </RecheckSection.Provider>
  )
}
