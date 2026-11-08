import { describe, expect, it } from 'vitest'
import { builtinTitle, isBuiltin, sharePercent, signed } from './context-breakdown'

describe('context breakdown', () => {
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

describe('builtinTitle', () => {
  it('reads a built-in source as words', () => {
    expect(builtinTitle('builtin:shared-memory-policy')).toBe('Shared memory policy')
    expect(builtinTitle('builtin:handoff')).toBe('Handoff')
  })
})
