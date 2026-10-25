import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { builtinChange, BuiltinChangeError, type BuiltinChange as Change } from '../builtin-change'
import { useApp } from './app-context'
import { Code, Switch, Toggle, ToolAvatar } from './ui'

type Action = 'preview' | 'apply' | 'undo'
type Pending = 'loading' | 'preparing' | 'applying' | 'undoing'
interface Controller {
  change?: Change
  pending?: Pending
  pendingName?: string
  error?: { message: string; conflict: boolean }
  proposal: boolean
  request: (action?: Action, name?: string, enabled?: boolean) => Promise<void>
  cancel: () => void
}

const BuiltinChanges = createContext<Controller | null>(null)

function useBuiltinChanges(): Controller {
  const value = useContext(BuiltinChanges)
  if (!value) throw new Error('Built-in controls need a BuiltinChangeProvider')
  return value
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

  useEffect(() => {
    if (!api || !active) return
    let current = true
    busy.current = true
    setPending('loading')
    setError(undefined)
    builtinChange().then((value) => { if (current) setChange(value) }).catch((err: unknown) => {
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
  const request = async (action?: Action, name?: string, enabled?: boolean) => {
    if (busy.current) return
    busy.current = true
    setPending(action === 'preview' ? 'preparing' : action === 'apply' ? 'applying' : action === 'undo' ? 'undoing' : 'loading')
    setPendingName(name)
    setError(undefined)
    try {
      const value = await builtinChange(action, action === 'preview' ? { name, enabled } : action ? { id: action === 'apply' ? change?.id : change?.undoId } : undefined)
      setChange(value)
      setCancelledId(undefined)
      if (action === 'apply' || action === 'undo' || !action) app.reload()
    } catch (err) {
      setError({ message: err instanceof Error ? err.message : 'The change failed.', conflict: err instanceof BuiltinChangeError && err.conflict })
    } finally {
      busy.current = false
      setPending(undefined)
      setPendingName(undefined)
    }
  }

  return <BuiltinChanges.Provider value={{ change, pending, pendingName, error, proposal, request, cancel: () => setCancelledId(change?.id) }}>{children}</BuiltinChanges.Provider>
}

export function BuiltinChangeControls({ labels }: { labels: { name: string; label: string }[] }) {
  const app = useApp()
  const { change, pending, error, proposal, request } = useBuiltinChanges()
  if (app.source.kind !== 'api') return null
  if (change && !change.supported) return <p className="muted">Saves source files. Run <Code>agnostic-ai sync --global</Code> to apply them to your tools.</p>
  const label = (name?: string) => labels.find((item) => item.name === name)?.label ?? name
  return <div className="builtin-change">
    <div className="builtin-change-controls">
      {!proposal && <span className="muted">Preview before applying to your tools.</span>}
      {change?.undoAvailable && !error && <>
        <button className="button button-quiet" type="button" disabled={!!pending} onClick={() => { void request('undo') }}>Undo last change</button>
        {change.undoName && <span className="muted">Last applied: {label(change.undoName)}.</span>}
      </>}
    </div>
    <div className="builtin-change-status" role="status" aria-live="polite">
      {pending === 'preparing' && 'Preparing preview…'}
      {pending === 'applying' && 'Applying to all configured tools…'}
      {pending === 'undoing' && 'Restoring the previous files…'}
      {pending === 'loading' && 'Checking the server…'}
      {!pending && !error && change?.status === 'applied' && `${label(change.name)} change applied. Undo is available until this server stops or the files change.`}
      {!pending && !error && change?.status === 'undone' && 'Previous files restored.'}
    </div>
    {error && <div className="builtin-change-error" role="alert">
      <strong>{error.conflict ? 'Files changed. Review a fresh preview.' : 'Change could not be completed.'}</strong>
      <p>{error.message}</p>
      <button className="button" type="button" disabled={!!pending} onClick={() => { void request() }}>Reload change state</button>
    </div>}
    {!proposal && !error && change?.status === 'undone' && change.findings.length > 0 && <ul className="builtin-change-findings">{change.findings.map((finding, index) => <li key={index}>{finding}</li>)}</ul>}
  </div>
}

export function BuiltinChange({ name, label, on, fallback, renderHeading }: {
  name: string
  label: string
  on: boolean
  fallback: ReactNode
  renderHeading: (control: ReactNode) => ReactNode
}) {
  const app = useApp()
  const { change, pending, pendingName, error, proposal, request, cancel } = useBuiltinChanges()
  if (app.source.kind !== 'api') return renderHeading(<Switch on={on} />)
  if (change && !change.supported) return renderHeading(fallback)
  const selected = proposal && change?.name === name
  const title = label.charAt(0).toUpperCase() + label.slice(1)
  return <>
    {renderHeading(<Toggle on={on} label={`${name} built-in`} disabled={!change || !!pending || proposal || !!error} pending={pending === 'preparing' && pendingName === name} onToggle={() => { void request('preview', name, !on) }} />)}
    {selected && change && <section className="builtin-change builtin-change-preview" aria-label={`${title} change preview`} aria-busy={!!pending}>
      <h3>{change.enabled ? 'Enable' : 'Disable'} {title}</h3>
      <p>Update <Code>{change.config}</Code> and sync <span className="tool-list">{change.targets.map((target) => <ToolAvatar key={target} target={target} />)}</span>.</p>
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
        <button className="button" type="button" disabled={!!pending} onClick={cancel}>Cancel</button>
      </div>
    </section>}
  </>
}
