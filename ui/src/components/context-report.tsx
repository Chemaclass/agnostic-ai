// The Context page loads one context report, for the tool and file in the
// address, and shares it between the verdict, the facts line and the
// Context size tab. Other pages never load it.

import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { loadContext, LoadError } from '../api'
import type { ContextView } from '../contract'
import { budgetFact } from '../model/context-verdict'
import { contextUsageAvailable } from '../model/url'
import { useApp } from './app-context'

export interface ContextSnapshot {
  view: ContextView
  /** The file this report was asked for; '' when none. */
  file: string
  at: string
}

interface ContextReportState {
  reachable: boolean
  current?: ContextSnapshot
  error?: Error
  loading: boolean
  /** Loads the report again for the tool and file in the address. */
  reload: () => void
}

const ContextReport = createContext<ContextReportState>({ reachable: false, loading: false, reload: () => {} })

export const useContextReport = () => useContext(ContextReport)

export function ContextReportProvider({ children }: { children: ReactNode }) {
  const app = useApp()
  const target = app.state.target
  const file = app.state.file ?? ''
  const [nonce, setNonce] = useState(0)
  const [current, setCurrent] = useState<ContextSnapshot>()
  const [error, setError] = useState<Error>()
  const [loading, setLoading] = useState(false)
  const reachable = contextUsageAvailable(app.source)
  const hasView = useRef(false)
  const droppedStale = useRef(false)

  useEffect(() => {
    if (!reachable) return
    const controller = new AbortController()
    setLoading(true)
    setError(undefined)
    // A target or file left in the address (a removed tool, a stale link) can answer 400 or another tool. Load the default once.
    const dropStale = () => {
      if (hasView.current || droppedStale.current || (!target && !file)) return false
      droppedStale.current = true
      app.navigate({ target: undefined, file: undefined }, { replace: true })
      return true
    }
    loadContext(app.source, target, file, controller.signal)
      .then((loaded) => {
        if (controller.signal.aborted) return
        if (loaded.data.supported && target && loaded.data.target && loaded.data.target !== target && dropStale()) return
        hasView.current = true
        setCurrent({ view: loaded.data, file, at: new Date().toISOString() })
      })
      .catch((err: unknown) => {
        if ((err as Error).name === 'AbortError' || controller.signal.aborted) return
        const refused = err instanceof LoadError && err.status === 400
        if (!(refused && dropStale())) setError(err instanceof Error ? err : new Error('Could not load context size.'))
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [reachable, app.source, target, file, nonce])

  const value = { reachable, current, error, loading, reload: () => setNonce((n) => n + 1) }
  return <ContextReport.Provider value={value}>{children}</ContextReport.Provider>
}

/** The loaded report for the tool in the address, or the last one while the next loads. */
function useShownReport() {
  const app = useApp()
  const { current, error, loading } = useContextReport()
  if (error || !current?.view.supported || !current.view.report) return undefined
  const target = app.state.target
  // While another tool loads, the last figures stay so the header does not jump.
  if (target && current.view.target !== target && !loading) return undefined
  return current.view
}

/** The Context page verdict: what agents read at the start, or the setup counts without a report. */
export function ContextVerdict({ fallback }: { fallback: string }) {
  const { reachable, error, current } = useContextReport()
  const view = useShownReport()
  if (view?.report) {
    const words = view.report.startup.words
    return <>Agents read <span className="context-verdict-key">{words.toLocaleString('en-US')} {words === 1 ? 'word' : 'words'}</span> when a session starts.</>
  }
  if (reachable && !error && (!current || current.view.supported)) return <>Counting what agents read when a session starts.</>
  return <>{fallback}</>
}

/** On-demand words and the budget, beside the verdict. */
export function ContextFacts() {
  const view = useShownReport()
  const report = view?.report
  if (!report) return null
  const budget = budgetFact(report.startup, view.startupWordBudget)
  return (
    <>
      <span className="page-fact">{report.on_demand.words.toLocaleString('en-US')} more on demand</span>
      {budget && <span className={`page-fact${budget.over ? ' page-fact-findings is-warning' : ''}`}>{budget.text}</span>}
    </>
  )
}
