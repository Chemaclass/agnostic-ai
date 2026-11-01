import { useEffect, useId, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import { EditError, postEdit } from '../api'
import { API, type ToolCatalog, type ToolChoice } from '../contract'
import { filterTools, offConsequence, saveHint, toolCount, toolOn } from '../model/coding-tools'
import { documentationUrl, targetDocPath } from '../model/documentation'
import { useApp } from './app-context'
import { ViewPersonalButton, useViewAudience, type EditAudience } from './EditAudience'
import { SectionHeading } from './SectionHeading'
import { Badge, Code, Switch, Toggle, ToolAvatar } from './ui'

interface Saved { file?: string }

const emptyCatalog: ToolCatalog = { source: 'default', items: [] }

export function CodingTools({ audienceControl }: { audienceControl?: ReactNode }) {
  const app = useApp()
  const catalog = app.overview?.tools ?? emptyCatalog
  const global = app.overview?.global === true
  const audience = useViewAudience() ?? 'shared'
  const pageQuery = app.state.q ?? ''
  const [query, setQuery] = useState(pageQuery)
  useEffect(() => setQuery(pageQuery), [pageQuery])
  const [onlyOn, setOnlyOn] = useState(false)
  const [saved, setSaved] = useState<Saved | undefined>()
  const filterId = useId()
  const live = app.source.kind === 'api'
  const sharedBlocked = catalog.source === 'personal' && audience === 'shared'
  const visible = filterTools(catalog.items, query, onlyOn, audience)
  const count = toolCount(catalog.items, audience)
  const command = global ? 'agnostic-ai sync --global' : 'agnostic-ai sync'
  return (
    <section className="block coding-tools" aria-labelledby="settings-targets">
      <SectionHeading topic="targets" id="settings-targets" focusable>
        Coding tools <span className="muted kind-count">{count.on} of {count.total} on</span>
        {catalog.source === 'personal' && audience !== 'shared' && <Badge tone="accent" title="Your personal config replaces the shared list">personal</Badge>}
      </SectionHeading>
      <p className="muted">Each tool you turn on gets its own files from sync. Turn one off and the next sync stops writing for it.</p>
      {live && audienceControl}
      {live && sharedBlocked && <div className="audience-notice"><p>These are the shared defaults. Your personal list of coding tools replaces them for you. Reset that list before changing the shared one.</p><ViewPersonalButton /></div>}
      {live && !sharedBlocked && <p className="muted coding-tools-hint">{saveHint(catalog, audience, global)}</p>}
      {live && audience === 'personal' && catalog.source !== 'personal' && <p className="muted">Inherited from shared. Changing a tool creates your personal list.</p>}
      {live && audience === 'personal' && catalog.source === 'personal' && <ResetTools expected={catalog.items.filter((tool) => tool.enabled).map((tool) => tool.target)} onSaved={setSaved} />}
      {!live && <p className="muted">Switches need a running <Code>agnostic-ai serve</Code>; this view only shows the list.</p>}
      <div className="coding-tools-filter" role="search" aria-label="Filter coding tools">
        <label htmlFor={filterId}>Filter tools</label>
        <input id={filterId} type="search" value={query} onChange={(event) => { setQuery(event.target.value); setSaved(undefined) }} placeholder="Name or vendor" />
        <label className="coding-tools-only-on"><input type="checkbox" checked={onlyOn} onChange={(event) => { setOnlyOn(event.target.checked); setSaved(undefined) }} /> On only</label>
      </div>
      <p className="muted coding-tools-status" role="status" aria-live="polite">
        {saved ? <>Saved{saved.file ? <> to <Code>{saved.file}</Code></> : ''}. Run <Code>{command}</Code> to apply the change.{global ? '' : ' A running sync --watch applies it on its own.'}</> : `${visible.length} of ${count.total} tools shown.`}
      </p>
      {catalog.items.length === 0 ? <p className="muted">No coding tools listed.</p> : visible.length === 0 ? <p className="muted">No tool matches.</p> : (
        <ul className="coding-tools-list">
          {visible.map((tool) => <ToolRow key={tool.target} tool={tool} on={toolOn(tool, audience)} audience={audience} global={global} live={live && !sharedBlocked} onSaved={setSaved} />)}
        </ul>
      )}
    </section>
  )
}

function ToolRow({ tool, on, audience, global, live, onSaved }: { tool: ToolChoice; on: boolean; audience: EditAudience; global: boolean; live: boolean; onSaved: (saved: Saved) => void }) {
  const docs = documentationUrl(targetDocPath(tool.target))
  return (
    <li className="coding-tool" data-target={tool.target}>
      <ToolAvatar target={tool.target} label={tool.label} />
      <div className="coding-tool-text">
        <span className="coding-tool-name">{tool.label} <code className="muted">{tool.target}</code></span>
        <span className="muted coding-tool-description">
          {tool.description}
          {tool.detected && !on && <Badge tone="info" muted title="The project already holds this tool's own files">found in project</Badge>}
          {tool.local && audience === 'personal' && <Badge tone="accent" muted title="Your personal list decides this tool">personal</Badge>}
        </span>
      </div>
      <a className="coding-tool-docs" href={docs} target="_blank" rel="noopener noreferrer" aria-label={`${tool.label} docs`}>Docs</a>
      {live ? <ToolSwitch tool={tool} on={on} audience={audience} global={global} onSaved={onSaved} /> : <Switch on={on} />}
    </li>
  )
}

/** Saves one switch through /api/edit/targets; turning a tool off asks first and names what the next sync does. */
function ToolSwitch({ tool, on, audience, global, onSaved }: { tool: ToolChoice; on: boolean; audience: EditAudience; global: boolean; onSaved: (saved: Saved) => void }) {
  const app = useApp()
  const [pending, setPending] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [error, setError] = useState<EditError | undefined>()
  const [choice, setChoice] = useState<boolean | undefined>()
  const reloadBasis = useRef<object | undefined>(undefined)
  const waitingForReload = useRef(false)
  const confirmButton = useRef<HTMLButtonElement>(null)
  const toggleButton = useRef<HTMLButtonElement>(null)
  const refocus = useRef(false)
  useEffect(() => {
    if (waitingForReload.current && app.overviewRevision !== reloadBasis.current) {
      waitingForReload.current = false
      setChoice(undefined)
      setError(undefined)
    }
  }, [app.overviewRevision])
  useEffect(() => {
    if (confirming) confirmButton.current?.focus()
    else if (refocus.current && !pending) {
      refocus.current = false
      toggleButton.current?.focus()
    }
  }, [confirming, pending])
  const cancel = () => {
    refocus.current = true
    setConfirming(false)
  }
  const save = async (enabled: boolean) => {
    refocus.current = true
    setPending(true)
    setError(undefined)
    setConfirming(false)
    try {
      const reply = await postEdit(API.editTargets, { audience, name: tool.target, enabled, expected: on })
      onSaved(reply)
      app.reload(true)
    } catch (err) {
      const failure = err instanceof EditError ? err : new EditError(String(err))
      if (failure.status === 409) setChoice(enabled)
      setError(failure)
    } finally {
      setPending(false)
    }
  }
  const toggle = () => {
    if (!(choice ?? on)) void save(true)
    else setConfirming(true)
  }
  return (
    <span className="edit-switch coding-tool-switch">
      <Toggle ref={toggleButton} on={choice ?? on} label={`${tool.label} coding tool`} pending={pending} disabled={choice !== undefined || confirming} onToggle={toggle} />
      {confirming && (
        <span className="coding-tool-confirm" role="group" aria-label={`Turn off ${tool.label}`} onKeyDown={(event: KeyboardEvent) => { if (event.key === 'Escape') cancel() }}>
          <span>{offConsequence(tool.label, global)}</span>
          <button ref={confirmButton} type="button" className="button button-quiet" onClick={() => { void save(false) }}>Turn off</button>
          <button type="button" className="button button-quiet" onClick={cancel}>Keep on</button>
        </span>
      )}
      {error && (
        <span className="edit-error" role="alert">
          {error.message}
          {error.status === 409 && <> <button type="button" className="button button-quiet" onClick={() => { reloadBasis.current = app.overviewRevision; waitingForReload.current = true; app.reload() }}>Reload saved values</button></>}
          {error.problems.length > 0 && <ul>{error.problems.map((p) => <li key={p}>{p}</li>)}</ul>}
        </span>
      )}
    </span>
  )
}

function ResetTools({ expected, onSaved }: { expected: string[]; onSaved: (saved: Saved) => void }) {
  const app = useApp()
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | undefined>()
  const reset = async () => {
    setPending(true)
    setError(undefined)
    try {
      onSaved(await postEdit(API.editTargets, { audience: 'personal', reset: true, expected }))
      app.reload(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setPending(false)
    }
  }
  return (
    <p className="coding-tools-reset">
      <button type="button" className="button button-quiet" disabled={pending} onClick={() => { void reset() }}>Reset coding tools to the shared list</button>
      {error && <span className="edit-error" role="alert">{error}</span>}
    </p>
  )
}
