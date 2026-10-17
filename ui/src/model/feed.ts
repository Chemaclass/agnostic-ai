// Feed ranking and the reader's own decisions. Dismissals and snoozes
// are kept in this browser only (localStorage), keyed by the card's
// fingerprint, which the server keeps stable across revisions until the
// finding itself changes.

import type { Card, CardAction } from '../contract'

export interface DismissRecord {
  at: number
  title: string
}

export interface SnoozeRecord {
  until: number
  title: string
}

export interface FeedDecisions {
  dismissed: Record<string, DismissRecord>
  snoozed: Record<string, SnoozeRecord>
}

export const STORAGE_KEY = 'agnostic-ai.feed.v1'
/** The contract carries no snooze length; a day is the client's choice. */
export const SNOOZE_MS = 24 * 60 * 60 * 1000

export function emptyDecisions(): FeedDecisions {
  return { dismissed: {}, snoozed: {} }
}

/** Priority first (lowest number), decisions before insights, then server order. */
export function rankFeed(cards: Card[]): Card[] {
  return cards
    .map((card, index) => ({ card, index }))
    .sort(
      (a, b) =>
        (a.card.priority ?? 99) - (b.card.priority ?? 99) ||
        Number(a.card.kind !== 'decision') - Number(b.card.kind !== 'decision') ||
        a.index - b.index,
    )
    .map(({ card }) => card)
}

export type HiddenReason = 'dismissed' | 'snoozed'

export interface FeedPartition {
  /** Cards waiting for a decision, ranked. */
  pending: Card[]
  /** Cards acted on in this session; they stay listed, after the pending ones. */
  acted: Card[]
  /** Dismissed or snoozed cards, with the reason. */
  hidden: Array<{ card: Card; reason: HiddenReason; until?: number }>
}

export function partitionFeed(cards: Card[], decisions: FeedDecisions, now: number, acted: ReadonlySet<string> = new Set()): FeedPartition {
  const result: FeedPartition = { pending: [], acted: [], hidden: [] }
  for (const card of rankFeed(cards)) {
    if (decisions.dismissed[card.fingerprint]) {
      result.hidden.push({ card, reason: 'dismissed' })
      continue
    }
    const snooze = decisions.snoozed[card.fingerprint]
    if (snooze && snooze.until > now) {
      result.hidden.push({ card, reason: 'snoozed', until: snooze.until })
      continue
    }
    if (acted.has(card.id)) result.acted.push(card)
    else result.pending.push(card)
  }
  return result
}

export function dismissCard(decisions: FeedDecisions, card: Card, now: number): FeedDecisions {
  return { ...decisions, dismissed: { ...decisions.dismissed, [card.fingerprint]: { at: now, title: card.title } } }
}

export function snoozeCard(decisions: FeedDecisions, card: Card, now: number, duration = SNOOZE_MS): FeedDecisions {
  return { ...decisions, snoozed: { ...decisions.snoozed, [card.fingerprint]: { until: now + duration, title: card.title } } }
}

export function restoreCard(decisions: FeedDecisions, fingerprint: string): FeedDecisions {
  const dismissed = { ...decisions.dismissed }
  const snoozed = { ...decisions.snoozed }
  delete dismissed[fingerprint]
  delete snoozed[fingerprint]
  return { dismissed, snoozed }
}

/** Drops expired snoozes so storage does not grow without bound. */
export function pruneDecisions(decisions: FeedDecisions, now: number): FeedDecisions {
  const snoozed: Record<string, SnoozeRecord> = {}
  for (const [key, record] of Object.entries(decisions.snoozed)) if (record.until > now) snoozed[key] = record
  return { dismissed: decisions.dismissed, snoozed }
}

interface StorageLike {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
}

export function loadDecisions(storage: StorageLike | undefined): FeedDecisions {
  if (!storage) return emptyDecisions()
  try {
    const raw = storage.getItem(STORAGE_KEY)
    if (!raw) return emptyDecisions()
    const parsed = JSON.parse(raw) as Partial<FeedDecisions>
    const decisions = emptyDecisions()
    for (const [key, value] of Object.entries(parsed.dismissed ?? {})) {
      if (value && typeof value.at === 'number') decisions.dismissed[key] = { at: value.at, title: String(value.title ?? '') }
    }
    for (const [key, value] of Object.entries(parsed.snoozed ?? {})) {
      if (value && typeof value.until === 'number') decisions.snoozed[key] = { until: value.until, title: String(value.title ?? '') }
    }
    return decisions
  } catch {
    return emptyDecisions()
  }
}

export function saveDecisions(storage: StorageLike | undefined, decisions: FeedDecisions): boolean {
  if (!storage) return false
  try {
    storage.setItem(STORAGE_KEY, JSON.stringify(decisions))
    return true
  } catch {
    return false
  }
}

/** The action `y` (or a right swipe) takes: the card's first action. */
export function primaryAction(card: Card): CardAction | undefined {
  return card.actions[0]
}

export const CARD_KIND_LABEL: Record<string, string> = { decision: 'Decision', insight: 'Insight' }

export function cardKindLabel(kind: string): string {
  return CARD_KIND_LABEL[kind] ?? kind
}
