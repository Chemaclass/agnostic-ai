// The application: URL-driven state, data loading, and page routing.

import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import type { AuthorityView, Change, ImpactResult, OverviewView } from './contract'
import { LoadError, loadAuthority, loadOverview, validateProposal, type Loaded } from './api'
import { AppContext, type AppApi, type InspectorTab } from './components/app-context'
import { AuthorityPage } from './components/Authority'
import { FeedPage } from './components/Feed'
import { InspectPage } from './components/Inspect'
import { JsonInspector } from './components/JsonInspector'
import { SectionPage } from './components/Section'
import { ErrorPanel } from './components/Banner'
import { Footer, Header, Nav, type Remote } from './components/Shell'
import { loadDecisions, partitionFeed, pruneDecisions, saveDecisions, type FeedDecisions } from './model/feed'
import { buildProposal } from './model/proposal'
import { nextState, parseUrlState, resolveDataSource, serializeUrlState, type UrlState } from './model/url'

export interface ImpactState {
  status: 'idle' | 'validating' | 'done' | 'error'
  key?: string
  result?: Loaded<ImpactResult>
  error?: Error
}

function safeStorage(): Storage | undefined {
  try {
    return window.localStorage
  } catch {
    return undefined
  }
}

function initialTheme(): 'dark' | 'light' {
  try {
    return window.localStorage.getItem('agnostic-ai.theme') === 'light' ? 'light' : 'dark'
  } catch {
    return 'dark'
  }
}

// Loads a document and keeps the previous one on screen while a new
// subject or tool loads; a different data source starts from nothing, so
// one source's data never stands in for another's.
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

  const authority = useRemote<AuthorityView>((signal) => loadAuthority(source, { agentId: state.agentId, target: state.target }, signal), sourceKey, [source, state.agentId, state.target])
  const overview = useRemote<OverviewView>((signal) => loadOverview(source, signal), sourceKey, [source])

  const [announcement, setAnnouncement] = useState('')
  const announce = useCallback((message: string) => {
    // Clearing first makes a repeated message announce again.
    setAnnouncement('')
    window.setTimeout(() => setAnnouncement(message), 30)
  }, [])

  const go = useCallback((next: UrlState, options?: { replace?: boolean }) => {
    const url = `${window.location.pathname}${serializeUrlState(next)}${window.location.hash}`
    if (options?.replace) window.history.replaceState(null, '', url)
    else window.history.pushState(null, '', url)
    stateRef.current = next
    setState(next)
  }, [])

  const navigate = useCallback((patch: Partial<UrlState>, options?: { replace?: boolean }) => go(nextState(stateRef.current, patch), options), [go])

  const [inspector, setInspector] = useState<{ open: boolean; tab: InspectorTab }>({ open: false, tab: 'authority' })
  const defaultInspectorTab = useCallback((): InspectorTab => (stateRef.current.view === 'authority' ? 'authority' : 'overview'), [])
  const openInspector = useCallback((tab?: InspectorTab) => setInspector({ open: true, tab: tab ?? defaultInspectorTab() }), [defaultInspectorTab])

  const app: AppApi = useMemo(
    () => ({
      state,
      source,
      navigate,
      go,
      href: (patch) => serializeUrlState(nextState(state, patch)) || window.location.pathname,
      announce,
      openInspector,
    }),
    [state, source, navigate, go, announce, openInspector],
  )

  // Move focus to the page heading after a navigation, for keyboard and
  // screen-reader users; the first render keeps the browser's own focus.
  const placeKey = `${state.view}|${state.domain ?? ''}|${state.resource ?? ''}`
  const firstPlace = useRef(true)
  useEffect(() => {
    if (firstPlace.current) {
      firstPlace.current = false
      return
    }
    window.scrollTo({ top: 0 })
    document.getElementById('page-title')?.focus({ preventScroll: true })
  }, [placeKey])

  // Feed decisions live in this browser only.
  const [decisions, setDecisions] = useState<FeedDecisions>(() => pruneDecisions(loadDecisions(safeStorage()), Date.now()))
  const updateDecisions = useCallback((update: (prev: FeedDecisions) => FeedDecisions) => {
    setDecisions((prev) => {
      const next = update(prev)
      saveDecisions(safeStorage(), next)
      return next
    })
  }, [])
  const [acted, setActed] = useState<Set<string>>(() => new Set())
  const markActed = useCallback((id: string) => setActed((prev) => new Set(prev).add(id)), [])
  const pendingCount = overview.loaded ? partitionFeed(overview.loaded.data.feed, decisions, Date.now(), acted).pending.length : undefined

  // Proposal validation. A result belongs to one exact proposal; editing
  // the draft drops it, so a stale validation is never shown.
  const [impact, setImpact] = useState<ImpactState>({ status: 'idle' })
  const view = authority.loaded?.data
  const changes: Change[] = state.proposal?.changes ?? []
  const proposalKey = view && changes.length ? JSON.stringify(buildProposal(view, changes)) : undefined
  const currentImpact: ImpactState = impact.key && impact.key === proposalKey ? impact : { status: 'idle' }
  const validate = useCallback(async () => {
    if (!view || !changes.length) return
    const proposal = buildProposal(view, changes)
    const key = JSON.stringify(proposal)
    setImpact({ status: 'validating', key })
    announce('Validating the proposal on the server.')
    try {
      const result = await validateProposal(proposal)
      setImpact({ status: 'done', key, result })
      const r = result.data
      announce(
        r.valid
          ? `Validated, not applied. ${r.summary.changed} operation${r.summary.changed === 1 ? '' : 's'} change: ${r.summary.narrowed} narrowed, ${r.summary.widened} widened, ${r.summary.moved} moved, ${r.summary.indeterminate ?? 0} indeterminate.`
          : `Not valid: ${r.problems.join(' ')}`,
      )
    } catch (error) {
      setImpact({ status: 'error', key, error: error as Error })
      announce(`Validation failed: ${(error as Error).message}`)
    }
  }, [view, changes, announce])

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

  let page: ReactNode
  if (state.view === 'feed') {
    page = <FeedPage overview={overview} authority={authority} decisions={decisions} updateDecisions={updateDecisions} acted={acted} markActed={markActed} />
  } else if (state.view === 'authority') {
    page = <AuthorityPage authority={authority} impact={currentImpact} onValidate={validate} />
  } else if (state.view === 'inspect') {
    page = <InspectPage authority={authority} overview={overview} />
  } else {
    page = <SectionPage sectionId={state.view} overview={overview} authority={authority} decisions={decisions} />
  }

  return (
    <AppContext.Provider value={app}>
      <div className="app">
        <a className="skip-link" href="#main">
          Skip to content
        </a>
        <Header authority={authority} overview={overview} inspectorOpen={inspector.open} onToggleInspector={() => (inspector.open ? setInspector((s) => ({ ...s, open: false })) : openInspector())} />
        <div className="shell">
          <Nav overview={overview} pendingCount={pendingCount} />
          <main id="main" className="main" tabIndex={-1}>
            {sourceError && <ErrorPanel error={sourceError} />}
            {page}
          </main>
        </div>
        <Footer authority={authority} overview={overview} theme={theme} onToggleTheme={() => setTheme((t) => (t === 'dark' ? 'light' : 'dark'))} />
        {inspector.open && (
          <JsonInspector
            tab={inspector.tab}
            onTab={(tab) => setInspector({ open: true, tab })}
            onClose={() => setInspector((s) => ({ ...s, open: false }))}
            authority={authority}
            overview={overview}
            impact={currentImpact}
          />
        )}
        <div className="sr-only" role="status" aria-live="polite" aria-atomic="true">
          {announcement}
        </div>
      </div>
    </AppContext.Provider>
  )
}
