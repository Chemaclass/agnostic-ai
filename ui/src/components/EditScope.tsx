import { createContext, useContext, useId, useState, type ReactNode } from 'react'
import { useApp } from './app-context'

/** Which files an edit writes: the committed team files, or personal ones Git ignores. */
export type EditScope = 'team' | 'local'

const STORAGE_KEY = 'agnostic-ai.edit-scope'

interface ScopeState {
  scope: EditScope
  setScope: (scope: EditScope) => void
}

const Scope = createContext<ScopeState | null>(null)

function storedScope(): EditScope {
  try {
    return window.localStorage.getItem(STORAGE_KEY) === 'local' ? 'local' : 'team'
  } catch {
    return 'team'
  }
}

/** Offers the scope choice on pages that enable it; every other page edits team files. */
export function EditScopeProvider({ enabled, children }: { enabled: boolean; children: ReactNode }) {
  const [scope, setScopeState] = useState<EditScope>(() => (enabled ? storedScope() : 'team'))
  const setScope = (next: EditScope) => {
    setScopeState(next)
    try {
      window.localStorage.setItem(STORAGE_KEY, next)
    } catch {
      // The choice still holds until the page reloads.
    }
  }
  return <Scope.Provider value={enabled ? { scope, setScope } : null}>{children}</Scope.Provider>
}

/** The scope the person chose, or undefined on a page that offers no choice. */
export function usePageScope(): EditScope | undefined {
  return useContext(Scope)?.scope
}

export function useEditScope(): EditScope {
  return usePageScope() ?? 'team'
}

export function EditScopeSwitch() {
  const app = useApp()
  const state = useContext(Scope)
  const labelId = useId()
  if (!state || app.source.kind !== 'api') return null
  const option = (value: EditScope, label: string) => (
    <button type="button" aria-pressed={state.scope === value} onClick={() => state.setScope(value)}>{label}</button>
  )
  return (
    <div className="edit-scope">
      <span id={labelId} className="edit-scope-label">Save changes for</span>
      <div className="segmented" role="group" aria-labelledby={labelId}>
        {option('team', 'Team (committed)')}
        {option('local', 'Just me (not committed)')}
      </div>
      <p className="muted edit-scope-hint" role="status">
        {state.scope === 'team' ? 'Changes go to the files your team commits.' : 'Changes go to personal files that Git ignores. Your team does not get them.'}
      </p>
    </div>
  )
}
