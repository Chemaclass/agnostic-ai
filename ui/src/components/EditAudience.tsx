import { createContext, useContext, useId, useState, type ReactNode } from 'react'
import { useApp } from './app-context'
import { audienceDescription, audienceLabel, levelName, readAudience, type EditAudience } from '../model/edit-audience'

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

/** Offers the audience choice on pages that enable it; every other page edits the level's own files. */
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

/** Whether the page serves the global setup rather than a project. */
export function useGlobalLevel(): boolean {
  return useApp().overview?.global === true
}

/** The level name for copy: `project` or `global`. */
export function useLevelName(): 'project' | 'global' {
  return levelName(useGlobalLevel())
}

export function ViewPersonalButton() {
  const state = useContext(Audience)
  if (!state) return null
  return <button type="button" className="button button-quiet" onClick={() => state.setAudience('personal')}>View local settings</button>
}

export function EditAudienceSwitch() {
  const app = useApp()
  const state = useContext(Audience)
  const labelId = useId()
  if (!state || app.source.kind !== 'api') return null
  const viewing = app.state.view === 'settings' || app.state.view === 'authority'
  const global = app.overview?.global === true
  const localFolder = global ? '~/.agnostic-ai/local' : '.agnostic-ai/local'
  const option = (value: EditAudience) => (
    <button type="button" aria-label={audienceDescription(value, global)} aria-pressed={state.audience === value} onClick={() => state.setAudience(value)}>
      {audienceLabel(value, global)}
    </button>
  )
  return (
    <div className="edit-scope">
      <span id={labelId} className="edit-scope-label">{viewing ? 'View and edit' : 'Save new servers for'}</span>
      <div className="segmented" role="group" aria-labelledby={labelId}>
        {option('shared')}
        {option('personal')}
      </div>
      <p className="muted edit-scope-hint" role="status">
        {state.audience === 'shared'
          ? `${global ? 'Global files in ~/.agnostic-ai, used by every project.' : 'Project files, tracked by Git.'}${viewing ? ' Local values win for you.' : ''}`
          : `Local files in ${localFolder}, ignored by Git.${viewing ? ' Local values win.' : ''}`}
      </p>
    </div>
  )
}
