// How much instruction text one coding tool loads, and duplicated
// passages that could go. It fetches its own endpoint; a static snapshot
// has none, so it only says what it needs.

import { useEffect, useId, useRef, useState } from 'react'
import { loadContext, LoadError } from '../api'
import { signed } from '../model/context-breakdown'
import type { ContextOpportunity, ContextReport, ContextSize, ContextView } from '../contract'
import { contextReviewPrompt, LOAD_CONDITIONS, LOAD_LABELS, sizeText } from '../model/context-prompt'
import { contextStartupPrompt, explainCommand, startHere } from '../model/context-advice'
import { startupVerdict } from '../model/context-verdict'
import { DOC_TOPICS, documentationUrl } from '../model/documentation'
import { formatTime, plural } from '../model/format'
import { fieldEntryId } from '../model/identity'
import { modelLine } from '../model/model-line'
import { contextUsageAvailable } from '../model/url'
import { AppLink, useApp } from './app-context'
import { ErrorPanel } from './Banner'
import { ContextSources, LOADS, typeLabel, useSourcePick, type SourcePick } from './ContextSources'
import { OpenInEditor } from './EditorLink'
import { SectionHeading } from './SectionHeading'
import { Select, Badge, Code, CopyButton, Icon, toolName } from './ui'

const LONG_PASSAGE = 280
const FILE_DELAY_MS = 500

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
  const target = app.state.target
  const file = app.state.file ?? ''
  const [fileDraft, setFileDraft] = useState(file)
  const [nonce, setNonce] = useState(0)
  const [current, setCurrent] = useState<Snapshot>()
  const [before, setBefore] = useState<Snapshot>()
  const [error, setError] = useState<Error>()
  const [loading, setLoading] = useState(false)
  const status = useRef<HTMLParagraphElement>(null)
  const [statusHeight, setStatusHeight] = useState<number>()
  const results = useRef<HTMLDivElement>(null)
  const [resultsHeight, setResultsHeight] = useState<number>()
  const reachable = contextUsageAvailable(app.source)
  const hasView = useRef(false)
  const droppedStale = useRef(false)

  useEffect(() => {
    if (!reachable) return
    const controller = new AbortController()
    setLoading(true)
    setError(undefined)
    // A target or file left in the address (a removed tool, a stale link) can answer 400 or another tool. Load the default once.
    const dropStale = () => {
      if (hasView.current || droppedStale.current || (!target && !file)) return false
      droppedStale.current = true
      app.navigate({ target: undefined, file: undefined }, { replace: true })
      return true
    }
    loadContext(app.source, target, file, controller.signal)
      .then((loaded) => {
        if (controller.signal.aborted) return
        if (loaded.data.supported && target && loaded.data.target && loaded.data.target !== target && dropStale()) return
        hasView.current = true
        setCurrent({ view: loaded.data, file, at: new Date().toISOString() })
      })
      .catch((err: unknown) => {
        if ((err as Error).name === 'AbortError' || controller.signal.aborted) return
        const refused = err instanceof LoadError && err.status === 400
        if (!(refused && dropStale())) setError(err instanceof Error ? err : new Error('Could not load context size.'))
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [reachable, app.source, target, file, nonce])

  const view = current?.view
  const selected = target ?? view?.target ?? ''
  const takesFile = !!view?.fileTargets.includes(selected)
  const draftFile = takesFile ? fileDraft.trim() : ''
  const reserveResults = () => {
    setStatusHeight(status.current?.getBoundingClientRect().height)
    setResultsHeight(results.current?.getBoundingClientRect().height)
  }
  const go = (nextTarget: string, nextFile: string) => app.navigate({ target: nextTarget || undefined, file: nextFile || undefined }, { replace: true })

  useEffect(() => { setFileDraft((draft) => (draft.trim() === file ? draft : file)) }, [file])
  useEffect(() => {
    if (!takesFile || draftFile === file) return
    const timer = setTimeout(() => {
      reserveResults()
      setBefore(undefined)
      go(selected, draftFile)
    }, FILE_DELAY_MS)
    return () => clearTimeout(timer)
  }, [draftFile, file, takesFile, selected])

  const targets = view?.targets.length ? view.targets : selected ? [selected] : []
  const matches = (snapshot: Snapshot | undefined): snapshot is Snapshot => !!snapshot && snapshot.view.target === selected && snapshot.file === file
  // A report for another target or file, or one a failed request left behind, is never shown as current.
  const shown = !error && matches(current) ? current : undefined
  const report = shown?.view.supported ? shown.view.report : undefined
  const pending = !error && (loading || !!current && !matches(current))
  const comparable = report && matches(before) && before.at !== shown?.at && before.view.report
  const pick = useSourcePick(report)
  const refresh = () => {
    reserveResults()
    setBefore(matches(current) && current.file === draftFile ? current : undefined)
    go(selected, draftFile)
    setNonce((n) => n + 1)
  }

  const heading = <SectionHeading id={titleId}>Context size</SectionHeading>
  if (!reachable) {
    return (
      <section className="block context-usage" aria-labelledby={titleId}>
        {heading}
        <p className="muted">Context size needs a running agnostic-ai serve.</p>
      </section>
    )
  }

  return (
    <section className="block context-usage" aria-labelledby={titleId} aria-busy={pending || undefined}>
      <div className="context-usage-head">
        {heading}
        <p className="muted">How much text a coding tool reads from your setup files, at startup and later, in words and bytes.</p>
      </div>
      {view?.supported !== false && (
        <form className="context-usage-controls" onSubmit={(event) => { event.preventDefault(); refresh() }}>
          <div className="context-usage-field">
            <label htmlFor={selectId}>Coding tool</label>
            <Select
              id={selectId}
              value={selected}
              disabled={targets.length === 0}
              onChange={(event) => {
                reserveResults()
                setError(undefined)
                setBefore(undefined)
                const nextTakesFile = !!view?.fileTargets.includes(event.target.value)
                if (!nextTakesFile) setFileDraft('')
                go(event.target.value, nextTakesFile ? draftFile : '')
              }}
            >
              {targets.map((t) => <option key={t} value={t}>{toolName(t)}</option>)}
            </Select>
          </div>
          <div className="context-usage-field">
            <label htmlFor={fileId}>Check against a file <span className="muted">(optional)</span></label>
            <input id={fileId} type="text" value={fileDraft} placeholder="src/main.go" spellCheck={false} disabled={!takesFile} aria-describedby={`${fileId}-hint`} onChange={(event) => setFileDraft(event.target.value)} />
          </div>
          <button type="submit" className="button" disabled={pending} title="Read your setup files again">{pending ? 'Reading…' : 'Read again'}</button>
          <p id={`${fileId}-hint`} className="muted context-usage-file-hint">{takesFile ? 'Name a file to add the scoped rules that match it. Results update as you type.' : `This page cannot check a single file for ${selected ? toolName(selected) : 'this tool'}.`}</p>
        </form>
      )}
      <p ref={status} className="muted context-usage-status" role="status" style={{ minHeight: pending ? statusHeight : undefined }}>
        {pending ? `Checking context size${selected ? ` for ${toolName(selected)}` : ''}…` : shown ? `Checked ${formatTime(shown.at)}${shown.file ? ` for ${shown.file}` : ''}.` : ''}
      </p>
      <div ref={results} className="context-usage-results" style={{ minHeight: pending && !report ? resultsHeight : undefined }}>
        {pending && !report && <ReportPlaceholder />}
        {error && <ErrorPanel error={error} title="Could not check context size" onRetry={refresh} />}
        {shown && !shown.view.supported && <p>{shown.view.reason || 'Context size is not available for this configuration.'}</p>}
        {report && shown && (
          <>
            <Totals report={report} file={shown.file} budget={shown.view.startupWordBudget} />
            {comparable && <Comparison before={before.view.report!} now={report} />}
            <NextSteps report={report} view={shown.view} pick={pick} />
            <Method report={report} file={shown.file} takesFile={takesFile} />
            <ContextSources report={report} root={shown.view.root} takesFile={takesFile} budget={shown.view.startupWordBudget} pick={pick} />
            {report.opportunities.length > 0 && <Opportunities report={report} root={shown.view.root} />}
          </>
        )}
      </div>
    </section>
  )
}

function ReportPlaceholder() {
  return (
    <div className="context-usage-placeholder" aria-hidden="true">
      {[0, 1].map((column) => <div key={column}>
        <span className="context-placeholder-label" />
        <span className="context-placeholder-value" />
        <span className="context-placeholder-line" />
      </div>)}
      <div className="context-placeholder-check" />
    </div>
  )
}

function NextSteps({ report, view, pick }: { report: ContextReport; view: ContextView; pick: SourcePick }) {
  const id = useId()
  const largest = startHere(report.contributions, 1)[0]
  const command = explainCommand(report.target, report.file)
  const tool = toolName(report.target)
  return (
    <section className="context-usage-part context-next-part" aria-labelledby={id}>
      <h3 id={id}>Next</h3>
      <ol className="context-next">
        {largest && <li><span>See what the largest startup source does.</span> <button type="button" className="button button-quiet" onClick={() => pick.show(largest)}>Show <Code>{largest.source}</Code></button></li>}
        <li><span>Check the count in a terminal.</span> <CopyButton text={command} label="Copy command" className="button-quiet" ariaLabel={`Copy command: ${command}`} /></li>
        <li><span>Ask a coding agent to review what {tool} loads at startup.</span> <CopyButton text={contextStartupPrompt(report, view.root, view.startupWordBudget)} label="Copy prompt" className="button-quiet" ariaLabel={`Copy prompt to review startup for ${tool}`} /></li>
      </ol>
    </section>
  )
}

function Method({ report, file, takesFile }: { report: ContextReport; file: string; takesFile: boolean }) {
  return (
    <details className="context-usage-method">
      <summary>How this is counted</summary>
      <div className="context-usage-method-body">
        <p className="muted">{LOADS.map((load) => `${LOAD_LABELS[load]} text loads ${LOAD_CONDITIONS[load]}.`).join(' ')}</p>
        {takesFile && !file && <p className="muted">Scoped rules count as on demand until you name a file.</p>}
        {report.note && <p className="muted context-usage-note">{report.note}</p>}
        <ModelLine target={report.target} />
      </div>
    </details>
  )
}

function ModelLine({ target }: { target: string }) {
  const app = useApp()
  const line = modelLine(app.overview, target)
  if (!line) return null
  return (
    <p className="context-model">
      {line.text}{' '}
      <AppLink to={{ view: 'settings', tab: 'preferences', entry: fieldEntryId(line.field) }}>{line.unset ? 'Set them in Settings' : 'Change in Settings'}</AppLink>
    </p>
  )
}

function Totals({ report, file, budget }: { report: ContextReport; file: string; budget?: number }) {
  const fileScope = !!file || report.file_scope.words > 0 || report.file_scope.bytes > 0
  const verdict = startupVerdict(report.startup, budget)
  return (
    <div className="context-summary">
      <dl className="facts context-usage-totals">
        <Total title="Startup" description="At the start of every session" size={report.startup} />
        {fileScope && (
          <Total title="File-scoped, additional" description="Rules that match the named file" size={report.file_scope} />
        )}
        <Total title="On demand" description="When the tool opens it" size={report.on_demand} />
      </dl>
      {verdict.text && (
        <p className={`context-verdict is-${verdict.state}`}>
          {verdict.state === 'over' && <Icon name="warning" />}
          <span>{verdict.text} <a href={documentationUrl(DOC_TOPICS.budget.path)} target="_blank" rel="noopener noreferrer">Change budget</a></span>
        </p>
      )}
    </div>
  )
}

function Total({ title, description, size }: { title: string; description: string; size: ContextSize }) {
  return (
    <div>
      <dt>{title}</dt>
      <dd>
        <span className="context-total-words">{size.words.toLocaleString()} <span>{size.words === 1 ? 'word' : 'words'}</span></span>
        <span className="sr-only">, </span>
        <span className="muted context-total-bytes">{size.bytes.toLocaleString()} {size.bytes === 1 ? 'byte' : 'bytes'}</span>
        <span className="muted context-total-description">{description}</span>
      </dd>
    </div>
  )
}

function Comparison({ before, now }: { before: ContextReport; now: ContextReport }) {
  const line = (r: ContextReport) => `${sizeText(r.startup)} at startup, ${plural(r.opportunities.length, 'duplicated passage')}.`
  const changes: [string, ContextSize, ContextSize][] = [
    ['Startup', before.startup, now.startup],
    ['File-scoped', before.file_scope, now.file_scope],
    ['On demand', before.on_demand, now.on_demand],
  ]
  return (
    <dl className="context-usage-compare">
      <div><dt>Before</dt><dd>{line(before)}</dd></div>
      <div><dt>Now</dt><dd>{line(now)}</dd></div>
      <div><dt>Change</dt><dd>{changes.map(([label, was, is]) => `${label} ${signed(is.words - was.words)}${is.words === was.words ? '' : ' words'}`).join(', ')}.</dd></div>
    </dl>
  )
}

function Opportunities({ report, root }: { report: ContextReport; root?: string }) {
  const id = useId()
  return (
    <div className="context-usage-part">
      <h3 id={id}>Duplicated passages</h3>
      <ul className="item-list" aria-labelledby={id}>
        {report.opportunities.map((o, index) => <Opportunity key={`${o.load}-${index}`} opportunity={o} root={root} />)}
      </ul>
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
          <span className="muted item-description">Could drop {sizeText(o.reduction)} from {LOAD_LABELS[o.load].toLowerCase()} content for {toolName(o.target)}. Each copy is {sizeText(o.size)}.</span>
        </div>
        <div className="item-meta">
          <Badge tone="neutral">{LOAD_LABELS[o.load]}</Badge>
          <Badge tone="neutral">{toolName(o.target)}</Badge>
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
            {s.category && <span className="muted"> {typeLabel(s.category)}</span>}
            <OpenInEditor path={root ? `${root}/${s.source}` : undefined} label={s.source} shownPath={s.source} compact />
          </li>
        ))}
      </ul>
      <div className="finding-actions">
        <CopyButton text={contextReviewPrompt(o, root)} label="Copy prompt" ariaLabel={`Copy prompt to review the passage in ${o.sources.map((s) => s.source).join(' and ')}`} />
      </div>
    </li>
  )
}
