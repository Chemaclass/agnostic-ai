import { useRef, useState } from 'react'
import type { SectionItem, Stage } from '../contract'
import { useApp } from './app-context'
import { AddHookForm, HookSavedNotice, useHookSaved } from './AddHook'
import { SectionHeading } from './SectionHeading'
import { useRechecks } from './Recheck'
import { findingEntryIds, itemEntryId, kindEntryId } from '../model/identity'
import { hookStages } from '../model/hook-form'
import { Badge, ToolAvatar } from './ui'
import { ItemList } from './Items'

export function Lifecycle({ stages, hooks, count, filtering, matchingHooks }: { stages: Stage[]; hooks: SectionItem[]; count: number; filtering: boolean; matchingHooks: Set<SectionItem> }) {
  const root = useRef<HTMLElement>(null)
  const app = useApp()
  const entry = app.state.entry
  const recheck = useRechecks()
  const [adding, setAdding] = useState(false)
  const [hookSaved, saveHookNotice] = useHookSaved()
  const portable = hookStages(stages)
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
        <ToolAvatar key={t.target} target={t.target} label={`${t.label} - ${t.event}`} />
      ))}
    </p>
  )
  if (count === 0) {
    return (
      <section className="block" aria-labelledby="lifecycle-hooks">
        <div className="section-head lifecycle-head">
          <SectionHeading topic="hook" id="lifecycle-hooks" entry={kindEntryId('hook')}>Hooks <span className="muted kind-count">0</span></SectionHeading>
          {addButton && <div className="lifecycle-actions">{addButton}</div>}
        </div>
        {addHook}
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
        <SectionHeading topic="hook" id="lifecycle-hooks" entry={kindEntryId('hook')}>Hooks <span className="muted kind-count">{count}</span></SectionHeading>
        <div className="lifecycle-actions">
          <button type="button" className="button button-quiet" onClick={() => setExpanded(true)}>Expand all</button>
          <button type="button" className="button button-quiet" onClick={() => setExpanded(false)}>Collapse all</button>
          {addButton}
        </div>
      </div>
      {addHook}
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
