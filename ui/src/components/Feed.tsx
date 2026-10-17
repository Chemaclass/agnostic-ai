// The feed: evidence-backed cards, most consequential first, one
// highlighted at a time. j/k move, y accepts the first action, n
// dismisses; a touch swipe right or left does the same. Dismissals and
// snoozes are kept in this browser only.

import { useCallback, useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'
import type { AuthorityView, Card, CardAction, OverviewView } from '../contract'
import { findResource, primaryDomain } from '../model/domains'
import { cardKindLabel, dismissCard, partitionFeed, restoreCard, snoozeCard, type FeedDecisions } from '../model/feed'
import { formatTime, plural, sourceRef } from '../model/format'
import { previewProposal } from '../model/proposal'
import { parseInAppHref, type UrlState } from '../model/url'
import type { Tone } from '../model/effects'
import { ErrorPanel } from './Banner'
import { PageHeader, useApp } from './app-context'
import type { Remote } from './Shell'
import { Badge, Code, copyText, useReducedMotion } from './ui'

const SOURCE_LABELS: Record<string, string> = {
  authority: 'the Authority view',
  drift: 'the drift check',
  doctor: 'agnostic-ai doctor',
  lint: 'agnostic-ai lint',
}

export function priorityTone(priority: number): Tone {
  if (priority <= 2) return 'accent'
  if (priority === 3) return 'info'
  return 'neutral'
}

const SWIPE_DISTANCE = 80

interface Status {
  text: string
  undo?: () => void
}

export function FeedPage({
  overview,
  authority,
  decisions,
  updateDecisions,
  acted,
  markActed,
}: {
  overview: Remote<OverviewView>
  authority: Remote<AuthorityView>
  decisions: FeedDecisions
  updateDecisions: (update: (prev: FeedDecisions) => FeedDecisions) => void
  acted: ReadonlySet<string>
  markActed: (id: string) => void
}) {
  const app = useApp()
  const reduced = useReducedMotion()
  const [showHidden, setShowHidden] = useState(false)
  const [highlightId, setHighlightId] = useState<string | undefined>(app.state.card)
  const [leaving, setLeaving] = useState<{ id: string; dir: 'left' | 'right' } | null>(null)
  const [status, setStatus] = useState<Status | null>(null)
  const pendingTimer = useRef<number | undefined>(undefined)
  useEffect(() => () => window.clearTimeout(pendingTimer.current), [])

  const data = overview.loaded?.data
  const partition = data ? partitionFeed(data.feed, decisions, Date.now(), acted) : undefined
  const visible = partition ? [...partition.pending, ...partition.acted] : []
  const current = visible.find((c) => c.id === highlightId) ?? visible[0]

  const focusCard = useCallback((id: string) => {
    const el = document.getElementById(`card-${id}`)
    el?.focus({ preventScroll: true })
    el?.scrollIntoView({ block: 'nearest', behavior: 'auto' })
  }, [])

  const move = useCallback(
    (delta: number) => {
      if (!current) return
      const index = visible.findIndex((c) => c.id === current.id)
      const next = visible[Math.min(visible.length - 1, Math.max(0, index + delta))]
      if (next && next.id !== current.id) {
        setHighlightId(next.id)
        window.requestAnimationFrame(() => focusCard(next.id))
      }
    },
    [current, visible, focusCard],
  )

  const proposeFrom = useCallback(
    (card: Card, action: CardAction) => {
      const proposal = action.proposal
      if (!proposal) return
      const view = authority.loaded?.data
      const patch: Partial<UrlState> = { view: 'authority', proposal: { ...proposal, changes: [...proposal.changes] }, domain: card.domainId, resource: undefined }
      if (proposal.agentId && proposal.agentId !== view?.agent.id) patch.agentId = proposal.agentId
      if (proposal.target && proposal.target !== view?.harness.target) patch.target = proposal.target
      // Land on the first affected resource when the loaded view can tell.
      if (view && !patch.agentId && !patch.target) {
        const first = previewProposal(view, proposal.changes).changes[0]
        const resource = first ? findResource(view, first.resourceId) : undefined
        if (resource) {
          patch.resource = resource.id
          patch.domain = card.domainId && resource.domainIds.includes(card.domainId) ? card.domainId : primaryDomain(view, resource)
        }
      }
      app.navigate(patch)
    },
    [app, authority.loaded],
  )

  const perform = useCallback(
    (card: Card, action: CardAction) => {
      switch (action.kind) {
        case 'open': {
          markActed(card.id)
          const target = parseInAppHref(action.href, app.state, window.location.href)
          if (target) app.go(target)
          else setStatus({ text: 'This card links outside the app, so nothing was opened.' })
          break
        }
        case 'propose':
          markActed(card.id)
          proposeFrom(card, action)
          break
        case 'copy-command': {
          markActed(card.id)
          const command = action.command ?? ''
          void copyText(command).then((ok) => setStatus({ text: ok ? `Copied: ${command}` : `Copy failed. The command is: ${command}` }))
          break
        }
        case 'snooze':
          updateDecisions((d) => snoozeCard(d, card, Date.now()))
          setStatus({ text: `Snoozed “${card.title}” for a day. Kept in this browser.`, undo: () => updateDecisions((d) => restoreCard(d, card.fingerprint)) })
          break
        case 'dismiss':
        default:
          updateDecisions((d) => dismissCard(d, card, Date.now()))
          setStatus({ text: `Dismissed “${card.title}”. Kept in this browser.`, undo: () => updateDecisions((d) => restoreCard(d, card.fingerprint)) })
          break
      }
    },
    [app, markActed, proposeFrom, updateDecisions],
  )

  const decide = useCallback(
    (card: Card, choice: 'accept' | 'dismiss', explicit?: CardAction) => {
      const action: CardAction | undefined = explicit ?? (choice === 'accept' ? card.actions[0] : card.actions.find((a) => a.kind === 'dismiss') ?? { id: 'dismiss', kind: 'dismiss', label: 'Dismiss' })
      if (!action) return
      const index = visible.findIndex((c) => c.id === card.id)
      const leavesList = action.kind === 'dismiss' || action.kind === 'snooze' || action.kind === 'copy-command'
      if (leavesList) {
        const next = visible[index + 1] ?? visible[index - 1]
        setHighlightId(next?.id)
      }
      const commit = () => {
        setLeaving(null)
        perform(card, action)
        if (leavesList) {
          const next = visible[index + 1] ?? visible[index - 1]
          if (next) window.requestAnimationFrame(() => focusCard(next.id))
        }
      }
      if (reduced || leaving) {
        commit()
        return
      }
      setLeaving({ id: card.id, dir: choice === 'accept' ? 'right' : 'left' })
      pendingTimer.current = window.setTimeout(commit, 240)
    },
    [visible, perform, reduced, leaving, focusCard],
  )

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) return
      const target = event.target as HTMLElement | null
      if (target?.closest('input, textarea, select, [contenteditable="true"], [role="dialog"]')) return
      if (!current) return
      const inFeed = !!target?.closest('.feed-list')
      switch (event.key) {
        case 'j':
          move(1)
          break
        case 'k':
          move(-1)
          break
        case 'ArrowDown':
          if (!inFeed) return
          move(1)
          break
        case 'ArrowUp':
          if (!inFeed) return
          move(-1)
          break
        case 'y':
          decide(current, 'accept')
          break
        case 'n':
          decide(current, 'dismiss')
          break
        default:
          return
      }
      event.preventDefault()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [current, move, decide])

  if (!data) {
    return (
      <>
        <PageHeader title="Feed" subtitle="Decisions and insights from read-only checks, most consequential first." />
        {overview.error ? <ErrorPanel error={overview.error} /> : <p className="loading" aria-busy="true">Loading the feed…</p>}
      </>
    )
  }

  const hiddenCount = partition!.hidden.length

  return (
    <>
      <PageHeader title="Feed" subtitle="Decisions and insights from read-only checks, most consequential first.">
        <div className="feed-toolbar">
          <p className="key-hint" id="feed-keys">
            <kbd>j</kbd>/<kbd>k</kbd> move · <kbd>y</kbd> accept the first action · <kbd>n</kbd> dismiss · swipe right or left on touch screens
          </p>
          <button type="button" className="button button-quiet" aria-pressed={showHidden} onClick={() => setShowHidden((v) => !v)}>
            Show dismissed{hiddenCount ? ` (${hiddenCount})` : ''}
          </button>
        </div>
        <div className="feed-status" role="status" aria-live="polite">
          {status && (
            <p>
              {status.text}{' '}
              {status.undo && (
                <button
                  type="button"
                  className="link-button"
                  onClick={() => {
                    status.undo?.()
                    setStatus({ text: 'Restored. Kept in this browser.' })
                  }}
                >
                  Undo
                </button>
              )}
            </p>
          )}
        </div>
      </PageHeader>


      {data.feed.length === 0 ? (
        <div className="empty">
          <p className="empty-title">Nothing to decide. Every check is clean at revision {data.snapshot.revision}.</p>
        </div>
      ) : visible.length === 0 ? (
        <div className="empty">
          <p className="empty-title">Nothing left to decide.</p>
          <p>
            {plural(hiddenCount, 'dismissed or snoozed card is', 'dismissed or snoozed cards are')} hidden; they are kept in this browser.
          </p>
        </div>
      ) : (
        <ol className="feed-list" aria-label="Feed cards" aria-describedby="feed-keys">
          {visible.map((card) => (
            <FeedCard
              key={card.id}
              card={card}
              highlighted={card.id === current?.id}
              acted={acted.has(card.id)}
              leaving={leaving?.id === card.id ? leaving.dir : undefined}
              onHighlight={() => setHighlightId(card.id)}
              onAction={(action) => decide(card, action.kind === 'dismiss' || action.kind === 'snooze' ? 'dismiss' : 'accept', action)}
              onSwipe={(dir) => decide(card, dir === 'right' ? 'accept' : 'dismiss')}
            />
          ))}
        </ol>
      )}

      {showHidden && (
        <section className="hidden-cards" aria-labelledby="hidden-title">
          <h2 id="hidden-title">Dismissed and snoozed</h2>
          <p className="muted">Kept in this browser only. A card comes back when its finding changes.</p>
          {hiddenCount === 0 ? (
            <p className="muted">No card is dismissed or snoozed.</p>
          ) : (
            <ul className="hidden-list">
              {partition!.hidden.map(({ card, reason, until }) => (
                <li key={card.id} className="hidden-item">
                  <span>
                    <strong>{card.title}</strong>
                    <span className="muted"> · {reason === 'dismissed' ? 'Dismissed' : `Snoozed until ${formatTime(new Date(until!).toISOString())}`}</span>
                  </span>
                  <button
                    type="button"
                    className="button button-quiet"
                    onClick={() => {
                      updateDecisions((d) => restoreCard(d, card.fingerprint))
                      setStatus({ text: `Restored “${card.title}”.` })
                    }}
                  >
                    Restore
                  </button>
                </li>
              ))}
            </ul>
          )}
        </section>
      )}
    </>
  )
}

function FeedCard({
  card,
  highlighted,
  acted,
  leaving,
  onHighlight,
  onAction,
  onSwipe,
}: {
  card: Card
  highlighted: boolean
  acted: boolean
  leaving?: 'left' | 'right'
  onHighlight: () => void
  onAction: (action: CardAction) => void
  onSwipe: (dir: 'left' | 'right') => void
}) {
  const start = useRef<{ x: number; y: number } | null>(null)
  const [dx, setDx] = useState(0)

  const swipe = highlighted
    ? {
        onPointerDown: (e: ReactPointerEvent) => {
          if (e.pointerType === 'mouse') return
          start.current = { x: e.clientX, y: e.clientY }
        },
        onPointerMove: (e: ReactPointerEvent) => {
          if (!start.current) return
          const x = e.clientX - start.current.x
          if (Math.abs(x) > Math.abs(e.clientY - start.current.y)) setDx(x)
        },
        onPointerUp: (e: ReactPointerEvent) => {
          if (!start.current) return
          const x = e.clientX - start.current.x
          start.current = null
          setDx(0)
          if (x > SWIPE_DISTANCE) onSwipe('right')
          else if (x < -SWIPE_DISTANCE) onSwipe('left')
        },
        onPointerCancel: () => {
          start.current = null
          setDx(0)
        },
      }
    : {}

  const commands = card.actions.filter((a) => a.kind === 'copy-command' && a.command)
  const classes = ['feed-card', highlighted ? 'is-current' : '', acted ? 'is-acted' : '', leaving ? `leaving-${leaving}` : '', dx ? 'is-dragging' : ''].filter(Boolean).join(' ')

  return (
    <li className="feed-item" data-card-id={card.id} data-priority={card.priority} data-first-action={card.actions[0]?.kind}>
      <article
        id={`card-${card.id}`}
        className={classes}
        tabIndex={highlighted ? 0 : -1}
        aria-current={highlighted ? 'true' : undefined}
        aria-labelledby={`card-title-${card.id}`}
        onClick={(e) => {
          if (!highlighted && !(e.target as HTMLElement).closest('button, a')) onHighlight()
        }}
        onFocus={() => {
          if (!highlighted) onHighlight()
        }}
        style={dx ? { transform: `translateX(${dx}px) rotate(${dx / 40}deg)` } : undefined}
        {...swipe}
      >
        <div className="card-meta">
          <Badge tone={priorityTone(card.priority)}>{card.priorityLabel}</Badge>
          <span className="card-kind">{cardKindLabel(card.kind)}</span>
          <span className="card-source">from {SOURCE_LABELS[card.source] ?? card.source}</span>
          {acted && <span className="card-acted">Acted on in this session</span>}
        </div>
        <h2 className="card-title" id={`card-title-${card.id}`}>
          {card.title}
        </h2>
        <p className="card-consequence">{card.consequence}</p>
        <ul className="card-evidence" aria-label="Evidence">
          {card.evidence.map((e, i) => (
            <li key={i}>
              <Code>{sourceRef(e)}</Code>
              {highlighted && e.detail && <span className="evidence-detail"> {e.detail}</span>}
            </li>
          ))}
        </ul>
        {highlighted && (
          <>
            {commands.map((a) => (
              <p key={a.id} className="card-command">
                <Code>{a.command}</Code>
              </p>
            ))}
            <div className="card-actions">
              {card.actions.map((action, i) => (
                <button key={action.id} type="button" className={`button${i === 0 ? ' button-primary' : action.kind === 'dismiss' || action.kind === 'snooze' ? ' button-quiet' : ''}`} onClick={() => onAction(action)}>
                  {action.label}
                  {i === 0 && <kbd aria-hidden="true">y</kbd>}
                  {action.kind === 'dismiss' && i !== 0 && <kbd aria-hidden="true">n</kbd>}
                </button>
              ))}
            </div>
            <p className="card-note">Dismiss and snooze are kept in this browser.</p>
          </>
        )}
      </article>
    </li>
  )
}
