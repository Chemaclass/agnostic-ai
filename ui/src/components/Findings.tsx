import { useContext, useId, useState } from 'react'
import type { ItemFinding } from '../contract'
import { OpenInEditor } from './EditorLink'
import { GoneFinding, isNewSinceRecheck, RecheckAction, RecheckNote, RecheckSection, useGoneFinding, useRechecks } from './Recheck'
import { findingFixPrompt } from '../model/finding-prompt'
import { findingHasRawMessage, findingTitle } from '../model/finding-title'
import { findingEntryIds } from '../model/identity'
import { Badge, Code, CopyButton } from './ui'
import { AppLink, useApp } from './app-context'
import type { ViewId } from '../model/url'
import { HEALTH_STATUS } from '../model/rail'
import { ConfigurationRoot, useEntry } from './section-context'
import { PickBox } from './pick'

/** One owner's findings; pass `ids` when the list is a part of the owner's findings, so each keeps its link id. */
export function Findings({ findings, owner, place, orphanPlace, ids: givenIds }: { findings: ItemFinding[]; owner?: string; place: string; orphanPlace?: string; ids?: string[] }) {
  const ids = givenIds ?? findingEntryIds(findings, owner)
  const gone = useGoneFinding(place, ids, orphanPlace)
  if (!findings.length && !gone) return null
  const titles = findings.map(findingTitle)
  return (
    <ul className="findings">
      {findings.map((f, i) => (
        <Finding key={ids[i]} finding={f} entryId={ids[i]} place={place} sharesTitle={titles.some((title, j) => j !== i && title === titles[i] && findings[j].path === f.path)} />
      ))}
      {gone && (gone.result.state === 'incomplete' ? <Finding key={gone.id} finding={gone.finding} entryId={gone.id} place={place} pickable={false} /> : <GoneFinding rechecked={gone} />)}
    </ul>
  )
}

// A title another finding in the same file shares gets the check's own message under it, so the two read apart.
function Finding({ finding: f, entryId, place, pickable = true, sharesTitle = false }: { finding: ItemFinding; entryId: string; place: string; pickable?: boolean; sharesTitle?: boolean }) {
  const app = useApp()
  const root = useContext(ConfigurationRoot)
  const sectionId = useContext(RecheckSection)
  const fresh = isNewSinceRecheck(useRechecks(), entryId)
  const prompt = findingFixPrompt(f, root)
  const promptId = useId()
  const { linked, ...entry } = useEntry(entryId)
  const severity = f.severity === 'error' ? 'error' : 'warning'
  const owner = sectionId && sectionId !== app.state.view ? app.overview?.sections.find((s) => s.id === sectionId) : undefined
  const [open, setOpen] = useState(false)
  const detailsId = useId()
  return (
    <li {...entry} className={`finding finding-${severity === 'error' ? 'error' : 'warn'}${linked ? ' is-linked' : ''}`}>
      <div className="finding-body">
        <div className="finding-text">
          <p className="finding-label">
            {pickable && <PickBox id={entryId} name={findingTitle(f)} />}
            <span>{HEALTH_STATUS[severity]}{owner && <> · <AppLink className="finding-section" to={{ view: owner.id as ViewId, entry: entryId }}>{owner.label}</AppLink></>}</span>
            {f.layer === 'global' && <Badge tone="info" title="On the global setup (~/.agnostic-ai), which reaches every project">global</Badge>}
            {fresh && <Badge tone="accent">New since recheck</Badge>}
          </p>
          <FindingMessage message={findingTitle(f)} />
          {sharesTitle && findingHasRawMessage(f) && <p className="finding-variant">{f.message}</p>}
          {f.path ? <p className="finding-source"><Code>{f.path}{f.line ? `:${f.line}` : ''}</Code></p> : f.line ? <p className="finding-source">Line {f.line}</p> : null}
        </div>
        <div className="finding-actions">
          <CopyButton text={prompt} label="Copy prompt" copiedLabel="Copied" className="button-primary finding-copy" />
          <button type="button" className="link-button finding-details-toggle" aria-expanded={open} aria-controls={detailsId} onClick={() => setOpen(!open)}>Details</button>
        </div>
        <RecheckNote id={entryId} finding={f} />
        <div id={detailsId} className="finding-details" hidden={!open}>
          {(f.path || app.source.kind === 'api') && (
            <div className="finding-detail-actions">
              {f.path && <OpenInEditor path={f.openPath} line={f.line} label={f.path} shownPath={f.path} />}
              <RecheckAction id={entryId} finding={f} place={place} />
            </div>
          )}
          <div className="finding-evidence">
            {f.code && <p className="finding-code"><Code>{f.code}</Code><span>{f.source === 'doctor' ? 'Found by the setup check' : 'Found by the configuration check'}</span></p>}
            {findingHasRawMessage(f) && <p><strong>Check message</strong><span className="finding-raw">{f.message}</span></p>}
            {f.fix && <p><strong>Suggested fix</strong><span className="finding-raw">{f.fix}</span></p>}
            {f.explanation && <p><strong>Why this appears</strong><span>{f.explanation}</span></p>}
            <div className="finding-prompt">
              <label htmlFor={promptId}>Fix prompt</label>
              <p id={`${promptId}-hint`}>Paste this prompt into your coding agent.</p>
              <textarea id={promptId} aria-describedby={`${promptId}-hint`} value={prompt} readOnly rows={12} spellCheck={false} />
            </div>
          </div>
        </div>
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
