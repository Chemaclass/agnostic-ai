import { useContext, useId, useState } from 'react'
import type { ItemFinding } from '../contract'
import { OpenInEditor } from './EditorLink'
import { GoneFinding, isNewSinceRecheck, RecheckAction, RecheckNote, RecheckSection, useGoneFinding, useRechecks } from './Recheck'
import { findingFixPrompt } from '../model/finding-prompt'
import { findingEntryIds } from '../model/identity'
import { Badge, Code, CopyButton } from './ui'
import { useApp } from './app-context'
import { HEALTH_STATUS } from '../model/rail'
import { ConfigurationRoot, useEntry } from './section-context'

/** One owner's findings; pass `ids` when the list is a part of the owner's findings, so each keeps its link id. */
export function Findings({ findings, owner, place, orphanPlace, ids: givenIds }: { findings: ItemFinding[]; owner?: string; place: string; orphanPlace?: string; ids?: string[] }) {
  const ids = givenIds ?? findingEntryIds(findings, owner)
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
  const app = useApp()
  const root = useContext(ConfigurationRoot)
  const sectionId = useContext(RecheckSection)
  const fresh = isNewSinceRecheck(useRechecks(), entryId)
  const prompt = findingFixPrompt(f, root)
  const promptId = useId()
  const { linked, ...entry } = useEntry(entryId)
  const severity = f.severity === 'error' ? 'error' : 'warning'
  const section = sectionId && sectionId !== app.state.view ? app.overview?.sections.find((s) => s.id === sectionId)?.label : undefined
  return (
    <li {...entry} className={`finding finding-${severity === 'error' ? 'error' : 'warn'}${linked ? ' is-linked' : ''}`}>
      <div className="finding-body">
        <p className="finding-label">
          <span>{section ? `${HEALTH_STATUS[severity]} · ${section}` : HEALTH_STATUS[severity]}</span>
          {f.layer === 'global' && <Badge tone="info" title="On the global setup (~/.agnostic-ai), which reaches every project">global</Badge>}
          {fresh && <Badge tone="accent">New since recheck</Badge>}
        </p>
        <FindingMessage message={f.message} />
        {f.fix && <p className="finding-fix">{f.fix}</p>}
        {f.path ? (
          <div className="finding-source">
            <Code>{f.path}{f.line ? `:${f.line}` : ''}</Code>
            <OpenInEditor path={f.openPath} line={f.line} label={f.path} shownPath={f.path} />
          </div>
        ) : f.line ? <p className="finding-source">Line {f.line}</p> : null}
        <div className="finding-actions">
          <CopyButton text={prompt} label="Copy prompt" copiedLabel="Copied" className="button-primary finding-copy" />
          <RecheckAction id={entryId} finding={f} place={place} />
        </div>
        <RecheckNote id={entryId} finding={f} />
        <details className="finding-details">
          <summary>Details</summary>
          <div className="finding-evidence">
            {f.code && <p className="finding-code"><Code>{f.code}</Code><span>{f.source === 'doctor' ? 'Found by the setup check' : 'Found by the configuration check'}</span></p>}
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
