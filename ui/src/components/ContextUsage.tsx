// How much instruction text one coding tool loads, and duplicated
// passages that could go. The page loads the report (context-report.tsx);
// a static snapshot has none, so it only says what it needs.

import { useEffect, useId, useRef, useState } from 'react'
import { signed } from '../model/context-breakdown'
import type { ContextOpportunity, ContextReport, ContextSize } from '../contract'
import { contextReviewPrompt, LOAD_CONDITIONS, LOAD_LABELS, sizeText } from '../model/context-prompt'
import { explainCommand } from '../model/context-advice'
import { startupVerdict } from '../model/context-verdict'
import { DOC_TOPICS, documentationUrl } from '../model/documentation'
import { formatTime, plural } from '../model/format'
import { fieldEntryId } from '../model/identity'
import { modelLine } from '../model/model-line'
import { AppLink, useApp } from './app-context'
import { ErrorPanel } from './Banner'
import { ContextSources, LOADS, typeLabel, useSourcePick } from './ContextSources'
import { useContextReport, type ContextSnapshot } from './context-report'
import { OpenInEditor } from './EditorLink'
import { Select, Badge, Code, CopyButton, Icon, toolName } from './ui'

const LONG_PASSAGE = 280
const FILE_DELAY_MS = 500

export function ContextUsage() {
  const app = useApp()
  const titleId = useId()
  const selectId = useId()
  const fileId = useId()
  const target = app.state.target
  const file = app.state.file ?? ''
  const { reachable, current, error, loading, reload } = useContextReport()
  const [fileDraft, setFileDraft] = useState(file)
  const [fileOpen, setFileOpen] = useState(!!file)
  const [before, setBefore] = useState<ContextSnapshot>()
  const status = useRef<HTMLSpanElement>(null)
  const [statusHeight, setStatusHeight] = useState<number>()
  const results = useRef<HTMLDivElement>(null)
  const [resultsHeight, setResultsHeight] = useState<number>()

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
  useEffect(() => { if (file) setFileOpen(true) }, [file])
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
  const matches = (snapshot: ContextSnapshot | undefined): snapshot is ContextSnapshot => !!snapshot && snapshot.view.target === selected && snapshot.file === file
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
    reload()
  }

  const heading = <h2 id={titleId} className="sr-only">Context size</h2>
  if (!reachable) {
    return (
      <section className="block context-usage" aria-labelledby={titleId}>
        {heading}
        <p className="muted">Context size needs agnostic-ai view running.</p>
      </section>
    )
  }

  const supported = view?.supported !== false
  return (
    <section className="block context-usage" aria-labelledby={titleId} aria-busy={pending || undefined}>
      {heading}
      <div ref={results} className="context-usage-results" style={{ minHeight: pending && !report ? resultsHeight : undefined }}>
        <div className="context-usage-top">
          <div className="context-usage-figures">
            {pending && !report && <ReportPlaceholder />}
            {error && <ErrorPanel error={error} title="Could not check context size" onRetry={refresh} />}
            {shown && !shown.view.supported && <p>{shown.view.reason || 'Context size is not available for this configuration.'}</p>}
            {report && shown && <Totals report={report} file={shown.file} budget={shown.view.startupWordBudget} />}
          </div>
          {supported && (
            <form className="context-usage-controls" onSubmit={(event) => { event.preventDefault(); refresh() }}>
              <label htmlFor={selectId}>Counted for</label>
              <Select
                id={selectId}
                value={selected}
                disabled={targets.length === 0}
                onChange={(event) => {
                  reserveResults()
                  setBefore(undefined)
                  const nextTakesFile = !!view?.fileTargets.includes(event.target.value)
                  if (!nextTakesFile) setFileDraft('')
                  go(event.target.value, nextTakesFile ? draftFile : '')
                }}
              >
                {targets.map((t) => <option key={t} value={t}>{toolName(t)}</option>)}
              </Select>
              <p className="context-usage-refresh">
                <span ref={status} className="muted context-usage-status" role="status" style={{ minHeight: pending ? statusHeight : undefined }}>
                  {pending ? `Checking context size${selected ? ` for ${toolName(selected)}` : ''}…` : shown ? `Checked ${formatTime(shown.at)}${shown.file ? ` for ${shown.file}` : ''}.` : ''}
                </span>
                <button type="submit" className="button button-quiet" disabled={pending} title="Read your setup files again">{pending ? 'Reading…' : 'Read again'}</button>
              </p>
            </form>
          )}
        </div>
        {report && shown && (
          <>
            {comparable && <Comparison before={before.view.report!} now={report} />}
            <ContextSources report={report} root={shown.view.root} takesFile={takesFile} budget={shown.view.startupWordBudget} pick={pick} />
            {report.opportunities.length > 0 && <Opportunities report={report} root={shown.view.root} />}
          </>
        )}
      </div>
      {supported && (
        <details className="context-usage-more context-usage-file" open={fileOpen} onToggle={(event) => setFileOpen(event.currentTarget.open)}>
          <summary>Check one file against these rules</summary>
          <form className="context-usage-file-form" onSubmit={(event) => { event.preventDefault(); refresh() }}>
            <label htmlFor={fileId}>File path</label>
            <input id={fileId} type="text" value={fileDraft} placeholder="src/main.go" spellCheck={false} disabled={!takesFile} aria-describedby={`${fileId}-hint`} onChange={(event) => setFileDraft(event.target.value)} />
            <p id={`${fileId}-hint`} className="muted context-usage-file-hint">{takesFile ? 'Name a file to add the scoped rules that match it. Results update as you type.' : `This page cannot check a single file for ${selected ? toolName(selected) : 'this tool'}.`}</p>
          </form>
        </details>
      )}
      {/* While another tool loads, the last report keeps this disclosure in place so the page does not jump. */}
      {(shown ?? (pending ? current : undefined))?.view.report && <Method report={(shown ?? current)!.view.report!} file={(shown ?? current)!.file} takesFile={takesFile} />}
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

function Method({ report, file, takesFile }: { report: ContextReport; file: string; takesFile: boolean }) {
  const command = explainCommand(report.target, report.file)
  return (
    <details className="context-usage-more context-usage-method">
      <summary>How this is counted</summary>
      <div className="context-usage-method-body">
        <p className="muted">{LOADS.map((load) => `${LOAD_LABELS[load]} text loads ${LOAD_CONDITIONS[load]}.`).join(' ')}</p>
        <p className="muted">In bytes: {report.startup.bytes.toLocaleString()} at startup{file ? `, ${report.file_scope.bytes.toLocaleString()} for ${file}` : ''}, {report.on_demand.bytes.toLocaleString()} on demand.</p>
        {takesFile && !file && <p className="muted">Scoped rules count as on demand until you name a file.</p>}
        {report.note && <p className="muted context-usage-note">{report.note}</p>}
        <ModelLine target={report.target} />
        <p className="context-usage-command"><span className="muted">Check the count in a terminal:</span> <Code>{command}</Code> <CopyButton text={command} label="Copy command" className="button-quiet" ariaLabel={`Copy command: ${command}`} /></p>
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
  return (
    <div className="context-summary">
      <dl className="context-usage-totals">
        <Total title="At start" description="Every session, before the first prompt" size={report.startup} />
        {fileScope && <Total title="For this file" description="Rules that match the named file" size={report.file_scope} />}
        <Total title="On demand" description="When a skill, agent or matching rule loads" size={report.on_demand} />
      </dl>
      <Budget startup={report.startup} budget={budget} />
    </div>
  )
}

function Budget({ startup, budget }: { startup: ContextSize; budget?: number }) {
  const verdict = startupVerdict(startup, budget)
  if (!budget || verdict.state === 'unknown') return null
  const over = verdict.state === 'over'
  const fill = Math.min(100, Math.round((startup.words / budget) * 100))
  return (
    <div className={`context-budget${over ? ' is-over' : ''}`}>
      <span className="context-budget-track" aria-hidden="true"><span className="context-budget-bar" style={{ width: `${fill}%` }} /></span>
      <p className="context-budget-line">
        <span>
          {over && <Icon name="warning" />}
          {startup.words.toLocaleString('en-US')} of {budget.toLocaleString('en-US')} words at startup{over ? `, ${(startup.words - budget).toLocaleString('en-US')} over` : ''}
        </span>
        <a href={documentationUrl(DOC_TOPICS.budget.path)} target="_blank" rel="noopener noreferrer">Change budget</a>
      </p>
    </div>
  )
}

function Total({ title, description, size }: { title: string; description: string; size: ContextSize }) {
  return (
    <div>
      <dt>{title}</dt>
      <dd>
        <span className="context-total-words">{size.words.toLocaleString()} <span>{size.words === 1 ? 'word' : 'words'}</span></span>
        <span className="sr-only">, {size.bytes.toLocaleString()} {size.bytes === 1 ? 'byte' : 'bytes'}, </span>
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
