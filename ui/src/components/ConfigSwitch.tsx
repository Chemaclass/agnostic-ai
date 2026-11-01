import { useState } from 'react'
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

interface Onboarding {
  root: string
  tab: Window | null
}

export function ConfigSwitch({ served }: { served: ServedConfig }) {
  const offer = switchOffer(served)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()
  const [blocked, setBlocked] = useState<string>()
  const [onboarding, setOnboarding] = useState<Onboarding>()
  const [git, setGit] = useState(true)
  const [created, setCreated] = useState<string>()
  if (!offer) return null

  const openTab = async (tab: Window | null): Promise<void> => {
    try {
      const reply = await requestSwitch()
      if (tab) tab.location.replace(reply.url)
      else setBlocked(reply.url)
      setOnboarding(undefined)
    } catch (err) {
      if (err instanceof GlobalSetupError) {
        setOnboarding({ root: err.root, tab })
        return
      }
      tab?.close()
      setOnboarding(undefined)
      setError((err as Error).message)
    }
  }

  const open = async () => {
    setBusy(true)
    setError(undefined)
    setBlocked(undefined)
    setCreated(undefined)
    // Opened before the request so the browser treats it as a click, not a pop-up.
    const tab = window.open('about:blank', '_blank')
    if (tab) tab.opener = null
    try {
      await openTab(tab)
    } finally {
      setBusy(false)
    }
  }

  const create = async () => {
    if (!onboarding) return
    setBusy(true)
    setError(undefined)
    try {
      const made = await createGlobalSetup(git)
      setCreated(globalSetupCreated(made.root))
    } catch (err) {
      // A 409 means the folder appeared since the page asked; the switch below settles it.
      setError((err as Error).message)
    }
    try {
      await openTab(onboarding.tab)
    } finally {
      setBusy(false)
    }
  }

  const cancel = () => {
    onboarding?.tab?.close()
    setOnboarding(undefined)
    setError(undefined)
  }

  return (
    <span className="config-switch">
      <button type="button" className="button button-quiet" title={offer.title} disabled={busy} onClick={() => { void open() }}>
        {busy ? 'Opening…' : offer.label}
      </button>
      {onboarding && (
        <section className="global-onboarding" role="dialog" aria-labelledby="global-onboarding-title">
          <h2 id="global-onboarding-title">{GLOBAL_SETUP_TITLE}</h2>
          <p>{GLOBAL_SETUP_BODY}</p>
          <p className="global-onboarding-path"><code>{onboarding.root}</code></p>
          <label className="global-onboarding-git">
            <input type="checkbox" checked={git} disabled={busy} onChange={(e) => setGit(e.target.checked)} />
            Also run git init
          </label>
          {error && <p className="config-switch-note" role="alert">{error}</p>}
          <div className="global-onboarding-actions">
            <button type="button" className="button button-primary" disabled={busy} onClick={() => { void create() }}>Create global setup</button>
            <button type="button" className="button button-quiet" disabled={busy} onClick={cancel}>Cancel</button>
          </div>
        </section>
      )}
      {created && !onboarding && <span className="config-switch-note" role="status">{created}</span>}
      {blocked && <a className="config-switch-note" href={blocked} target="_blank" rel="noopener noreferrer">Open {blocked}</a>}
      {error && !onboarding && (
        <span className="config-switch-note" role="alert">
          {error}{!error.includes(offer.command) && <> Run <code>{offer.command}</code>.</>}
        </span>
      )}
    </span>
  )
}
