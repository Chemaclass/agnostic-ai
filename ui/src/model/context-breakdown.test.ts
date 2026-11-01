import { describe, expect, it } from 'vitest'
import type { ContextContribution } from '../contract'
import { isBuiltin, kindTotals, sharePercent, signed } from './context-breakdown'

const row = (source: string, category: string, load: ContextContribution['load'], words: number): ContextContribution => ({ source, category, load, words, bytes: words * 6 })

describe('context breakdown', () => {
  it('sums each kind for one load, largest first', () => {
    const rows = [row('a', 'skill discovery', 'startup', 20), row('b', 'skill discovery', 'startup', 30), row('c', 'always-on rule', 'startup', 40), row('d', 'skill body', 'on-demand', 900)]
    expect(kindTotals(rows, 'startup')).toEqual([
      { category: 'skill discovery', sources: 2, words: 50, bytes: 300 },
      { category: 'always-on rule', sources: 1, words: 40, bytes: 240 },
    ])
    expect(kindTotals(rows, 'file-scope')).toEqual([])
  })

  it('rounds shares and survives an empty total', () => {
    expect(sharePercent(270, 872)).toBe(31)
    expect(sharePercent(1, 0)).toBe(0)
  })

  it('signs changes and spots built-ins', () => {
    expect(signed(7)).toBe('+7')
    expect(signed(-1200)).toBe('-1,200')
    expect(signed(0)).toBe('no change')
    expect(isBuiltin('builtin:caveman')).toBe(true)
    expect(isBuiltin('.agnostic-ai/memory')).toBe(false)
  })
})
