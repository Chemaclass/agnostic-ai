// The application: URL-driven state, data loading, and page routing.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { OverviewView } from './contract'
import { loadOverview, type Loaded } from './api'
import { AppContext, type AppApi } from './components/app-context'
import { GlobalSearch } from './components/GlobalSearch'
import { JsonInspector } from './components/JsonInspector'
import { SectionPage } from './components/SectionFrame'
import { HealthPage } from './components/Health'
import { ConversationsPage } from './components/Conversations'
import { RefreshFailure } from './components/Banner'
import { Header, MenuSheet, Nav, SourceNotice, useServedConfig, type Remote } from './components/Shell'
import { RecheckProvider, useRecheck } from './components/Recheck'
import { APPLE_PLATFORM } from './components/ui'
import { nextState, parseUrlState, resolveDataSource, serializeUrlState, type UrlState, type ViewId } from './model/url'

// A build for a host with no server can name a snapshot to read by default.
const DEFAULT_SOURCE = import.meta.env.VITE_DEFAULT_SOURCE

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

function Page({ view, overview, stale }: { view: ViewId; overview: Remote<OverviewView>; stale: boolean }) {
  if (view === 'conversations') return <ConversationsPage />
  if (view === 'health') return <HealthPage overview={overview} stale={stale} />
  return <SectionPage sectionId={view} overview={overview} stale={stale} />
}

export default function App() {
  const [state, setState] = useState<UrlState>(() => parseUrlState(window.location.search, DEFAULT_SOURCE))
  const stateRef = useRef(state)
  stateRef.current = state

  useEffect(() => {
    const onPop = () => setState(parseUrlState(window.location.search, DEFAULT_SOURCE))
    window.addEventListener('popstate', onPop)
    return () => window.removeEventListener('popstate', onPop)
  }, [])

  const source = useMemo(() => resolveDataSource(state, DEFAULT_SOURCE), [state.mock, state.source])
  const sourceKey = JSON.stringify(source)
  const [reloads, setReloads] = useState(0)
  const [savedBeforeRefresh, setSavedBeforeRefresh] = useState(false)
  const reload = useCallback((saved = false) => {
    if (saved) {
      setSavedBeforeRefresh(true)
    }
    setReloads((n) => n + 1)
  }, [])
  const retry = useCallback(() => setReloads((n) => n + 1), [])
  const refresh = retry
  const overview = useRemote<OverviewView>((signal) => loadOverview(source, signal), sourceKey, [source, reloads])
  const { served, pending: servedPending } = useServedConfig(source.kind, reloads)
  const stale = overview.loaded !== undefined && overview.error !== undefined
  const recheck = useRecheck(overview, refresh, source.kind === 'api', sourceKey)
  useEffect(() => {
    if (overview.status !== 'ready') return
    setSavedBeforeRefresh(false)
  }, [overview.status])

  const go = useCallback((next: UrlState, options?: { replace?: boolean }) => {
    const url = `${window.location.pathname}${serializeUrlState(next, DEFAULT_SOURCE)}${window.location.hash}`
    try {
      if (options?.replace) window.history.replaceState(null, '', url)
      else window.history.pushState(null, '', url)
    } catch {
      // Browsers limit history updates per second; the screen still follows the state.
    }
    stateRef.current = next
    setState(next)
  }, [])

  const [reveals, setReveals] = useState(0)
  const navigate = useCallback((patch: Partial<UrlState>, options?: { replace?: boolean }) => {
    if (patch.entry) setReveals((n) => n + 1)
    go(nextState(stateRef.current, patch), options)
  }, [go])

  const [inspectorOpen, setInspectorOpen] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  const closeMenu = useCallback(() => setMenuOpen(false), [])
  const [searchOpen, setSearchOpen] = useState(false)
  const searchOpener = useRef<HTMLElement | null>(null)
  const menuButton = useRef<HTMLButtonElement>(null)
  // The Menu sheet is a modal dialog, so search opens only after it closes.
  const openSearch = useCallback(() => {
    searchOpener.current = menuOpen ? menuButton.current : document.activeElement as HTMLElement | null
    setMenuOpen(false)
    setSearchOpen(true)
  }, [menuOpen])
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
      href: (patch) => serializeUrlState(nextState(state, patch), DEFAULT_SOURCE) || window.location.pathname,
      reload,
      overviewRevision: overview.loaded,
      overview: overview.loaded?.data,
      reveals,
    }),
    [state, source, navigate, reload, overview.loaded, reveals],
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

  const rail = {
    overview,
    served,
    servedPending,
    theme,
    onToggleTheme: toggleTheme,
    onOpenSearch: openSearch,
    inspectorOpen,
    onToggleInspector: () => setInspectorOpen((open) => !open),
  }

  return (
    <AppContext.Provider value={app}>
      <RecheckProvider value={recheck}>
        <div className="app">
          <a className="skip-link" href="#main">
            Skip to content
          </a>
          <div className="rail">
            <Header overview={overview} served={served} servedPending={servedPending} menuButton={menuButton} menuOpen={menuOpen} onOpenMenu={() => setMenuOpen(true)} onOpenSearch={openSearch} />
            <Nav {...rail} />
          </div>
          <main id="main" className="main" tabIndex={-1}>
            <div className="page">
              <SourceNotice overview={overview} />
              {stale && overview.error && <RefreshFailure error={overview.error} saved={savedBeforeRefresh} retrying={overview.status === 'loading'} onRetry={retry} />}
              <Page view={state.view} overview={overview} stale={stale} />
            </div>
          </main>
          <MenuSheet {...rail} open={menuOpen} onClose={closeMenu} />
          {searchOpen && <GlobalSearch overview={overview.loaded?.data} onClose={closeSearch} />}
          {inspectorOpen && <JsonInspector onClose={() => setInspectorOpen(false)} overview={overview} global={served?.global === true} />}
        </div>
      </RecheckProvider>
    </AppContext.Provider>
  )
}
