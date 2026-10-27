// How much instruction text one coding tool loads, and duplicated
// passages that could go. It fetches its own endpoint; a static snapshot
// has none, so it only says what it needs.

import { useEffect, useId, useState } from 'react'
import { loadContext } from '../api'
import type { ContextContribution, ContextLoad, ContextOpportunity, ContextReport, ContextView } from '../contract'
import { contextReviewPrompt, LOAD_CONDITIONS, LOAD_LABELS, sizeText } from '../model/context-prompt'
import { formatTime, plural } from '../model/format'
import { useApp } from './app-context'
import { ErrorPanel } from './Banner'
import { OpenInEditor } from './EditorLink'
import { Badge, Code, CopyButton } from './ui'

const LOADS: ContextLoad[] = ['startup', 'file-scope', 'on-demand']
const LONG_PASSAGE = 280

interface Snapshot {
  view: ContextView
  /** The file this report was asked for; '' when none. */
  file: string
  at: string
}

export function ContextUsage() {
  const app = useApp()
  const titleId = useId()
  const selectId = useId()
  const fileId = useId()
  const [target, setTarget] = useState<string>()
  const [file, setFile] = useState('')
  const [fileDraft, setFileDraft] = useState('')
  const [nonce, setNonce] = useState(0)
  const [current, setCurrent] = useState<Snapshot>()
  const [before, setBefore] = useState<Snapshot>()
  const [error, setError] = useState<Error>()
  const [loading, setLoading] = useState(false)
  const reachable = app.source.kind === 'api' || app.source.kind === 'mock'

  useEffect(() => {
    if (!reachable) return
    const controller = new AbortController()
    setLoading(true)
    setError(undefined)
    loadContext(app.source, target, file, controller.signal)
      .then((loaded) => setCurrent({ view: loaded.data, file, at: new Date().toISOString() }))
      .catch((err: unknown) => {
        if ((err as Error).name !== 'AbortError') setError(err instanceof Error ? err : new Error('Could not load context size.'))
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [reachable, app.source, target, file, nonce])

  const heading = <h2 id={titleId}>Context size</h2>
  if (!reachable) {
    return (
      <section className="block context-usage" aria-labelledby={titleId}>
        {heading}
        <p className="muted">Context size needs a running agnostic-ai serve.</p>
      </section>
    )
  }

  const view = current?.view
  const selected = target ?? view?.target ?? ''
  const targets = view?.targets.length ? view.targets : selected ? [selected] : []
  const takesFile = !!view?.fileTargets.includes(selected)
  const matches = (snapshot: Snapshot | undefined): snapshot is Snapshot => !!snapshot && snapshot.view.target === selected && snapshot.file === file
  const refresh = () => {
    const nextFile = takesFile ? fileDraft.trim() : ''

    setBefore(matches(current) && current.file === nextFile ? current : undefined)
    if (selected) setTarget(selected)
    setFile(nextFile)
    setNonce((n) => n + 1)
  }
  // A report for another target or file, or one a failed request left behind, is never shown as current.
  const shown = !error && matches(current) ? current : undefined
  const report = shown?.view.supported ? shown.view.report : undefined
  const comparable = report && matches(before) && before.at !== shown?.at && before.view.report

  return (
    <section className="block context-usage" aria-labelledby={titleId} aria-busy={loading || undefined}>
      <div className="context-usage-head">
        {heading}
        {view?.supported !== false && (
          <form className="context-usage-controls" onSubmit={(event) => { event.preventDefault(); refresh() }}>
            <label htmlFor={selectId}>Coding tool</label>
            <select
              id={selectId}
              value={selected}
              disabled={targets.length === 0}
              onChange={(event) => {
                setBefore(undefined)
                setTarget(event.target.value)
                if (!view?.fileTargets.includes(event.target.value)) setFile('')
              }}
            >
              {targets.map((t) => <option key={t} value={t}>{t}</option>)}
            </select>
            <label htmlFor={fileId}>Working on file</label>
            <input id={fileId} type="text" value={fileDraft} placeholder="src/main.go" spellCheck={false} disabled={!takesFile} title={takesFile ? undefined : `${selected || 'This tool'} has no file-scoped rules to check.`} onChange={(event) => setFileDraft(event.target.value)} />
            <button type="submit" className="button" disabled={loading}>{loading ? 'Refreshing…' : 'Refresh'}</button>
          </form>
        )}
      </div>
      <p className="muted context-usage-status" role="status">
        {loading ? 'Checking context size…' : shown ? `Checked ${formatTime(shown.at)}${shown.file ? ` for ${shown.file}` : ''}.` : ''}
      </p>
      {error && <ErrorPanel error={error} />}
      {shown && !shown.view.supported && <p>{shown.view.reason || 'Context size is not available for this configuration.'}</p>}
      {report && shown && (
        <>
          <Totals report={report} file={shown.file} />
          {report.note && <p className="muted context-usage-note">{report.note}</p>}
          {comparable && <Comparison before={before.view.report!} now={report} />}
          <Opportunities report={report} root={shown.view.root} />
          <Contributions contributions={report.contributions} />
        </>
      )}
    </section>
  )
}

function Totals({ report, file }: { report: ContextReport; file: string }) {
  const fileScope = !!file || report.file_scope.words > 0 || report.file_scope.bytes > 0
  return (
    <>
      <dl className="facts context-usage-totals">
        <div>
          <dt>Startup</dt>
          <dd>{sizeText(report.startup)}</dd>
        </div>
        {fileScope && (
          <div>
            <dt>File-scoped, additional</dt>
            <dd>{sizeText(report.file_scope)}</dd>
          </div>
        )}
        <div>
          <dt>On demand</dt>
          <dd>{sizeText(report.on_demand)}</dd>
        </div>
      </dl>
      {!file && <p className="muted context-usage-note">Scoped rules count as on demand until you name a file.</p>}
    </>
  )
}

function Comparison({ before, now }: { before: ContextReport; now: ContextReport }) {
  const line = (r: ContextReport) => `${sizeText(r.startup)} at startup, ${plural(r.opportunities.length, 'duplicated passage')}.`
  return (
    <dl className="context-usage-compare">
      <div><dt>Before refresh</dt><dd>{line(before)}</dd></div>
      <div><dt>Now</dt><dd>{line(now)}</dd></div>
    </dl>
  )
}

function Opportunities({ report, root }: { report: ContextReport; root?: string }) {
  const id = useId()
  return (
    <div className="context-usage-part">
      <h3 id={id}>Duplicated passages</h3>
      {report.opportunities.length === 0
        ? <p className="muted">No duplicated passages found for {report.target}.</p>
        : (
          <ul className="item-list" aria-labelledby={id}>
            {report.opportunities.map((o, index) => <Opportunity key={`${o.load}-${index}`} opportunity={o} root={root} />)}
          </ul>
        )}
    </div>
  )
}

function Opportunity({ opportunity: o, root }: { opportunity: ContextOpportunity; root?: string }) {
  const [open, setOpen] = useState(false)
  const passageId = useId()
  const long = o.passage.length > LONG_PASSAGE
  return (
    <li className="item-row context-opportunity">
      <div className="item-heading">
        <div className="item-main">
          <strong>{o.copies} copies load {LOAD_CONDITIONS[o.load]}</strong>
          <span className="muted item-description">Could drop {sizeText(o.reduction)} from {LOAD_LABELS[o.load].toLowerCase()} content for {o.target}. Each copy is {sizeText(o.size)}.</span>
        </div>
        <div className="item-meta">
          <Badge tone="neutral">{LOAD_LABELS[o.load]}</Badge>
          <Badge tone="neutral">{o.target}</Badge>
        </div>
      </div>
      <blockquote id={passageId} className={`context-passage${long && !open ? ' is-clamped' : ''}`}>{o.passage}</blockquote>
      {long && (
        <button type="button" className="button button-quiet context-passage-toggle" aria-expanded={open} aria-controls={passageId} onClick={() => setOpen(!open)}>
          {open ? 'Show less' : 'Show full passage'}
        </button>
      )}
      {o.suggestion && <p className="context-suggestion">{o.suggestion}</p>}
      <ul className="context-sources" aria-label="Sources">
        {o.sources.map((s) => (
          <li key={`${s.source}-${s.category}`}>
            <Code>{s.source}</Code>
            {s.category && <span className="muted"> {s.category}</span>}
            <OpenInEditor path={root ? `${root}/${s.source}` : undefined} label={s.source} shownPath={s.source} compact />
          </li>
        ))}
      </ul>
      <div className="finding-actions">
        <CopyButton text={contextReviewPrompt(o, root)} label="Copy review prompt" ariaLabel={`Copy review prompt for the passage in ${o.sources.map((s) => s.source).join(' and ')}`} />
      </div>
    </li>
  )
}

function Contributions({ contributions }: { contributions: ContextContribution[] }) {
  if (contributions.length === 0) return null
  return (
    <details className="context-usage-part context-contributions">
      <summary>{plural(contributions.length, 'source')}, largest first</summary>
      {LOADS.map((load) => {
        const rows = contributions.filter((c) => c.load === load)
        if (rows.length === 0) return null
        const id = `context-contributions-${load}`
        return (
          <div key={load} className="table-wrap">
            <table className="table" aria-labelledby={id}>
              <caption id={id}>{LOAD_LABELS[load]}: loads {LOAD_CONDITIONS[load]}</caption>
              <thead>
                <tr>
                  <th scope="col">Source</th>
                  <th scope="col">Kind</th>
                  <th scope="col">Words</th>
                  <th scope="col">Bytes</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((c, index) => (
                  <tr key={`${c.load}-${index}-${c.source}-${c.category}`}>
                    <th scope="row"><Code>{c.source}</Code></th>
                    <td>{c.category}</td>
                    <td className="context-number">{c.words}</td>
                    <td className="context-number">{c.bytes}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )
      })}
    </details>
  )
}
