import { createContext, useContext, useRef, useState } from 'react'
import { moveSpec } from '../api'
import type { SectionItem, SpecMovePreview } from '../contract'
import { canMoveSpec, destinationNote, parseSpecMoved, specMoveLabel, specMovedText, specMoveRequest, type SpecMoved, type SpecMoveMode } from '../model/spec-move'
import { useApp } from './app-context'
import { useLevelName } from './EditAudience'
import { Code, CopyButton } from './ui'

/** Saves the notice shown after a copy or move, across the reload that follows it. */
export const SpecMovedSaver = createContext<((moved: SpecMoved) => void) | undefined>(undefined)

export function useSpecMovedNotice(root: string | undefined): [SpecMoved | undefined, (moved?: SpecMoved) => void] {
  const app = useApp()
  const key = `agnostic-ai.spec-moved:${root ?? ''}`
  const [moved, setMoved] = useState<SpecMoved | undefined>(() => {
    try {
      return parseSpecMoved(JSON.parse(sessionStorage.getItem(key) ?? 'null'))
    } catch {
      return undefined
    }
  })
  const save = (next?: SpecMoved) => {
    setMoved(next)
    try {
      if (next) sessionStorage.setItem(key, JSON.stringify(next))
      else sessionStorage.removeItem(key)
    } catch { /* Keep the notice until this page reloads. */ }
    if (next) app.reload(true)
  }
  return [moved, save]
}

export function SpecMoveAction({ item }: { item: SectionItem }) {
  const app = useApp()
  const level = useLevelName()
  const saved = useContext(SpecMovedSaver)
  const [preview, setPreview] = useState<SpecMovePreview>()
  const [overwrite, setOverwrite] = useState(false)
  const [pending, setPending] = useState<'preview' | SpecMoveMode>()
  const [error, setError] = useState<string>()
  const busy = useRef(false)

  if (app.source.kind !== 'api' || !saved || !canMoveSpec(item, level)) return null

  const run = async (action: 'preview' | SpecMoveMode) => {
    if (busy.current) return
    busy.current = true
    setPending(action)
    setError(undefined)
    try {
      if (action === 'preview') {
        setOverwrite(false)
        setPreview(await moveSpec(specMoveRequest(item, level, 'preview')))
        return
      }
      const done = await moveSpec(specMoveRequest(item, level, 'apply', action, overwrite, preview?.expected))
      setPreview(undefined)
      saved({ name: item.name, mode: action, destination: done.destination, syncCommands: done.syncCommands })
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : 'Could not copy this spec. Try again.')
    } finally {
      busy.current = false
      setPending(undefined)
    }
  }

  const label = specMoveLabel(level)
  return <div className="builtin-change">
    {!preview && <div className="builtin-change-controls">
      <button className="button button-quiet" type="button" disabled={!!pending} aria-label={`${label}: ${item.label}`} onClick={() => { void run('preview') }}>{label}</button>
      <span role="status" aria-live="polite">{pending === 'preview' ? 'Checking the destination…' : ''}</span>
    </div>}
    {error && <p className="builtin-change-error" role="alert">{error}</p>}
    {preview && <section className="builtin-change-preview" aria-label={`${label}: ${item.label}`} aria-busy={!!pending}>
      <h3>{label}: {item.label}</h3>
      <p>Write <Code>{preview.destination}</Code>. {destinationNote(preview)}</p>
      {preview.files.length > 1 && <p className="muted">The whole skill folder goes: {preview.files.map((file, index) => <span key={file}>{index > 0 && ', '}<Code>{file}</Code></span>)}.</p>}
      {preview.exists && <label className="global-onboarding-git">
        <input type="checkbox" checked={overwrite} disabled={!!pending} onChange={(event) => setOverwrite(event.target.checked)} />
        Replace it
      </label>}
      <p className="muted">Move also deletes <Code>{preview.source}</Code>.</p>
      <div className="builtin-change-actions">
        <button className="button button-primary" type="button" disabled={!!pending || (preview.exists && !overwrite)} onClick={() => { void run('copy') }}>{pending === 'copy' ? 'Copying…' : 'Copy'}</button>
        <button className="button" type="button" disabled={!!pending || (preview.exists && !overwrite)} onClick={() => { void run('move') }}>{pending === 'move' ? 'Moving…' : 'Move'}</button>
        <button className="button button-quiet" type="button" disabled={!!pending} onClick={() => { setPreview(undefined); setError(undefined) }}>Cancel</button>
      </div>
    </section>}
  </div>
}

export function SpecMovedNotice({ moved, dismiss }: { moved: SpecMoved; dismiss: () => void }) {
  return <div className="builtin-change builtin-change-controls" role="status">
    <span>{specMovedText(moved)} Run {moved.syncCommands.map((command, index) => <span key={command}>{index > 0 && ' and '}<Code>{command}</Code> <CopyButton text={command} label="Copy" ariaLabel={`Copy ${command}`} /></span>)} so your tools pick it up.</span>
    <button className="button button-quiet" type="button" onClick={dismiss}>Dismiss</button>
  </div>
}
