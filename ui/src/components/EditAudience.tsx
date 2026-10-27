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

export function EditAudienceSwitch() {
  const app = useApp()
  const state = useContext(Audience)
  const labelId = useId()
  if (!state || app.source.kind !== 'api') return null
  const option = (value: EditAudience, label: string) => (
    <button type="button" aria-pressed={state.audience === value} onClick={() => state.setAudience(value)}>{label}</button>
  )
  return (
    <div className="edit-scope">
      <span id={labelId} className="edit-scope-label">Save changes for</span>
      <div className="segmented" role="group" aria-labelledby={labelId}>
        {option('shared', 'Shared (committed)')}
        {option('personal', 'Personal (not committed)')}
      </div>
      <p className="muted edit-scope-hint" role="status">
        {state.audience === 'shared' ? 'Changes go to the files your team commits.' : 'Changes go to personal files that Git ignores. Your team does not get them.'}
      </p>
    </div>
  )
}
