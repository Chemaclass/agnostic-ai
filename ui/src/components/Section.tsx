import { createContext, useContext, useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { API, type ItemFinding, type NativeServer, type OverviewView, type Section, type SectionItem, type SettingField, type Stage, type TargetSupport, type TargetValue, type ToolListing, type Value } from '../contract'
import { EditError, postEdit } from '../api'
import { PageHeader, useApp } from './app-context'
import { ErrorPanel } from './Banner'
import { ContextUsage } from './ContextUsage'
import { BuiltinChange, BuiltinChangeControls, BuiltinChangeProvider, ChangeError, ChangeStatus, SettingChangePreview, useSettingChange } from './BuiltinChange'
import { EditAudienceProvider, EditAudienceSwitch, ViewPersonalButton, useEditAudience, usePageAudience, useViewAudience, type EditAudience } from './EditAudience'
import { OpenInEditor } from './EditorLink'
import { MCPSpecAction, MCPSpecNotice, type MCPSpecSaved } from './MCPSpecAction'
import { PermissionEditProvider, PermissionEditor, PermissionSaveNotice } from './PermissionEditor'
import { SectionTabs } from './SectionTabs'
import { GoneFinding, isNewSinceRecheck, RecheckAction, RecheckNote, RecheckSection, RecheckSectionEntries, useGoneFinding, useRechecks } from './Recheck'
import { findingFixPrompt } from '../model/finding-prompt'
import { instructionsRefinePrompt } from '../model/refine-prompt'
import { generatedAnswer, installedAnswer, valueGroups } from '../model/field-resolution'
import { addressKey, configOverride, originLabel, personalSetsWhole, personalValueAt, perToolEdit, valueAt } from '../model/field-address'
import { builtinEnabled, comparisonValue, sourcesForAudience } from '../model/audience-view'
import { fieldEntryId, findingEntryIds, itemEntryId, sectionEntryIds } from '../model/identity'
import { fieldSearchText, itemSearchText } from '../model/search'
import { entryTab, nativeSearchText, sectionTabs } from '../model/section-tabs'
import type { Remote } from './Shell'
import { Badge, Chip, Code, CopyButton, Icon, Switch, Toggle, ToolAvatar } from './ui'

const ConfigurationRoot = createContext<string | undefined>(undefined)
const ConfiguredTools = createContext<TargetSupport[]>([])
const PersonalBuiltins = createContext(false)

export function SectionPage({ sectionId, overview, stale = false }: { sectionId: string; overview: Remote<OverviewView>; stale?: boolean }) {
  const data = overview.loaded?.data
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

  return (
    <>
      <PageHeader title={section.label} subtitle={section.question} />
      <fieldset className="stale-guard" disabled={stale} data-stale={stale || undefined}>
        <ConfigurationRoot.Provider value={data.snapshot.project.root}>
          <ConfiguredTools.Provider value={data.targets}>
            <EditAudienceProvider key={`${data.snapshot.project.root}:${section.id}`} enabled={section.id === 'settings' || section.id === 'tools' || section.id === 'authority'}>
              <SectionContents section={section} />
            </EditAudienceProvider>
          </ConfiguredTools.Provider>
        </ConfigurationRoot.Provider>
        <SupportTable section={section} toolLabel={toolLabel} />
      </fieldset>
    </>
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
  const personalBuiltins = useContext(PersonalBuiltins)
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
  const search = app.state.q ?? ''
  const entry = app.state.entry
  const { rechecked, pendingPlace } = useRechecks()
  const sectionPlace = `section:${section.id}`
  const remainingHere = rechecked?.result.state === 'remaining' && sectionEntryIds(section).has(rechecked.result.id ?? rechecked.id)
  const orphan = rechecked?.section === section.id && rechecked.place !== sectionPlace && !remainingHere && !section.items.some((item) => itemEntryId(item) === rechecked.place) ? rechecked.place : undefined
  const searchInput = useRef<HTMLInputElement>(null)
  const searchId = useId()
  const query = search.trim().toLowerCase()
  const matches = (parts: (string | undefined)[]) => parts.some((part) => part?.toLowerCase().includes(query))
  const matchingItems = new Set(section.items.filter((item) => matches(itemSearchText(item))))
  const fieldMatches = section.fields?.filter((field) => matches(fieldSearchText(field)))
  const matchingFields = section.fields?.filter((field) => fieldMatches?.includes(field) || fieldEntryId(field) === entry)
  const visibleItems = section.items.filter((item) => matchingItems.has(item) || item.findings.length > 0 || itemEntryId(item) === entry || itemEntryId(item) === pendingPlace || itemEntryId(item) === rechecked?.place)
  const tabs = sectionTabs(section)
  const nativeMatches = section.native?.filter((server) => matches(nativeSearchText(server))) ?? []
  const hasMatches = (tab: typeof tabs[number]) => tab.kinds.some((kind) => section.items.some((item) => item.kind === kind && matchingItems.has(item)) || kind === 'settings' && (fieldMatches?.length ?? 0) > 0) || tab.extra === 'native' && nativeMatches.length > 0
  const firstPopulated = tabs.find((tab) => tab.extra === 'native' ? (section.native?.length ?? 0) > 0 : section.items.some((item) => tab.kinds.includes(item.kind)) || tab.kinds.includes('settings') && (section.fields?.some((field) => field.set.length > 0) ?? false))
  const active = entryTab(section, tabs, entry)
    ?? tabs.find((tab) => tab.id === app.state.tab)?.id
    ?? (query ? tabs.find(hasMatches)?.id : undefined)
    ?? firstPopulated?.id
    ?? tabs[0]?.id
  const setSearch = (value: string) => app.navigate({ q: value || undefined, tab: value.trim() ? undefined : active }, { replace: true })
  const selectTab = (tab: string) => app.navigate({ tab, entry: undefined })
  const panels = tabs.length > 0 ? tabs : [{ id: 'all', label: section.label, kinds: section.kinds.map((kind) => kind.kind), extra: undefined }]
  const showAudience = section.id === 'settings' ? active !== 'configuration' : section.id !== 'tools' || active === 'native'
  const tabLabels = tabs.map((tab) => {
    const items = section.items.filter((item) => tab.kinds.includes(item.kind))
    const fields = tab.kinds.includes('settings') ? section.fields ?? [] : []
    let count: string | number | undefined
    if (tab.extra === 'usage') count = undefined
    else if (query) {
      const matched = tab.extra === 'native' ? nativeMatches.length : items.filter((item) => matchingItems.has(item)).length + fields.filter((field) => fieldMatches?.includes(field)).length
      count = `${matched} match${matched === 1 ? '' : 'es'}`
    }
    else if (tab.extra === 'native') count = section.native?.length ?? 0
    else if (tab.kinds.includes('builtin')) count = `${items.filter((item) => builtinEnabled(item, audience)).length} on`
    else if (tab.kinds.includes('settings')) count = `${fields.filter((field) => sourcesForAudience(field.set, audience).length > 0).length} set`
    else count = items.length
    return { id: tab.id, label: tab.label, count, findings: items.reduce((sum, item) => sum + item.findings.length, 0) }
  })
  const entryMissing = entry !== undefined && !sectionEntryIds(section).has(entry)
  useEffect(() => {
    if (!entry) return
    const target = document.querySelector<HTMLElement>(`[data-entry="${CSS.escape(entry)}"]`)
    if (!target) return
    const details = target.closest('details')
    if (details) details.open = true
    target.scrollIntoView({ block: 'center' })
    target.focus({ preventScroll: true })
  }, [entry, section.id])
  const matchCount = matchingItems.size + (fieldMatches?.length ?? 0) + nativeMatches.length
  const totalCount = section.items.length + (section.fields?.length ?? 0) + (section.native?.length ?? 0)
  const retainedFindings = section.items.some((item) => item.findings.length > 0 && !matchingItems.has(item))
  const errors = section.findings.filter((finding) => finding.severity === 'error').length
  const warnings = section.findings.length - errors
  const findingsId = `${section.id}-findings`
  const findingCounts = [errors ? `${errors} error${errors === 1 ? '' : 's'}` : '', warnings ? `${warnings} warning${warnings === 1 ? '' : 's'}` : ''].filter(Boolean).join(', ')
  const otherFindings = (section.findings.length > 0 || rechecked?.place === sectionPlace || orphan !== undefined) && (
    <section className="block" aria-labelledby={findingsId}>
      <h2 id={findingsId} tabIndex={-1}>Other findings</h2>
      <Findings findings={section.findings} place={sectionPlace} orphanPlace={orphan} />
    </section>
  )
  return (
    <RecheckSection.Provider value={section.id}>
      <RecheckSectionEntries.Provider value={sectionEntryIds(section)}>
      {entryMissing && (
        <div className="entry-missing" role="status">
          <p>The linked {entry.startsWith('finding:') ? 'finding' : entry.startsWith('field:') ? 'setting' : 'item'} is not in this {section.label.toLowerCase()} section. It may have been removed, renamed, or fixed, or the link may come from another configuration.</p>
          <button type="button" className="button button-quiet" onClick={() => app.navigate({ entry: undefined }, { replace: true })}>Dismiss</button>
        </div>
      )}
      <div className="section-search" role="search" aria-label={`Search ${section.label}`}>
        <label htmlFor={searchId}>Search {section.label.toLowerCase()}</label>
        <div className="section-search-controls">
          <div className="section-search-input">
            <Icon name="search" />
            <input ref={searchInput} id={searchId} type="search" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Name, path, description, or value" aria-describedby={`${searchId}-status`} />
          </div>
          {search && <button type="button" className="button button-quiet" onClick={() => {
            setSearch('')
            searchInput.current?.focus()
          }}>Clear search</button>}
        </div>
        <p id={`${searchId}-status`} className="muted section-search-status" role="status">
          {query ? `${matchCount} of ${totalCount} items${section.fields ? ' and fields' : ''} match.` : `${totalCount} items${section.fields ? ' and fields' : ''}`}
          {query && retainedFindings && ' Items with findings stay visible.'}
        </p>
      </div>
      {tabs.length > 1 && <SectionTabs section={section.id} label={section.label} active={active!} select={selectTab} tabs={tabLabels} />}
      {showAudience && <EditAudienceSwitch />}
      {section.id === 'authority' && <PermissionSaveNotice />}
      {section.findings.length > 0 && (
        <div className={`section-findings-summary${errors ? ' has-errors' : ''}`} role={errors ? 'alert' : undefined}>
          <Icon name="warning" />
          <strong>{findingCounts}</strong>
          <button type="button" className="button button-quiet" onClick={() => {
            const heading = document.getElementById(findingsId)
            if (heading) {
              heading.scrollIntoView({ block: 'start' })
              heading.focus({ preventScroll: true })
            }
          }}>Review {section.findings.length} finding{section.findings.length === 1 ? '' : 's'}</button>
        </div>
      )}
      {errors > 0 && otherFindings}
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
        if (k.kind === 'settings' && matchingFields) return <SettingsFields key={k.kind} title={k.label} fields={matchingFields} files={items} filtering={Boolean(query)} status={changes && <ChangeStatus labels={section.items} settings builtins={section.kinds.some((other) => other.kind === 'builtin')} />} />
        const titleId = `${section.id}-${k.kind}`
        const unlisted = sharedUnlistedCommand(section.items.filter((it) => it.kind === k.kind))
        const sharedBlocked = k.kind === 'builtin' && audience === 'shared' && personalBuiltins
        const count = k.kind === 'builtin' ? section.items.filter((item) => item.kind === 'builtin' && builtinEnabled(item, audience)).length : k.count
        return (
          <section className="block" aria-labelledby={titleId} key={k.kind}>
            <h2 id={titleId} tabIndex={-1}>
              {k.label} <span className="muted kind-count">{count}{k.kind === 'builtin' ? ' enabled' : ''}</span>
              {k.local && audience !== 'shared' && <Badge tone="accent" title="Your personal config replaces the shared list">personal</Badge>}
            </h2>
            {k.kind === 'builtin' && <ChangeStatus labels={section.items} settings={false} builtins />}
            {sharedBlocked && <div className="audience-notice"><p>These are the shared defaults. Your personal built-in list replaces them for you. Reset that list before changing shared built-ins.</p><ViewPersonalButton /></div>}
            {k.kind === 'builtin' && audience === 'personal' && !personalBuiltins && <p className="muted">Inherited from shared. Changing a built-in creates your personal list.</p>}
            {k.kind === 'builtin' && !sharedBlocked && <BuiltinChangeControls local={audience === 'personal' && personalBuiltins} resetSource={<ResetAction label="Reset built-ins to the shared list" reset={() => postEdit(API.editBuiltin, { audience: 'personal', reset: true, expected: section.items.filter((item) => item.kind === 'builtin' && item.enabled).map((item) => item.name) })} />} />}
            {k.kind === 'config' && audience && <p className="muted">Combined configuration used for sync, including personal overrides.</p>}
            {unlisted !== undefined && items.length > 0 && (
              <p className="muted tool-state tools-unlisted">
                Tools not listed yet. Run <Code>{unlisted}</Code> to list them.
              </p>
            )}
            {items.length === 0 ? <p className="muted">{query ? 'No matches.' : 'None configured.'}</p> : <GroupedItems items={items} sharedUnlisted={unlisted !== undefined} />}
          </section>
        )
      })}
      {panel.extra === 'native' && (section.native?.length
        ? <NativeServers servers={section.native} matches={matches} filtering={Boolean(query)} onSaved={saveMCPNotice} />
        : <section className="block"><h2>Found in tools</h2><p className="muted">No additional servers are listed in your tools’ own configuration.</p></section>)}
      {panel.extra === 'usage' && <ContextUsage />}
      </div>)}
      {section.id === 'tools' && app.source.kind === 'api' && mcpSaved && <MCPSpecNotice saved={mcpSaved} dismiss={() => saveMCPNotice()} />}
      {errors === 0 && otherFindings}
      </RecheckSectionEntries.Provider>
    </RecheckSection.Provider>
  )
}

function useEntry(id: string): { 'data-entry': string; tabIndex?: number; linked: boolean } {
  const linked = useApp().state.entry === id
  return { 'data-entry': id, tabIndex: linked ? -1 : undefined, linked }
}

function fieldsHeadingId(title: string) {
  return `fields-${title.toLowerCase().replace(/\W+/g, '-')}`
}

function SettingsFields({ title, fields, files, filtering, status }: { title: string; fields: SettingField[]; files: SectionItem[]; filtering: boolean; status?: ReactNode }) {
  const entry = useApp().state.entry
  const audience = useViewAudience()
  const id = fieldsHeadingId(title)
  const configured = fields.filter((field) => sourcesForAudience(field.set, audience).length > 0)
  const unconfigured = fields.filter((field) => sourcesForAudience(field.set, audience).length === 0)
  const editablePermissions = unconfigured.filter((field) => field.editable?.kind === 'permission-list')
  const remaining = unconfigured.filter((field) => !editablePermissions.includes(field))
  const visibleFields = fields.filter((field) => configured.includes(field) || editablePermissions.includes(field))
  return (
    <section className="block" aria-labelledby={id}>
      <h2 id={id} tabIndex={-1}>{title}</h2>
      {status}
      {configured.length === 0 && <p className="muted">{filtering ? 'No matching configured settings.' : editablePermissions.length ? 'No permission rules configured. Add rules below.' : 'No settings configured.'}</p>}
      {visibleFields.length > 0 && <FieldList fields={visibleFields} files={files} />}
      {remaining.length > 0 && (
        <details className="unconfigured-fields" open={filtering || remaining.some((field) => fieldEntryId(field) === entry) || undefined}>
          <summary>Unconfigured fields ({remaining.length})</summary>
          <FieldList fields={remaining} files={files} />
        </details>
      )}
      {files.length > 0 && (
        <>
          <h3 className="item-group-title">Files</h3>
          <ItemList items={files} hideValues />
        </>
      )}
    </section>
  )
}

function FieldList({ fields, files }: { fields: SettingField[]; files: SectionItem[] }) {
  const viewAudience = useViewAudience()
  return (
    <ul className="fields">
      {fields.map((f) => (
        <FieldRow key={f.key} field={f}>
          <div className="field-head">
            <strong>{f.label}</strong>
            {f.key !== 'x-' && <code className="muted">{f.key}</code>}
            {viewAudience === 'personal' && f.set.some((src) => src.local) && <Badge tone="accent" title="Set in a personal file Git ignores">personal</Badge>}
            {viewAudience === 'personal' && f.set.length > 0 && !f.set.some((src) => src.local) && <span className="item-layer">Inherited from shared</span>}
            <p className="muted">{f.description}</p>
          </div>
          {sourcesForAudience(f.set, viewAudience).length === 0 && <p className="muted field-unset">Not set</p>}
          <FieldEditor field={f} files={files} sources={(shown) => sourcesForAudience(f.set, viewAudience).filter((src) => src.path !== shown).map((src) => (
            <div key={`${src.spec}:${src.path}`} className="field-source">
              {f.key === 'x-' ? <NativeValues
                values={src.values}
                file={src.path}
                localKeys={src.local ? files.find((item) => item.name === src.spec && item.path === src.path)?.localKeys : undefined}
                overrides={viewAudience === 'shared' ? f.set.filter((other) => other.local && other.spec === src.spec).flatMap((other) => other.values) : undefined}
                baseline={viewAudience === 'personal' && src.local ? f.set.filter((other) => !other.local && other.spec === src.spec).flatMap((other) => other.values) : undefined}
              /> : <Values values={src.values} file={src.path} fieldKey={f.key} />}
              <p className="muted field-from">
                from <Code>{src.path}</Code>
              </p>
            </div>
          ))} />
          {f.resolved && f.resolved.length > 0 && <FieldResolution field={f} files={files} resolved={f.resolved} />}
        </FieldRow>
      ))}
    </ul>
  )
}

function FieldResolution({ field, files, resolved }: { field: SettingField; files: SectionItem[]; resolved: TargetValue[] }) {
  const app = useApp()
  const label = field.label
  const tools = useContext(ConfiguredTools)
  const [selected, setSelected] = useState(resolved[0].target)
  const current = resolved.find((r) => r.target === selected) ?? resolved[0]
  const groups = valueGroups(resolved)
  const origin = originLabel(current)
  const override = configOverride(current)
  const toolEdit = app.source.kind === 'api' && perToolEdit(field, current.target)
  return (
    <details className="field-resolution">
      <summary>What each tool gets</summary>
      <ul className="field-resolution-groups" aria-label={`${label} by tool`}>
        {groups.map((g) => <li key={g.unchecked ? 'unchecked' : g.value ?? ''}>{g.labels.join(', ')}: {g.unchecked ? <span className="muted">not checked</span> : g.value ? <Code>{g.value}</Code> : <span className="muted">no value written</span>}</li>)}
      </ul>
      {resolved.length > 1 && <label className="field-resolution-tool">Tool <select value={current.target} onChange={(event) => setSelected(event.target.value)}>
        {resolved.map((r) => <option key={r.target} value={r.target}>{r.label}</option>)}
      </select></label>}
      <div className="field-resolution-detail" aria-live="polite">
        <p>{generatedAnswer(current)}</p>
        {current.from && <p className="muted">From <Code>{current.from.path}</Code>{origin ? <>, {origin}</> : ' in the config'}</p>}
        {override && <p className="muted"><Code>{override}</Code> in the config replaces every spec value for {current.label}. An edit below changes the spec only; remove that key for {current.label} to get it.</p>}
        {current.others.length > 0 && <>
          <p className="muted">Values that do not win:</p>
          <ul className="field-resolution-others">
            {current.others.map((o, i) => <li key={`${o.path}:${i}`}>{o.value ? <Code>{o.value}</Code> : 'No value'} in <Code>{o.path}</Code>: {o.reason}</li>)}
          </ul>
        </>}
        {current.native?.map((n) => <p key={n.path} className="muted"><Code>{n.value}</Code> is also set under <Code>{`x-${current.target}.model`}</Code> in <Code>{n.path}</Code>. {current.checked ? `Sync writes that native key as is, so check the generated file for the value ${current.label} reads.` : 'Sync was not rendered here, so this native key was not checked.'}</p>)}
        <p className="muted">{installedAnswer(current.label, tools.find((t) => t.target === current.target)?.sync.state)} agnostic-ai does not see what a running tool uses. Other <Code>{`x-${current.target}`}</Code> keys and imported native settings files are not covered here.</p>
        {typeof toolEdit === 'string' && <p className="muted field-tool-edit">{toolEdit}</p>}
        {toolEdit === true && <div className="field-tool-edit">
          <p className="muted">Set a value for {current.label} only. It is saved as <Code>{addressKey(field.key, current.target)}</Code> and wins over the value for every tool.</p>
          <FieldEditControl key={current.target} field={field} files={files} target={current.target} toolLabel={current.label} sources={() => null} />
        </div>}
      </div>
    </details>
  )
}

function FieldRow({ field, children }: { field: SettingField; children: ReactNode }) {
  const audience = useViewAudience()
  const { linked, ...entry } = useEntry(fieldEntryId(field))
  return <li {...entry} className={`field-row${sourcesForAudience(field.set, audience).length ? ' is-set' : ''}${linked ? ' is-linked' : ''}`}>{children}</li>
}

interface FieldEditControlProps {
  field: SettingField
  files: SectionItem[]
  sources: (shown?: string) => ReactNode
  target?: string
  toolLabel?: string
}

function FieldEditor(props: FieldEditControlProps) {
  return props.field.editable?.kind === 'permission-list' ? <PermissionEditor {...props} /> : <FieldEditControl {...props} />
}

function FieldEditControl(props: FieldEditControlProps) {
  const audience = useEditAudience()
  const [path, setPath] = useState(props.field.editable?.path ?? props.field.editable?.paths?.[0] ?? '')
  return <FieldEditForm key={`${audience}:${path}`} {...props} path={path} setPath={setPath} />
}

function FieldEditForm({ field, files, sources, target, toolLabel, path, setPath }: FieldEditControlProps & { path: string; setPath: (path: string) => void }) {
  const app = useApp()
  const audience = useEditAudience()
  const transaction = useSettingChange(field.key, target)
  const edit = field.editable
  const paths = edit?.paths?.length ? edit.paths : edit?.path ? [edit.path] : []
  const valueOf = (source?: SettingField['set'][number]) => source && valueAt(source.values, field.key, target)
  const specPath = path
  const specName = field.set.find((source) => source.path === specPath)?.spec
  const localSource = field.set.find((source) => source.local && (specName ? source.spec === specName : source.path === edit?.localPath))
  const localKeys = localSource && files.find((item) => item.name === localSource.spec && item.path === localSource.path)?.localKeys
  const teamSource = field.set.find((source) => source.path === path && !source.local)
  const localCurrent = localSource && personalValueAt(localSource.values, localKeys, field.key, target)
  const wholePersonal = target !== undefined && localSource !== undefined && personalSetsWhole(localKeys, field.key)
  const teamCurrent = valueOf(teamSource)
  const selectedCurrent = audience === 'personal' ? localCurrent ?? teamCurrent : teamCurrent
  const current = selectedCurrent === 'null' ? '' : selectedCurrent ?? ''
  const [value, setValue] = useState(current)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<EditError>()
  const [reloaded, setReloaded] = useState(false)
  // A draft keeps the values it started from, so a save after another reload still detects the change.
  const basis = useRef({ current, local: localCurrent, team: teamCurrent })
  const draft = useRef(value)
  draft.current = value
  const latest = useRef(basis.current)
  latest.current = { current, local: localCurrent, team: teamCurrent }
  const conflict = useRef(false)
  useEffect(() => {
    const clean = draft.current === basis.current.current || draft.current === current
    if (conflict.current) {
      conflict.current = false
      setError(undefined)
      setReloaded(true)
    } else if (!clean) {
      const was = basis.current
      if (current !== was.current || localCurrent !== was.local || teamCurrent !== was.team) setReloaded(true)
      return
    }
    basis.current = { current, local: localCurrent, team: teamCurrent }
    if (clean) setValue(current)
  }, [current, localCurrent, teamCurrent])
  if (!edit) return sources()
  if (edit.disabledReason) return <>{sources()}<p className="muted">{edit.disabledReason}{field.key === 'session-retention-days' && !target && <> <a href="https://agnostic-ai.org/docs/cli-reference/inspect/#serve">Global view instructions</a></>}</p></>
  if (app.source.kind !== 'api') return sources()
  const specFile = specPath
  const post = async (body: Record<string, unknown>, expected: string | undefined) => {
    setPending(true)
    setError(undefined)
    setReloaded(false)
    conflict.current = false
    try {
      await postEdit(API.editField, { path: specFile || undefined, key: field.key, target, ...body, expected: expected ?? null })
      const saved = body.value === null ? undefined : String(body.value)
      const next = body.audience === 'personal' ? { ...basis.current, local: saved } : { ...basis.current, team: saved }
      basis.current = { ...next, current: (audience === 'personal' ? next.local ?? next.team : next.team) ?? '' }
      setValue(basis.current.current)
      app.reload(true)
    } catch (err) {
      const failure = err instanceof EditError ? err : new EditError(String(err))
      const now = latest.current
      const shown = basis.current
      if (failure.status === 409 && (now.current !== shown.current || now.local !== shown.local || now.team !== shown.team)) {
        basis.current = now
        setReloaded(true)
        conflict.current = true
        app.reload()
        return
      }
      conflict.current = failure.status === 409
      setError(failure)
    } finally { setPending(false) }
  }
  const save = (clear: boolean) => post({ audience, value: clear ? null : edit.kind === 'number' ? Number(value) : value }, audience === 'personal' ? basis.current.local : basis.current.team)
  const reset = () => post({ audience: 'personal', value: null }, basis.current.local)
  const overridden = audience === 'shared' && (localCurrent !== undefined || wholePersonal)
  const resetLabel = target ? 'Use the value for every tool' : 'Reset to shared value'
  const valueLabel = `${field.label}${toolLabel ? ` for ${toolLabel}` : ''} value`
  const allowed = target ? edit.toolAllowed?.[target] : edit.allowed
  const configured = audience === 'personal' ? localCurrent !== undefined : teamCurrent !== undefined
  const syncCommand = edit.syncCommand ?? `agnostic-ai sync${field.key === 'session-retention-days' ? ' --global' : ''}`
  // A shown preview keeps its draft, so the applied value becomes the new basis.
  const locked = pending || !!transaction.proposal
  const resetButton = audience === 'personal' && !transaction.supported && localCurrent !== undefined && <button type="button" className="button button-quiet" disabled={locked} onClick={reset}>{resetLabel}</button>
  const previewing = pending || !transaction.ready
  const preview = async (next: string | null, where: EditAudience) => {
    setError(undefined)
    setReloaded(false)
    conflict.current = false
    const stale = await transaction.previewSetting({ path: specFile || undefined, key: field.key as 'model' | 'effort', target, value: next, expected: (where === 'personal' ? basis.current.local : basis.current.team) ?? null }, where)
    if (stale) {
      const now = latest.current
      const shown = basis.current
      if (now.current !== shown.current || now.local !== shown.local || now.team !== shown.team) {
        basis.current = now
        setReloaded(true)
      } else conflict.current = true
    }
  }
  const shown = overridden ? undefined : audience === 'personal' ? (localCurrent !== undefined ? localSource?.path : undefined) : teamCurrent !== undefined ? teamSource?.path : undefined
  return <>{sources(shown)}<div className="field-edit">
    {(localCurrent !== undefined || wholePersonal) && <OverrideComparison shared={teamCurrent ?? (target ? valueAt(teamSource?.values ?? [], field.key) : undefined)} personal={localCurrent ?? valueAt(localSource?.values ?? [], field.key)} />}
    {overridden
      ? <div className="field-edit-row">
        <input aria-label={valueLabel} value={teamCurrent ?? ''} placeholder="Not set" disabled />
        <p className="muted">{localCurrent === undefined ? 'Your personal value for every tool replaces the shared per-tool values.' : 'Your personal value overrides the shared value.'} Reset it in Personal to change this shared value.</p>
        <ViewPersonalButton />
      </div>
      : <div className="field-edit-row">
        {allowed && allowed.length > 0
          ? <select aria-label={valueLabel} value={value} onChange={(event) => setValue(event.target.value)} disabled={locked}>
            {!allowed.includes(value) && <option value={value} disabled>{value || 'Choose a value'}</option>}
            {allowed.map((option) => <option key={option} value={option}>{option}</option>)}
          </select>
          : <input aria-label={valueLabel} type={edit.kind === 'number' ? 'number' : 'text'} min={edit.kind === 'number' ? 1 : undefined} step={edit.kind === 'number' ? 1 : undefined} value={value} placeholder={selectedCurrent === 'null' ? 'Cleared for you' : field.set.length > 0 && !current ? 'Not set in this file' : 'Not set'} onChange={(event) => setValue(event.target.value)} disabled={locked} />}
        {!transaction.supported && <button type="button" className="button button-primary" disabled={locked || !value.trim() || value === current} onClick={() => save(false)}>{configured ? 'Update' : 'Save'}</button>}
        {!transaction.supported && audience === 'shared' && configured && <button type="button" className="button button-quiet" disabled={locked} onClick={() => save(true)}>{target ? resetLabel : 'Clear'}</button>}
        {resetButton}
      </div>}
    {paths.length > 1 && <label className="field-edit-target">{audience === 'shared' ? 'Save to' : 'Override spec'} <select value={path} onChange={(event) => setPath(event.target.value)}>{paths.map((candidate) => <option key={candidate} value={candidate}>{candidate}</option>)}</select></label>}
    {error && <span className="edit-error" role="alert">{error.message}{error.problems.length > 0 && <ul>{error.problems.map((problem) => <li key={problem}>{problem}</li>)}</ul>}
      {error.status === 409 && <> <button type="button" className="button button-quiet" onClick={() => app.reload()}>Reload saved values</button></>}</span>}
    {transaction.supported && <div className="field-edit-row field-edit-apply">
      <span className="muted">Apply to your tools now:</span>
      {!overridden && <button type="button" className="button" disabled={previewing || !value.trim() || value === current} onClick={() => { void preview(value, audience) }}>Preview and apply</button>}
      {!overridden && audience === 'shared' && configured && <button type="button" className="button button-quiet" disabled={previewing} onClick={() => { void preview(null, 'shared') }}>{target ? 'Preview using the value for every tool' : 'Preview removal'}</button>}
      {audience === 'personal' && localCurrent !== undefined && <button type="button" className="button button-quiet" disabled={previewing} onClick={() => { void preview(null, 'personal') }}>Preview personal reset</button>}
      {transaction.preparing && <span className="muted">Preparing preview…</span>}
    </div>}
    {transaction.error && <ChangeError reloadLabel="Reload saved values" />}
    {transaction.proposal && <SettingChangePreview change={transaction.proposal} label={toolLabel ? `${field.label} for ${toolLabel}` : field.label} />}
    {reloaded && <p className="muted" role="status">The saved value changed after you started editing and is now {current ? <Code>{current}</Code> : 'not set'}. Your draft is kept; save it to replace the saved value.</p>}
    <p className="muted field-edit-hint">
      {audience === 'personal' && (localSource?.path || path === edit.path && edit.localPath) && <>Saves to <Code>{localSource?.path ?? edit.localPath}</Code> · </>}
      {audience === 'personal' && !localSource?.path && path !== edit.path && <>Saves to your personal overlay for <Code>{path}</Code> · </>}
      {audience === 'shared' && configured && paths.length <= 1 && <>from <Code>{path}</Code> · </>}
      {audience === 'shared' && !configured && field.set.length > 0 && paths.length <= 1 && path && <>Saves to <Code>{path}</Code> · </>}
      {!transaction.supported && <>Run <Code>{syncCommand}</Code> to apply.</>}
    </p>
  </div></>
}

function Lifecycle({ stages, hooks, count, filtering, matchingHooks }: { stages: Stage[]; hooks: SectionItem[]; count: number; filtering: boolean; matchingHooks: Set<SectionItem> }) {
  const root = useRef<HTMLElement>(null)
  const entry = useApp().state.entry
  const recheck = useRechecks()
  const setExpanded = (open: boolean) => {
    root.current?.querySelectorAll<HTMLDetailsElement>('details.stage').forEach((stage) => { stage.open = open })
  }
  const stageTools = (st: Stage) => st.targets.length > 0 && (
    <p className="stage-tools">
      {st.targets.map((t) => (
        <ToolAvatar key={t.target} target={t.target} label={`${t.label} - ${t.event}`} />
      ))}
    </p>
  )
  if (count === 0) {
    return (
      <section className="block" aria-labelledby="lifecycle-hooks">
        <h2 id="lifecycle-hooks">Hooks <span className="muted kind-count">0</span></h2>
        <p className="muted">None configured. A hook can run at each of these stages.</p>
        <ol className="stages is-quiet">
          {stages.map((st) => (
            <li key={st.id} className="stage-step">
              <div className="stage">
                <h3 className="stage-summary">
                  <span className="stage-title">{st.label}</span>
                  <span className="muted stage-description">{st.description}</span>
                </h3>
                {stageTools(st)}
              </div>
            </li>
          ))}
        </ol>
      </section>
    )
  }
  return (
    <section ref={root} className="block" aria-labelledby="lifecycle-hooks">
      <div className="section-head lifecycle-head">
        <h2 id="lifecycle-hooks">Hooks <span className="muted kind-count">{count}</span></h2>
        <div className="lifecycle-actions">
          <button type="button" className="button button-quiet" onClick={() => setExpanded(true)}>Expand all</button>
          <button type="button" className="button button-quiet" onClick={() => setExpanded(false)}>Collapse all</button>
        </div>
      </div>
      <ol className="stages">
        {stages.map((st) => {
          const here = hooks.filter((h) => h.group === st.id)
          const matching = filtering ? here.filter((hook) => matchingHooks.has(hook)) : here
          const linked = entry !== undefined && here.some((hook) => itemEntryId(hook) === entry || findingEntryIds(hook.findings, itemEntryId(hook)).includes(entry))
          const rechecking = here.some((hook) => itemEntryId(hook) === (recheck.pendingPlace ?? recheck.rechecked?.place))
          return (
            <li key={st.id} className={`stage-step${here.length ? ' has-hooks' : ''}`}>
              <details className="stage" open={linked || rechecking || (filtering && matching.length > 0)}>
                <summary className="stage-head">
                  <h3 className="stage-summary">
                    <span className="stage-label">
                      <span className="stage-title">{st.label}</span>
                      <Badge tone="neutral" className="stage-count">{matching.length ? `${matching.length} ${matching.length === 1 ? 'hook' : 'hooks'}` : filtering ? 'No matching hooks' : 'No hooks'}</Badge>
                    </span>
                    <span className="muted stage-description">{st.description}</span>
                  </h3>
                </summary>
                <div className="stage-body">
                  {stageTools(st)}
                  {!here.length && <p className="muted stage-empty">{filtering ? 'No matching hooks.' : 'No hook runs here.'}</p>}
                  {here.length > 0 && <ItemList items={here} />}
                </div>
              </details>
            </li>
          )
        })}
      </ol>
    </section>
  )
}

// When no server has a tool list yet, one notice under the heading replaces
// the same line on every server.
function sharedUnlistedCommand(items: SectionItem[]): string | undefined {
  const listings = items.flatMap((item) => (item.tools ? [item.tools] : []))
  const command = listings[0]?.command
  return listings.length > 0 && listings.every((listing) => listing.state === 'not-listed' && listing.command === command) ? command : undefined
}

function GroupedItems({ items, sharedUnlisted }: { items: SectionItem[]; sharedUnlisted?: boolean }) {
  if (!items.some((it) => it.group)) return <ItemList items={items} sharedUnlisted={sharedUnlisted} />
  const groups: { name: string; items: SectionItem[] }[] = []
  for (const it of items) {
    const name = it.group ?? ''
    const last = groups[groups.length - 1]
    if (last && last.name === name) last.items.push(it)
    else groups.push({ name, items: [it] })
  }
  return (
    <div className="item-groups">
      {groups.map((g) => (
        <div key={g.name} className="item-group">
          <h3 className="item-group-title">{g.name}</h3>
          <ItemList items={g.items} sharedUnlisted={sharedUnlisted} />
        </div>
      ))}
    </div>
  )
}

function ItemList({ items, hideValues, sharedUnlisted }: { items: SectionItem[]; hideValues?: boolean; sharedUnlisted?: boolean }) {
  const audience = useEditAudience()
  const viewAudience = useViewAudience()
  const personalBuiltins = useContext(PersonalBuiltins)
  const sharedBlocked = viewAudience === 'shared' && personalBuiltins
  return (
    <ul className="item-list">
      {items.map((item) => {
        const on = builtinEnabled(item, viewAudience)
        const control = item.enabled !== undefined && <>
          {sharedBlocked
            ? <Toggle on={on} label={`${item.name} built-in`} disabled onToggle={() => {}} />
            : <EditSwitch key={audience} on={on} label={`${item.name} built-in`} save={(enabled) => postEdit(API.editBuiltin, { audience, name: item.name, enabled, expected: on })} />}
          {viewAudience === 'personal' && item.local && <ResetAction label={`Reset ${item.name} to shared value`} reset={() => postEdit(API.editBuiltin, { audience: 'personal', name: item.name, enabled: !on, expected: on })} />}
        </>
        const heading = (toggle: ReactNode) => <ItemHeading item={item} control={toggle} />
        return (
          <ItemRow key={itemEntryId(item)} item={item}>
            {item.kind === 'builtin' && item.enabled !== undefined && !sharedBlocked
              ? <BuiltinChange name={item.name} label={item.label} on={on} local={viewAudience === 'personal' && (item.local ?? false)} fallback={control} renderHeading={heading} />
              : heading(control)}
            {!hideValues && item.values.length > 0 && <Values values={item.values} file={item.kind === 'settings' ? item.path : undefined} personal={item.layer === 'project-user' || item.layer === 'global-local'} localKeys={item.localKeys} />}
            {item.tools && !(sharedUnlisted && item.tools.state === 'not-listed') && <Tools listing={item.tools} />}
            <Findings findings={item.findings} owner={itemEntryId(item)} place={itemEntryId(item)} />
          </ItemRow>
        )
      })}
    </ul>
  )
}

function ItemRow({ item, children }: { item: SectionItem; children: ReactNode }) {
  const { linked, ...entry } = useEntry(itemEntryId(item))
  return <li {...entry} className={`item-row${item.findings.length ? ' has-findings' : ''}${linked ? ' is-linked' : ''}`}>{children}</li>
}

function ItemHeading({ item, control }: { item: SectionItem; control: ReactNode }) {
  const root = useContext(ConfigurationRoot)
  const audience = useViewAudience()
  const refinable = item.kind === 'instructions' && item.layer === 'global'
  return (
    <div className="item-heading">
      <div className="item-main">
        <strong>{item.label}</strong>
        {item.description && <span className="muted item-description">{item.description}</span>}
        {item.kind === 'builtin' && item.local && audience && <OverrideComparison shared={builtinEnabled(item, 'shared') ? 'On' : 'Off'} personal={item.enabled ? 'On' : 'Off'} />}
        {item.enabled === undefined && <code className="item-path" title={item.path}>{item.path}</code>}
      </div>
      <div className="item-meta">
        {item.local && audience !== 'shared' && <Badge tone="accent" title="Set in a personal file Git ignores">personal</Badge>}
        {control}
        {item.enabled === undefined && <span className="item-layer" title="Layer the item comes from">{item.layer}</span>}
        {item.scope && <Chip>{item.scope}</Chip>}
        {refinable && <CopyButton text={instructionsRefinePrompt(item.openPath ?? (root ? `${root}/${item.path}` : item.path))} label="Copy refine prompt" ariaLabel={`Copy refine prompt for ${item.label}`} className="refine-copy" />}
        <OpenInEditor path={item.openPath} label={item.label} compact />
      </div>
    </div>
  )
}

function NativeServers({ servers, matches, filtering, onSaved }: { servers: NativeServer[]; matches: (parts: (string | undefined)[]) => boolean; filtering: boolean; onSaved: (saved: MCPSpecSaved) => void }) {
  const visible = servers.filter((server) => matches(nativeSearchText(server)))
  return (
    <section className="block" aria-labelledby="tools-native">
      <h2 id="tools-native">
        Found in a tool's own config <span className="muted kind-count">{filtering ? `${visible.length} of ${servers.length} match` : servers.length}</span>
      </h2>
      <p className="muted">These servers work only in the tool that holds them. No spec covers them, so sync does not share them with your other tools.</p>
      {visible.length === 0 && filtering ? <p className="muted">No matches.</p> : (
        <ul className="item-list">
          {visible.map((s) => {
            const tools = [...new Set(s.holders.map((h) => h.label))]
            const transports = [...new Set(s.holders.flatMap((h) => (h.transport ? [h.transport] : [])))]
            return (
              <li key={s.name} className="item-row">
                <div className="item-heading">
                  <div className="item-main">
                    <strong>{s.name}</strong>
                    <span className="muted item-description">{tools.length === 1 ? `${tools[0]} only` : tools.join(', ')}</span>
                    {s.holders.map((h) => (
                      <code key={`${h.target}:${h.path}:${h.scope}`} className="item-path" title={`${h.label}, ${h.scope === 'user' ? 'every project' : 'this project'}`}>{h.path}</code>
                    ))}
                  </div>
                  <div className="item-meta">
                    {transports.map((t) => <Chip key={t}>{t}</Chip>)}
                  </div>
                </div>
                {s.import && (
                  <p className="muted tool-state">
                    Run <Code>{s.import}</Code> to make it a spec.
                  </p>
                )}
                <MCPSpecAction server={s} onSaved={onSaved} />
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}

/** An MCP server's tools, from the cache `agnostic-ai mcp tools` fills. */
function Tools({ listing }: { listing: ToolListing }) {
  const when = listing.listedAt ? new Date(listing.listedAt).toLocaleString() : ''
  return (
    <div className="tool-listing">
      {listing.tools.length > 0 && (
        <span className="value-chips">
          {listing.tools.map((t) => (
            <Chip key={t.name} title={t.description || t.title}>
              {t.name}
            </Chip>
          ))}
        </span>
      )}
      {listing.state === 'stale' && (
        <p className="tool-state is-warning">
          The spec changed after these were listed ({when}). Run <Code>{listing.command}</Code> to list them again.
        </p>
      )}
      {listing.state === 'failed' && (
        <p className="tool-state is-warning">
          Could not list the tools: {listing.error}. Run <Code>{listing.command}</Code> to try again.
        </p>
      )}
      {listing.state === 'not-listed' && (
        <p className="muted tool-state">
          Tools not listed yet. Run <Code>{listing.command}</Code> to list them.
        </p>
      )}
    </div>
  )
}

/**
 * A switch that saves its setting through the server. The server
 * validates the edit and restores the file when it fails; the reason
 * shows under the switch. Snapshots and the mock have no server, so the
 * switch only shows the value there.
 */
function EditSwitch({ on, label, save }: { on: boolean; label: string; save: (on: boolean) => Promise<void> }) {
  const app = useApp()
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<EditError | undefined>()
  const [choice, setChoice] = useState<boolean | undefined>()
  const reloadBasis = useRef<object | undefined>(undefined)
  const waitingForReload = useRef(false)
  useEffect(() => {
    if (waitingForReload.current && app.overviewRevision !== reloadBasis.current) {
      waitingForReload.current = false
      setChoice(undefined)
      setError(undefined)
    }
  }, [app.overviewRevision])
  if (app.source.kind !== 'api') return <Switch on={on} />
  const toggle = async () => {
    setPending(true)
    setError(undefined)
    const selected = !(choice ?? on)
    try {
      await save(selected)
      app.reload(true)
    } catch (err) {
      const failure = err instanceof EditError ? err : new EditError(String(err))
      if (failure.status === 409) setChoice(selected)
      setError(failure)
    } finally {
      setPending(false)
    }
  }
  return (
    <span className="edit-switch">
      <Toggle on={choice ?? on} label={label} pending={pending} disabled={choice !== undefined} onToggle={toggle} />
      {error && (
        <span className="edit-error" role="alert">
          {error.message}
          {error.status === 409 && <> <button type="button" className="button button-quiet" onClick={() => { reloadBasis.current = app.overviewRevision; waitingForReload.current = true; app.reload() }}>Reload saved values</button></>}
          {error.problems.length > 0 && (
            <ul>
              {error.problems.map((p) => (
                <li key={p}>{p}</li>
              ))}
            </ul>
          )}
        </span>
      )}
    </span>
  )
}

/** Removes a personal override; the server refuses it when nothing is overridden. */
function ResetAction({ label, reset }: { label: string; reset: () => Promise<void> }) {
  const app = useApp()
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<EditError | undefined>()
  const reloadBasis = useRef<object | undefined>(undefined)
  const waitingForReload = useRef(false)
  useEffect(() => {
    if (waitingForReload.current && app.overviewRevision !== reloadBasis.current) {
      waitingForReload.current = false
      setError(undefined)
    }
  }, [app.overviewRevision])
  if (app.source.kind !== 'api') return null
  const run = async () => {
    setPending(true)
    setError(undefined)
    try {
      await reset()
      app.reload(true)
    } catch (err) {
      setError(err instanceof EditError ? err : new EditError(String(err)))
    } finally {
      setPending(false)
    }
  }
  return (
    <span className="edit-switch">
      <button type="button" className="button button-quiet" aria-label={label} disabled={pending} onClick={() => { void run() }}>Reset to shared value</button>
      {error && <span className="edit-error" role="alert">{error.message}{error.status === 409 && <> <button type="button" className="button button-quiet" onClick={() => { reloadBasis.current = app.overviewRevision; waitingForReload.current = true; app.reload() }}>Reload saved values</button></>}</span>}
    </span>
  )
}

function OverrideComparison({ shared, personal }: { shared?: string; personal?: string }) {
  if (shared === personal) return null
  const label = (value?: string) => value === 'null' ? 'Cleared' : value ?? 'Not set'
  return <span className="override-comparison" role="note" aria-label="Personal overrides shared">
    <span>Shared: <strong>{label(shared)}</strong></span>
    <span aria-hidden="true">·</span>
    <span>Personal: <strong>{label(personal)}</strong></span>
  </span>
}

function NativeComparison({ value, overrides, baseline }: { value: Value; overrides?: Value[]; baseline?: Value[] }) {
  const key = value.key
  const own = comparisonValue([value], key)
  if (baseline) return <OverrideComparison shared={comparisonValue(baseline, key)} personal={own} />
  if (overrides?.some((override) => override.key === key || key.startsWith(`${override.key}.`))) return <OverrideComparison shared={own} personal={comparisonValue(overrides, key)} />
  return null
}

function NativeValues({ values, file, localKeys, overrides, baseline }: { values: Value[]; file: string; localKeys?: string[]; overrides?: Value[]; baseline?: Value[] }) {
  const groups = new Map<string, Value[]>()
  for (const value of values) {
    const prefix = value.key.split('.')[0]
    const group = groups.get(prefix) ?? []
    group.push(value)
    groups.set(prefix, group)
  }
  return (
    <div className="native-groups">
      {[...groups].map(([prefix, entries]) => (
        <div key={prefix} className="native-group">
          <h4 className="native-group-title"><ToolAvatar target={prefix.slice(2)} /><code>{prefix}</code></h4>
          <Values values={entries} file={file} prefix={prefix} localKeys={localKeys} overrides={overrides} baseline={baseline} />
        </div>
      ))}
    </div>
  )
}

function Values({ values, file, prefix, personal, localKeys, fieldKey, overrides, baseline }: { values: Value[]; file?: string; prefix?: string; personal?: boolean; localKeys?: string[]; fieldKey?: string; overrides?: Value[]; baseline?: Value[] }) {
  const audience = usePageAudience() ?? (personal ? 'personal' : 'shared')
  return (
    <dl className="values">
      {values.map((v) => (
        <div key={v.key} className={`value-row${v.key === fieldKey ? ' repeats-field-key' : ''}`}>
          <dt>
            <code title={prefix ? v.key : undefined}>
              {(prefix && v.key.startsWith(`${prefix}.`) ? v.key.slice(prefix.length + 1) : v.key).split('.').map((part, i) => (
                <span key={i}>{i > 0 && <>.<wbr /></>}{part}</span>
              ))}
            </code>
          </dt>
          <dd>
            {v.type === 'list' ? (
              <span className="value-chips">
                {(v.list ?? []).length ? (v.list ?? []).map((entry, i) => v.key === 'targets'
                  ? <ToolAvatar key={`${i}-${entry}`} target={entry} />
                  : <Chip key={`${i}-${entry}`}>{entry}</Chip>) : <span className="muted">Empty</span>}
              </span>
            ) : v.type === 'bool' ? (
              file ? (
                <>
                  {overrides?.some((override) => override.key === v.key || v.key.startsWith(`${override.key}.`))
                    ? <span className="value-override"><Toggle on={v.value === 'true'} label={v.key} disabled onToggle={() => {}} /><span className="muted">Overridden in Personal</span><ViewPersonalButton /></span>
                    : <EditSwitch key={audience} on={v.value === 'true'} label={v.key} save={(on) => postEdit(API.editValue, { audience, path: file, key: v.key, value: on, expected: v.value === 'true' })} />}
                  {audience === 'personal' && localKeys?.includes(v.key) && <ResetAction label={`Reset ${v.key} to shared value`} reset={() => postEdit(API.editValue, { audience: 'personal', path: file, key: v.key, value: null, expected: v.value === 'true' })} />}
                </>
              ) : (
                <Switch on={v.value === 'true'} />
              )
            ) : (
              <span className="value-text">{v.value}</span>
            )}
            <NativeComparison value={v} overrides={overrides} baseline={baseline} />
          </dd>
        </div>
      ))}
    </dl>
  )
}

function Findings({ findings, owner, place, orphanPlace }: { findings: ItemFinding[]; owner?: string; place: string; orphanPlace?: string }) {
  const ids = findingEntryIds(findings, owner)
  const gone = useGoneFinding(place, ids, orphanPlace)
  if (!findings.length && !gone) return null
  return (
    <ul className="findings">
      {findings.map((f, i) => (
        <Finding key={ids[i]} finding={f} entryId={ids[i]} place={place} />
      ))}
      {gone && (gone.result.state === 'incomplete' ? <Finding key={gone.id} finding={gone.finding} entryId={gone.id} place={place} /> : <GoneFinding rechecked={gone} />)}
    </ul>
  )
}

function Finding({ finding: f, entryId, place }: { finding: ItemFinding; entryId: string; place: string }) {
  const root = useContext(ConfigurationRoot)
  const fresh = isNewSinceRecheck(useRechecks(), entryId)
  const prompt = findingFixPrompt(f, root)
  const promptId = useId()
  const { linked, ...entry } = useEntry(entryId)
  return (
    <li {...entry} className={`finding finding-${f.severity === 'error' ? 'error' : 'warn'}${linked ? ' is-linked' : ''}`}>
      <div className="finding-body">
        <div className="finding-head">
          <div className="finding-meta">
            <Icon name="warning" className="finding-icon" />
            <Badge tone={f.severity === 'error' ? 'blocked' : 'approval'}>{f.severity === 'error' ? 'Error' : 'Warning'}</Badge>
            {f.code && <Code>{f.code}</Code>}
            <span className="muted">{f.source}</span>
            {fresh && <Badge tone="accent">New since recheck</Badge>}
          </div>
          <div className="finding-actions">
            <CopyButton text={prompt} label="Copy fix prompt" copiedLabel="Copied" className="finding-copy" />
            <RecheckAction id={entryId} finding={f} place={place} />
          </div>
        </div>
        <FindingMessage message={f.message} />
        <RecheckNote id={entryId} finding={f} />
        {f.path ? (
          <div className="finding-source muted">
            <Code>{f.path}{f.line ? `:${f.line}` : ''}</Code>
            <OpenInEditor path={f.openPath} line={f.line} label={f.path} shownPath={f.path} />
          </div>
        ) : f.line ? <p className="finding-source muted">Line {f.line}</p> : null}
        <details className="finding-details">
          <summary>Details</summary>
          <div className="finding-evidence muted">
            {f.fix && <p><strong>Suggested fix</strong><span>{f.fix}</span></p>}
            {f.explanation && <p><strong>Why this appears</strong><span>{f.explanation}</span></p>}
            <div className="finding-prompt">
              <label htmlFor={promptId}>Fix prompt</label>
              <p id={`${promptId}-hint`}>Paste this prompt into your coding agent.</p>
              <textarea id={promptId} aria-describedby={`${promptId}-hint`} value={prompt} readOnly rows={12} spellCheck={false} />
            </div>
          </div>
        </details>
      </div>
    </li>
  )
}

function FindingMessage({ message }: { message: string }) {
  const [expanded, setExpanded] = useState(false)
  const messageId = useId()
  const collapsible = message.length > 240
  return (
    <div className="finding-message-wrap">
      <p id={messageId} className={`finding-message${collapsible && !expanded ? ' is-collapsed' : ''}`}>{message}</p>
      {collapsible && (
        <button type="button" className="finding-expand" aria-expanded={expanded} aria-controls={messageId} onClick={() => setExpanded(!expanded)}>
          {expanded ? 'Show less' : 'Show full message'}
        </button>
      )}
    </div>
  )
}

function SupportTable({ section, toolLabel }: { section: Section; toolLabel: (target: string) => string }) {
  if (!section.support.length) return null
  return (
    <section className="block" aria-labelledby="support-title">
      <h2 id="support-title">Coding tool support</h2>
      <div className="table-wrap">
        <table className="table support-table" role="table" aria-labelledby="support-title">
          <thead role="rowgroup">
            <tr role="row">
              <th id="support-kind" scope="col" role="columnheader">Kind</th>
              <th id="support-supported" scope="col" role="columnheader">Supported on</th>
              <th id="support-unsupported" scope="col" role="columnheader">Not supported on</th>
            </tr>
          </thead>
          <tbody role="rowgroup">
            {section.support.map((k) => (
              <tr key={k.kind} role="row">
                <th id={`${section.id}-${k.kind}-support`} scope="row" role="rowheader">{k.label}</th>
                <td role="cell" headers={`${section.id}-${k.kind}-support support-supported`}>
                  <span className="support-status-label" aria-hidden="true">Supported on</span>
                  {k.supported.length ? (
                    <span className="tool-list">
                      {k.supported.map((t) => (
                        <ToolAvatar key={t} target={t} label={toolLabel(t)} />
                      ))}
                    </span>
                  ) : (
                    <span className="muted">No configured tool</span>
                  )}
                </td>
                <td role="cell" headers={`${section.id}-${k.kind}-support support-unsupported`}>
                  <span className="support-status-label" aria-hidden="true">Not supported on</span>
                  {k.unsupported.length ? (
                    <span className="tool-list">
                      {k.unsupported.map((t) => (
                        <ToolAvatar key={t} target={t} label={toolLabel(t)} />
                      ))}
                    </span>
                  ) : (
                    <span className="muted">Every configured tool supports it</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  )
}
