// Recheck runs the shared refresh for one finding, then says whether that
// finding still appears. The page, search, and unsaved input stay as they
// are, because the refresh keeps the previous overview on screen.

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import type { ItemFinding, OverviewView } from '../contract'
import { findingHasRawMessage, findingTitle } from '../model/finding-title'
import { recheckAllResult, recheckMessage, recheckResult, type RecheckAllResult, type RecheckResult } from '../model/recheck'
import type { Remote } from './Shell'
import { Badge, Code } from './ui'

interface Request {
  id: string
  finding: ItemFinding
  /** The section, and the item or section list, the finding sat in; a result shows there once the finding is gone. */
  section: string
  place: string
  before: OverviewView
}

export interface Rechecked extends Request {
  result: RecheckResult
}

export interface RecheckApi {
  live: boolean
  pending?: string
  pendingPlace?: string
  rechecked?: Rechecked
  /** True while one refresh rechecks every finding. */
  pendingAll?: boolean
  /** What the last Recheck all found, until the next refresh. */
  all?: RecheckAllResult
  start(id: string, finding: ItemFinding, section: string, place: string): void
  startAll(): void
  dismiss(): void
}

const RecheckContext = createContext<RecheckApi>({ live: false, start() {}, startAll() {}, dismiss() {} })
export const RecheckProvider = RecheckContext.Provider
/** The id of the section whose findings render below. */
export const RecheckSection = createContext('')
export const RecheckSectionEntries = createContext<Set<string>>(new Set())

export function useRecheck(overview: Remote<OverviewView>, refresh: () => void, live: boolean, sourceKey: string): RecheckApi {
  const [request, setRequest] = useState<Request>()
  const [rechecked, setRechecked] = useState<Rechecked>()
  const [allBefore, setAllBefore] = useState<OverviewView>()
  const [all, setAll] = useState<RecheckAllResult>()
  const sawLoading = useRef(false)
  const shownWith = useRef<unknown>(undefined)
  useEffect(() => {
    setRequest(undefined)
    setRechecked(undefined)
    setAllBefore(undefined)
    setAll(undefined)
  }, [sourceKey])
  useEffect(() => {
    if (!request && !allBefore) {
      if ((rechecked || all) && overview.status === 'ready' && overview.loaded !== shownWith.current) {
        setRechecked(undefined)
        setAll(undefined)
      }
      return
    }
    if (overview.status === 'loading') {
      sawLoading.current = true
      return
    }
    if (!sawLoading.current) return
    const after = overview.loaded?.data
    const failed = overview.status === 'error' || !after
    shownWith.current = overview.loaded
    if (request) {
      const result: RecheckResult = failed ? { state: 'failed', added: [] } : recheckResult(request, request.before, after)
      setRechecked({ ...request, result })
      setRequest(undefined)
    } else if (allBefore) {
      setAll(failed ? { state: 'failed', resolved: 0, unconfirmed: 0, unknown: 0, added: 0, remaining: 0 } : recheckAllResult(allBefore, after))
      setAllBefore(undefined)
    }
  }, [overview, request, rechecked, allBefore, all])
  const before = overview.loaded?.data
  const start = useCallback((id: string, finding: ItemFinding, section: string, place: string) => {
    if (!before) return
    sawLoading.current = false
    setRechecked(undefined)
    setAll(undefined)
    setRequest({ id, finding, section, place, before })
    refresh()
  }, [before, refresh])
  const startAll = useCallback(() => {
    if (!before) return
    sawLoading.current = false
    setRechecked(undefined)
    setAll(undefined)
    setAllBefore(before)
    refresh()
  }, [before, refresh])
  const dismiss = useCallback(() => setRechecked(undefined), [])
  return useMemo(
    () => ({ live, pending: request?.id ?? (allBefore ? '' : undefined), pendingPlace: request?.place, pendingAll: allBefore !== undefined, rechecked, all, start, startAll, dismiss }),
    [live, request, allBefore, rechecked, all, start, startAll, dismiss],
  )
}

export function useRechecks(): RecheckApi {
  return useContext(RecheckContext)
}

// A result replaces what had focus when the finding left the page; it
// never takes focus from somewhere the person moved to meanwhile.
function takeFocus(element: HTMLElement | null, fromButton: boolean) {
  const active = document.activeElement
  if (!element || (active && active !== document.body && !(fromButton && active.closest('.finding-recheck')))) return
  element.scrollIntoView({ block: 'nearest' })
  element.focus({ preventScroll: true })
}

/** Runs the checks again for one finding; only a running server can. */
export function RecheckAction({ id, finding, place }: { id: string; finding: ItemFinding; place: string }) {
  const recheck = useRechecks()
  const section = useContext(RecheckSection)
  if (!recheck.live) return null
  const busy = recheck.pending !== undefined
  return (
    <button type="button" className="button finding-recheck" aria-disabled={busy || undefined} onClick={() => !busy && recheck.start(id, finding, section, place)}>
      {recheck.pending === id ? 'Rechecking…' : 'Recheck'}
    </button>
  )
}

const SHOWN_ON_FINDING = new Set<RecheckResult['state']>(['remaining', 'failed', 'incomplete'])

/** What the last recheck found for a finding still on the page, or shown as last known. */
export function RecheckNote({ id, finding }: { id: string; finding: ItemFinding }) {
  const recheck = useRechecks()
  const ref = useRef<HTMLParagraphElement>(null)
  const shown = recheck.rechecked
  const result = shown && (shown.result.id ?? shown.id) === id && SHOWN_ON_FINDING.has(shown.result.state) ? shown.result : undefined
  useEffect(() => {
    if (result) takeFocus(ref.current, false)
  }, [result])
  if (!recheck.live) return null
  const text = recheck.pending === id ? 'Running the checks again…' : result ? recheckMessage(result, finding) : ''
  return <p ref={ref} tabIndex={result ? -1 : undefined} className={`finding-recheck-result${text ? '' : ' is-empty'}`} role="status">{text}</p>
}

export function isNewSinceRecheck(recheck: RecheckApi, id: string): boolean {
  return recheck.rechecked?.result.added.some((f) => f.id === id) ?? false
}

/** The result for a finding that left this place, or whose item is gone from the section. */
export function useGoneFinding(place: string, ids: string[], orphanPlace?: string): Rechecked | undefined {
  const shown = useRechecks().rechecked
  const entries = useContext(RecheckSectionEntries)
  if (!shown || (shown.place !== place && shown.place !== orphanPlace)) return undefined
  if (shown.result.state !== 'resolved' && shown.result.state !== 'unconfirmed' && shown.result.state !== 'incomplete' && !(shown.result.state === 'remaining' && !entries.has(shown.result.id ?? shown.id))) return undefined
  return ids.includes(shown.id) ? undefined : shown
}

export function GoneFinding({ rechecked }: { rechecked: Rechecked }) {
  const recheck = useRechecks()
  const ref = useRef<HTMLLIElement>(null)
  useEffect(() => {
    takeFocus(ref.current, true)
  }, [rechecked])
  const f = rechecked.finding
  const resolved = rechecked.result.state === 'resolved'
  const remaining = rechecked.result.state === 'remaining'
  const dismiss = () => {
    const heading = ref.current?.closest('section')?.querySelector<HTMLElement>('h2')
    recheck.dismiss()
    if (heading) {
      if (!heading.hasAttribute('tabindex')) heading.tabIndex = -1
      heading.focus({ preventScroll: true })
    }
  }
  return (
    <li ref={ref} tabIndex={-1} className={`finding finding-rechecked${resolved ? ' is-resolved' : ' finding-warn'}`}>
      <div className="finding-body">
        <p className="finding-recheck-result" role="status">{recheckMessage(rechecked.result, f)}</p>
        <div className="finding-meta">
          <Badge tone={resolved ? 'permitted' : 'approval'}>{resolved ? 'No longer appears' : remaining ? 'Still appears' : 'Not confirmed'}</Badge>
        </div>
        <p className="finding-message muted">{findingTitle(f)}</p>
        {findingHasRawMessage(f) && <p className="muted">{f.message}</p>}
        {f.path && <p className="finding-source muted"><Code>{f.path}{f.line ? `:${f.line}` : ''}</Code></p>}
        <button type="button" className="button button-quiet" onClick={dismiss}>Dismiss</button>
      </div>
    </li>
  )
}
