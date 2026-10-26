// Recheck runs the shared refresh for one finding, then says whether that
// finding still appears. The page, search, and unsaved input stay as they
// are, because the refresh keeps the previous overview on screen.

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import type { ItemFinding, OverviewView } from '../contract'
import { recheckMessage, recheckResult, type RecheckResult } from '../model/recheck'
import type { Remote } from './Shell'
import { Badge, Code } from './ui'

interface Request {
  id: string
  finding: ItemFinding
  /** The item or section the finding sat in, where a result shows once the finding is gone. */
  place: string
  before: OverviewView
}

export interface Rechecked extends Request {
  result: RecheckResult
}

export interface RecheckApi {
  live: boolean
  pending?: string
  rechecked?: Rechecked
  start(id: string, finding: ItemFinding, place: string): void
  dismiss(): void
}

const RecheckContext = createContext<RecheckApi>({ live: false, start() {}, dismiss() {} })
export const RecheckProvider = RecheckContext.Provider

export function useRecheck(overview: Remote<OverviewView>, refresh: () => void, live: boolean, sourceKey: string): RecheckApi {
  const [request, setRequest] = useState<Request>()
  const [rechecked, setRechecked] = useState<Rechecked>()
  const sawLoading = useRef(false)
  useEffect(() => {
    setRequest(undefined)
    setRechecked(undefined)
  }, [sourceKey])
  useEffect(() => {
    if (!request) return
    if (overview.status === 'loading') {
      sawLoading.current = true
      return
    }
    if (!sawLoading.current) return
    const after = overview.loaded?.data
    const result: RecheckResult = overview.status === 'error' || !after ? { state: 'failed', added: [] } : recheckResult(request, request.before, after)
    setRechecked({ ...request, result })
    setRequest(undefined)
  }, [overview, request])
  const before = overview.loaded?.data
  const start = useCallback((id: string, finding: ItemFinding, place: string) => {
    if (!before) return
    sawLoading.current = false
    setRechecked(undefined)
    setRequest({ id, finding, place, before })
    refresh()
  }, [before, refresh])
  const dismiss = useCallback(() => setRechecked(undefined), [])
  return useMemo(() => ({ live, pending: request?.id, rechecked, start, dismiss }), [live, request, rechecked, start, dismiss])
}

export function useRechecks(): RecheckApi {
  return useContext(RecheckContext)
}

/** Runs the checks again for one finding; only a running server can. */
export function RecheckAction({ id, finding, place }: { id: string; finding: ItemFinding; place: string }) {
  const recheck = useRechecks()
  if (!recheck.live) return null
  return (
    <button type="button" className="button finding-recheck" disabled={recheck.pending !== undefined} onClick={() => recheck.start(id, finding, place)}>
      {recheck.pending === id ? 'Rechecking…' : 'Recheck'}
    </button>
  )
}

/** What the last recheck found for a finding still on the page. */
export function RecheckNote({ id, finding }: { id: string; finding: ItemFinding }) {
  const recheck = useRechecks()
  if (!recheck.live) return null
  const shown = recheck.rechecked
  const result = shown && (shown.result.id ?? shown.id) === id && (shown.result.state === 'remaining' || shown.result.state === 'failed') ? shown.result : undefined
  const text = recheck.pending === id ? 'Running the checks again…' : result ? recheckMessage(result, finding) : ''
  return <p className={`finding-recheck-result${text ? '' : ' is-empty'}`} role="status">{text}</p>
}

export function isNewSinceRecheck(recheck: RecheckApi, id: string): boolean {
  return recheck.rechecked?.result.added.some((f) => f.id === id) ?? false
}

/** The result for a finding that left this place: fixed, or unknown because lint stopped. */
export function useGoneFinding(place: string, ids: string[]): Rechecked | undefined {
  const shown = useRechecks().rechecked
  if (!shown || shown.place !== place) return undefined
  if (shown.result.state !== 'resolved' && shown.result.state !== 'incomplete') return undefined
  return ids.includes(shown.id) ? undefined : shown
}

export function GoneFinding({ rechecked }: { rechecked: Rechecked }) {
  const recheck = useRechecks()
  const ref = useRef<HTMLLIElement>(null)
  useEffect(() => {
    ref.current?.focus({ preventScroll: true })
  }, [rechecked])
  const f = rechecked.finding
  const resolved = rechecked.result.state === 'resolved'
  return (
    <li ref={ref} tabIndex={-1} className={`finding finding-rechecked${resolved ? ' is-resolved' : ' finding-warn'}`}>
      <div className="finding-body">
        <p className="finding-recheck-result" role="status">{recheckMessage(rechecked.result, f)}</p>
        <div className="finding-meta">
          <Badge tone={resolved ? 'permitted' : 'approval'}>{resolved ? 'No longer appears' : 'Last known'}</Badge>
          {f.code && <Code>{f.code}</Code>}
          <span className="muted">{f.source}</span>
        </div>
        <p className={`finding-message${resolved ? ' muted' : ''}`}>{f.message}</p>
        {f.path && <p className="finding-source muted"><Code>{f.path}{f.line ? `:${f.line}` : ''}</Code></p>}
        <button type="button" className="button button-quiet" onClick={recheck.dismiss}>Dismiss</button>
      </div>
    </li>
  )
}
