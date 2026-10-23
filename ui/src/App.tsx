// The application: URL-driven state, data loading, and page routing.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { OverviewView } from './contract'
import { LoadError, loadOverview, type Loaded } from './api'
import { AppContext, type AppApi } from './components/app-context'
import { JsonInspector } from './components/JsonInspector'
import { SectionPage } from './components/Section'
import { ErrorPanel } from './components/Banner'
import { Header, Nav, type Remote } from './components/Shell'
import { nextState, parseUrlState, resolveDataSource, serializeUrlState, type UrlState } from './model/url'

function initialTheme(): 'dark' | 'light' {
  let stored: string | null = null
  try {
    stored = window.localStorage.getItem('agnostic-ai.theme')
  } catch {
    // Storage may be unavailable; fall through to the OS preference.
  }
  if (stored === 'light' || stored === 'dark') return stored
  return window.matchMedia?.('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
}

// Loads a document and keeps the previous one on screen while it
// reloads; a different data source starts from nothing, so one source's
// data never stands in for another's.
function useRemote<T>(load: (signal: AbortSignal) => Promise<Loaded<T>>, sourceKey: string, deps: unknown[]): Remote<T> {
  const [remote, setRemote] = useState<Remote<T>>({ status: 'loading' })
  const lastSource = useRef(sourceKey)
  useEffect(() => {
    const controller = new AbortController()
    if (lastSource.current !== sourceKey) {
      lastSource.current = sourceKey
      setRemote({ status: 'loading' })
    } else {
      setRemote((prev) => ({ ...prev, status: 'loading' }))
    }
    load(controller.signal).then(
      (loaded) => {
        if (!controller.signal.aborted) setRemote({ status: 'ready', loaded })
      },
      (error: Error) => {
        if (!controller.signal.aborted && error.name !== 'AbortError') setRemote((prev) => ({ ...prev, status: 'error', error }))
      },
    )
    return () => controller.abort()
  }, deps)
  return remote
}

export default function App() {
  const [state, setState] = useState<UrlState>(() => parseUrlState(window.location.search))
  const stateRef = useRef(state)
  stateRef.current = state

  useEffect(() => {
    const onPop = () => setState(parseUrlState(window.location.search))
    window.addEventListener('popstate', onPop)
    return () => window.removeEventListener('popstate', onPop)
  }, [])

  const source = useMemo(() => resolveDataSource(state, import.meta.env.VITE_DEFAULT_SOURCE), [state.mock, state.source])
  const sourceKey = JSON.stringify(source)
  const overview = useRemote<OverviewView>((signal) => loadOverview(source, signal), sourceKey, [source])

  const go = useCallback((next: UrlState, options?: { replace?: boolean }) => {
    const url = `${window.location.pathname}${serializeUrlState(next)}${window.location.hash}`
    if (options?.replace) window.history.replaceState(null, '', url)
    else window.history.pushState(null, '', url)
    stateRef.current = next
    setState(next)
  }, [])

  const navigate = useCallback((patch: Partial<UrlState>, options?: { replace?: boolean }) => go(nextState(stateRef.current, patch), options), [go])

  const [inspectorOpen, setInspectorOpen] = useState(false)

  const app: AppApi = useMemo(
    () => ({
      state,
      source,
      navigate,
      href: (patch) => serializeUrlState(nextState(state, patch)) || window.location.pathname,
    }),
    [state, source, navigate],
  )

  // Move focus to the page heading after a navigation, for keyboard and
  // screen-reader users; the first render keeps the browser's own focus.
  const firstPlace = useRef(true)
  useEffect(() => {
    if (firstPlace.current) {
      firstPlace.current = false
      return
    }
    document.getElementById('main')?.scrollTo({ top: 0 })
    document.getElementById('page-title')?.focus({ preventScroll: true })
  }, [state.view])

  const [theme, setTheme] = useState<'dark' | 'light'>(initialTheme)
  useEffect(() => {
    document.documentElement.dataset.theme = theme
    try {
      window.localStorage.setItem('agnostic-ai.theme', theme)
    } catch {
      // Storage may be unavailable; the theme still applies for this visit.
    }
  }, [theme])

  const sourceError = source.kind === 'invalid' ? new LoadError(source.reason, `Received: ${source.value}`) : undefined

  return (
    <AppContext.Provider value={app}>
      <div className="app">
        <a className="skip-link" href="#main">
          Skip to content
        </a>
        <Header
          overview={overview}
          inspectorOpen={inspectorOpen}
          onToggleInspector={() => setInspectorOpen((open) => !open)}
          theme={theme}
          onToggleTheme={() => setTheme((t) => (t === 'dark' ? 'light' : 'dark'))}
        />
        <div className="shell">
          <Nav overview={overview} />
          <main id="main" className="main" tabIndex={-1}>
            <div className="page">
              {sourceError && <ErrorPanel error={sourceError} />}
              <SectionPage sectionId={state.view} overview={overview} />
            </div>
          </main>
        </div>
        {inspectorOpen && <JsonInspector onClose={() => setInspectorOpen(false)} overview={overview} />}
      </div>
    </AppContext.Provider>
  )
}
