import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { builtinChange, BuiltinChangeError, type BuiltinChange as Change, type BuiltinChangeAction as Action } from '../builtin-change'
import { useApp } from './app-context'
import { useEditScope, type EditScope } from './EditScope'
import { Code, Switch, Toggle, ToolAvatar } from './ui'

type Pending = 'loading' | 'preparing' | 'applying' | 'undoing' | 'cancelling'
interface Controller {
  change?: Change
  pending?: Pending
  pendingName?: string
  error?: { message: string; conflict: boolean; action?: Action; title?: string }
  proposal: boolean
  notice?: string
  request: (action?: Action, name?: string, enabled?: boolean, scope?: EditScope) => Promise<void>
  resetList: () => Promise<void>
  cancel: () => Promise<void>
}

const BuiltinChanges = createContext<Controller | null>(null)

function useBuiltinChanges(): Controller {
  const value = useContext(BuiltinChanges)
  if (!value) throw new Error('Built-in controls need a BuiltinChangeProvider')
  return value
}

// A proposal this page hides for good is also discarded on the server, so a
// remount does not restore it. A failure leaves it hidden here only.
function discard(id?: string) {
  if (id) builtinChange('cancel', { id }).catch(() => undefined)
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
  const [notice, setNotice] = useState<string>()
  const busy = useRef(false)
  const api = app.source.kind === 'api'
  const pageScope = useEditScope()
  const shownScope = useRef(pageScope)

  useEffect(() => {
    if (shownScope.current === pageScope) return
    shownScope.current = pageScope
    if (change?.status !== 'proposed' || change.id === cancelledId) return
    setCancelledId(change.id)
    discard(change.id)
  }, [pageScope, change, cancelledId])

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
  const request = async (action?: Action, name?: string, enabled?: boolean, scope: EditScope = pageScope, reset = false) => {
    if (busy.current) return
    busy.current = true
    setPending(action === 'preview' ? 'preparing' : action === 'apply' ? 'applying' : action === 'undo' ? 'undoing' : action === 'cancel' ? 'cancelling' : 'loading')
    setPendingName(name)
    setError(undefined)
    setNotice(undefined)
    const requestedIn = shownScope.current
    try {
      const value = await builtinChange(action, action === 'preview' ? (reset ? { scope: 'local', reset } : { scope, name, enabled }) : action ? { id: action === 'undo' ? change?.undoId : change?.id } : undefined)
      setChange(value)
      // A preview that answers after the page switched scope targets the old files.
      const stale = action === 'preview' ? shownScope.current !== requestedIn : !action && otherScope(value, shownScope.current)
      setCancelledId(stale ? value.id : undefined)
      if (stale && action === 'preview') discard(value.id)
      if (action === 'cancel') setNotice('Preview discarded.')
      if (action === 'apply' || action === 'undo' || !action) app.reload(true)
    } catch (err) {
      if (action === 'cancel' && err instanceof BuiltinChangeError && err.conflict) {
        const now = await builtinChange().catch(() => undefined)
        if (now) {
          const applied = now.status === 'applied' && now.id === change?.id
          setChange(now)
          app.reload(applied)
          if (now.status === 'proposed') {
            setError({ title: 'Another tab opened a newer preview.', message: 'Reload the change state to review it.', conflict: true, action })
          } else {
            setNotice(applied ? 'Another tab applied this preview.' : 'Preview discarded.')
          }
          return
        }
      }
      setError({ message: err instanceof Error ? err.message : 'The change failed.', conflict: err instanceof BuiltinChangeError && err.conflict, action })
    } finally {
      busy.current = false
      setPending(undefined)
      setPendingName(undefined)
    }
  }

  const resetList = () => request('preview', undefined, undefined, 'local', true)
  return <BuiltinChanges.Provider value={{ change, pending, pendingName, error, proposal, notice, request, resetList, cancel: () => request('cancel') }}>{children}</BuiltinChanges.Provider>
}

export function BuiltinChangeControls({ labels, local, resetSource }: { labels: { name: string; label: string }[]; local: boolean; resetSource: ReactNode }) {
  const app = useApp()
  const { change, pending, error, proposal, notice, request, resetList } = useBuiltinChanges()
  if (app.source.kind !== 'api') return null
  if (change && !change.supported) return <><p className="muted">Saves source files. Run <Code>agnostic-ai sync --global</Code> to apply them to your tools.</p>{local && resetSource}</>
  const label = (name?: string) => name ? labels.find((item) => item.name === name)?.label ?? name : 'Built-in list'
  return <div className="builtin-change">
    <div className="builtin-change-controls">
      {!proposal && <span className="muted">Preview before applying to your tools.</span>}
      {local && <button className="button button-quiet" type="button" disabled={!change || !!pending || proposal} onClick={() => { void resetList() }}>Reset built-ins to the team list</button>}
      {change?.undoAvailable && !error && <>
        <button className="button button-quiet" type="button" disabled={!!pending} onClick={() => { void request('undo') }}>Undo last change</button>
        {change.undoName && <span className="muted">Last applied: {label(change.undoName)}.</span>}
      </>}
    </div>
    <div className="builtin-change-status" role="status" aria-live="polite">
      {pending === 'preparing' && 'Preparing preview…'}
      {pending === 'applying' && 'Applying to all configured tools…'}
      {pending === 'undoing' && 'Restoring the previous files…'}
      {pending === 'cancelling' && 'Discarding the preview…'}
      {pending === 'loading' && 'Checking the server…'}
      {!pending && !error && notice}
      {!pending && !error && !notice && change?.status === 'applied' && `${label(change.name)} change applied. Undo is available until this server stops or the files change.`}
      {!pending && !error && !notice && change?.status === 'undone' && 'Previous files restored.'}
    </div>
    {error && <div className="builtin-change-error" role="alert">
      <strong>{error.title ?? (error.action === 'cancel' ? 'Preview could not be cancelled.' : error.conflict ? 'Files changed. Review a fresh preview.' : 'Change could not be completed.')}</strong>
      <p>{error.message}</p>
      <button className="button" type="button" disabled={!!pending} onClick={() => { void request() }}>Reload change state</button>
    </div>}
    {!proposal && !error && change?.status === 'undone' && change.findings.length > 0 && <ul className="builtin-change-findings">{change.findings.map((finding, index) => <li key={index}>{finding}</li>)}</ul>}
    {proposal && change?.reset && <Preview change={change} title="Built-in list reset" heading="Reset the built-in list to the team list" />}
  </div>
}

function Preview({ change, title, heading }: { change: Change; title: string; heading: string }) {
  const { pending, request, cancel } = useBuiltinChanges()
  return <section className="builtin-change builtin-change-preview" aria-label={`${title} preview`} aria-busy={!!pending}>
    <h3>{heading}</h3>
    <p>Update <Code>{change.config}</Code> {change.scope === 'local' ? 'for you only' : 'for the team'} and sync <span className="tool-list">{change.targets.map((target) => <ToolAvatar key={target} target={target} />)}</span>.</p>
    <p className="muted">{change.changes.length} files will change. Secret values are hidden in the preview.</p>
    <div className="builtin-change-files">
      {change.changes.map((file) => <details key={file.path}>
        <summary><span className="builtin-change-action">{file.action}</span> <Code>{file.path}</Code></summary>
        <pre tabIndex={0} aria-label={`Diff for ${file.path}`}>{file.diff || 'File permissions will change.'}</pre>
      </details>)}
    </div>
    {change.findings.length > 0 && <ul className="builtin-change-findings">{change.findings.map((finding, index) => <li key={index}>{finding}</li>)}</ul>}
    <div className="builtin-change-actions">
      <button className="button button-primary" type="button" disabled={!!pending} onClick={() => { void request('apply') }}>Apply change</button>
      <button className="button" type="button" disabled={!!pending} onClick={() => { void cancel() }}>Cancel</button>
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
  if (app.source.kind !== 'api') return renderHeading(<Switch on={on} />)
  if (change && !change.supported) return renderHeading(fallback)
  const selected = proposal && change?.name === name
  const title = label.charAt(0).toUpperCase() + label.slice(1)
  return <>
    {renderHeading(<>
      <Toggle on={on} label={`${name} built-in`} disabled={!change || !!pending || proposal || !!error} pending={pending === 'preparing' && pendingName === name} onToggle={() => { void request('preview', name, !on) }} />
      {local && <button type="button" className="button button-quiet" aria-label={`Reset ${name} to team value`} disabled={!change || !!pending || proposal} onClick={() => { void request('preview', name, !on, 'local') }}>Reset to team value</button>}
    </>)}
    {selected && change && <Preview change={change} title={`${title} change`} heading={`${change.enabled ? 'Enable' : 'Disable'} ${title}`} />}
  </>
}
