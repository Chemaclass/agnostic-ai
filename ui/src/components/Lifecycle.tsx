import { useRef, useState } from 'react'
import type { SectionItem, Stage } from '../contract'
import { useApp } from './app-context'
import { AddHookForm, HookSavedNotice, useHookSaved } from './AddHook'
import { SectionHeading } from './SectionHeading'
import { useRechecks } from './Recheck'
import { findingEntryIds, itemEntryId, kindEntryId } from '../model/identity'
import { hookStages } from '../model/hook-form'
import { isTurnedOff, plural } from '../model/counts'
import { Badge, ToolAvatar } from './ui'
import { EmptyState } from './EmptyState'
import { ItemList } from './Items'

export function Lifecycle({ stages, hooks, count, filtering, matchingHooks }: { stages: Stage[]; hooks: SectionItem[]; count: number; filtering: boolean; matchingHooks: Set<SectionItem> }) {
  const root = useRef<HTMLElement>(null)
  const app = useApp()
  const entry = app.state.entry
  const recheck = useRechecks()
  const [adding, setAdding] = useState(false)
  const [hookSaved, saveHookNotice] = useHookSaved()
  const portable = hookStages(stages)
  const support = app.overview?.sections.find((section) => section.id === 'lifecycle')?.support.find((entry) => entry.kind === 'hook')
  const addButton = app.source.kind === 'api' && portable.length > 0 && <button type="button" className="button" aria-expanded={adding} onClick={() => setAdding(!adding)}>Add hook</button>
  const addHook = <>
    {adding && <AddHookForm stages={portable} onClose={() => setAdding(false)} onSaved={(saved) => { setAdding(false); saveHookNotice(saved) }} />}
    {app.source.kind === 'api' && hookSaved && <HookSavedNotice saved={hookSaved} dismiss={() => saveHookNotice()} />}
  </>
  const setExpanded = (open: boolean) => {
    root.current?.querySelectorAll<HTMLDetailsElement>('details.stage').forEach((stage) => { stage.open = open })
  }
  const stageTools = (st: Stage) => st.targets.length > 0 && (
    <p className="stage-tools">
      {st.targets.map((t) => (
        <span key={t.target} className="stage-tool"><span aria-hidden="true"><ToolAvatar target={t.target} label={`${t.label} - ${t.event}`} /></span>{t.label}</span>
      ))}
    </p>
  )
  if (count === 0) {
    return (
      <section className="block" aria-labelledby="lifecycle-hooks">
        <div className="section-head lifecycle-head">
          <SectionHeading topic="hook" id="lifecycle-hooks" entry={kindEntryId('hook')}>Hooks</SectionHeading>
          {addButton && <div className="lifecycle-actions">{addButton}</div>}
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
        <SectionHeading topic="hook" id="lifecycle-hooks" entry={kindEntryId('hook')}>Hooks <span className="muted kind-count">{count}</span></SectionHeading>
        <div className="lifecycle-actions">
          <button type="button" className="button button-quiet stage-toggle" onClick={() => setExpanded(true)}>Expand all</button>
          <button type="button" className="button button-quiet stage-toggle" onClick={() => setExpanded(false)}>Collapse all</button>
          {addButton}
        </div>
      </div>
      {addHook}
      <ol className="stages">
        {stages.map((st) => {
          const here = hooks.filter((h) => h.group === st.id)
          const matching = filtering ? here.filter((hook) => matchingHooks.has(hook)) : here
          const off = matching.filter(isTurnedOff).length
          const live = matching.length - off
          const hasLive = here.some((hook) => !isTurnedOff(hook))
          const linked = entry !== undefined && here.some((hook) => itemEntryId(hook) === entry || findingEntryIds(hook.findings, itemEntryId(hook)).includes(entry))
          const rechecking = here.some((hook) => itemEntryId(hook) === (recheck.pendingPlace ?? recheck.rechecked?.place))
          return (
            <li key={st.id} className={`stage-step${hasLive ? ' has-hooks' : ''}${here.length && !hasLive ? ' is-off' : ''}`}>
              <details className="stage" open={linked || rechecking || (filtering && matching.length > 0)}>
                <summary className="stage-head">
                  <h3 className="stage-summary">
                    <span className="stage-label">
                      <span className="stage-title">{st.label}</span>
                      {matching.length > 0 && <Badge tone="neutral" className="stage-count">{[live > 0 && plural(live, 'hook'), off > 0 && `${off} turned off`].filter(Boolean).join(', ')}</Badge>}
                    </span>
                    <span className="muted stage-description">{st.description}</span>
                  </h3>
                </summary>
                <div className="stage-body">
                  {stageTools(st)}
                  {!here.length && filtering && <p className="muted stage-empty">No matching hooks.</p>}
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
