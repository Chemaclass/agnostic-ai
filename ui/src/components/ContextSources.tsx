// Ranked sources with per-row advice. A source is keyed by load, source and
// category because one path can appear twice (a skill's description and body).

import { Fragment, useRef, useState } from 'react'
import type { Dispatch, MutableRefObject, SetStateAction } from 'react'
import type { ContextContribution, ContextLoad, ContextReport, ContextSize } from '../contract'
import { contextAdvice, startHere } from '../model/context-advice'
import { isBuiltin, kindTotals, sharePercent } from '../model/context-breakdown'
import { LOAD_CONDITIONS, LOAD_LABELS } from '../model/context-prompt'
import { plural } from '../model/format'
import { useApp } from './app-context'
import { OpenInEditor } from './EditorLink'
import { Code, CopyButton } from './ui'

export const LOADS: ContextLoad[] = ['startup', 'file-scope', 'on-demand']
const ROWS_SHOWN = 10

export const rowKey = (c: ContextContribution) => `${c.load}|${c.source}|${c.category}`

export interface SourcePick {
  advice: string | undefined
  setAdvice: Dispatch<SetStateAction<string | undefined>>
  expanded: Set<ContextLoad>
  setExpanded: Dispatch<SetStateAction<Set<ContextLoad>>>
  open: boolean
  setOpen: (open: boolean) => void
  focusKey: MutableRefObject<string | undefined>
  /** Opens one source's advice, expanding its group and the list when needed, and moves focus to its toggle. */
  show: (c: ContextContribution) => void
}

export function useSourcePick(report: ContextReport | undefined, openByDefault: boolean): SourcePick {
  const [advice, setAdvice] = useState<string>()
  const [expanded, setExpanded] = useState<Set<ContextLoad>>(new Set())
  const [open, setOpen] = useState(openByDefault)
  const [seen, setSeen] = useState({ report, openByDefault })
  const focusKey = useRef<string | undefined>(undefined)
  // Reset while rendering, so a new report never paints with the old open state.
  if (seen.report !== report || seen.openByDefault !== openByDefault) {
    setSeen({ report, openByDefault })
    setAdvice(undefined)
    setExpanded(new Set())
    setOpen(openByDefault)
  }
  const show = (c: ContextContribution) => {
    const key = rowKey(c)
    const rows = (report?.contributions ?? []).filter((r) => r.load === c.load)
    if (rows.indexOf(c) >= ROWS_SHOWN) setExpanded((prev) => new Set(prev).add(c.load))
    setOpen(true)
    setAdvice(key)
    focusKey.current = key
  }
  return { advice, setAdvice, expanded, setExpanded, open, setOpen, focusKey, show }
}

export function ContextSources({ report, root, takesFile, pick }: { report: ContextReport; root?: string; takesFile: boolean; pick: SourcePick }) {
  if (report.contributions.length === 0) return null
  return (
    <>
      <StartHere report={report} onPick={pick.show} />
      <Contributions report={report} root={root} takesFile={takesFile} open={pick.open} onToggle={pick.setOpen} advice={pick.advice} setAdvice={pick.setAdvice} expanded={pick.expanded} setExpanded={pick.setExpanded} focusKey={pick.focusKey} />
    </>
  )
}

function StartHere({ report, onPick }: { report: ContextReport; onPick: (c: ContextContribution) => void }) {
  const top = startHere(report.contributions)
  if (top.length === 0) return null
  const total = report.startup.words
  return (
    <div className="context-usage-part context-start">
      <h3 id="context-start-here">Start here</h3>
      <p className="muted">The largest sources at startup for {report.target}. Open one to see what it is and what you can do.</p>
      <ol className="context-start-list" aria-labelledby="context-start-here">
        {top.map((c) => (
          <li key={rowKey(c)}>
            <button type="button" className="button button-quiet context-start-pick" onClick={() => onPick(c)}>
              <Code>{c.source}</Code>
              <span className="muted">{c.category}, {c.words.toLocaleString()} words, {sharePercent(c.words, total)}% of startup</span>
            </button>
          </li>
        ))}
      </ol>
    </div>
  )
}

interface ContributionsProps {
  report: ContextReport
  root?: string
  takesFile: boolean
  open: boolean
  onToggle: (open: boolean) => void
  advice: string | undefined
  setAdvice: Dispatch<SetStateAction<string | undefined>>
  expanded: Set<ContextLoad>
  setExpanded: Dispatch<SetStateAction<Set<ContextLoad>>>
  focusKey: MutableRefObject<string | undefined>
}

function Contributions({ report, root, takesFile, open, onToggle, advice, setAdvice, expanded, setExpanded, focusKey }: ContributionsProps) {
  const app = useApp()
  const { contributions } = report
  const totals: Record<ContextLoad, ContextSize> = { startup: report.startup, 'file-scope': report.file_scope, 'on-demand': report.on_demand }
  return (
    <details className="context-usage-part context-contributions" open={open} onToggle={(e) => onToggle((e.target as HTMLDetailsElement).open)}>
      <summary>{plural(contributions.length, 'source')}, largest first</summary>
      {LOADS.map((load) => {
        const rows = contributions.filter((c) => c.load === load)
        if (rows.length === 0) return null
        const id = `context-contributions-${load}`
        const all = expanded.has(load)
        const shownRows = all ? rows : rows.slice(0, ROWS_SHOWN)
        const kinds = kindTotals(contributions, load)
        const total = totals[load].words
        return (
          <div key={load} className="context-contribution-load">
            <ul className="context-kinds" aria-label={`${LOAD_LABELS[load]} by kind`}>
              {kinds.map((kind) => (
                <li key={kind.category}>
                  <strong>{kind.category || 'other'}</strong>
                  <span className="muted"> {plural(kind.sources, 'source')}, {kind.words.toLocaleString()} words, {sharePercent(kind.words, total)}% of {LOAD_LABELS[load].toLowerCase()}</span>
                </li>
              ))}
            </ul>
            <div className="table-wrap">
              <table className="table" aria-labelledby={id}>
                <caption id={id}>{LOAD_LABELS[load]}: loads {LOAD_CONDITIONS[load]}</caption>
                <thead>
                  <tr>
                    <th scope="col">Source</th>
                    <th scope="col">Kind</th>
                    <th scope="col">Words</th>
                    <th scope="col">Share</th>
                    <th scope="col">Bytes</th>
                  </tr>
                </thead>
                <tbody>
                  {shownRows.map((c) => {
                    const key = rowKey(c)
                    const share = sharePercent(c.words, total)
                    const isOpen = advice === key
                    const adviceId = `context-advice-${key.replace(/[^a-z0-9]+/gi, '-')}`
                    return (
                      <Fragment key={key}>
                        <tr className={isOpen ? 'is-advised' : undefined}>
                          <th scope="row">
                            <Code>{c.source}</Code>
                            {isBuiltin(c.source)
                              ? <span className="muted context-builtin"> Built in. <button type="button" className="button button-quiet" onClick={() => app.navigate({ view: 'settings', tab: 'builtins', entry: undefined })}>Change in Settings</button></span>
                              : <OpenInEditor path={root ? `${root}/${c.source}` : undefined} label={c.source} shownPath={c.source} compact />}
                            <button
                              type="button"
                              className="button button-quiet context-advice-toggle"
                              aria-expanded={isOpen}
                              aria-controls={adviceId}
                              ref={(el) => { if (el && focusKey.current === key) { el.focus(); focusKey.current = undefined } }}
                              onClick={() => setAdvice(isOpen ? undefined : key)}
                            >{isOpen ? 'Hide advice' : 'What can I do?'}</button>
                          </th>
                          <td>{c.category}</td>
                          <td className="context-number">{c.words.toLocaleString()}</td>
                          <td className="context-number context-share"><span className="context-share-bar" aria-hidden="true" style={{ width: `${share}%` }} />{share}%</td>
                          <td className="context-number">{c.bytes.toLocaleString()}</td>
                        </tr>
                        {isOpen && (
                          <tr className="context-advice-row">
                            <td colSpan={5} id={adviceId}>
                              <Advice contribution={c} report={report} root={root} takesFile={takesFile} />
                            </td>
                          </tr>
                        )}
                      </Fragment>
                    )
                  })}
                </tbody>
              </table>
            </div>
            {rows.length > ROWS_SHOWN && (
              <button type="button" className="button button-quiet" aria-expanded={all} onClick={() => setExpanded((prev) => {
                const next = new Set(prev)
                if (all) next.delete(load)
                else next.add(load)
                return next
              })}>{all ? `Show top ${ROWS_SHOWN}` : `Show all ${rows.length}`}</button>
            )}
          </div>
        )
      })}
    </details>
  )
}

function Advice({ contribution: c, report, root, takesFile }: { contribution: ContextContribution; report: ContextReport; root?: string; takesFile: boolean }) {
  const app = useApp()
  const a = contextAdvice(c, { ...report, takesFile }, root)
  return (
    <div className="context-advice">
      <dl className="context-advice-facts">
        <div><dt>What it is</dt><dd>{a.what}</dd></div>
        <div><dt>Why it loads</dt><dd>{a.why}</dd></div>
      </dl>
      <p className="context-advice-heading">What you can do</p>
      <ol className="context-advice-actions">
        {a.actions.map((action, index) => <li key={index}>{action}</li>)}
      </ol>
      <div className="finding-actions">
        <CopyButton text={a.prompt} label="Copy advice prompt" ariaLabel={`Copy advice prompt for ${c.source}`} />
        {a.builtin && <button type="button" className="button button-quiet" onClick={() => app.navigate({ view: 'settings', tab: 'builtins', entry: undefined })}>Open Built-ins</button>}
      </div>
    </div>
  )
}
