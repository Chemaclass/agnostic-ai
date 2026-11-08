import { useRef, useState } from 'react'
import type { SectionItem, Stage } from '../contract'
import { AppLink, useApp } from './app-context'
import { AddHookForm, HookSavedNotice, useHookSaved } from './AddHook'
import { SectionHeading } from './SectionHeading'
import { useRechecks } from './Recheck'
import { findingEntryIds, itemEntryId, kindEntryId } from '../model/identity'
import { hookStages } from '../model/hook-form'
import { isTurnedOff, plural } from '../model/counts'
import { layerLabel } from '../model/edit-audience'
import { Badge, Chip, RowActions, ToolAvatar } from './ui'
import { EmptyState } from './EmptyState'
import { ToolbarAction } from './ToolbarAction'
import { OpenInEditor } from './EditorLink'
import { SpecMoveAction } from './SpecMoveAction'
import { Findings } from './Findings'
import { HookCommand } from './HookDetails'
import { useEntry } from './section-context'

const isNative = (stage: Stage) => stage.id.startsWith('native:')

export function Lifecycle({ stages, hooks, count, filtering, matchingHooks }: { stages: Stage[]; hooks: SectionItem[]; count: number; filtering: boolean; matchingHooks: Set<SectionItem> }) {
  const root = useRef<HTMLElement>(null)
  const app = useApp()
  const entry = app.state.entry
  const recheck = useRechecks()
  const [adding, setAdding] = useState(false)
  const [allStages, setAllStages] = useState(false)
  const [hookSaved, saveHookNotice] = useHookSaved()
  const portable = hookStages(stages)
  const support = app.overview?.sections.find((section) => section.id === 'lifecycle')?.support.find((entry) => entry.kind === 'hook')
  const addButton = app.source.kind === 'api' && portable.length > 0 && (
    <ToolbarAction>
      <button type="button" className="button button-primary" aria-expanded={adding} onClick={() => setAdding(!adding)}>Add hook</button>
    </ToolbarAction>
  )
  const addHook = <>
    {addButton}
    {adding && <AddHookForm stages={portable} onClose={() => setAdding(false)} onSaved={(saved) => { setAdding(false); saveHookNotice(saved) }} />}
    {app.source.kind === 'api' && hookSaved && <HookSavedNotice saved={hookSaved} dismiss={() => saveHookNotice()} />}
  </>
  const setExpanded = (open: boolean) => {
    root.current?.querySelectorAll<HTMLDetailsElement>('details.stage').forEach((stage) => { stage.open = open })
  }
  if (count === 0) {
    return (
      <section className="block" aria-labelledby="lifecycle-hooks">
        <div className="section-head lifecycle-head">
          <SectionHeading id="lifecycle-hooks" entry={kindEntryId('hook')}>Hooks</SectionHeading>
        </div>
        {addHook}
        <EmptyState kind={{ kind: 'hook', label: 'Hooks' }} support={support} filtering={filtering} />
        <p className="muted stages-intro">A hook can run at each of these stages.</p>
        <ol className="stages is-quiet">
          {stages.map((st) => (
            <li key={st.id} className="stage-step">
              <div className="stage">
                <h3 className="stage-summary">
                  <span className="stage-title">{st.label}</span>
                  <span className="muted stage-description">{st.description}</span>
                </h3>
                <StageTools stage={st} />
              </div>
            </li>
          ))}
        </ol>
      </section>
    )
  }
  const hooksAt = (st: Stage) => hooks.filter((h) => h.group === st.id)
  const shown = stages.filter((st) => allStages || hooksAt(st).length > 0)
  const renderStage = (st: Stage) => {
    const here = hooksAt(st)
    const matching = filtering ? here.filter((hook) => matchingHooks.has(hook)) : here
    const off = matching.filter(isTurnedOff).length
    const live = matching.length - off
    const hasLive = here.some((hook) => !isTurnedOff(hook))
    const linked = entry !== undefined && here.some((hook) => itemEntryId(hook) === entry || findingEntryIds(hook.findings, itemEntryId(hook)).includes(entry))
    const rechecking = here.some((hook) => itemEntryId(hook) === (recheck.pendingPlace ?? recheck.rechecked?.place))
    // A tool's own event says little on its own; what its hooks do says more.
    const description = isNative(st) && here.length ? [...new Set(here.map((hook) => hook.description).filter(Boolean))].join(' ') || st.description : st.description
    return (
      <li key={st.id} className={`stage-step${hasLive ? ' has-hooks' : ''}${here.length && !hasLive ? ' is-off' : ''}`}>
        <details className="stage" open={linked || rechecking || (filtering && matching.length > 0)}>
          <summary className="stage-head">
            <h3 className="stage-summary">
              <span className="stage-label">
                <span className="stage-title">{st.label}</span>
                {matching.length > 0 && <Badge tone="neutral" className="stage-count">{[live > 0 && plural(live, 'hook'), off > 0 && `${off} turned off`].filter(Boolean).join(', ')}</Badge>}
              </span>
              <span className="muted stage-description">{description}</span>
            </h3>
          </summary>
          <div className="stage-body">
            <StageTools stage={st} />
            {here.length > 0 && <ul className="hook-list">{here.map((hook) => <HookBlock key={itemEntryId(hook)} item={hook} />)}</ul>}
          </div>
        </details>
      </li>
    )
  }
  const shared = shown.filter((st) => !isNative(st))
  const native = shown.filter(isNative)
  const hidden = stages.length - stages.filter((st) => hooksAt(st).length > 0).length
  return (
    <section ref={root} className="block" aria-labelledby="lifecycle-hooks">
      <div className="section-head lifecycle-head">
        <SectionHeading id="lifecycle-hooks" entry={kindEntryId('hook')}>Hooks <span className="muted kind-count">{count}</span></SectionHeading>
        <div className="lifecycle-actions">
          <button type="button" className="button button-quiet stage-toggle" onClick={() => setExpanded(true)}>Expand all</button>
          <button type="button" className="button button-quiet stage-toggle" onClick={() => setExpanded(false)}>Collapse all</button>
        </div>
      </div>
      {addHook}
      {!shown.length && <p className="muted stage-empty">No matching hooks.</p>}
      {shared.length > 0 && <ol className="stages">{shared.map(renderStage)}</ol>}
      {native.length > 0 && <>
        <h3 id="lifecycle-native" className="stages-group">Events only one tool uses</h3>
        <ol className="stages" aria-labelledby="lifecycle-native">{native.map(renderStage)}</ol>
      </>}
      {hidden > 0 && (
        <button type="button" className="button button-quiet stages-more" aria-expanded={allStages} onClick={() => setAllStages(!allStages)}>
          {allStages ? 'Show only stages with hooks' : `Show all ${stages.length} stages`}
        </button>
      )}
    </section>
  )
}

function StageTools({ stage }: { stage: Stage }) {
  if (!stage.targets.length) return null
  return (
    <p className="stage-tools">
      {stage.targets.map((t) => (
        <span key={t.target} className="stage-tool"><span aria-hidden="true"><ToolAvatar target={t.target} label={`${t.label} - ${t.event}`} /></span>{t.label}</span>
      ))}
    </p>
  )
}

function HookBlock({ item }: { item: SectionItem }) {
  const app = useApp()
  const { linked, ...entry } = useEntry(itemEntryId(item))
  const builtin = item.layer === 'builtin'
  const openable = app.source.kind === 'api' && item.openPath !== undefined
  const layer = layerLabel(item.layer)
  const origin = builtin ? 'Built in' : `${layer.charAt(0).toUpperCase()}${layer.slice(1)}`
  return (
    <li {...entry} className={`hook-block row-hover${isTurnedOff(item) ? ' is-off' : ''}${item.findings.length ? ' has-findings' : ''}${linked ? ' is-linked' : ''}`}>
      <div className="hook-block-head">
        <strong className="hook-block-title">{item.description || item.label}</strong>
        <span className="hook-block-actions">
          {isTurnedOff(item) && <Chip>Turned off</Chip>}
          {builtin && <AppLink className="hook-settings-link" to={{ view: 'settings', tab: 'builtins', q: undefined, entry: undefined }}>Change in Settings</AppLink>}
          {openable && <RowActions label={item.label}><OpenInEditor path={item.openPath} label={item.label} compact /></RowActions>}
        </span>
      </div>
      <p className="muted hook-block-meta">
        {origin}
        {!builtin && <> <code className="hook-path" title={item.path}>{item.path}</code></>}
      </p>
      <SpecMoveAction item={item} />
      <HookCommand item={item} />
      <Findings findings={item.findings} owner={itemEntryId(item)} place={itemEntryId(item)} />
    </li>
  )
}
