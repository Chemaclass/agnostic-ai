import { createContext, useContext, useId, useState, type ReactNode } from 'react'
import { useApp } from './app-context'
import { readAudience, type EditAudience } from '../model/edit-audience'

export type { EditAudience }

const STORAGE_KEY = 'agnostic-ai.edit-scope'

interface AudienceState {
  audience: EditAudience
  setAudience: (audience: EditAudience) => void
}

const Audience = createContext<AudienceState | null>(null)

function storedAudience(): EditAudience {
  try {
    return readAudience(window.localStorage.getItem(STORAGE_KEY))
  } catch {
    return 'shared'
  }
}

/** Offers the audience choice on pages that enable it; every other page edits shared files. */
export function EditAudienceProvider({ enabled, children }: { enabled: boolean; children: ReactNode }) {
  const [audience, setAudienceState] = useState<EditAudience>(() => (enabled ? storedAudience() : 'shared'))
  const setAudience = (next: EditAudience) => {
    setAudienceState(next)
    try {
      window.localStorage.setItem(STORAGE_KEY, next)
    } catch {
      // The choice still holds until the page reloads.
    }
  }
  return <Audience.Provider value={enabled ? { audience, setAudience } : null}>{children}</Audience.Provider>
}

/** The audience the person chose, or undefined on a page that offers no choice. */
export function usePageAudience(): EditAudience | undefined {
  return useContext(Audience)?.audience
}

export function useEditAudience(): EditAudience {
  return usePageAudience() ?? 'shared'
}

export function useViewAudience(): EditAudience | undefined {
  const app = useApp()
  const audience = usePageAudience()
  return app.source.kind === 'api' && (app.state.view === 'settings' || app.state.view === 'authority') ? audience : undefined
}

export function ViewPersonalButton() {
  const state = useContext(Audience)
  if (!state) return null
  return <button type="button" className="button button-quiet" onClick={() => state.setAudience('personal')}>View personal settings</button>
}

export function EditAudienceSwitch() {
  const app = useApp()
  const state = useContext(Audience)
  const labelId = useId()
  if (!state || app.source.kind !== 'api') return null
  const viewing = app.state.view === 'settings' || app.state.view === 'authority'
  const option = (value: EditAudience, label: string) => (
    <button type="button" aria-label={label} aria-pressed={state.audience === value} onClick={() => state.setAudience(value)}>
      {value === 'shared' ? 'Shared' : 'Personal'}
    </button>
  )
  return (
    <div className="edit-scope">
      <span id={labelId} className="edit-scope-label">{viewing ? 'View and edit' : 'Save new specs for'}</span>
      <div className="segmented" role="group" aria-labelledby={labelId}>
        {option('shared', 'Shared (committed)')}
        {option('personal', 'Personal (not committed)')}
      </div>
      <p className="muted edit-scope-hint" role="status">
        {state.audience === 'shared'
          ? `Shared files, tracked by Git.${viewing ? ' Personal overrides take priority for you.' : ''}`
          : `${viewing ? 'Your personal overrides take priority over Shared. ' : ''}Personal files, ignored by Git.`}
      </p>
    </div>
  )
}
