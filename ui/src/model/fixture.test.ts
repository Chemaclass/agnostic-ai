// Contract conformance of the fixture the other tests read. Pointed at
// real server output (AUTHORITY_FIXTURE=…), it doubles as a check that
// the server and the client agree on the contract.

import { describe, expect, it } from 'vitest'
import { SCHEMA_VERSION, OVERVIEW_SCHEMA_VERSION } from '../contract'
import { EFFECTS } from './effects'
import { SECTION_VIEWS } from './url'
import { AUTHORITY_FIXTURE, OVERVIEW_FIXTURE, loadAuthorityFixture, loadOverviewFixture, readJson } from './testing'

const RESOLUTIONS = ['translated', 'translated-unverified', 'hook', 'not-expressed', 'agent-scope', 'no-rule', 'disabled']
const INSTALLED = ['in-sync', 'stale', 'not-generated', 'not-checked', 'not-applicable']
const ENFORCEMENT = ['native-rule', 'hook', 'not-expressed', 'not-applicable']

describe(`authority fixture (${AUTHORITY_FIXTURE})`, () => {
  const view = loadAuthorityFixture()
  const raw = readJson(AUTHORITY_FIXTURE) as Record<string, unknown>

  it('declares the schema this client renders', () => {
    expect(view.schemaVersion).toBe(SCHEMA_VERSION)
    expect(raw.snapshot).toBeTruthy()
  })

  it('links every provider, domain, and resource to things that exist', () => {
    const providers = new Set(view.providers.map((p) => p.id))
    const resources = new Set(view.resources.map((r) => r.id))
    for (const r of view.resources) expect(providers, `provider of ${r.id}`).toContain(r.providerId)
    for (const d of view.domains) {
      for (const id of d.providerIds) expect(providers, `provider ${id} in ${d.id}`).toContain(id)
      for (const id of d.resourceIds) expect(resources, `resource ${id} in ${d.id}`).toContain(id)
    }
    for (const p of view.providers) for (const id of p.resourceIds) expect(resources).toContain(id)
  })

  it('gives every operation a valid effect, resolution, and evidence', () => {
    for (const r of view.resources) {
      for (const op of r.operations) {
        const where = `${op.id}`
        expect(EFFECTS, where).toContain(op.effect)
        expect(EFFECTS, where).toContain(op.declared)
        expect(RESOLUTIONS, where).toContain(op.resolution)
        expect(INSTALLED, where).toContain(op.installed)
        expect(op.runtime, where).toBe('not-observed')
        expect(typeof op.hypothetical, where).toBe('boolean')
        // The effective effect equals the declared one unless it is unresolved.
        if (op.effect !== 'unresolved') expect(op.effect, where).toBe(op.declared)
        if (op.resolution === 'translated-unverified') expect(op.effect, where).toBe('unresolved')
        if (op.hypothetical) expect(['stale', 'not-generated', 'not-checked'], where).toContain(op.installed)
        expect(Array.isArray(op.routeCoverage.checked), where).toBe(true)
        expect(Array.isArray(op.routeCoverage.notModeled), where).toBe(true)
        for (const route of op.routeCoverage.checked) expect(EFFECTS, `${where} route ${route.route}`).toContain(route.effect)
        for (const e of op.enforcement) {
          expect(ENFORCEMENT, where).toContain(e.status)
          expect(EFFECTS, where).toContain(e.effect)
        }
        const winners = op.grants.filter((g) => g.wins)
        expect(winners.length, `${where} has at most one winning grant`).toBeLessThanOrEqual(1)
      }
    }
  })

  it('draws one edge per operation for the selected subject', () => {
    const ops = new Map(view.resources.flatMap((r) => r.operations.map((op) => [op.id, { op, resource: r }] as const)))
    for (const edge of view.edges) {
      expect(edge.subjectId).toBe(view.agent.id)
      expect(edge.id).toBe(`${edge.subjectId}|${edge.operationId}`)
      const found = ops.get(edge.operationId)
      expect(found, edge.id).toBeTruthy()
      expect(found!.resource.id).toBe(edge.resourceId)
      expect(edge.effect).toBe(found!.op.effect)
    }
  })

  it('counts each domain summary from the effective effects of its operations', () => {
    for (const d of view.domains) {
      const counts = { allowed: 0, ask: 0, denied: 0, unavailable: 0, default: 0, unresolved: 0 }
      for (const r of view.resources.filter((res) => d.resourceIds.includes(res.id))) for (const op of r.operations) counts[op.effect]++
      expect(d.summary, d.id).toEqual(counts)
    }
  })

  it('lists the selected harness among the available ones', () => {
    expect(view.harness.available.map((h) => h.target)).toContain(view.harness.target)
    expect(view.agents.map((a) => a.id)).toContain(view.agent.id)
  })
})

describe(`overview fixture (${OVERVIEW_FIXTURE})`, () => {
  const overview = loadOverviewFixture()

  it('declares the overview schema and the six sections in harness order', () => {
    expect(overview.schemaVersion).toBe(OVERVIEW_SCHEMA_VERSION)
    const ordered = [...overview.sections].sort((a, b) => a.order - b.order).map((s) => s.id)
    expect(ordered).toEqual([...SECTION_VIEWS])
  })

  it('points every section card id at a feed card, with unique fingerprints', () => {
    const ids = new Set(overview.feed.map((c) => c.id))
    for (const s of overview.sections) for (const id of s.cardIds) expect(ids, `${s.id} → ${id}`).toContain(id)
    const fingerprints = overview.feed.map((c) => c.fingerprint)
    expect(new Set(fingerprints).size).toBe(fingerprints.length)
    for (const card of overview.feed) expect(card.evidence.length, card.id).toBeGreaterThan(0)
  })
})
