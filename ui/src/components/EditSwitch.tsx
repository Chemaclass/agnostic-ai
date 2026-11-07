import { useEffect, useRef, useState } from 'react'
import { EditError } from '../api'
import { useApp } from './app-context'
import { useLevelName } from './EditAudience'
import { Switch, Toggle } from './ui'

/**
 * A switch that saves its setting through the server. The server
 * validates the edit and restores the file when it fails; the reason
 * shows under the switch. Snapshots and the mock have no server, so the
 * switch only shows the value there.
 */
export function EditSwitch({ on, label, save }: { on: boolean; label: string; save: (on: boolean) => Promise<unknown> }) {
  const app = useApp()
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<EditError | undefined>()
  const [choice, setChoice] = useState<boolean | undefined>()
  const reloadBasis = useRef<object | undefined>(undefined)
  const waitingForReload = useRef(false)
  useEffect(() => {
    if (waitingForReload.current && app.overviewRevision !== reloadBasis.current) {
      waitingForReload.current = false
      setChoice(undefined)
      setError(undefined)
    }
  }, [app.overviewRevision])
  if (app.source.kind !== 'api') return <Switch on={on} />
  const toggle = async () => {
    setPending(true)
    setError(undefined)
    const selected = !(choice ?? on)
    try {
      await save(selected)
      app.reload(true)
    } catch (err) {
      const failure = err instanceof EditError ? err : new EditError(String(err))
      if (failure.status === 409) setChoice(selected)
      setError(failure)
    } finally {
      setPending(false)
    }
  }
  return (
    <span className="edit-switch">
      <Toggle on={choice ?? on} label={label} pending={pending} disabled={choice !== undefined} onToggle={toggle} />
      {error && (
        <span className="edit-error" role="alert">
          {error.message}
          {error.status === 409 && <> <button type="button" className="button button-quiet" onClick={() => { reloadBasis.current = app.overviewRevision; waitingForReload.current = true; app.reload() }}>Reload saved values</button></>}
          {error.problems.length > 0 && (
            <ul>
              {error.problems.map((p) => (
                <li key={p}>{p}</li>
              ))}
            </ul>
          )}
        </span>
      )}
    </span>
  )
}

/** Removes a personal override; the server refuses it when nothing is overridden. */
export function ResetAction({ label, reset }: { label: string; reset: () => Promise<unknown> }) {
  const app = useApp()
  const level = useLevelName()
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<EditError | undefined>()
  const reloadBasis = useRef<object | undefined>(undefined)
  const waitingForReload = useRef(false)
  useEffect(() => {
    if (waitingForReload.current && app.overviewRevision !== reloadBasis.current) {
      waitingForReload.current = false
      setError(undefined)
    }
  }, [app.overviewRevision])
  if (app.source.kind !== 'api') return null
  const run = async () => {
    setPending(true)
    setError(undefined)
    try {
      await reset()
      app.reload(true)
    } catch (err) {
      setError(err instanceof EditError ? err : new EditError(String(err)))
    } finally {
      setPending(false)
    }
  }
  return (
    <span className="edit-switch">
      <button type="button" className="button button-quiet" aria-label={label} disabled={pending} onClick={() => { void run() }}>Reset to the {level} value</button>
      {error && <span className="edit-error" role="alert">{error.message}{error.status === 409 && <> <button type="button" className="button button-quiet" onClick={() => { reloadBasis.current = app.overviewRevision; waitingForReload.current = true; app.reload() }}>Reload saved values</button></>}</span>}
    </span>
  )
}
