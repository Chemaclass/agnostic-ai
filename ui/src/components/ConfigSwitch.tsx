import { useState } from 'react'
import { requestSwitch, switchOffer, type ServedConfig } from '../model/config-switch'

export function ConfigSwitch({ served }: { served: ServedConfig }) {
  const offer = switchOffer(served)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()
  const [blocked, setBlocked] = useState<string>()
  if (!offer) return null

  const open = async () => {
    setBusy(true)
    setError(undefined)
    setBlocked(undefined)
    // Opened before the request so the browser treats it as a click, not a pop-up.
    const tab = window.open('about:blank', '_blank')
    if (tab) tab.opener = null
    try {
      const reply = await requestSwitch()
      if (tab) tab.location.replace(reply.url)
      else setBlocked(reply.url)
    } catch (err) {
      tab?.close()
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <span className="config-switch">
      <button type="button" className="button button-quiet" title={offer.title} disabled={busy} onClick={() => { void open() }}>
        {busy ? 'Opening…' : offer.label}
      </button>
      {blocked && <a className="config-switch-note" href={blocked} target="_blank" rel="noopener noreferrer">Open {blocked}</a>}
      {error && (
        <span className="config-switch-note" role="alert">
          {error}{!error.includes(offer.command) && <> Run <code>{offer.command}</code>.</>}
        </span>
      )}
    </span>
  )
}
