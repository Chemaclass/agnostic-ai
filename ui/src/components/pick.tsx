// Setup health lets a reader tick findings and suggestions and copy one
// prompt for all of them. Other pages provide no picks, so no checkboxes.

import { createContext, useContext } from 'react'
import { CopyButton } from './ui'

export interface Picks {
  has: (id: string) => boolean
  set: (id: string, on: boolean) => void
}

export const PicksContext = createContext<Picks | undefined>(undefined)

/** A checkbox that adds one finding or suggestion to the combined prompt; nothing outside Setup health. */
export function PickBox({ id, name, className }: { id: string; name: string; className?: string }) {
  const picks = useContext(PicksContext)
  if (!picks) return null
  return (
    <input
      type="checkbox"
      className={`pick-box${className ? ` ${className}` : ''}`}
      aria-label={`Add to prompt: ${name}`}
      checked={picks.has(id)}
      onChange={(event) => picks.set(id, event.target.checked)}
    />
  )
}

/** The bar at the foot of the page: how many are picked, one Copy prompt, and Clear. Hidden while nothing is picked. */
export function PickTray({ count, prompt, onClear }: { count: number; prompt: string; onClear: () => void }) {
  const selected = count > 0 ? `${count} selected` : ''
  return (
    <>
      {/* Stays on the page, so the first pick and Clear are announced too. */}
      <p className="sr-only" role="status">{selected}</p>
      <section className="pick-tray" aria-label="Picked for one prompt" hidden={count === 0}>
        <p className="pick-tray-count">{selected}</p>
        <CopyButton text={prompt} label="Copy prompt" copiedLabel="Copied" className="button-primary pick-tray-copy" />
        <button type="button" className="button button-quiet pick-tray-clear" onClick={onClear}>Clear</button>
      </section>
    </>
  )
}
