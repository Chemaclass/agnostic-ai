import { useEffect, useRef, useState, type ReactNode } from 'react'
import { builtinChange, BuiltinChangeError, type BuiltinChange as Change } from '../builtin-change'
import { useApp } from './app-context'
import { Code, Switch, Toggle } from './ui'

type Pending = 'loading' | 'preparing' | 'applying' | 'undoing'

export function CavemanChange({ on, fallback }: { on: boolean; fallback: ReactNode }) {
  const app = useApp()
  const [change, setChange] = useState<Change>()
  const [pending, setPending] = useState<Pending>()
  const [error, setError] = useState<{ message: string; conflict: boolean }>()
  const [cancelled, setCancelled] = useState(false)
  const busy = useRef(false)
  const api = app.source.kind === 'api'

  useEffect(() => {
    if (!api) return
    let active = true
    builtinChange().then((value) => { if (active) setChange(value) }).catch((err: unknown) => {
      if (active) setError({ message: err instanceof Error ? err.message : 'Could not load the change.', conflict: false })
    })
    return () => { active = false }
  }, [api])

  if (!api) return <Switch on={on} />
  if (change && !change.supported) return fallback

  const proposal = change?.status === 'proposed' && !cancelled && !error
  const enabled = on
  const request = async (action?: 'preview' | 'apply' | 'undo') => {
    if (busy.current) return
    busy.current = true
    setPending(action === 'preview' ? 'preparing' : action === 'apply' ? 'applying' : action === 'undo' ? 'undoing' : 'loading')
    setError(undefined)
    try {
      const value = await builtinChange(action, action === 'preview' ? { name: 'caveman', enabled: !enabled } : action ? { id: action === 'apply' ? change?.id : change?.undoId } : undefined)
      setChange(value)
      setCancelled(false)
      if (action === 'apply' || action === 'undo' || !action) app.reload()
    } catch (err) {
      setError({ message: err instanceof Error ? err.message : 'The change failed.', conflict: err instanceof BuiltinChangeError && err.conflict })
    } finally {
      busy.current = false
      setPending(undefined)
    }
  }

  return (
    <div className="builtin-change">
      <div className="builtin-change-controls">
        <Toggle on={enabled} label="caveman built-in" disabled={!change || !!pending || !!proposal || !!error} pending={pending === 'preparing'} onToggle={() => { void request('preview') }} />
        {!proposal && <span className="muted">Preview before applying to your tools.</span>}
        {change?.undoAvailable && !proposal && !error && <button className="button button-quiet" type="button" disabled={!!pending} onClick={() => { void request('undo') }}>Undo last change</button>}
      </div>
      <div className="builtin-change-status" role="status" aria-live="polite">
        {pending === 'preparing' && 'Preparing preview…'}
        {pending === 'applying' && 'Applying to all configured tools…'}
        {pending === 'undoing' && 'Restoring the previous files…'}
        {pending === 'loading' && 'Checking the server…'}
        {!pending && !error && change?.status === 'applied' && 'Caveman change applied. Undo is available until this server stops or the files change.'}
        {!pending && !error && change?.status === 'undone' && 'Previous files restored.'}
      </div>
      {error && <div className="builtin-change-error" role="alert">
        <strong>{error.conflict ? 'Files changed. Review a fresh preview.' : 'Change could not be completed.'}</strong>
        <p>{error.message}</p>
        <button className="button" type="button" disabled={!!pending} onClick={() => { void request() }}>Reload change state</button>
      </div>}
      {proposal && <section className="builtin-change-preview" aria-label="Caveman change preview" aria-busy={!!pending}>
        <h3>{change.enabled ? 'Enable' : 'Disable'} Caveman</h3>
        <p>Update <Code>{change.config}</Code> and sync {change.targets.join(', ')}.</p>
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
          <button className="button" type="button" disabled={!!pending} onClick={() => setCancelled(true)}>Cancel</button>
        </div>
      </section>}
      {!proposal && !error && change?.status === 'undone' && change.findings.length > 0 && <ul className="builtin-change-findings">{change.findings.map((finding, index) => <li key={index}>{finding}</li>)}</ul>}
    </div>
  )
}
