// Contract conformance of the overview fixture. Pointed at real server
// output (OVERVIEW_FIXTURE=…), it doubles as a check that the server and
// the client agree on the contract.

import { describe, expect, it } from 'vitest'
import { OVERVIEW_SCHEMA_VERSION } from '../contract'
import { SECTION_VIEWS } from './url'
import { OVERVIEW_FIXTURE, loadOverviewFixture } from './testing'

describe(`overview fixture (${OVERVIEW_FIXTURE})`, () => {
  const overview = loadOverviewFixture()

  it('declares the overview schema and the five sections in harness order', () => {
    expect(overview.schemaVersion).toBe(OVERVIEW_SCHEMA_VERSION)
    const ordered = [...overview.sections].sort((a, b) => a.order - b.order).map((s) => s.id)
    expect(ordered).toEqual([...SECTION_VIEWS])
  })

  it('counts each spec kind as the items it lists, and gives every target a sync state', () => {
    for (const s of overview.sections) {
      for (const k of s.kinds) {
        const items = s.items.filter((it) => it.kind === k.kind)
        // A built-in row lists every built-in and counts the enabled ones.
        const expected = k.kind === 'builtin' ? items.filter((it) => it.enabled).length : items.length
        expect(expected, `${s.id} ${k.kind}`).toBe(k.count)
      }
      for (const it of s.items) {
        expect(Array.isArray(it.findings), `${s.id} ${it.name}`).toBe(true)
        expect(Array.isArray(it.values), `${s.id} ${it.name}`).toBe(true)
        for (const v of it.values) expect(['text', 'bool', 'list']).toContain(v.type)
      }
    }
    for (const t of overview.targets) expect(['in-sync', 'drifted', 'not-checked']).toContain(t.sync.state)
  })

  it('lists every lifecycle stage in session order, even with no hooks', () => {
    const lifecycle = overview.sections.find((s) => s.id === 'lifecycle')
    expect(lifecycle?.stages?.slice(0, 7).map((st) => st.id)).toEqual(['session-start', 'prompt-submit', 'before-tool', 'after-tool', 'after-edit', 'stop', 'session-end'])
  })

  it('lists every built-in with its state', () => {
    const builtins = overview.sections.find((s) => s.id === 'settings')?.items.filter((it) => it.kind === 'builtin') ?? []
    expect(builtins.map((b) => b.name)).toEqual(expect.arrayContaining(['memory', 'handoff', 'handoff-hook', 'rtk', 'caveman']))
    for (const b of builtins) expect(typeof b.enabled, b.name).toBe('boolean')
  })
})
