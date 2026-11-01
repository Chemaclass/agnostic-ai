import { useEffect, useRef, useState } from 'react'
import {
  createGlobalSetup,
  GLOBAL_SETUP_BODY,
  GLOBAL_SETUP_TITLE,
  GlobalSetupError,
  globalSetupCreated,
  requestSwitch,
  switchOffer,
  type ServedConfig,
} from '../model/config-switch'

// Opened inside a click handler so the browser treats it as a click, not a pop-up.
function openBlankTab(): Window | null {
  const tab = window.open('about:blank', '_blank')
  if (tab) tab.opener = null
  return tab
}

function closeTab(tab: Window | null) {
  if (tab && !tab.closed) tab.close()
}

export function ConfigSwitch({ served }: { served: ServedConfig }) {
  const offer = switchOffer(served)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()
  const [blocked, setBlocked] = useState<string>()
  const [missingRoot, setMissingRoot] = useState<string>()
  const [git, setGit] = useState(true)
  const [created, setCreated] = useState<string>()
  const createButton = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    if (missingRoot !== undefined) createButton.current?.focus()
  }, [missingRoot])

  if (!offer) return null

  // Returns true when the other configuration opened (or its link is shown).
  const openTab = async (tab: Window | null): Promise<boolean> => {
    try {
      const reply = await requestSwitch()
      if (tab && !tab.closed) tab.location.replace(reply.url)
      else setBlocked(reply.url)
      setMissingRoot(undefined)
      setError(undefined)
      return true
    } catch (err) {
      closeTab(tab)
      if (err instanceof GlobalSetupError) {
        setMissingRoot(err.root)
      } else {
        setMissingRoot(undefined)
        setError((err as Error).message)
      }
      return false
    }
  }

  const open = async () => {
    setBusy(true)
    setError(undefined)
    setBlocked(undefined)
    setCreated(undefined)
    try {
      await openTab(openBlankTab())
    } finally {
      setBusy(false)
    }
  }

  const create = async () => {
    // Opened first: the panel is in this tab, so the new page gets its own.
    const tab = openBlankTab()
    setBusy(true)
    setError(undefined)
    setBlocked(undefined)
    let note: string | undefined
    let failure: string | undefined
    try {
      const made = await createGlobalSetup(git)
      note = globalSetupCreated(made.root)
      if (git && !made.git.startsWith('initialized')) note += ` Git: ${made.git}.`
    } catch (err) {
      failure = (err as Error).message
    }
    try {
      // A failed create can mean the folder appeared since the page asked; the switch settles it.
      const opened = await openTab(tab)
      if (opened) setCreated(note)
      else if (failure) setError(failure)
    } finally {
      setBusy(false)
    }
  }

  const cancel = () => {
    setMissingRoot(undefined)
    setError(undefined)
  }

  return (
    <span className="config-switch">
      <button type="button" className="button button-quiet" title={offer.title} disabled={busy || missingRoot !== undefined} onClick={() => { void open() }}>
        {busy ? 'Opening…' : offer.label}
      </button>
      {missingRoot !== undefined && (
        <section
          className="global-onboarding"
          role="dialog"
          aria-labelledby="global-onboarding-title"
          onKeyDown={(e) => { if (e.key === 'Escape' && !busy) cancel() }}
        >
          <h2 id="global-onboarding-title">{GLOBAL_SETUP_TITLE}</h2>
          <p>{GLOBAL_SETUP_BODY}</p>
          <p className="global-onboarding-path"><code>{missingRoot}</code></p>
          <label className="global-onboarding-git">
            <input type="checkbox" checked={git} disabled={busy} onChange={(e) => setGit(e.target.checked)} />
            Also run git init
          </label>
          {error && <p className="config-switch-note" role="alert">{error}</p>}
          <div className="global-onboarding-actions">
            <button ref={createButton} type="button" className="button button-primary" disabled={busy} onClick={() => { void create() }}>Create global setup</button>
            <button type="button" className="button button-quiet" disabled={busy} onClick={cancel}>Cancel</button>
          </div>
        </section>
      )}
      {created && missingRoot === undefined && <span className="config-switch-note" role="status">{created}</span>}
      {blocked && <a className="config-switch-note" href={blocked} target="_blank" rel="noopener noreferrer">Open {blocked}</a>}
      {error && missingRoot === undefined && (
        <span className="config-switch-note" role="alert">
          {error}{!error.includes(offer.command) && <> Run <code>{offer.command}</code>.</>}
        </span>
      )}
    </span>
  )
}
