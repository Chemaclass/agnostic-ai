import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { builtinChange, BuiltinChangeError, type BuiltinChange as Change } from '../builtin-change'
import { useApp } from './app-context'
import { useEditScope, type EditScope } from './EditScope'
import { Code, Switch, Toggle, ToolAvatar } from './ui'

type Action = 'preview' | 'apply' | 'undo'
type Pending = 'loading' | 'preparing' | 'applying' | 'undoing'
/** A model or effort edit, sent as /api/edit/field takes it. */
export interface SettingPreview { path?: string; key: 'model' | 'effort'; value: string | null; expected: string | null }
interface Controller {
  change?: Change
  pending?: Pending
  pendingName?: string
  error?: { message: string; conflict: boolean; field?: string }
  proposal: boolean
  request: (action?: Action, name?: string, enabled?: boolean, scope?: EditScope) => Promise<boolean>
  resetList: () => Promise<boolean>
  previewSetting: (setting: SettingPreview, scope: EditScope) => Promise<boolean>
  cancel: () => void
}

const BuiltinChanges = createContext<Controller | null>(null)
const pendingSourceFinding = 'This preview also applies pending source changes.'

function useBuiltinChanges(): Controller {
  const value = useContext(BuiltinChanges)
  if (!value) throw new Error('Built-in controls need a BuiltinChangeProvider')
  return value
}

// A proposal restored from the server is shown only on the scope it saves to.
function otherScope(change: Change, scope: EditScope): boolean {
  return change.status === 'proposed' && change.scope !== undefined && change.scope !== scope
}

export function BuiltinChangeProvider({ active, children }: { active: boolean; children: ReactNode }) {
  const app = useApp()
  const [change, setChange] = useState<Change>()
  const [pending, setPending] = useState<Pending>()
  const [pendingName, setPendingName] = useState<string>()
  const [error, setError] = useState<Controller['error']>()
  const [cancelledId, setCancelledId] = useState<string>()
  const busy = useRef(false)
  const api = app.source.kind === 'api'
  const pageScope = useEditScope()
  const shownScope = useRef(pageScope)

  useEffect(() => {
    if (shownScope.current === pageScope) return
    shownScope.current = pageScope
    if (change?.status === 'proposed') setCancelledId(change.id)
  }, [pageScope, change])

  useEffect(() => {
    if (!api || !active) return
    let current = true
    busy.current = true
    setPending('loading')
    setError(undefined)
    builtinChange().then((value) => {
      if (!current) return
      setChange(value)
      if (otherScope(value, shownScope.current)) setCancelledId(value.id)
    }).catch((err: unknown) => {
      if (current) setError({ message: err instanceof Error ? err.message : 'Could not load the change.', conflict: false })
    }).finally(() => {
      if (current) {
        busy.current = false
        setPending(undefined)
      }
    })
    return () => { current = false; busy.current = false }
  }, [api, active])

  const proposal = change?.status === 'proposed' && change.id !== cancelledId && !error
  // Resolves true when the server refused the request as a conflict.
  const request = async (action?: Action, name?: string, enabled?: boolean, scope: EditScope = pageScope, reset = false, setting?: SettingPreview) => {
    if (busy.current) return false
    busy.current = true
    setPending(action === 'preview' ? 'preparing' : action === 'apply' ? 'applying' : action === 'undo' ? 'undoing' : 'loading')
    setPendingName(name)
    setError(undefined)
    const requestedIn = shownScope.current
    try {
      const value = await builtinChange(action, action === 'preview' ? (setting ? { scope, setting } : reset ? { scope: 'local', reset } : { scope, name, enabled }) : action ? { id: action === 'apply' ? change?.id : change?.undoId } : undefined)
      setChange(value)
      // A preview that answers after the page switched scope targets the old files.
      const stale = action === 'preview' ? shownScope.current !== requestedIn : !action && otherScope(value, shownScope.current)
      setCancelledId(stale ? value.id : undefined)
      if (action === 'apply' || action === 'undo' || !action) app.reload(true)
      return false
    } catch (err) {
      const conflict = err instanceof BuiltinChangeError && err.conflict
      setError({ message: err instanceof Error ? err.message : 'The change failed.', conflict, field: setting?.key ?? (action === 'apply' ? change?.key : undefined) })
      return conflict
    } finally {
      busy.current = false
      setPending(undefined)
      setPendingName(undefined)
    }
  }

  const resetList = () => request('preview', undefined, undefined, 'local', true)
  const previewSetting = (setting: SettingPreview, scope: EditScope) => request('preview', setting.key, undefined, scope, false, setting)
  return <BuiltinChanges.Provider value={{ change, pending, pendingName, error, proposal, request, resetList, previewSetting, cancel: () => setCancelledId(change?.id) }}>{children}</BuiltinChanges.Provider>
}

function capitalized(text: string) {
  return text.charAt(0).toUpperCase() + text.slice(1)
}

/** Undo, progress, and errors shared by built-in and settings changes. */
// A section with built-ins shows the status there, unless the latest change is a setting.
export function ChangeStatus({ labels, settings, builtins }: { labels: { name: string; label: string }[]; settings: boolean; builtins: boolean }) {
  const app = useApp()
  const { change, pending, error: anyError, proposal, request } = useBuiltinChanges()
  if (app.source.kind !== 'api' || (change && !change.supported)) return null
  if (settings ? builtins && !change?.key : !!change?.key) return null
  const error = anyError?.field ? undefined : anyError
  const label = (name?: string) => name ? labels.find((item) => item.name === name)?.label ?? name : 'Built-in list'
  const undoTitle = change?.undoTitle ?? (change?.undoName ? label(change.undoName) : undefined)
  return <div className="builtin-change change-status">
    {change?.undoAvailable && !error && <div className="builtin-change-controls">
      <button className="button button-quiet" type="button" disabled={!!pending} onClick={() => { void request('undo') }}>Undo last change</button>
      {undoTitle && <span className="muted">Last applied: {undoTitle}.</span>}
    </div>}
    <div className="builtin-change-status" role="status" aria-live="polite">
      {pending === 'preparing' && 'Preparing preview…'}
      {pending === 'applying' && 'Applying to all configured tools…'}
      {pending === 'undoing' && 'Restoring the previous files…'}
      {pending === 'loading' && 'Checking the server…'}
      {!pending && !error && change?.status === 'applied' && `${change.title ? capitalized(change.title) : `${label(change.name)} change`}: applied and verified in the generated files. Undo is available until this server stops or the files change.`}
      {!pending && !error && change?.status === 'undone' && 'Previous files restored.'}
    </div>
    {error && <ChangeError />}
    {!proposal && !error && change?.status === 'undone' && change.findings.length > 0 && <ul className="builtin-change-findings">{change.findings.map((finding, index) => <li key={index}>{finding}</li>)}</ul>}
  </div>
}

export function ChangeError() {
  const { pending, error, request } = useBuiltinChanges()
  if (!error) return null
  return <div className="builtin-change-error" role="alert">
    <strong>{error.conflict ? 'Files changed. Review a fresh preview.' : 'Change could not be completed.'}</strong>
    <p>{error.message}</p>
    <button className="button" type="button" disabled={!!pending} onClick={() => { void request() }}>Reload change state</button>
  </div>
}

export function BuiltinChangeControls({ local, resetSource }: { local: boolean; resetSource: ReactNode }) {
  const app = useApp()
  const { change, pending, proposal, resetList } = useBuiltinChanges()
  if (app.source.kind !== 'api') return null
  if (change && !change.supported) return <><p className="muted">Saves source files. Run <Code>agnostic-ai sync --global</Code> to apply them to your tools.</p>{local && resetSource}</>
  return <div className="builtin-change">
    <div className="builtin-change-controls">
      {!proposal && <span className="muted">Preview before applying to your tools.</span>}
      {local && <button className="button button-quiet" type="button" disabled={!change || !!pending || proposal} onClick={() => { void resetList() }}>Reset built-ins to the team list</button>}
    </div>
    {proposal && change?.reset && <Preview change={change} title="Built-in list reset" heading="Reset the built-in list to the team list" />}
  </div>
}

/**
 * The preview flow for one model or effort field. supported is false in the
 * global view, where a field save only writes its source file.
 */
export function useSettingChange(key: string) {
  const app = useApp()
  const { change, pending, pendingName, error, proposal, previewSetting } = useBuiltinChanges()
  return {
    supported: app.source.kind === 'api' && (key === 'model' || key === 'effort') && change?.supported !== false,
    ready: !!change && !pending && !proposal,
    preparing: pending === 'preparing' && pendingName === key,
    proposal: proposal && change?.key === key ? change : undefined,
    error: error?.field === key,
    previewSetting,
  }
}

export function SettingChangePreview({ change, label }: { change: Change; label: string }) {
  return <Preview change={change} title={`${label} change`} heading={capitalized(change.title ?? label)} />
}

function Preview({ change, title, heading }: { change: Change; title: string; heading: string }) {
  const { pending, request, cancel } = useBuiltinChanges()
  return <section className="builtin-change builtin-change-preview" aria-label={`${title} preview`} aria-busy={!!pending}>
    <h3>{heading}</h3>
    <p>Update <Code>{change.config}</Code> {change.scope === 'local' ? 'for you only' : 'for the team'} and sync <span className="tool-list">{change.targets.map((target) => <ToolAvatar key={target} target={target} />)}</span>.</p>
    {change.findings.includes(pendingSourceFinding) && <p className="builtin-change-pending" role="note"><strong>{pendingSourceFinding}</strong> Applying writes source changes that were not synced yet, too. Every affected file is listed below.</p>}
    <p className="muted">{change.changes.length} files will change. Secret values are hidden in the preview.</p>
    <div className="builtin-change-files">
      {change.changes.map((file) => <details key={file.path}>
        <summary><span className="builtin-change-action">{file.action}</span> <Code>{file.path}</Code></summary>
        <pre tabIndex={0} aria-label={`Diff for ${file.path}`}>{file.diff || 'File permissions will change.'}</pre>
      </details>)}
    </div>
    {change.findings.some((finding) => finding !== pendingSourceFinding) && <ul className="builtin-change-findings">{change.findings.filter((finding) => finding !== pendingSourceFinding).map((finding, index) => <li key={index}>{finding}</li>)}</ul>}
    <div className="builtin-change-actions">
      <button className="button button-primary" type="button" disabled={!!pending} onClick={() => { void request('apply') }}>Apply change</button>
      <button className="button" type="button" disabled={!!pending} onClick={cancel}>Cancel</button>
    </div>
  </section>
}

export function BuiltinChange({ name, label, on, local, fallback, renderHeading }: {
  name: string
  label: string
  on: boolean
  local: boolean
  fallback: ReactNode
  renderHeading: (control: ReactNode) => ReactNode
}) {
  const app = useApp()
  const { change, pending, pendingName, error, proposal, request } = useBuiltinChanges()
  const blocked = !!error && !error.field
  if (app.source.kind !== 'api') return renderHeading(<Switch on={on} />)
  if (change && !change.supported) return renderHeading(fallback)
  const selected = proposal && change?.name === name
  const title = label.charAt(0).toUpperCase() + label.slice(1)
  return <>
    {renderHeading(<>
      <Toggle on={on} label={`${name} built-in`} disabled={!change || !!pending || proposal || blocked} pending={pending === 'preparing' && pendingName === name} onToggle={() => { void request('preview', name, !on) }} />
      {local && <button type="button" className="button button-quiet" aria-label={`Reset ${name} to team value`} disabled={!change || !!pending || proposal} onClick={() => { void request('preview', name, !on, 'local') }}>Reset to team value</button>}
    </>)}
    {selected && change && <Preview change={change} title={`${title} change`} heading={`${change.enabled ? 'Enable' : 'Disable'} ${title}`} />}
  </>
}
