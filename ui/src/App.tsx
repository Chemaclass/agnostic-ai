// The application: URL-driven state, data loading, and page routing.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { OverviewView } from './contract'
import { LoadError, loadOverview, type Loaded } from './api'
import { AppContext, type AppApi } from './components/app-context'
import { GlobalSearch } from './components/GlobalSearch'
import { JsonInspector } from './components/JsonInspector'
import { SectionPage } from './components/Section'
import { ErrorPanel, RefreshFailure } from './components/Banner'
import { Header, Nav, type Remote } from './components/Shell'
import { RecheckProvider, useRecheck } from './components/Recheck'
import { APPLE_PLATFORM } from './components/ui'
import { nextState, parseUrlState, resolveDataSource, serializeUrlState, type UrlState } from './model/url'

// The theme follows the system until the person picks one with the
// toggle; only that choice is stored.
const THEME_KEY = 'agnostic-ai.theme-choice'

function storedTheme(): 'dark' | 'light' | undefined {
  try {
    const stored = window.localStorage.getItem(THEME_KEY)
    return stored === 'light' || stored === 'dark' ? stored : undefined
  } catch {
    return undefined
  }
}

function systemTheme(): 'dark' | 'light' {
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
  const [reloads, setReloads] = useState(0)
  const [savedBeforeRefresh, setSavedBeforeRefresh] = useState(false)
  const [savedSinceRefresh, setSavedSinceRefresh] = useState(false)
  const refreshRequested = useRef(false)
  const reload = useCallback((saved = false) => {
    if (saved) {
      setSavedBeforeRefresh(true)
      setSavedSinceRefresh(true)
      refreshRequested.current = false
    }
    setReloads((n) => n + 1)
  }, [])
  const retry = useCallback(() => setReloads((n) => n + 1), [])
  const refresh = useCallback(() => {
    refreshRequested.current = true
    setReloads((n) => n + 1)
  }, [])
  const overview = useRemote<OverviewView>((signal) => loadOverview(source, signal), sourceKey, [source, reloads])
  const stale = overview.loaded !== undefined && overview.error !== undefined
  const recheck = useRecheck(overview, refresh, source.kind === 'api', sourceKey)
  useEffect(() => {
    if (overview.status !== 'ready') return
    setSavedBeforeRefresh(false)
    if (refreshRequested.current) {
      refreshRequested.current = false
      setSavedSinceRefresh(false)
    }
  }, [overview.status])
  useEffect(() => {
    refreshRequested.current = false
    setSavedSinceRefresh(false)
  }, [sourceKey])

  const go = useCallback((next: UrlState, options?: { replace?: boolean }) => {
    const url = `${window.location.pathname}${serializeUrlState(next)}${window.location.hash}`
    try {
      if (options?.replace) window.history.replaceState(null, '', url)
      else window.history.pushState(null, '', url)
    } catch {
      // Browsers limit history updates per second; the screen still follows the state.
    }
    stateRef.current = next
    setState(next)
  }, [])

  const navigate = useCallback((patch: Partial<UrlState>, options?: { replace?: boolean }) => go(nextState(stateRef.current, patch), options), [go])

  const [inspectorOpen, setInspectorOpen] = useState(false)
  const [searchOpen, setSearchOpen] = useState(false)
  const searchOpener = useRef<HTMLElement | null>(null)
  const openSearch = useCallback(() => {
    searchOpener.current = document.activeElement as HTMLElement | null
    setSearchOpen(true)
  }, [])
  const closeSearch = useCallback((navigated: boolean) => {
    setSearchOpen(false)
    if (!navigated) searchOpener.current?.focus?.()
  }, [])
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((APPLE_PLATFORM ? event.metaKey : event.ctrlKey) && !event.altKey && !event.shiftKey && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        if (!searchOpen) openSearch()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [searchOpen, openSearch])

  const app: AppApi = useMemo(
    () => ({
      state,
      source,
      navigate,
      href: (patch) => serializeUrlState(nextState(state, patch)) || window.location.pathname,
      reload,
      overviewRevision: overview.loaded,
    }),
    [state, source, navigate, reload, overview.loaded],
  )

  // Move focus to the page heading after a navigation, for keyboard and
  // screen-reader users; the first render keeps the browser's own focus.
  const firstPlace = useRef(true)
  useEffect(() => {
    if (firstPlace.current) {
      firstPlace.current = false
      return
    }
    if (stateRef.current.entry) return
    window.scrollTo({ top: 0 })
    document.getElementById('page-title')?.focus({ preventScroll: true })
  }, [state.view])

  const [choice, setChoice] = useState<'dark' | 'light' | undefined>(storedTheme)
  const [system, setSystem] = useState<'dark' | 'light'>(systemTheme)
  useEffect(() => {
    const query = window.matchMedia?.('(prefers-color-scheme: light)')
    if (!query) return
    const onChange = () => setSystem(query.matches ? 'light' : 'dark')
    query.addEventListener('change', onChange)
    return () => query.removeEventListener('change', onChange)
  }, [])
  const theme = choice ?? system
  useEffect(() => {
    document.documentElement.dataset.theme = theme
  }, [theme])
  const toggleTheme = useCallback(() => {
    const next = theme === 'dark' ? 'light' : 'dark'
    setChoice(next)
    try {
      window.localStorage.setItem(THEME_KEY, next)
    } catch {
      // Storage may be unavailable; the choice still applies for this visit.
    }
  }, [theme])

  const sourceError = source.kind === 'invalid' ? new LoadError(source.reason, `Received: ${source.value}`) : undefined

  return (
    <AppContext.Provider value={app}>
      <RecheckProvider value={recheck}>
        <div className="app">
          <a className="skip-link" href="#main">
            Skip to content
          </a>
          <Header
            overview={overview}
            inspectorOpen={inspectorOpen}
            onToggleInspector={() => setInspectorOpen((open) => !open)}
            theme={theme}
            onToggleTheme={toggleTheme}
            onOpenSearch={openSearch}
          />
          <div className="shell">
            <Nav overview={overview} snapshot={source.kind !== 'api'} saved={savedSinceRefresh} onRefresh={refresh} />
            <main id="main" className="main" tabIndex={-1}>
              <div className="page">
                {sourceError && <ErrorPanel error={sourceError} />}
                {stale && overview.error && <RefreshFailure error={overview.error} saved={savedBeforeRefresh} retrying={overview.status === 'loading'} onRetry={retry} />}
                <SectionPage sectionId={state.view} overview={overview} stale={stale} />
              </div>
            </main>
          </div>
          {searchOpen && <GlobalSearch overview={overview.loaded?.data} onClose={closeSearch} />}
          {inspectorOpen && <JsonInspector onClose={() => setInspectorOpen(false)} overview={overview} />}
        </div>
      </RecheckProvider>
    </AppContext.Provider>
  )
}
