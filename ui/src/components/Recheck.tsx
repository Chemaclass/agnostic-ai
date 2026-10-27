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
  rechecked?: Rechecked
  start(id: string, finding: ItemFinding, section: string, place: string): void
  dismiss(): void
}

const RecheckContext = createContext<RecheckApi>({ live: false, start() {}, dismiss() {} })
export const RecheckProvider = RecheckContext.Provider
/** The id of the section whose findings render below. */
export const RecheckSection = createContext('')
export const RecheckSectionEntries = createContext<Set<string>>(new Set())

export function useRecheck(overview: Remote<OverviewView>, refresh: () => void, live: boolean, sourceKey: string): RecheckApi {
  const [request, setRequest] = useState<Request>()
  const [rechecked, setRechecked] = useState<Rechecked>()
  const sawLoading = useRef(false)
  const shownWith = useRef<unknown>(undefined)
  useEffect(() => {
    setRequest(undefined)
    setRechecked(undefined)
  }, [sourceKey])
  useEffect(() => {
    if (!request) {
      if (rechecked && overview.status === 'ready' && overview.loaded !== shownWith.current) setRechecked(undefined)
      return
    }
    if (overview.status === 'loading') {
      sawLoading.current = true
      return
    }
    if (!sawLoading.current) return
    const after = overview.loaded?.data
    const result: RecheckResult = overview.status === 'error' || !after ? { state: 'failed', added: [] } : recheckResult(request, request.before, after)
    shownWith.current = overview.loaded
    setRechecked({ ...request, result })
    setRequest(undefined)
  }, [overview, request, rechecked])
  const before = overview.loaded?.data
  const start = useCallback((id: string, finding: ItemFinding, section: string, place: string) => {
    if (!before) return
    sawLoading.current = false
    setRechecked(undefined)
    setRequest({ id, finding, section, place, before })
    refresh()
  }, [before, refresh])
  const dismiss = useCallback(() => setRechecked(undefined), [])
  return useMemo(() => ({ live, pending: request?.id, rechecked, start, dismiss }), [live, request, rechecked, start, dismiss])
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
          {f.code && <Code>{f.code}</Code>}
          <span className="muted">{f.source}</span>
        </div>
        <p className="finding-message muted">{f.message}</p>
        {f.path && <p className="finding-source muted"><Code>{f.path}{f.line ? `:${f.line}` : ''}</Code></p>}
        <button type="button" className="button button-quiet" onClick={dismiss}>Dismiss</button>
      </div>
    </li>
  )
}
