// Ranked sources with per-row advice. A source is keyed by load, source and
// category because one path can appear twice (a skill's description and body).

import { Fragment, useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import type { ContextContribution, ContextLoad, ContextReport, ContextSize } from '../contract'
import { contextAdvice, contextAllAdvicePrompt, contextStartupPrompt } from '../model/context-advice'
import { builtinTitle, isBuiltin, sharePercent } from '../model/context-breakdown'
import { plural } from '../model/format'
import { useApp } from './app-context'
import { OpenInEditor } from './EditorLink'
import { CopyButton, RowActions, toolName } from './ui'

export const LOADS: ContextLoad[] = ['startup', 'file-scope', 'on-demand']
const ROWS_SHOWN: Record<ContextLoad, number> = { startup: 5, 'file-scope': 5, 'on-demand': 4 }
const TABLE_TITLES: Record<ContextLoad, string> = { startup: 'Read at every start', 'file-scope': 'Read for the named file', 'on-demand': 'Read on demand' }

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
}

export function useSourcePick(report: ContextReport | undefined): SourcePick {
  const [advice, setAdvice] = useState<string>()
  const [expanded, setExpanded] = useState<Set<ContextLoad>>(new Set())
  const [seen, setSeen] = useState(report)
  // Reset while rendering, so a new report never paints with the old advice open.
  if (seen !== report) {
    setSeen(report)
    setAdvice(undefined)
    setExpanded(new Set())
  }
  return { advice, setAdvice, expanded, setExpanded }
}

export function ContextSources({ report, root, takesFile, budget, pick }: { report: ContextReport; root?: string; takesFile: boolean; budget?: number; pick: SourcePick }) {
  const tool = toolName(report.target)
  const totals: Record<ContextLoad, ContextSize> = { startup: report.startup, 'file-scope': report.file_scope, 'on-demand': report.on_demand }
  return (
    <>
      {LOADS.map((load) => {
        const rows = report.contributions.filter((c) => c.load === load)
        if (rows.length === 0) return null
        // Startup asks for a review of everything that loads at start; the other loads ask for the advice on each source.
        const prompt = load === 'startup' ? contextStartupPrompt(report, root, budget) : contextAllAdvicePrompt({ ...report, takesFile, contributions: rows }, root, budget)
        return <SourceTable key={load} load={load} rows={rows} total={totals[load].words} report={report} root={root} takesFile={takesFile} pick={pick} prompt={prompt} tool={tool} />
      })}
    </>
  )
}

function SourceTable({ load, rows, total, report, root, takesFile, pick, prompt, tool }: { load: ContextLoad; rows: ContextContribution[]; total: number; report: ContextReport; root?: string; takesFile: boolean; pick: SourcePick; prompt: string; tool: string }) {
  const app = useApp()
  const { advice, setAdvice, expanded, setExpanded } = pick
  const id = `context-contributions-${load}`
  const all = expanded.has(load)
  const limit = ROWS_SHOWN[load]
  const shownRows = all ? rows : rows.slice(0, limit)
  const title = TABLE_TITLES[load]
  return (
    <section className="context-usage-part context-contributions" aria-labelledby={id}>
      <div className="context-contributions-head">
        <div>
          <h3 id={id}>{title}</h3>
          <p className="muted">{plural(rows.length, 'source')}, largest first</p>
        </div>
        {prompt && <CopyButton text={prompt} label="Copy prompt" className="button-quiet context-table-prompt" ariaLabel={`Copy prompt to review what ${tool} reads ${load === 'startup' ? 'at every start' : load === 'file-scope' ? 'for the named file' : 'on demand'}`} />}
      </div>
      <div className="table-wrap">
        <table className="table" aria-labelledby={id}>
          <thead>
            <tr>
              <th scope="col">Source</th>
              <th scope="col">Type</th>
              <th scope="col">Words</th>
              <th scope="col">Share</th>
            </tr>
          </thead>
          <tbody>
            {shownRows.map((c) => {
              const key = rowKey(c)
              const share = sharePercent(c.words, total)
              const isOpen = advice === key
              const adviceId = `context-advice-${key.replace(/[^a-z0-9]+/gi, '-')}`
              const builtin = isBuiltin(c.source)
              const name = builtin ? builtinTitle(c.source) : c.source
              const toggle = (
                <button
                  type="button"
                  className="button button-quiet context-advice-toggle"
                  aria-expanded={isOpen}
                  aria-controls={adviceId}
                  onClick={() => setAdvice(isOpen ? undefined : key)}
                >{isOpen ? 'Hide advice' : 'What can I do?'}</button>
              )
              return (
                <Fragment key={key}>
                  <tr className={`row-hover${isOpen ? ' is-advised' : ''}`}>
                    <th scope="row">
                      <span className="context-source">
                        {builtin
                          ? <><strong title={c.source}>{name}</strong> <span className="muted context-builtin">Built in</span></>
                          : <span className="context-source-path">{c.source}</span>}
                        <RowActions label={name}>
                          {builtin
                            ? <span className="editor-actions"><button type="button" className="button button-quiet" onClick={() => app.navigate({ view: 'settings', tab: 'builtins', entry: undefined })}>Change in Settings</button>{toggle}</span>
                            : <OpenInEditor path={root ? `${root}/${c.source}` : undefined} label={c.source} shownPath={c.source} compact trailing={toggle} />}
                        </RowActions>
                      </span>
                    </th>
                    <td className="context-type">{typeLabel(c.category)}</td>
                    <td className="context-number" title={`${c.bytes.toLocaleString()} bytes`}>{c.words.toLocaleString()}</td>
                    <td className="context-number context-share"><span className="context-share-track" aria-hidden="true"><span className="context-share-bar" style={{ width: `${share}%` }} /></span>{share}%</td>
                  </tr>
                  {isOpen && (
                    <tr className="context-advice-row">
                      <td colSpan={4} id={adviceId}>
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
      {rows.length > limit && (
        <button type="button" className="button button-quiet context-show-all" aria-expanded={all} onClick={() => setExpanded((prev) => {
          const next = new Set(prev)
          if (all) next.delete(load)
          else next.add(load)
          return next
        })}>{all ? `Show top ${limit}` : `Show all ${rows.length}`}</button>
      )}
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
