import { describe, expect, it } from 'vitest'
import { startupVerdict } from './context-verdict'

describe('startup verdict', () => {
  it('says within, over, or only the size', () => {
    expect(startupVerdict({ words: 184, bytes: 1212 }, 2000)).toEqual({ state: 'within', text: 'Startup is 184 words, within the 2,000-word lint budget (lint.instructions-words).' })
    expect(startupVerdict({ words: 2000, bytes: 1 }, 2000).state).toBe('within')
    expect(startupVerdict({ words: 2396, bytes: 1 }, 2000)).toEqual({ state: 'over', text: 'Startup is 2,396 words, 396 words over the 2,000-word lint budget (lint.instructions-words); agnostic-ai lint reports LINT011.' })
    expect(startupVerdict({ words: 2001, bytes: 1 }, 2000).text).toContain('1 word over')
    expect(startupVerdict({ words: 184, bytes: 1 }, undefined)).toEqual({ state: 'unknown', text: 'Startup is 184 words.' })
    expect(startupVerdict({ words: 184, bytes: 1 }, 0).state).toBe('unknown')
    for (const v of [startupVerdict({ words: 1, bytes: 1 }, 2000), startupVerdict({ words: 3000, bytes: 1 }, 2000)]) {
      expect(v.text).not.toMatch(/[–—]/)
      expect(v.text).not.toMatch(/token/i)
    }
  })
})
