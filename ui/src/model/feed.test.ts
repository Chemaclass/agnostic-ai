import { describe, expect, it } from 'vitest'
import type { Card } from '../contract'
import {
  SNOOZE_MS,
  STORAGE_KEY,
  dismissCard,
  emptyDecisions,
  loadDecisions,
  partitionFeed,
  primaryAction,
  pruneDecisions,
  rankFeed,
  restoreCard,
  saveDecisions,
  snoozeCard,
} from './feed'
import { loadOverviewFixture, usingMockOverview } from './testing'

function card(id: string, priority: number, kind: Card['kind'] = 'insight'): Card {
  return {
    id,
    fingerprint: `fp-${id}`,
    kind,
    priority,
    priorityLabel: String(priority),
    title: `Card ${id}`,
    consequence: '',
    sectionId: 'authority',
    source: 'authority',
    evidence: [],
    actions: [{ id: 'dismiss', kind: 'dismiss', label: 'Dismiss' }],
    revision: 'r1',
  }
}

class MemoryStorage {
  data = new Map<string, string>()
  getItem(key: string) {
    return this.data.get(key) ?? null
  }
  setItem(key: string, value: string) {
    this.data.set(key, value)
  }
}

describe('feed ranking', () => {
  it('orders by priority, then decisions before insights, then server order', () => {
    const ranked = rankFeed([card('a', 3), card('b', 1), card('c', 3, 'decision'), card('d', 1), card('e', 2)])
    expect(ranked.map((c) => c.id)).toEqual(['b', 'd', 'e', 'c', 'a'])
  })

  it('ranks the fixture with priorities never decreasing', () => {
    const ranked = rankFeed(loadOverviewFixture().feed)
    for (let i = 1; i < ranked.length; i++) expect(ranked[i].priority).toBeGreaterThanOrEqual(ranked[i - 1].priority)
  })

  it.runIf(usingMockOverview)('puts the security decision about Railway first in the mock', () => {
    expect(rankFeed(loadOverviewFixture().feed)[0].id).toBe('card-railway-deploy-unscoped')
  })
})

describe('dismissal filtering', () => {
  const now = 1_000_000
  const cards = [card('a', 1), card('b', 2), card('c', 3)]

  it('hides dismissed cards by fingerprint and keeps the rest pending', () => {
    const decisions = dismissCard(emptyDecisions(), cards[0], now)
    const result = partitionFeed(cards, decisions, now)
    expect(result.pending.map((c) => c.id)).toEqual(['b', 'c'])
    expect(result.hidden).toEqual([{ card: cards[0], reason: 'dismissed' }])
  })

  it('hides a snoozed card until the snooze ends', () => {
    const decisions = snoozeCard(emptyDecisions(), cards[1], now)
    expect(partitionFeed(cards, decisions, now + 1).hidden.map((h) => h.reason)).toEqual(['snoozed'])
    expect(partitionFeed(cards, decisions, now + SNOOZE_MS + 1).pending.map((c) => c.id)).toEqual(['a', 'b', 'c'])
    expect(Object.keys(pruneDecisions(decisions, now + SNOOZE_MS + 1).snoozed)).toEqual([])
  })

  it('lists cards acted on in this session after the pending ones', () => {
    const result = partitionFeed(cards, emptyDecisions(), now, new Set(['a']))
    expect(result.pending.map((c) => c.id)).toEqual(['b', 'c'])
    expect(result.acted.map((c) => c.id)).toEqual(['a'])
  })

  it('keeps a dismissal while the fingerprint holds and drops it when the finding changes', () => {
    const decisions = dismissCard(emptyDecisions(), cards[0], now)
    const sameFinding = { ...cards[0], id: 'a-new-revision', revision: 'r2' }
    const changedFinding = { ...cards[0], fingerprint: 'fp-changed' }
    expect(partitionFeed([sameFinding], decisions, now).pending).toEqual([])
    expect(partitionFeed([changedFinding], decisions, now).pending).toEqual([changedFinding])
  })

  it('restores a dismissed or snoozed card', () => {
    let decisions = dismissCard(emptyDecisions(), cards[0], now)
    decisions = snoozeCard(decisions, cards[1], now)
    decisions = restoreCard(restoreCard(decisions, cards[0].fingerprint), cards[1].fingerprint)
    expect(decisions).toEqual(emptyDecisions())
  })

  it('stores decisions in browser storage and survives corrupt data', () => {
    const storage = new MemoryStorage()
    const decisions = dismissCard(emptyDecisions(), cards[0], now)
    expect(saveDecisions(storage, decisions)).toBe(true)
    expect(loadDecisions(storage)).toEqual(decisions)
    storage.setItem(STORAGE_KEY, '{not json')
    expect(loadDecisions(storage)).toEqual(emptyDecisions())
    expect(loadDecisions(undefined)).toEqual(emptyDecisions())
    const throwing = { getItem: () => { throw new Error('blocked') }, setItem: () => { throw new Error('blocked') } }
    expect(loadDecisions(throwing)).toEqual(emptyDecisions())
    expect(saveDecisions(throwing, decisions)).toBe(false)
  })

  it('accepts with the first action', () => {
    expect(primaryAction(cards[0])?.kind).toBe('dismiss')
  })
})
