// Ranked sources with per-row advice. A source is keyed by load, source and
// category because one path can appear twice (a skill's description and body).

import { Fragment, useRef, useState } from 'react'
import type { Dispatch, MutableRefObject, SetStateAction } from 'react'
import type { ContextContribution, ContextLoad, ContextReport, ContextSize } from '../contract'
import { contextAdvice, contextAllAdvicePrompt } from '../model/context-advice'
import { isBuiltin, kindTotals, sharePercent } from '../model/context-breakdown'
import { LOAD_CONDITIONS, LOAD_LABELS } from '../model/context-prompt'
import { plural } from '../model/format'
import { useApp } from './app-context'
import { OpenInEditor } from './EditorLink'
import { Code, CopyButton, toolName } from './ui'

export const LOADS: ContextLoad[] = ['startup', 'file-scope', 'on-demand']
const ROWS_SHOWN = 10

const TYPE_LABELS: Record<string, string> = {
  'entry-point layer': 'instructions file',
  'generated framing': 'tool wrapper text',
  'skill discovery': 'skill name and description',
  'agent discovery': 'agent name and description',
}

/** The plain name of a source's type; prompts keep the raw category. */
export const typeLabel = (category: string) => TYPE_LABELS[category] ?? (category || 'other')

export const rowKey = (c: ContextContribution) => `${c.load}|${c.source}|${c.category}`

export interface SourcePick {
  advice: string | undefined
  setAdvice: Dispatch<SetStateAction<string | undefined>>
  expanded: Set<ContextLoad>
  setExpanded: Dispatch<SetStateAction<Set<ContextLoad>>>
  focusKey: MutableRefObject<string | undefined>
  /** Opens one source's advice, expanding its group when needed, and moves focus to its toggle. */
  show: (c: ContextContribution) => void
}

export function useSourcePick(report: ContextReport | undefined): SourcePick {
  const [advice, setAdvice] = useState<string>()
  const [expanded, setExpanded] = useState<Set<ContextLoad>>(new Set())
  const [seen, setSeen] = useState(report)
  const [, rerender] = useState(0)
  const focusKey = useRef<string | undefined>(undefined)
  // Reset while rendering, so a new report never paints with the old advice open.
  if (seen !== report) {
    setSeen(report)
    setAdvice(undefined)
    setExpanded(new Set())
  }
  const show = (c: ContextContribution) => {
    const key = rowKey(c)
    const rows = (report?.contributions ?? []).filter((r) => r.load === c.load)
    if (rows.indexOf(c) >= ROWS_SHOWN) setExpanded((prev) => new Set(prev).add(c.load))
    setAdvice(key)
    focusKey.current = key
    // Advice already open for this row changes no state; render anyway so the toggle takes focus now, not on a later keystroke.
    rerender((n) => n + 1)
  }
  return { advice, setAdvice, expanded, setExpanded, focusKey, show }
}

export function ContextSources({ report, root, takesFile, budget, pick }: { report: ContextReport; root?: string; takesFile: boolean; budget?: number; pick: SourcePick }) {
  if (report.contributions.length === 0) return null
  const allAdvice = contextAllAdvicePrompt({ ...report, takesFile }, root, budget)
  return <Contributions report={report} root={root} takesFile={takesFile} pick={pick} allAdvice={allAdvice} />
}

function Contributions({ report, root, takesFile, pick, allAdvice }: { report: ContextReport; root?: string; takesFile: boolean; pick: SourcePick; allAdvice: string }) {
  const app = useApp()
  const { advice, setAdvice, expanded, setExpanded, focusKey } = pick
  const { contributions } = report
  const totals: Record<ContextLoad, ContextSize> = { startup: report.startup, 'file-scope': report.file_scope, 'on-demand': report.on_demand }
  const tool = toolName(report.target)
  return (
    <section className="context-usage-part context-contributions" aria-labelledby="context-contributions-title">
      <div className="context-contributions-head">
        <h3 id="context-contributions-title">{plural(contributions.length, 'source')}, largest first</h3>
        {allAdvice && <CopyButton text={allAdvice} label="Copy all prompts" className="button-quiet context-all-advice" ariaLabel={`Copy all prompts for ${tool}`} />}
      </div>
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
            <ul className="context-kinds" aria-label={`${LOAD_LABELS[load]} by type`}>
              {kinds.map((kind) => (
                <li key={kind.category}>
                  <strong>{typeLabel(kind.category)}</strong>
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
                    <th scope="col">Type</th>
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
                    const toggle = (
                      <button
                        type="button"
                        className="button button-quiet context-advice-toggle"
                        aria-expanded={isOpen}
                        aria-controls={adviceId}
                        ref={(el) => { if (el && focusKey.current === key) { el.focus(); focusKey.current = undefined } }}
                        onClick={() => setAdvice(isOpen ? undefined : key)}
                      >{isOpen ? 'Hide advice' : 'What can I do?'}</button>
                    )
                    return (
                      <Fragment key={key}>
                        <tr className={isOpen ? 'is-advised' : undefined}>
                          <th scope="row">
                            <Code>{c.source}</Code>
                            {isBuiltin(c.source)
                              ? <><span className="muted context-builtin"> Built in. <button type="button" className="button button-quiet" onClick={() => app.navigate({ view: 'settings', tab: 'builtins', entry: undefined })}>Change in Settings</button></span>{toggle}</>
                              : <OpenInEditor path={root ? `${root}/${c.source}` : undefined} label={c.source} shownPath={c.source} compact trailing={toggle} />}
                          </th>
                          <td className="context-type">{typeLabel(c.category)}</td>
                          <td className="context-number">{c.words.toLocaleString()}</td>
                          <td className="context-number context-share"><span className="context-share-track" aria-hidden="true"><span className="context-share-bar" style={{ width: `${share}%` }} /></span>{share}%</td>
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
    </section>
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
        <CopyButton text={a.prompt} label="Copy prompt" ariaLabel={`Copy prompt for ${c.source}`} />
        {a.builtin && <button type="button" className="button button-quiet" onClick={() => app.navigate({ view: 'settings', tab: 'builtins', entry: undefined })}>Open Built-ins</button>}
      </div>
    </div>
  )
}
